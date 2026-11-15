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
	"strings"
	"sync"
	"testing"
	"time"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/config"
	"appstoreanalytics/internal/store"
)

const header = "Provider\tSKU\tTitle\tProduct Type Identifier\tUnits\tDeveloper Proceeds\tCountry Code\tCurrency of Proceeds\tApple Identifier\tParent Identifier\n"

// apple stands in for App Store Connect: one app, a report for 2026-11-14 with a download of it
// and a purchase in it, no sales the day before, nothing yet for the day after, and analytics
// refused as for a key without the Admin role.
type apple struct {
	mu    sync.Mutex
	asked map[string]int
}

func (a *apple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	day := r.URL.Query().Get("filter[reportDate]")
	a.asked[day]++
	a.mu.Unlock()
	refuse := func(status int, detail string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"detail": detail}}})
	}
	switch {
	case r.URL.Path == "/v1/apps":
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"id": "1234567890", "attributes": map[string]any{"name": "Dots", "bundleId": "com.acme.dots", "sku": "dots.app"}},
		}})
	case r.URL.Path == "/v1/salesReports" && day == "2026-11-14":
		var b bytes.Buffer
		z := gzip.NewWriter(&b)
		z.Write([]byte(header +
			"APPLE\tdots.app\tDots\t1F\t5\t0\tUS\tUSD\t1234567890\t \n" +
			"APPLE\tdots.coins\tCoins\tIA1\t2\t1.50\tUS\tUSD\t555\tdots.app\n"))
		z.Close()
		w.Write(b.Bytes())
	case r.URL.Path == "/v1/salesReports" && day == "2026-11-15":
		refuse(http.StatusNotFound, "Report is not available yet.")
	case r.URL.Path == "/v1/salesReports":
		refuse(http.StatusNotFound, "There were no sales for the date specified.")
	case strings.HasSuffix(r.URL.Path, "/analyticsReportRequests"):
		refuse(http.StatusForbidden, "This request is forbidden for security reasons")
	default:
		refuse(http.StatusNotFound, "no such thing")
	}
}

func TestAPassReadsTheYearOfSalesAndSaysWhyAnalyticsAreNot(t *testing.T) {
	a := &apple{asked: map[string]int{}}
	srv := httptest.NewServer(a)
	defer srv.Close()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := asc.New(config.Apple{BaseURL: srv.URL, IssuerID: "i", KeyID: "k", Key: key})

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	p := New(st, client, "00000000", slog.New(slog.DiscardHandler))
	// 10:00 on the 16th in Pacific time: the 15th has ended and is not reported yet.
	p.now = func() time.Time { return time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC) }
	p.rates = func(context.Context) (map[string]float64, error) { return map[string]float64{"USD": 1}, nil }
	ctx := context.Background()
	p.Pass(ctx)

	first, last, days, err := st.SalesSpan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Every day of the year but the 15th, which is asked for again next pass.
	if days != SalesHistory-1 || last != "2026-11-14" || first != "2025-11-16" {
		t.Errorf("read %d days, %s to %s", days, first, last)
	}
	sales, err := st.SalesIn(ctx, "2026-11-14", "2026-11-14", "2026-11-13", "")
	if err != nil {
		t.Fatal(err)
	}
	if sales.Totals.Downloads != 5 || sales.Totals.InApp != 2 || sales.Totals.Proceeds != 3 {
		t.Errorf("totals = %+v", sales.Totals)
	}
	// The purchase is the app's, by the SKU it names as its parent.
	if len(sales.Apps) != 1 || sales.Apps[0].ID != "1234567890" || sales.Apps[0].Name != "Dots" {
		t.Errorf("apps = %+v", sales.Apps)
	}

	states, _ := st.SyncStates(ctx)
	if states[store.PartSales].Error != "" || states[store.PartApps].Error != "" {
		t.Errorf("states = %+v", states)
	}
	if why := states[store.PartAnalytics].Error; !strings.Contains(why, "Admin") {
		t.Errorf("analytics said %q, want it to name the role it takes", why)
	}

	// A second pass asks only for the days not settled: the 15th, and the 14th, which was read
	// before it settled.
	a.asked = map[string]int{}
	p.Pass(ctx)
	delete(a.asked, "")
	if len(a.asked) != 2 || a.asked["2026-11-15"] != 1 || a.asked["2026-11-14"] != 1 {
		t.Errorf("the second pass asked for %v", a.asked)
	}
}

// Stopping the server is not a part failing, and what the last pass said stands.
func TestAPassCutShortRecordsNothing(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p := New(st, asc.New(config.Apple{BaseURL: "http://127.0.0.1:1", IssuerID: "i", KeyID: "k", Key: key}), "1", slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Pass(ctx)
	states, _ := st.SyncStates(context.Background())
	if len(states) != 0 {
		t.Errorf("states = %+v, want none", states)
	}
	if p.Reading() != "" || p.Running() {
		t.Error("a finished pass still says it is reading")
	}
}

func TestAProductTypeSaysWhatALineCounts(t *testing.T) {
	for productType, kind := range map[string]string{
		"1": store.KindDownload, "1F": store.KindDownload, "F1": store.KindDownload, "1-B": store.KindDownload,
		"3": store.KindRedownload, "3F": store.KindRedownload,
		"7": store.KindUpdate, "F7": store.KindUpdate,
		"IA1": store.KindInApp, "IAY": store.KindInApp, "IA1-M": store.KindInApp, "FI1": store.KindInApp,
		"XX": store.KindOther,
	} {
		if got := Kind(productType); got != kind {
			t.Errorf("Kind(%q) = %q, want %q", productType, got, kind)
		}
	}
}
