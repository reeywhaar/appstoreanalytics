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

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/config"
	"appstoreanalytics/internal/store"
)

const (
	downloadsHeader = "Date\tApp Name\tApp Apple Identifier\tDownload Type\tApp Version\tDevice\tPlatform Version\tSource Type\tPage Type\tPre-Order\tTerritory\tCounts\n"
	purchasesHeader = "Date\tApp Name\tApp Apple Identifier\tPurchase Type\tContent Name\tContent Apple Identifier\tTerritory\tPurchases\tProceeds in USD\tSales in USD\tPaying Users\n"
)

// apple stands in for App Store Connect: one app, whose ongoing request makes a daily
// downloads instance and a purchases one, and whose snapshot, processed a day later, says the
// same day again with one more download.
type apple struct {
	srv *httptest.Server

	mu       sync.Mutex
	forbid   bool
	requests []map[string]any
	created  []string
	files    int
}

var reports = map[string][]map[string]any{
	"ongoing": {
		{"id": "o-downloads", "attributes": map[string]any{"name": "App Downloads Standard"}},
		{"id": "o-purchases", "attributes": map[string]any{"name": "App Store Purchases Standard"}},
		{"id": "o-detailed", "attributes": map[string]any{"name": "App Downloads Detailed"}},
	},
	"snapshot": {
		{"id": "s-downloads", "attributes": map[string]any{"name": "App Downloads Standard"}},
	},
}

var instances = map[string][]map[string]any{
	"o-downloads": {{"id": "daily-13", "attributes": map[string]any{"processingDate": "2026-11-13"}}},
	"o-purchases": {{"id": "buys-13", "attributes": map[string]any{"processingDate": "2026-11-13"}}},
	"s-downloads": {{"id": "snap-14", "attributes": map[string]any{"processingDate": "2026-11-14"}}},
}

var files = map[string]string{
	"daily-13": downloadsHeader +
		"2026-11-13\tDots\t1234567890\tFirst-time download\t1.2\tiPhone\tiOS 26.1\tApp Store search\tProduct page\tNo\tUS\t5\n" +
		"2026-11-13\tDots\t1234567890\tAuto-update\t1.2\tiPhone\tiOS 26.1\tApp Store search\tProduct page\tNo\tUS\t40\n",
	"snap-14": downloadsHeader +
		"2026-11-12\tDots\t1234567890\tFirst-time download\t1.2\tiPhone\tiOS 26.1\tApp Store search\tProduct page\tNo\tUS\t3\n" +
		"2026-11-13\tDots\t1234567890\tFirst-time download\t1.2\tiPhone\tiOS 26.1\tApp Store search\tProduct page\tNo\tUS\t6\n",
	"buys-13": purchasesHeader +
		"2026-11-13\tDots\t1234567890\tIn-app purchases\t100 Coins\t555\tUS\t2\t1.40\t1.98\t2\n",
}

