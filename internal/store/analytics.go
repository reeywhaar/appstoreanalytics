package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
)

// Measures read from analytics reports. See docs/app-store-connect.md for which report each
// comes from.
const (
	MetricImpressions    = "impressions"
	MetricPageViews      = "page_views"
	MetricFirstDownloads = "first_downloads"
	MetricRedownloads    = "redownloads"
	MetricUpdates        = "updates"
	MetricInstalls       = "installs"
	MetricDeletions      = "deletions"
	MetricSessions       = "sessions"
	MetricCrashes        = "crashes"
	// MetricPurchases counts in-app purchases, refunds taken back.
	MetricPurchases = "purchases"
	// MetricProceeds is in millionths of a US dollar here, and in dollars once it leaves.
	MetricProceeds = "proceeds"
)

// reports groups the measures by the analytics report they come from. A report that says
// anything of a day has processed that day for all of its measures, and one with nothing to
// count leaves no line rather than a zero.
var reports = [][]string{
	{MetricImpressions, MetricPageViews},
	{MetricFirstDownloads, MetricRedownloads, MetricUpdates},
	{MetricPurchases, MetricProceeds},
	{MetricInstalls, MetricDeletions},
	{MetricSessions},
	{MetricCrashes},
}

// countsEveryOne is whether a measure's analytics report counts every event however few, which
// the purchases report does: a day it has processed and says nothing of had none. The others
// leave counts too small to share out, so their silence on a day may be a count left out.
func countsEveryOne(metric string) bool {
	return metric == MetricPurchases || metric == MetricProceeds
}

// SameReport is a measure and every other its analytics report gives.
func SameReport(metric string) []string {
	for _, r := range reports {
		if slices.Contains(r, metric) {
			return r
		}
	}
	return []string{metric}
}

// marks is a placeholder for each of n values in an IN list, and the values as arguments.
func marks(values []string) (string, []any) {
	args := make([]any, len(values))
	for i, v := range values {
		args[i] = v
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(values)), ","), args
}

// Measured is one measure of one app on one day, from one territory and one source, of one
// item bought, which is "" for every measure but purchases and proceeds.
type Measured struct {
	AppID     string
	Day       string
	Metric    string
	Territory string
	Source    string
	Item      string
	Value     int64
}

// Item is something bought: an app, or something inside one.
type Item struct {
	ID    string
	AppID string
	Name  string
	InApp bool
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

// SaveInstance records an instance as read, with what it measured and the items it named.
//
// The days it covers are replaced rather than added to, for the measures its report yields: an
// instance processed later corrects the days an earlier one said, and a snapshot repeats days the
// daily instances say too. For a report that counts every event, a sales report's placeholder
// inside the days it has now processed goes: those days are the report's to say, and the two
// date one purchase a day apart, so it would be counted twice.
func (s *Store) SaveInstance(ctx context.Context, inst Instance, yields []string, measured []Measured, items []Item) error {
	days := map[string]bool{}
	for _, m := range measured {
		days[m.Day] = true
	}
	in, metrics := marks(yields)
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if len(yields) > 0 {
			for day := range days {
				args := append([]any{inst.AppID, day}, metrics...)
				if _, err := tx.ExecContext(ctx,
					`DELETE FROM metrics WHERE app_id = ? AND day = ? AND metric IN (`+in+`)`, args...); err != nil {
					return fmt.Errorf("clear %s: %w", day, err)
				}
			}
		}
		for _, m := range measured {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO metrics (app_id, day, metric, territory, source, item, value, origin) VALUES (?, ?, ?, ?, ?, ?, ?, 'analytics')
				 ON CONFLICT DO UPDATE SET value = value + excluded.value`,
				m.AppID, m.Day, m.Metric, m.Territory, m.Source, m.Item, m.Value)
			if err != nil {
				return fmt.Errorf("save a measure: %w", err)
			}
		}
		if len(yields) > 0 && countsEveryOne(yields[0]) {
			args := append([]any{inst.AppID}, metrics...)
			args = append(args, inst.AppID)
			args = append(args, metrics...)
			args = append(args, inst.AppID)
			args = append(args, metrics...)
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM metrics WHERE origin = 'sales' AND app_id = ? AND metric IN (`+in+`)
				   AND day >= (SELECT MIN(day) FROM metrics WHERE origin = 'analytics' AND app_id = ? AND metric IN (`+in+`))
				   AND day <= (SELECT MAX(day) FROM metrics WHERE origin = 'analytics' AND app_id = ? AND metric IN (`+in+`))`,
				args...); err != nil {
				return fmt.Errorf("clear the sales placeholders analytics has reached: %w", err)
			}
		}
		for _, it := range items {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO items (id, app_id, name, in_app) VALUES (?, ?, ?, ?)
				 ON CONFLICT (id) DO UPDATE SET app_id = excluded.app_id, name = excluded.name, in_app = excluded.in_app`,
				it.ID, it.AppID, it.Name, it.InApp)
			if err != nil {
				return fmt.Errorf("save an item: %w", err)
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO analytics_instances (id, app_id, report, processing_date, rows, read_at) VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (id) DO NOTHING`,
			inst.ID, inst.AppID, inst.Report, inst.ProcessingDate, len(measured), unix(s.Now()))
		return err
	})
	if err == nil {
		// Apple deletes an instance 35 days after it makes it, so what it said is kept only here.
		s.changed()
	}
	return err
}

