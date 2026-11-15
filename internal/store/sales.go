package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"
)

// What a sales row counts, told from its product type. See docs/app-store-connect.md.
const (
	KindDownload   = "download"
	KindRedownload = "redownload"
	KindUpdate     = "update"
	KindInApp      = "in_app"
	KindOther      = "other"
)

// What a product is.
const (
	ProductApp   = "app"
	ProductInApp = "in_app"
	ProductOther = "other"
)

// Sale is one line of a day's sales report, summed over what is not kept: the device, the
// price tier, the promo code.
type Sale struct {
	AppID       string
	ProductID   string
	ProductType string
	Kind        string
	Country     string
	Currency    string
	Units       int64
	// Proceeds is in millionths of Currency, every unit's together.
	Proceeds int64
}

// Product is what a sales report says about something it counts.
type Product struct {
	ID    string
	AppID string
	Title string
	SKU   string
	Kind  string
}

// SalesDays is when each day's report was read.
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

// SaveSalesDay replaces a day's sales with what its report says, at once: a day is either the
// whole of a report or not known.
func (s *Store) SaveSalesDay(ctx context.Context, day string, sales []Sale, products []Product) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sales WHERE day = ?`, day); err != nil {
			return err
		}
		for _, r := range sales {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO sales (day, app_id, product_id, product_type, kind, country, currency, units, proceeds)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT DO UPDATE SET units = units + excluded.units, proceeds = proceeds + excluded.proceeds`,
				day, r.AppID, r.ProductID, r.ProductType, r.Kind, r.Country, r.Currency, r.Units, r.Proceeds)
			if err != nil {
				return fmt.Errorf("save sales of %s: %w", day, err)
			}
		}
		// The newest report's word on a product wins, and an older day read later does not undo it.
		for _, p := range products {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO products (id, app_id, title, sku, kind, seen_day) VALUES (?, ?, ?, ?, ?, ?)
				 ON CONFLICT (id) DO UPDATE SET app_id = excluded.app_id, title = excluded.title,
				   sku = excluded.sku, kind = excluded.kind, seen_day = excluded.seen_day
				 WHERE excluded.seen_day >= products.seen_day`,
				p.ID, p.AppID, p.Title, p.SKU, p.Kind, day)
			if err != nil {
				return fmt.Errorf("save product %s: %w", p.ID, err)
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO sales_days (day, rows, fetched_at) VALUES (?, ?, ?)
			 ON CONFLICT (day) DO UPDATE SET rows = excluded.rows, fetched_at = excluded.fetched_at`,
			day, len(sales), unix(s.Now()))
		return err
	})
}

