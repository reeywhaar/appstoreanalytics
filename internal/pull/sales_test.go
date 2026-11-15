package pull

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/config"
	"appstoreanalytics/internal/store"
)

func gzipped(s string) []byte {
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	z.Write([]byte(s))
	z.Close()
	return b.Bytes()
}

const summaryHeader = "Provider\tSKU\tTitle\tProduct Type Identifier\tUnits\tDeveloper Proceeds\tCountry Code\tCurrency of Proceeds\tApple Identifier\tParent Identifier\n"

// fallbackApple stands in for App Store Connect with one app, Dots, whose sales reports say the
// 13th and the 14th, and whose analytics reports say only the 14th, differently.
func fallbackApple(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(v any) { json.NewEncoder(w).Encode(map[string]any{"data": v}) }
		switch r.URL.Path {
		case "/v1/apps":
			reply([]map[string]any{{"id": "42", "attributes": map[string]any{"name": "Dots", "sku": "dots"}}})
		case "/v1/salesReports":
			switch r.URL.Query().Get("filter[reportDate]") {
			case "2026-11-13":
				w.Write(gzipped(summaryHeader +
					"APPLE\tdots\tDots\t1F\t4\t0\tUS\tUSD\t42\t \n" +
					"APPLE\tdots.coins\t100 Coins\tIA1\t2\t1.50\tDE\tEUR\t77\tdots\n"))
			case "2026-11-14":
				w.Write(gzipped(summaryHeader + "APPLE\tdots\tDots\t1F\t5\t0\tUS\tUSD\t42\t \n"))
			default:
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"detail": "There were no sales for the date specified."}}})
			}
		case "/v1/apps/42/analyticsReportRequests":
			reply([]map[string]any{
				{"id": "ongoing", "attributes": map[string]any{"accessType": "ONGOING"}},
				{"id": "snapshot", "attributes": map[string]any{"accessType": "ONE_TIME_SNAPSHOT"}},
			})
		case "/v1/analyticsReportRequests/ongoing/reports":
			reply([]map[string]any{{"id": "downloads", "attributes": map[string]any{"name": "App Downloads Standard"}}})
		case "/v1/analyticsReports/downloads/instances":
			reply([]map[string]any{{"id": "day-14", "attributes": map[string]any{"processingDate": "2026-11-14"}}})
		case "/v1/analyticsReportInstances/day-14/segments":
			reply([]map[string]any{{"attributes": map[string]any{"url": srv.URL + "/files/day-14"}}})
		case "/files/day-14":
			w.Write(gzipped("Date\tDownload Type\tSource Type\tTerritory\tCounts\n2026-11-14\tFirst-time download\tApp Store search\tUS\t6\n"))
		default:
			reply([]any{})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Analytics says the 14th, and the sales report fills in the 13th, in dollars, with the app the
// purchase was bought in.
func TestSalesReportsFillInTheDaysAnalyticsHaveNotSaid(t *testing.T) {
	srv := fallbackApple(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := New(st, asc.New(config.Apple{BaseURL: srv.URL, IssuerID: "i", KeyID: "k", Key: key}), "85", slog.New(slog.DiscardHandler))
	p.now = func() time.Time { return time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC) }
	p.rates = func(context.Context) (map[string]float64, error) {
		return map[string]float64{"USD": 1, "EUR": 0.5}, nil
	}
	ctx := context.Background()
	p.Pass(ctx)

	report, err := st.ReportIn(ctx, "2026-11-13", "2026-11-14", "2026-11-11", "")
	if err != nil {
		t.Fatal(err)
	}
	byDay := map[string]map[string]any{}
	for _, d := range report.Days {
		byDay[d["day"].(string)] = d
	}
	if got := byDay["2026-11-14"][store.MetricFirstDownloads]; got != int64(6) {
		t.Errorf("downloads of the 14th = %v, want analytics' 6 rather than the sales report's 5", got)
	}
	if got := byDay["2026-11-13"][store.MetricFirstDownloads]; got != int64(4) {
		t.Errorf("downloads of the 13th = %v, want the sales report's 4", got)
	}
	// €1.50 at 0.5 to the dollar, twice.
	if report.Totals[store.MetricProceeds] != 6.0 || report.Totals[store.MetricPurchases] != int64(2) {
		t.Errorf("totals = %v", report.Totals)
	}
	if len(report.InApp) != 1 || report.InApp[0]["name"] != "100 Coins" || report.InApp[0]["app_name"] != "Dots" {
		t.Errorf("in-app purchases = %v", report.InApp)
	}
	states, _ := st.SyncStates(ctx)
	for _, part := range []string{store.PartApps, store.PartAnalytics, store.PartRates, store.PartSales} {
		if states[part].Error != "" || states[part].OKAt.IsZero() {
			t.Errorf("%s = %+v", part, states[part])
		}
	}
}

// Without a vendor number there is nothing to fill in with, and no sales part at all.
func TestWithoutAVendorNumberNoSalesAreRead(t *testing.T) {
	srv := fallbackApple(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := New(st, asc.New(config.Apple{BaseURL: srv.URL, IssuerID: "i", KeyID: "k", Key: key}), "", slog.New(slog.DiscardHandler))
	ctx := context.Background()
	p.Pass(ctx)
	states, _ := st.SyncStates(ctx)
	if _, ok := states[store.PartSales]; ok {
		t.Errorf("sales ran: %+v", states[store.PartSales])
	}
	if _, _, days, _ := st.SalesSpan(ctx); days != 0 {
		t.Errorf("read %d days of sales", days)
	}
}