// Report is what a range of days measured: per day, in all, and broken down.
//
// A measure missing from a day is one not yet known for it, which is not zero: the reports of a
// day arrive separately, over a day or two.
type Report struct {
	// Days holds each day that measured anything: "day", then a number for each measure known.
	Days []map[string]any `json:"days"`
	// Totals sums each measure over the days that have it, and Covered counts those days.
	Totals  map[string]any `json:"totals"`
	Covered map[string]int `json:"covered"`
	// Previous and PreviousCovered are the same of as many days before the range.
	Previous        map[string]any `json:"previous"`
	PreviousCovered map[string]int `json:"previous_covered"`
	// Each breakdown holds "id", "name", and a number for each measure it is of.
	Countries []map[string]any `json:"countries"`
	Sources   []map[string]any `json:"sources"`
	Apps      []map[string]any `json:"apps"`
	InApp     []map[string]any `json:"in_app"`
}

// ReportIn reads [from, to], and before up to the day before from for Previous. appID narrows
// it to one app; "" is every app.
func (s *Store) ReportIn(ctx context.Context, from, to, before, appID string) (*Report, error) {
	out := &Report{}
	var err error
	if out.Days, out.Totals, out.Covered, err = s.measuredDays(ctx, from, to, appID); err != nil {
		return nil, err
	}
	if _, out.Previous, out.PreviousCovered, err = s.measuredDays(ctx, before, dayBefore(from), appID); err != nil {
		return nil, err
	}
	if out.Countries, err = s.breakdown(ctx, "territory", from, to, appID,
		MetricFirstDownloads, MetricProceeds, MetricPageViews); err != nil {
		return nil, err
	}
	if out.Sources, err = s.breakdown(ctx, "source", from, to, appID,
		MetricPageViews, MetricImpressions, MetricFirstDownloads); err != nil {
		return nil, err
	}
	if out.Apps, err = s.breakdown(ctx, "app_id", from, to, appID,
		MetricProceeds, MetricFirstDownloads); err != nil {
		return nil, err
	}
	if out.InApp, err = s.breakdown(ctx, "item", from, to, appID,
		MetricProceeds, MetricPurchases); err != nil {
		return nil, err
	}
	return out, nil
}

// worth is a stored value as it leaves: proceeds in dollars and cents, a count as it is.
func worth(metric string, v int64) any {
	if metric == MetricProceeds {
		return math.Round(float64(v)/1e4) / 100
	}
	return v
}

func (s *Store) measuredDays(ctx context.Context, from, to, appID string) ([]map[string]any, map[string]any, map[string]int, error) {
	query, args := daysQuery(from, to, appID)
	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("measured days: %w", err)
	}
	defer rows.Close()
	days := []map[string]any{}
	sums := map[string]int64{}
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
		days[len(days)-1][metric] = worth(metric, value)
		sums[metric] += value
		covered[metric]++
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if days, err = s.knownFromSales(ctx, from, to, days, covered); err != nil {
		return nil, nil, nil, err
	}
	totals := map[string]any{}
	for m, v := range sums {
		totals[m] = worth(m, v)
	}
	for m := range covered {
		if _, ok := totals[m]; !ok {
			totals[m] = worth(m, 0)
		}
	}
	return days, totals, covered, nil
}

// daysQuery sums each day's measures in [from, to], of one app or, for "", of every one.
func daysQuery(from, to, appID string) (string, []any) {
	app, args := ofApp(appID, from, to)
	return `SELECT day, metric, SUM(value) FROM metrics
		  WHERE day BETWEEN ? AND ?` + app + `
		  GROUP BY day, metric ORDER BY day`, args
}

