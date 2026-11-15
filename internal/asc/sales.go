package asc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SalesVersion is the version of the daily sales summary asked for: the one Apple documents.
// Columns are read by name, so a newer one would read the same.
const SalesVersion = "1_0"

// A sales report that is not there is one of two things, and only Apple's words tell which.
var (
	// ErrNoSales is a day with nothing sold, downloaded or updated. It is known, and empty.
	ErrNoSales = errors.New("no sales that day")
	// ErrNotYet is a day whose report Apple has not made yet.
	ErrNotYet = errors.New("the report is not ready yet")
	// ErrGone is a day older than Apple keeps daily reports for.
	ErrGone = errors.New("the report is no longer kept")
)

// SalesLine is one line of a daily sales summary.
type SalesLine struct {
	AppleID     string
	SKU         string
	Title       string
	ProductType string
	Units       int64
	// Proceeds is every unit's together, in millionths of Currency. A refund's is negative, as
	// its units are.
	Proceeds  int64
	Currency  string
	Country   string
	ParentSKU string
}

// SalesReport reads a day's sales summary, a YYYY-MM-DD in Pacific time.
func (c *Client) SalesReport(ctx context.Context, vendor, day string) ([]SalesLine, error) {
	q := url.Values{}
	q.Set("filter[frequency]", "DAILY")
	q.Set("filter[reportType]", "SALES")
	q.Set("filter[reportSubType]", "SUMMARY")
	q.Set("filter[vendorNumber]", vendor)
	q.Set("filter[reportDate]", day)
	q.Set("filter[version]", SalesVersion)
	body, err := c.do(ctx, http.MethodGet, "/v1/salesReports?"+q.Encode(), nil, "application/a-gzip")
	switch StatusOf(err) {
	case http.StatusNotFound:
		// "There were no sales for the date specified." against "Report is not available yet.
		// Daily reports for the Americas are available by 5 am Pacific Time; …"
		if strings.Contains(strings.ToLower(detail(err)), "no sales") {
			return nil, ErrNoSales
		}
		return nil, ErrNotYet
	case http.StatusGone:
		return nil, ErrGone
	}
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return ParseSales(body)
}

// proceeds is what one unit earns. Apple's help names the column with "(per unit)", and its
// files without.
func proceeds(r row) int64 {
	if r.t.has("Developer Proceeds") {
		return r.micros("Developer Proceeds")
	}
	return r.micros("Developer Proceeds (per unit)")
}

// ParseSales reads a sales summary.
func ParseSales(r io.Reader) ([]SalesLine, error) {
	t, err := newTable(r)
	if err != nil {
		return nil, err
	}
	if !t.has("Apple Identifier") || !t.has("Units") {
		return nil, errors.New("the sales report has no Apple Identifier or Units column, so it is not one this reads")
	}
	var out []SalesLine
	for {
		row, err := t.next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		units := row.integer("Units")
		out = append(out, SalesLine{
			AppleID:     row.get("Apple Identifier"),
			SKU:         row.get("SKU"),
			Title:       row.get("Title"),
			ProductType: row.get("Product Type Identifier"),
			Units:       units,
			Proceeds:    units * proceeds(row),
			Currency:    row.get("Currency of Proceeds"),
			Country:     row.get("Country Code"),
			ParentSKU:   row.get("Parent Identifier"),
		})
	}
}
