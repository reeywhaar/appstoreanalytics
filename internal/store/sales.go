package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SalesDays is when each day's sales report was read.
func (s *Store) SalesDays(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.reader.QueryContext(ctx, `SELECT day, fetched_at FROM sales_days`)
	if err != nil {
		return nil, fmt.Errorf("sales days: %w", err)
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var (
			day string
			at  int64
		)
		if err := rows.Scan(&day, &at); err != nil {
			return nil, err
		}
		out[day] = fromUnix(at)
	}
	return out, rows.Err()
}

// SaveSalesDay keeps what a day's sales report says, as placeholders for what analytics reports
// have not said yet.
//
// What it said before is replaced, and an app's measure that an analytics report has already
// given for the day is left as analytics said it: a sales report fills in, and never corrects.
func (s *Store) SaveSalesDay(ctx context.Context, day string, measured []Measured, items []Item) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM metrics WHERE day = ? AND origin = 'sales'`, day); err != nil {
			return fmt.Errorf("clear the sales of %s: %w", day, err)
		}
		for _, m := range measured {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO metrics (app_id, day, metric, territory, source, item, value, origin)
				 SELECT ?, ?, ?, ?, ?, ?, ?, 'sales'
				  WHERE NOT EXISTS (SELECT 1 FROM metrics
				                     WHERE app_id = ? AND day = ? AND metric = ? AND origin = 'analytics')
				 ON CONFLICT DO UPDATE SET value = value + excluded.value`,
				m.AppID, day, m.Metric, m.Territory, m.Source, m.Item, m.Value,
				m.AppID, day, m.Metric)
			if err != nil {
				return fmt.Errorf("save the sales of %s: %w", day, err)
			}
		}
		// What an analytics report calls something wins over a sales report's title.
		for _, it := range items {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO items (id, app_id, name, in_app) VALUES (?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`,
				it.ID, it.AppID, it.Name, it.InApp)
			if err != nil {
				return fmt.Errorf("save an item: %w", err)
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO sales_days (day, rows, fetched_at) VALUES (?, ?, ?)
			 ON CONFLICT (day) DO UPDATE SET rows = excluded.rows, fetched_at = excluded.fetched_at`,
			day, len(measured), unix(s.Now()))
		return err
	})
}

// AppsBySKU is what an in-app purchase's Parent Identifier, its app's SKU, is resolved through.
func (s *Store) AppsBySKU(ctx context.Context) (map[string]string, error) {
	rows, err := s.reader.QueryContext(ctx, `SELECT sku, id FROM apps WHERE sku != ''`)
	if err != nil {
		return nil, fmt.Errorf("apps by sku: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var sku, id string
		if err := rows.Scan(&sku, &id); err != nil {
			return nil, err
		}
		out[sku] = id
	}
	return out, rows.Err()
}

// SalesSpan is the first and last day whose sales report has been read, and how many have.
func (s *Store) SalesSpan(ctx context.Context) (first, last string, days int, err error) {
	err = s.reader.QueryRowContext(ctx,
		`SELECT COALESCE(MIN(day), ''), COALESCE(MAX(day), ''), COUNT(*) FROM sales_days`).
		Scan(&first, &last, &days)
	return first, last, days, err
}

// SaveRates replaces what a dollar buys, currency by currency.
func (s *Store) SaveRates(ctx context.Context, perUSD map[string]float64) error {
	now := unix(s.Now())
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for currency, rate := range perUSD {
			if rate <= 0 {
				continue
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO rates (currency, per_usd, fetched_at) VALUES (?, ?, ?)
				 ON CONFLICT (currency) DO UPDATE SET per_usd = excluded.per_usd, fetched_at = excluded.fetched_at`,
				currency, rate, now)
			if err != nil {
				return fmt.Errorf("save the rate of %s: %w", currency, err)
			}
		}
		return nil
	})
}

// Rates is what a dollar buys in each currency, and when they were last read; the zero time
// when they never were.
func (s *Store) Rates(ctx context.Context) (map[string]float64, time.Time, error) {
	rows, err := s.reader.QueryContext(ctx, `SELECT currency, per_usd, fetched_at FROM rates`)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("rates: %w", err)
	}
	defer rows.Close()
	out := map[string]float64{}
	var latest int64
	for rows.Next() {
		var (
			currency string
			rate     float64
			at       int64
		)
		if err := rows.Scan(&currency, &rate, &at); err != nil {
			return nil, time.Time{}, err
		}
		out[currency] = rate
		latest = max(latest, at)
	}
	if latest == 0 {
		return out, time.Time{}, rows.Err()
	}
	return out, fromUnix(latest), rows.Err()
}