// breakdownQuery sums measures in [from, to] by one dimension and app, of one app or, for "", of
// every one.
func breakdownQuery(by, from, to, appID string, metrics []string) (string, []any) {
	app, args := ofApp(appID, from, to)
	for _, m := range metrics {
		args = append(args, m)
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(metrics)), ",")
	return `SELECT ` + by + `, app_id, metric, SUM(value) FROM metrics
		WHERE day BETWEEN ? AND ?` + app + ` AND metric IN (` + marks + `)
		  AND ` + by + ` != ''
		GROUP BY 1, 2, 3`, args
}

// ofApp narrows a range of days to one app, and leaves it every app's for "". The app is a
// clause of its own, not one query for both: SQLite plans a query once, and planned for every
// app it would read every app's rows for one, where app_id = ? seeks the app's days by the
// primary key.
func ofApp(appID, from, to string) (string, []any) {
	if appID == "" {
		return "", []any{from, to}
	}
	return " AND app_id = ?", []any{from, to, appID}
}

// DaysByApp is [from, to] app by app: for each app, each day it measured anything, and what. A
// day an app is missing from, or a measure missing from its day, is one it had none of on a day
// DaysIn knows; the charts that stack the apps fill it with zero only where DaysIn has the day.
func (s *Store) DaysByApp(ctx context.Context, from, to string) (map[string]map[string]map[string]any, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT app_id, day, metric, SUM(value) FROM metrics
		  WHERE day BETWEEN ? AND ?
		  GROUP BY app_id, day, metric`, from, to)
	if err != nil {
		return nil, fmt.Errorf("days by app: %w", err)
	}
	defer rows.Close()
	out := map[string]map[string]map[string]any{}
	for rows.Next() {
		var (
			app, day, metric string
			value            int64
		)
		if err := rows.Scan(&app, &day, &metric, &value); err != nil {
			return nil, err
		}
		if out[app] == nil {
			out[app] = map[string]map[string]any{}
		}
		if out[app][day] == nil {
			out[app][day] = map[string]any{}
		}
		out[app][day][metric] = worth(metric, value)
	}
	return out, rows.Err()
}

// DaysIn is each day of [from, to] that measured anything, as ReportIn's Days are.
func (s *Store) DaysIn(ctx context.Context, from, to, appID string) ([]map[string]any, error) {
	days, _, _, err := s.measuredDays(ctx, from, to, appID)
	return days, err
}

// FirstDay is the first day anything was measured or a sales report read, and "" when none was.
func (s *Store) FirstDay(ctx context.Context) (string, error) {
	var day string
	err := s.reader.QueryRowContext(ctx,
		`SELECT COALESCE(MIN(day), '') FROM (
		   SELECT MIN(day) AS day FROM metrics UNION ALL SELECT MIN(day) FROM sales_days)
		 WHERE day IS NOT NULL`).Scan(&day)
	return day, err
}

// salesMeasures are what a sales report says, and so what a day whose report was read knows,
// at zero where it had none.
var salesMeasures = []string{MetricFirstDownloads, MetricRedownloads, MetricUpdates, MetricPurchases, MetricProceeds}

// knownFromSales adds the days whose sales report was read and that measured none of a sales
// measure: zero, rather than not known. A sales report is of every app, so an app it says
// nothing of had none.
func (s *Store) knownFromSales(ctx context.Context, from, to string, days []map[string]any, covered map[string]int) ([]map[string]any, error) {
	rows, err := s.reader.QueryContext(ctx, `SELECT day FROM sales_days WHERE day BETWEEN ? AND ?`, from, to)
	if err != nil {
		return nil, fmt.Errorf("sales days: %w", err)
	}
	defer rows.Close()
	byDay := map[string]map[string]any{}
	for _, d := range days {
		byDay[d["day"].(string)] = d
	}
	added := false
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			return nil, err
		}
		d := byDay[day]
		if d == nil {
			d = map[string]any{"day": day}
			byDay[day] = d
			days = append(days, d)
			added = true
		}
		for _, m := range salesMeasures {
			if _, ok := d[m]; !ok {
				d[m] = worth(m, 0)
				covered[m]++
			}
		}
	}
	if added {
		sort.Slice(days, func(i, j int) bool { return days[i]["day"].(string) < days[j]["day"].(string) })
	}
	return days, rows.Err()
}

// breakdown sums measures by one dimension, largest first by the first measure. by is a column
// name from this package, never from a request.
//
// Apps and items are named from what Apple called them; an in-app purchase also says its app.
// The item breakdown is of in-app purchases only: an app bought is the app, which has its own.
func (s *Store) breakdown(ctx context.Context, by, from, to, appID string, metrics ...string) ([]map[string]any, error) {
	query, args := breakdownQuery(by, from, to, appID, metrics)
	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("by %s: %w", by, err)
	}
	defer rows.Close()
	// Summed as stored and made into dollars once, so a row's proceeds are rounded once too.
	sums := map[string]map[string]int64{}
	apps := map[string]map[string]map[string]int64{}
	for rows.Next() {
		var (
			key, app, metric string
			value            int64
		)
		if err := rows.Scan(&key, &app, &metric, &value); err != nil {
			return nil, err
		}
		if sums[key] == nil {
			sums[key] = map[string]int64{}
			apps[key] = map[string]map[string]int64{}
		}
		if apps[key][app] == nil {
			apps[key][app] = map[string]int64{}
		}
		sums[key][metric] += value
		apps[key][app][metric] += value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Each app's part of a row, where every app is read and the rows are not the apps themselves,
	// for a bar that shows whose its length is.
	parts := appID == "" && by != "app_id"
	byKey := map[string]map[string]any{}
	raw := map[string]int64{}
	for key, measured := range sums {
		row := map[string]any{"id": key, "name": key}
		for metric, value := range measured {
			row[metric] = worth(metric, value)
		}
		if parts {
			own := map[string]map[string]any{}
			for app, measured := range apps[key] {
				own[app] = map[string]any{}
				for metric, value := range measured {
					own[app][metric] = worth(metric, value)
				}
			}
			row["by_app"] = own
		}
		byKey[key] = row
		raw[key] = measured[metrics[0]]
	}
	if err := s.name(ctx, by, byKey); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(byKey))
	for _, row := range byKey {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := raw[out[i]["id"].(string)], raw[out[j]["id"].(string)]
		if a != b {
			return a > b
		}
		return out[i]["name"].(string) < out[j]["name"].(string)
	})
	return out, nil
}

// name fills in what apps and items are called, and keeps only in-app purchases of items.
func (s *Store) name(ctx context.Context, by string, rows map[string]map[string]any) error {
	switch by {
	case "app_id":
		apps, err := s.Apps(ctx)
		if err != nil {
			return err
		}
		for _, a := range apps {
			if row := rows[a.ID]; row != nil {
				row["name"] = a.Name
			}
		}
	case "item":
		q, err := s.reader.QueryContext(ctx,
			`SELECT i.id, i.name, i.in_app, COALESCE(a.name, '') FROM items i LEFT JOIN apps a ON a.id = i.app_id`)
		if err != nil {
			return fmt.Errorf("items: %w", err)
		}
		defer q.Close()
		keep := map[string]bool{}
		for q.Next() {
			var (
				id, name, app string
				inApp         bool
			)
			if err := q.Scan(&id, &name, &inApp, &app); err != nil {
				return err
			}
			if row := rows[id]; row != nil && inApp {
				row["name"] = name
				row["app_name"] = app
				keep[id] = true
			}
		}
		if err := q.Err(); err != nil {
			return err
		}
		for id := range rows {
			if !keep[id] {
				delete(rows, id)
			}
		}
	}
	return nil
}

// MetricsSpan is the first and last day anything was measured, and the last day downloads
// were, which is the day a range ends on when none is asked for.
func (s *Store) MetricsSpan(ctx context.Context) (first, last, downloads string, err error) {
	err = s.reader.QueryRowContext(ctx,
		`SELECT COALESCE(MIN(day), ''), COALESCE(MAX(day), ''),
		   COALESCE((SELECT MAX(day) FROM metrics WHERE metric = ?), '')
		 FROM metrics`, MetricFirstDownloads).Scan(&first, &last, &downloads)
	return first, last, downloads, err
}

// InstanceCount is how many analytics report instances have been read.
func (s *Store) InstanceCount(ctx context.Context) (int, error) {
	var n int
	err := s.reader.QueryRowContext(ctx, `SELECT COUNT(*) FROM analytics_instances`).Scan(&n)
	return n, err
}

// dayBefore is the day before a YYYY-MM-DD.
func dayBefore(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, -1).Format(time.DateOnly)
}
