package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/config"
	"appstoreanalytics/internal/pull"
	"appstoreanalytics/internal/store"
)

// stand is the stand-in on a test server, its clock the test's, and appstoreanalytics' store
// and sync reading it.
type stand struct {
	mu   sync.Mutex
	now  time.Time
	st   *store.Store
	pass func()
}

func (s *stand) at(t time.Time) { s.mu.Lock(); s.now = t; s.mu.Unlock() }

func (s *stand) clock() time.Time { s.mu.Lock(); defer s.mu.Unlock(); return s.now }

func newStand(t *testing.T, signer *ecdsa.PrivateKey) *stand {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if signer == nil {
		signer = key
	}
	s := &stand{}
	fake := &Server{
		Issuer: "issuer", KeyID: "KEY", Key: &key.PublicKey,
		Log: slog.New(slog.DiscardHandler), Now: s.clock,
	}
	var h http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)
	fake.Origin = srv.URL
	state, err := loadState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	h = fake.Handler(state)

	if s.st, err = store.Open(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.st.Close() })
	s.st.SetClock(s.clock)
	client := asc.New(config.Apple{BaseURL: srv.URL, IssuerID: "issuer", KeyID: "KEY", Key: signer})
	p := pull.New(s.st, client, "", slog.New(slog.DiscardHandler))
	s.pass = func() { p.Pass(context.Background()) }
	return s
}

// A first sync asks for each app's reports and reads the history the snapshot holds at once, to
// two days before; the days after come one a day from the ongoing request.
func TestASyncReadsTheAccountsHistoryThenADayADay(t *testing.T) {
	s := newStand(t, nil)
	ctx := context.Background()
	// Morning in California on 2024-02-20.
	s.at(time.Date(2024, 2, 20, 18, 0, 0, 0, time.UTC))
	s.pass()

	states, err := s.st.SyncStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if why := states[store.PartAnalytics].Error; why != "" || states[store.PartAnalytics].OKAt.IsZero() {
		t.Fatalf("the analytics part did not go through: %q", why)
	}
	listed, err := s.st.Apps(ctx)
	if err != nil || len(listed) != len(apps) {
		t.Fatalf("apps = %d, %v; want %d", len(listed), err, len(apps))
	}
	first, last, _, err := s.st.MetricsSpan(ctx)
	if err != nil || first != "2024-01-01" || last != "2024-02-18" {
		t.Fatalf("measured %s to %s, %v; want 2024-01-01 to 2024-02-18", first, last, err)
	}

	s.at(time.Date(2024, 2, 23, 18, 0, 0, 0, time.UTC))
	s.pass()
	if _, last, _, _ = s.st.MetricsSpan(ctx); last != "2024-02-21" {
		t.Fatalf("three days on, measured to %s; want 2024-02-21", last)
	}

	// Every report says something of every app's days, and the downloads read are near what the
	// account's figures expect.
	r, err := s.st.ReportIn(ctx, "2024-02-01", "2024-02-21", "2024-01-11", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{
		store.MetricFirstDownloads, store.MetricRedownloads, store.MetricUpdates, store.MetricProceeds,
		store.MetricPurchases, store.MetricImpressions, store.MetricPageViews, store.MetricInstalls,
		store.MetricDeletions, store.MetricSessions, store.MetricCrashes,
	} {
		if r.Covered[m] != 21 {
			t.Errorf("%s covers %d days; want 21", m, r.Covered[m])
		}
	}
	var want float64
	for day := date(2024, 2, 1); !day.After(date(2024, 2, 21)); day = day.AddDate(0, 0, 1) {
		for _, a := range apps {
			want += a.figures(day).Downloads
		}
	}
	got := float64(r.Totals[store.MetricFirstDownloads].(int64))
	if math.Abs(got-want)/want > 0.03 {
		t.Errorf("downloads = %.0f; the figures expect about %.0f", got, want)
	}
	if len(r.Countries) < 20 || len(r.Sources) != len(sources) || len(r.InApp) == 0 {
		t.Errorf("countries %d, sources %d, in-app purchases %d", len(r.Countries), len(r.Sources), len(r.InApp))
	}
}

// A key App Store Connect does not know is refused, and the sync says so.
func TestAnotherKeyIsRefused(t *testing.T) {
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s := newStand(t, other)
	s.at(time.Date(2024, 2, 20, 18, 0, 0, 0, time.UTC))
	s.pass()
	states, _ := s.st.SyncStates(context.Background())
	if why := states[store.PartApps].Error; why == "" {
		t.Fatal("a token signed with another key went through")
	}
}

// The same day of the same report is written the same every time it is asked for.
func TestADayIsWrittenTheSameEachTime(t *testing.T) {
	a, rep := apps[1], reportNamed("App Store Purchases Standard")
	write := func() string {
		r, w := io.Pipe()
		go func() { w.CloseWithError(rep.write(w, a, date(2025, 12, 20), date(2025, 12, 31))) }()
		b, _ := io.ReadAll(r)
		return string(b)
	}
	if first, again := write(), write(); first != again || len(first) < 1000 {
		t.Errorf("written %d bytes, then %d differently", len(first), len(again))
	}
}
