package pull

import (
	"context"
	"errors"
	"math"
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

// sales reads the days of sales reports that are missing or not yet settled, newest first, and
// keeps them as placeholders for what analytics reports have not said yet.
func (p *Puller) sales(ctx context.Context) error {
	read, err := p.st.SalesDays(ctx)
	if err != nil {
		return err
	}
	bySKU, err := p.st.AppsBySKU(ctx)
	if err != nil {
		return err
	}
	perUSD, _, err := p.st.Rates(ctx)
	if err != nil {
		return err
	}
	now := p.now()
	today := midnight(now.In(Pacific))
	var (
		fetched, failed int
		first           error
		unconverted     = map[string]bool{}
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
		case slowDown(err) || refused(err) || asc.StatusOf(err) == http.StatusForbidden || ctx.Err() != nil:
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
		measured, items := tally(day, lines, bySKU, perUSD, unconverted)
		if err := p.st.SaveSalesDay(ctx, day, measured, items); err != nil {
			return err
		}
		fetched++
	}
	if fetched > 0 {
		p.log.Info("sales reports read", "days", fetched)
	}
	if len(unconverted) > 0 {
		var list []string
		for c := range unconverted {
			list = append(list, c)
		}
		p.log.Warn("sales proceeds left out for want of a rate", "currencies", strings.Join(list, ","))
	}
	return first
}

func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// tally turns a day's sales report into the measures analytics reports give: first-time
// downloads, redownloads, updates and in-app purchases counted, and proceeds in millionths of a
// dollar, by country and by what was bought. A currency with no rate is noted in unconverted
// and its proceeds left out.
//
// An in-app purchase names its app by the app's SKU, which the apps list knows, and so does any
// line of the app itself in the same report.
func tally(day string, lines []asc.SalesLine, bySKU map[string]string, perUSD map[string]float64, unconverted map[string]bool) ([]store.Measured, []store.Item) {
	skus := map[string]string{}
	for k, v := range bySKU {
		skus[k] = v
	}
	for _, l := range lines {
		if isApp(Kind(l.ProductType)) && l.SKU != "" {
			skus[l.SKU] = l.AppleID
		}
	}

	type key struct{ app, metric, territory, item string }
	sums := map[key]int64{}
	var order []key
	add := func(k key, v int64) {
		if v == 0 {
			return
		}
		if _, ok := sums[k]; !ok {
			order = append(order, k)
		}
		sums[k] += v
	}
	items := map[string]store.Item{}
	for _, l := range lines {
		kind := Kind(l.ProductType)
		app := l.AppleID
		if !isApp(kind) {
			app = skus[l.ParentSKU]
		}
		inApp := kind == kindInApp
		item := ""
		if inApp {
			item = l.AppleID
		}
		if metric := counts[kind]; metric != "" {
			add(key{app, metric, l.Country, item}, l.Units)
		}
		if l.Proceeds == 0 {
			continue
		}
		rate := 1.0
		if l.Currency != "USD" {
			if rate = perUSD[l.Currency]; rate <= 0 {
				unconverted[l.Currency] = true
				continue
			}
		}
		add(key{app, store.MetricProceeds, l.Country, l.AppleID}, int64(math.Round(float64(l.Proceeds)/rate)))
		items[l.AppleID] = store.Item{ID: l.AppleID, AppID: app, Name: l.Title, InApp: inApp}
	}

	out := make([]store.Measured, 0, len(order))
	for _, k := range order {
		out = append(out, store.Measured{AppID: k.app, Day: day, Metric: k.metric, Territory: k.territory, Item: k.item, Value: sums[k]})
	}
	list := make([]store.Item, 0, len(items))
	for _, it := range items {
		list = append(list, it)
	}
	return out, list
}

// What a sales line counts, told from its product type.
const (
	kindDownload   = "download"
	kindRedownload = "redownload"
	kindUpdate     = "update"
	kindInApp      = "in_app"
	kindOther      = "other"
)

// counts is the measure a line's units are, by what the line counts.
var counts = map[string]string{
	kindDownload:   store.MetricFirstDownloads,
	kindRedownload: store.MetricRedownloads,
	kindUpdate:     store.MetricUpdates,
	kindInApp:      store.MetricPurchases,
}

// Kind tells what a line counts from its Product Type Identifier. See docs/app-store-connect.md.
func Kind(productType string) string {
	t := strings.ToUpper(strings.TrimSpace(productType))
	switch {
	case t == "7" || t == "7F" || t == "7T" || t == "F7":
		return kindUpdate
	case t == "3" || t == "3F" || t == "3T" || t == "F3":
		return kindRedownload
	case strings.HasPrefix(t, "IA") || strings.HasPrefix(t, "FI"):
		return kindInApp
	case strings.HasPrefix(t, "1") || strings.HasPrefix(t, "F1"):
		return kindDownload
	}
	return kindOther
}

func isApp(kind string) bool {
	return kind == kindDownload || kind == kindRedownload || kind == kindUpdate
}