// AppsBySKU is what a sales report's Parent Identifier, an app's SKU, is resolved through.
func (s *Store) AppsBySKU(ctx context.Context) (map[string]string, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT sku, id FROM products WHERE kind = 'app' AND sku != ''
		 UNION ALL
		 SELECT sku, id FROM apps WHERE sku != ''`)
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

// SalesDay is one known day.
type SalesDay struct {
	Day         string  `json:"day"`
	Downloads   int64   `json:"downloads"`
	Redownloads int64   `json:"redownloads"`
	Updates     int64   `json:"updates"`
	InApp       int64   `json:"in_app"`
	Proceeds    float64 `json:"proceeds"`
}

// SalesTotals sums a range's known days, and says how many there were.
type SalesTotals struct {
	Downloads   int64   `json:"downloads"`
	Redownloads int64   `json:"redownloads"`
	Updates     int64   `json:"updates"`
	InApp       int64   `json:"in_app"`
	Proceeds    float64 `json:"proceeds"`
	Days        int     `json:"days"`
}

// SalesBy is a range's sales of one country, app or product.
type SalesBy struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Downloads int64   `json:"downloads"`
	Units     int64   `json:"units"`
	Proceeds  float64 `json:"proceeds"`
	// AppName is the name of the app an in-app purchase is bought in, and "" for anything else.
	AppName string `json:"app_name,omitempty"`
}

// Sales is everything a range of days says about sales, in US dollars.
type Sales struct {
	Days      []SalesDay  `json:"days"`
	Totals    SalesTotals `json:"totals"`
	Previous  SalesTotals `json:"previous"`
	Countries []SalesBy   `json:"countries"`
	Apps      []SalesBy   `json:"apps"`
	InApp     []SalesBy   `json:"in_app"`
	// Unconverted are the currencies with proceeds in the range and no rate, left out of every
	// sum of proceeds.
	Unconverted []string `json:"unconverted"`
}

// usd is a row's proceeds in dollars, NULL when its currency has no rate, which SUM skips.
const usd = `(s.proceeds * (CASE WHEN s.currency = 'USD' THEN 1.0 ELSE 1.0 / r.per_usd END) / 1000000.0)`

const salesFrom = `FROM sales s LEFT JOIN rates r ON r.currency = s.currency
	WHERE s.day BETWEEN ? AND ? AND (? = '' OR s.app_id = ?)`

// namedFrom is salesFrom with what names a group: the apps the account lists, and failing that
// the sales report's own title.
const namedFrom = `FROM sales s LEFT JOIN rates r ON r.currency = s.currency
	LEFT JOIN apps a ON a.id = s.app_id
	LEFT JOIN products p ON p.id = CASE WHEN s.kind = 'in_app' THEN s.product_id ELSE s.app_id END
	LEFT JOIN products ap ON ap.id = s.app_id
	WHERE s.day BETWEEN ? AND ? AND (? = '' OR s.app_id = ?)`

// SalesIn reads [from, to], and the same number of days before it for Previous. appID narrows
// it to one app; "" is every app.
func (s *Store) SalesIn(ctx context.Context, from, to, before, appID string) (*Sales, error) {
	out := &Sales{Days: []SalesDay{}, Countries: []SalesBy{}, Apps: []SalesBy{}, InApp: []SalesBy{}, Unconverted: []string{}}

	days, err := s.salesDays(ctx, from, to, appID)
	if err != nil {
		return nil, err
	}
	out.Days = days
	out.Totals = sum(days)
	previous, err := s.salesDays(ctx, before, dayBefore(from), appID)
	if err != nil {
		return nil, err
	}
	out.Previous = sum(previous)

	if out.Countries, err = s.salesBy(ctx,
		`SELECT s.country, s.country, `+tally+`, '' `+namedFrom+` GROUP BY s.country ORDER BY 3 DESC, 5 DESC, 1`,
		from, to, appID); err != nil {
		return nil, err
	}
	if out.Apps, err = s.salesBy(ctx,
		`SELECT s.app_id, COALESCE(a.name, p.title, ''), `+tally+`, '' `+namedFrom+`
		 GROUP BY s.app_id ORDER BY 5 DESC, 3 DESC, 2`,
		from, to, appID); err != nil {
		return nil, err
	}
	if out.InApp, err = s.salesBy(ctx,
		`SELECT s.product_id, COALESCE(p.title, ''), `+tally+`, COALESCE(a.name, ap.title, '') `+namedFrom+` AND s.kind = 'in_app'
		 GROUP BY s.product_id ORDER BY 5 DESC, 4 DESC, 2`,
		from, to, appID); err != nil {
		return nil, err
	}

	rows, err := s.reader.QueryContext(ctx,
		`SELECT DISTINCT s.currency `+salesFrom+` AND s.currency != 'USD' AND r.currency IS NULL AND s.proceeds != 0
		 ORDER BY 1`, from, to, appID, appID)
	if err != nil {
		return nil, fmt.Errorf("unconverted currencies: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out.Unconverted = append(out.Unconverted, c)
	}
	return out, rows.Err()
}

// tally is a group's downloads, units and proceeds.
const tally = `SUM(CASE WHEN s.kind = 'download' THEN s.units ELSE 0 END), SUM(s.units), COALESCE(SUM(` + usd + `), 0)`

func (s *Store) salesBy(ctx context.Context, query, from, to, appID string) ([]SalesBy, error) {
	rows, err := s.reader.QueryContext(ctx, query, from, to, appID, appID)
	if err != nil {
		return nil, fmt.Errorf("sales by: %w", err)
	}
	defer rows.Close()
	out := []SalesBy{}
	for rows.Next() {
		var b SalesBy
		if err := rows.Scan(&b.ID, &b.Name, &b.Downloads, &b.Units, &b.Proceeds, &b.AppName); err != nil {
			return nil, err
		}
		b.Proceeds = cents(b.Proceeds)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) salesDays(ctx context.Context, from, to, appID string) ([]SalesDay, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT d.day,
		   COALESCE(SUM(CASE WHEN s.kind = 'download' THEN s.units END), 0),
		   COALESCE(SUM(CASE WHEN s.kind = 'redownload' THEN s.units END), 0),
		   COALESCE(SUM(CASE WHEN s.kind = 'update' THEN s.units END), 0),
		   COALESCE(SUM(CASE WHEN s.kind = 'in_app' THEN s.units END), 0),
		   COALESCE(SUM(`+usd+`), 0)
		 FROM sales_days d
		 LEFT JOIN sales s ON s.day = d.day AND (? = '' OR s.app_id = ?)
		 LEFT JOIN rates r ON r.currency = s.currency
		 WHERE d.day BETWEEN ? AND ?
		 GROUP BY d.day ORDER BY d.day`,
		appID, appID, from, to)
	if err != nil {
		return nil, fmt.Errorf("sales days: %w", err)
	}
	defer rows.Close()
	out := []SalesDay{}
	for rows.Next() {
		var d SalesDay
		if err := rows.Scan(&d.Day, &d.Downloads, &d.Redownloads, &d.Updates, &d.InApp, &d.Proceeds); err != nil {
			return nil, err
		}
		d.Proceeds = cents(d.Proceeds)
		out = append(out, d)
	}
	return out, rows.Err()
}

func sum(days []SalesDay) SalesTotals {
	var t SalesTotals
	for _, d := range days {
		t.Downloads += d.Downloads
		t.Redownloads += d.Redownloads
		t.Updates += d.Updates
		t.InApp += d.InApp
		t.Proceeds += d.Proceeds
		t.Days++
	}
	t.Proceeds = cents(t.Proceeds)
	return t
}

// SalesSpan is the first and last day whose report has been read, and how many have.
func (s *Store) SalesSpan(ctx context.Context) (first, last string, days int, err error) {
	err = s.reader.QueryRowContext(ctx,
		`SELECT COALESCE(MIN(day), ''), COALESCE(MAX(day), ''), COUNT(*) FROM sales_days`).
		Scan(&first, &last, &days)
	return first, last, days, err
}

func cents(v float64) float64 { return math.Round(v*100) / 100 }

// dayBefore is the day before a YYYY-MM-DD.
func dayBefore(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, -1).Format(time.DateOnly)
}