func (a *apple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	reply := func(v any) { json.NewEncoder(w).Encode(map[string]any{"data": v}) }
	path := r.URL.Path
	switch {
	case path == "/v1/apps":
		reply([]map[string]any{{"id": "1234567890", "attributes": map[string]any{"name": "Dots", "bundleId": "com.acme.dots"}}})
	case path == "/v1/apps/1234567890/analyticsReportRequests":
		reply(a.requests)
	case path == "/v1/analyticsReportRequests" && r.Method == http.MethodPost:
		if a.forbid {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"detail": "This request is forbidden for security reasons"}}})
			return
		}
		var body struct {
			Data struct {
				Attributes struct {
					AccessType string `json:"accessType"`
				} `json:"attributes"`
			} `json:"data"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		id := map[string]string{asc.Ongoing: "ongoing", asc.Snapshot: "snapshot"}[body.Data.Attributes.AccessType]
		a.created = append(a.created, id)
		made := map[string]any{"id": id, "attributes": map[string]any{"accessType": body.Data.Attributes.AccessType}}
		a.requests = append(a.requests, made)
		w.WriteHeader(http.StatusCreated)
		reply(made)
	case strings.HasPrefix(path, "/v1/analyticsReportRequests/"):
		reply(reports[strings.Split(path, "/")[3]])
	case strings.HasPrefix(path, "/v1/analyticsReports/"):
		if r.URL.Query().Get("filter[granularity]") != "DAILY" {
			reply([]any{})
			return
		}
		reply(instances[strings.Split(path, "/")[3]])
	case strings.HasPrefix(path, "/v1/analyticsReportInstances/"):
		id := strings.Split(path, "/")[3]
		reply([]map[string]any{{"id": "seg-" + id, "attributes": map[string]any{"url": a.srv.URL + "/files/" + id}}})
	case strings.HasPrefix(path, "/files/"):
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "a signed address takes no token", http.StatusBadRequest)
			return
		}
		a.files++
		var b bytes.Buffer
		z := gzip.NewWriter(&b)
		z.Write([]byte(files[strings.TrimPrefix(path, "/files/")]))
		z.Close()
		w.Write(b.Bytes())
	default:
		http.NotFound(w, r)
	}
}

func setup(t *testing.T) (*apple, *Puller, *store.Store) {
	t.Helper()
	a := &apple{}
	a.srv = httptest.NewServer(a)
	t.Cleanup(a.srv.Close)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	client := asc.New(config.Apple{BaseURL: a.srv.URL, IssuerID: "i", KeyID: "k", Key: key})
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return a, New(st, client, "", slog.New(slog.DiscardHandler)), st
}

func TestAPassAsksForReportsAndReadsThemOldestFirst(t *testing.T) {
	a, p, st := setup(t)
	ctx := context.Background()
	p.Pass(ctx)

	if len(a.created) != 2 {
		t.Errorf("asked for %v, want an ongoing request and a snapshot", a.created)
	}
	if apps, since, err := st.Requested(ctx); err != nil || apps != 1 || since.IsZero() {
		t.Errorf("requested = %d apps since %v, err %v", apps, since, err)
	}
	report, err := st.ReportIn(ctx, "2026-11-12", "2026-11-13", "2026-11-10", "")
	if err != nil {
		t.Fatal(err)
	}
	// The snapshot was processed after the daily instance, and says the 13th had 6.
	if report.Totals[store.MetricFirstDownloads] != int64(9) {
		t.Errorf("first-time downloads = %v, want 3 + 6", report.Totals[store.MetricFirstDownloads])
	}
	// The snapshot's report yields updates too, and says none: what the daily one said goes.
	if _, ok := report.Totals[store.MetricUpdates]; ok {
		t.Errorf("updates = %v, want the snapshot's word on the 13th", report.Totals[store.MetricUpdates])
	}
	if report.Totals[store.MetricProceeds] != 1.4 || report.Totals[store.MetricPurchases] != int64(2) {
		t.Errorf("totals = %v", report.Totals)
	}
	if len(report.InApp) != 1 || report.InApp[0]["name"] != "100 Coins" || report.InApp[0]["app_name"] != "Dots" {
		t.Errorf("in-app purchases = %v", report.InApp)
	}
	states, _ := st.SyncStates(ctx)
	if states[store.PartAnalytics].Error != "" || states[store.PartApps].Error != "" {
		t.Errorf("states = %+v", states)
	}

	// The second pass asks for nothing new and reads nothing twice.
	a.files, a.created = 0, nil
	p.Pass(ctx)
	if a.files != 0 || len(a.created) != 0 {
		t.Errorf("the second pass read %d files and asked for %v", a.files, a.created)
	}
}

func TestAKeyWithoutTheAdminRoleIsToldSo(t *testing.T) {
	a, p, st := setup(t)
	a.forbid = true
	ctx := context.Background()
	p.Pass(ctx)
	states, _ := st.SyncStates(ctx)
	if why := states[store.PartAnalytics].Error; !strings.Contains(why, "Admin") {
		t.Errorf("analytics said %q, want it to name the role it takes", why)
	}
	if states[store.PartApps].Error != "" {
		t.Errorf("apps said %q", states[store.PartApps].Error)
	}
}

// A key Apple refuses is the whole story: the analytics part is not said to have gone through
// with nothing to read.
func TestARefusedKeyEndsThePass(t *testing.T) {
	_, p, st := setup(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"detail": "Provide a properly configured and signed bearer token."}}})
	}))
	defer srv.Close()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p.asc = asc.New(config.Apple{BaseURL: srv.URL, IssuerID: "i", KeyID: "k", Key: key})
	ctx := context.Background()
	p.Pass(ctx)
	states, _ := st.SyncStates(ctx)
	if !strings.Contains(states[store.PartApps].Error, "refused the key") {
		t.Errorf("apps said %q", states[store.PartApps].Error)
	}
	if _, ok := states[store.PartAnalytics]; ok {
		t.Errorf("analytics said %+v, want nothing", states[store.PartAnalytics])
	}
}

// Stopping the server is not a part failing, and what the last pass said stands.
func TestAPassCutShortRecordsNothing(t *testing.T) {
	_, p, st := setup(t)
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
