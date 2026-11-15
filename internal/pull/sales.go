package pull

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/store"
)

// SalesHistory is how many days back sales reports are read. Apple keeps daily reports for a
// year.
const SalesHistory = 365

// settled is how long after a day ends its report is taken as final. Until then it is read
// again each pass, because the territories' reports arrive at different hours.
const settled = 36 * time.Hour

// notThere is how old a day whose report never came is before it is taken as having had none,
// so that it is not asked for every hour forever.
const notThere = 5 * 24 * time.Hour

// giveUp is how many days in a row may fail before a pass stops asking: one day's refusal is
// that day's, and five in a row are everybody's, such as a vendor number that is not the key's.
const giveUp = 5

// Pacific is the zone Apple's report days are in.
var Pacific = mustZone("America/Los_Angeles")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// sales reads the days that are missing or not yet settled, newest first, so the days a person
// looks at first are there first.
func (p *Puller) sales(ctx context.Context) error {
	read, err := p.st.SalesDays(ctx)
	if err != nil {
		return err
	}
	bySKU, err := p.st.AppsBySKU(ctx)
	if err != nil {
		return err
	}
	now := p.now()
	today := midnight(now.In(Pacific))
	var (
		fetched, empty, failed int
		first                  error
	)
	for back := 1; back <= SalesHistory; back++ {
		start := today.AddDate(0, 0, -back)
		day := start.Format(time.DateOnly)
		end := start.AddDate(0, 0, 1)
		if at, ok := read[day]; ok && at.Sub(end) >= settled {
			continue
		}
		lines, err := p.asc.SalesReport(ctx, p.vendor, day)
		switch {
		case errors.Is(err, asc.ErrGone):
			// Every day before it is older still.
			back = SalesHistory
			continue
		case errors.Is(err, asc.ErrNotYet) && now.Sub(end) < notThere:
			continue
		case errors.Is(err, asc.ErrNotYet), errors.Is(err, asc.ErrNoSales):
			lines = nil
		case slowDown(err) || refused(err) || ctx.Err() != nil:
			return err
		case err != nil:
			first = keep(first, err)
			p.log.Warn("a sales report could not be read", "day", day, "error", err.Error())
			if failed++; failed >= giveUp {
				return first
			}
			continue
		}
		failed = 0
		sales, products := tally(lines, bySKU)
		if err := p.st.SaveSalesDay(ctx, day, sales, products); err != nil {
			return err
		}
		fetched++
		if len(lines) == 0 {
			empty++
		}
	}
	if fetched > 0 {
		p.log.Info("sales reports read", "days", fetched, "empty", empty)
	}
	return first
}

// refused is a key App Store Connect will not take, which no other day will either.
func refused(err error) bool {
	s := asc.StatusOf(err)
	return s == http.StatusUnauthorized || s == http.StatusForbidden
}

func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// tally sums a report's lines over what is not kept, and says which app each belongs to.
//
// An in-app purchase names its app by the app's SKU, which the apps list knows, and so does any
// line of the app itself in the same report.
func tally(lines []asc.SalesLine, bySKU map[string]string) ([]store.Sale, []store.Product) {
	skus := map[string]string{}
	for k, v := range bySKU {
		skus[k] = v
	}
	for _, l := range lines {
		if product(Kind(l.ProductType)) == store.ProductApp && l.SKU != "" {
			skus[l.SKU] = l.AppleID
		}
	}

	type key struct{ product, ptype, country, currency string }
	sums := map[key]*store.Sale{}
	var order []key
	products := map[string]store.Product{}
	for _, l := range lines {
		kind := Kind(l.ProductType)
		app := l.AppleID
		if product(kind) != store.ProductApp {
			app = skus[l.ParentSKU]
		}
		k := key{l.AppleID, l.ProductType, l.Country, l.Currency}
		s := sums[k]
		if s == nil {
			s = &store.Sale{AppID: app, ProductID: l.AppleID, ProductType: l.ProductType, Kind: kind, Country: l.Country, Currency: l.Currency}
			sums[k] = s
			order = append(order, k)
		}
		s.Units += l.Units
		s.Proceeds += l.Proceeds
		products[l.AppleID] = store.Product{ID: l.AppleID, AppID: app, Title: l.Title, SKU: l.SKU, Kind: product(kind)}
	}
	sales := make([]store.Sale, 0, len(order))
	for _, k := range order {
		sales = append(sales, *sums[k])
	}
	list := make([]store.Product, 0, len(products))
	for _, p := range products {
		list = append(list, p)
	}
	return sales, list
}

// Kind tells what a line counts from its Product Type Identifier. See docs/app-store-connect.md.
func Kind(productType string) string {
	t := strings.ToUpper(strings.TrimSpace(productType))
	switch {
	case t == "7" || t == "7F" || t == "7T" || t == "F7":
		return store.KindUpdate
	case t == "3" || t == "3F" || t == "3T" || t == "F3":
		return store.KindRedownload
	case strings.HasPrefix(t, "IA") || strings.HasPrefix(t, "FI"):
		return store.KindInApp
	case strings.HasPrefix(t, "1") || strings.HasPrefix(t, "F1"):
		return store.KindDownload
	}
	return store.KindOther
}

func product(kind string) string {
	switch kind {
	case store.KindDownload, store.KindRedownload, store.KindUpdate:
		return store.ProductApp
	case store.KindInApp:
		return store.ProductInApp
	}
	return store.ProductOther
}
