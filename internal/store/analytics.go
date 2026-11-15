package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Measures read from analytics reports. See docs/app-store-connect.md for which report each
// comes from.
const (
	MetricImpressions    = "impressions"
	MetricPageViews      = "page_views"
	MetricFirstDownloads = "first_downloads"
	MetricRedownloads    = "redownloads"
	MetricInstalls       = "installs"
	MetricDeletions      = "deletions"
	MetricSessions       = "sessions"
	MetricCrashes        = "crashes"
)

// Metrics is every measure, in the order the interface shows them.
var Metrics = []string{
	MetricImpressions, MetricPageViews, MetricFirstDownloads, MetricRedownloads,
	MetricInstalls, MetricDeletions, MetricSessions, MetricCrashes,
}

// Measured is one measure of one app on one day, from one territory and one source.
type Measured struct {
	AppID     string
	Day       string
	Metric    string
	Territory string
	Source    string
	Value     int64
}

// Instance is one analytics report instance: a report's data as Apple produced it on a day.
type Instance struct {
	ID             string
	AppID          string
	Report         string
	ProcessingDate string
}

// InstancesRead is the ids of the instances already read.
func (s *Store) InstancesRead(ctx context.Context) (map[string]bool, error) {
	rows, err := s.reader.QueryContext(ctx, `SELECT id FROM analytics_instances`)
	if err != nil {
		return nil, fmt.Errorf("instances read: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// SaveInstance records an instance as read, with what it measured.
//
// The days it covers are replaced rather than added to, for the measures its report yields: a
// snapshot and the daily instance of the same day say the same thing, and reading both must not
// count it twice.
func (s *Store) SaveInstance(ctx context.Context, inst Instance, yields []string, measured []Measured) error {
	days := map[string]bool{}
	for _, m := range measured {
		days[m.Day] = true
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if len(yields) > 0 {
			marks := strings.TrimSuffix(strings.Repeat("?,", len(yields)), ",")
			for day := range days {
				args := []any{inst.AppID, day}
				for _, y := range yields {
					args = append(args, y)
				}
				if _, err := tx.ExecContext(ctx,
					`DELETE FROM metrics WHERE app_id = ? AND day = ? AND metric IN (`+marks+`)`, args...); err != nil {
					return fmt.Errorf("clear %s: %w", day, err)
				}
			}
		}
		for _, m := range measured {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO metrics (app_id, day, metric, territory, source, value) VALUES (?, ?, ?, ?, ?, ?)
				 ON CONFLICT DO UPDATE SET value = value + excluded.value`,
				m.AppID, m.Day, m.Metric, m.Territory, m.Source, m.Value)
			if err != nil {
				return fmt.Errorf("save a measure: %w", err)
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO analytics_instances (id, app_id, report, processing_date, rows, read_at) VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (id) DO NOTHING`,
			inst.ID, inst.AppID, inst.Report, inst.ProcessingDate, len(measured), unix(s.Now()))
		return err
	})
}

// Analytics is what a range of days measured, per day and in all.
//
// A measure missing from a day is one not yet known for it, which is not zero: the reports of a
// day arrive separately, over a day or two.
type Analytics struct {
	// Days holds each day that measured anything: "day", then a number for each measure known.
	Days []map[string]any `json:"days"`
	// Totals sums each measure over the days that have it, and Covered counts those days.
	Totals  map[string]int64 `json:"totals"`
	Covered map[string]int   `json:"covered"`
	// Previous and PreviousCovered are the same of as many days before the range.
	Previous        map[string]int64 `json:"previous"`
	PreviousCovered map[string]int   `json:"previous_covered"`
	// Sources holds where visitors came from: "source", then a number for each measure.
	Sources []map[string]any `json:"sources"`
}

// AnalyticsIn reads [from, to], and before up to the day before from for Previous. appID
// narrows it to one app; "" is every app.
func (s *Store) AnalyticsIn(ctx context.Context, from, to, before, appID string) (*Analytics, error) {
	out := &Analytics{Days: []map[string]any{}, Sources: []map[string]any{}}
	var err error
	if out.Days, out.Totals, out.Covered, err = s.measuredDays(ctx, from, to, appID); err != nil {
		return nil, err
	}
	if _, out.Previous, out.PreviousCovered, err = s.measuredDays(ctx, before, dayBefore(from), appID); err != nil {
		return nil, err
	}

	rows, err := s.reader.QueryContext(ctx,
		`SELECT source, metric, SUM(value) FROM metrics
		  WHERE day BETWEEN ? AND ? AND (? = '' OR app_id = ?) AND source != ''
		  GROUP BY source, metric`, from, to, appID, appID)
	if err != nil {
		return nil, fmt.Errorf("sources: %w", err)
	}
	defer rows.Close()
	bySource := map[string]map[string]any{}
	for rows.Next() {
		var (
			source, metric string
			value          int64
		)
		if err := rows.Scan(&source, &metric, &value); err != nil {
			return nil, err
		}
		row := bySource[source]
		if row == nil {
			row = map[string]any{"source": source}
			bySource[source] = row
		}
		row[metric] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, row := range bySource {
		out.Sources = append(out.Sources, row)
	}
	sort.Slice(out.Sources, func(i, j int) bool {
		a, _ := out.Sources[i][MetricPageViews].(int64)
		b, _ := out.Sources[j][MetricPageViews].(int64)
		if a != b {
			return a > b
		}
		return out.Sources[i]["source"].(string) < out.Sources[j]["source"].(string)
	})
	return out, nil
}

func (s *Store) measuredDays(ctx context.Context, from, to, appID string) ([]map[string]any, map[string]int64, map[string]int, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT day, metric, SUM(value) FROM metrics
		  WHERE day BETWEEN ? AND ? AND (? = '' OR app_id = ?)
		  GROUP BY day, metric ORDER BY day`, from, to, appID, appID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("measured days: %w", err)
	}
	defer rows.Close()
	days := []map[string]any{}
	totals := map[string]int64{}
	covered := map[string]int{}
	for rows.Next() {
		var (
			day, metric string
			value       int64
		)
		if err := rows.Scan(&day, &metric, &value); err != nil {
			return nil, nil, nil, err
		}
		if len(days) == 0 || days[len(days)-1]["day"] != day {
			days = append(days, map[string]any{"day": day})
		}
		days[len(days)-1][metric] = value
		totals[metric] += value
		covered[metric]++
	}
	return days, totals, covered, rows.Err()
}

// MetricsSpan is the first and last day anything was measured.
func (s *Store) MetricsSpan(ctx context.Context) (first, last string, err error) {
	err = s.reader.QueryRowContext(ctx,
		`SELECT COALESCE(MIN(day), ''), COALESCE(MAX(day), '') FROM metrics`).Scan(&first, &last)
	return first, last, err
}

// InstanceCount is how many analytics report instances have been read.
func (s *Store) InstanceCount(ctx context.Context) (int, error) {
	var n int
	err := s.reader.QueryRowContext(ctx, `SELECT COUNT(*) FROM analytics_instances`).Scan(&n)
	return n, err
}
