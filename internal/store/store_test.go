package store

import (
	"context"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// The suite is not testing bcrypt.
func TestMain(m *testing.M) {
	SetBcryptCost(bcrypt.MinCost)
	os.Exit(m.Run())
}

// A temporary file, not :memory:. WAL behaves differently in memory and WAL is what is relied on.
func openStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestOpenRefusesADirectoryThatIsNotThere(t *testing.T) {
	if _, err := Open(t.TempDir() + "/missing"); err == nil {
		t.Fatal("opened a directory that does not exist")
	}
}

func TestChangingThePasswordSignsEveryBrowserOut(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	if _, err := st.KeepCredential(ctx, "misha", "a good password"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(ctx, "token", "Firefox"); err != nil {
		t.Fatal(err)
	}

	out, err := st.KeepCredential(ctx, "misha", "a good password")
	if err != nil || out {
		t.Fatalf("the same password signed out = %v, err %v", out, err)
	}
	if _, err := st.SessionByToken(ctx, "token"); err != nil {
		t.Fatalf("the session went with the same password: %v", err)
	}

	out, err = st.KeepCredential(ctx, "misha", "a new password")
	if err != nil || !out {
		t.Fatalf("a new password signed out = %v, err %v", out, err)
	}
	if _, err := st.SessionByToken(ctx, "token"); err != ErrNotFound {
		t.Fatalf("the session outlived the password: %v", err)
	}
}

func TestASessionLapses(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	st.SetClock(func() time.Time { return now })
	if _, err := st.CreateSession(ctx, "token", ""); err != nil {
		t.Fatal(err)
	}
	now = now.Add(SessionLifetime + time.Second)
	if _, err := st.SessionByToken(ctx, "token"); err != ErrNotFound {
		t.Fatalf("a lapsed session answered: %v", err)
	}
	if n, _ := st.SweepSessions(ctx); n != 1 {
		t.Errorf("swept %d sessions, want 1", n)
	}
}

func TestSalesAreSummedInDollarsAndComparedWithTheDaysBefore(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	if err := st.SaveApps(ctx, []App{{ID: "1", Name: "Dots", SKU: "dots"}, {ID: "2", Name: "Lines", SKU: "lines"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveRates(ctx, map[string]float64{"USD": 1, "EUR": 0.5}); err != nil {
		t.Fatal(err)
	}
	day := func(d string, sales ...Sale) {
		t.Helper()
		if err := st.SaveSalesDay(ctx, d, sales, nil); err != nil {
			t.Fatal(err)
		}
	}
	day("2026-11-01", Sale{AppID: "1", ProductID: "1", ProductType: "1", Kind: KindDownload, Country: "US", Currency: "USD", Units: 4})
	day("2026-11-02",
		Sale{AppID: "1", ProductID: "1", ProductType: "1", Kind: KindDownload, Country: "US", Currency: "USD", Units: 10},
		Sale{AppID: "1", ProductID: "11", ProductType: "IA1", Kind: KindInApp, Country: "DE", Currency: "EUR", Units: 2, Proceeds: 1_000_000},
		Sale{AppID: "2", ProductID: "2", ProductType: "1F", Kind: KindDownload, Country: "JP", Currency: "JPY", Units: 1, Proceeds: 300_000_000},
	)
	day("2026-11-03")

	got, err := st.SalesIn(ctx, "2026-11-02", "2026-11-03", "2026-11-00", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Totals.Days != 2 || got.Totals.Downloads != 11 || got.Totals.InApp != 2 {
		t.Errorf("totals = %+v", got.Totals)
	}
	// €1 at 0.5 a dollar is $2; yen have no rate and are left out, and said to be.
	if got.Totals.Proceeds != 2 {
		t.Errorf("proceeds = %v, want 2", got.Totals.Proceeds)
	}
	if len(got.Unconverted) != 1 || got.Unconverted[0] != "JPY" {
		t.Errorf("unconverted = %v", got.Unconverted)
	}
	if got.Previous.Days != 1 || got.Previous.Downloads != 4 {
		t.Errorf("previous = %+v", got.Previous)
	}
	if len(got.Apps) != 2 || got.Apps[0].Name != "Dots" || got.Apps[0].Proceeds != 2 {
		t.Errorf("apps = %+v", got.Apps)
	}

	one, err := st.SalesIn(ctx, "2026-11-02", "2026-11-03", "2026-11-00", "2")
	if err != nil {
		t.Fatal(err)
	}
	if one.Totals.Downloads != 1 || len(one.Countries) != 1 || one.Countries[0].ID != "JP" {
		t.Errorf("one app = %+v", one)
	}
}

// A snapshot and the day's own instance say the same thing, and reading both must not count it
// twice.
func TestAnInstanceReplacesTheDaysItCovers(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	yields := []string{MetricImpressions, MetricPageViews}
	save := func(id string, impressions int64) {
		t.Helper()
		err := st.SaveInstance(ctx, Instance{ID: id, AppID: "1", Report: "r"}, yields, []Measured{
			{AppID: "1", Day: "2026-11-02", Metric: MetricImpressions, Territory: "US", Source: "App Store search", Value: impressions},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	save("daily", 100)
	save("snapshot", 120)

	got, err := st.AnalyticsIn(ctx, "2026-11-01", "2026-11-03", "2026-10-29", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Totals[MetricImpressions] != 120 || got.Covered[MetricImpressions] != 1 {
		t.Errorf("totals = %v, covered = %v", got.Totals, got.Covered)
	}
	if _, ok := got.Totals[MetricSessions]; ok {
		t.Error("a measure nothing reported has a total")
	}
	if len(got.Sources) != 1 || got.Sources[0]["source"] != "App Store search" {
		t.Errorf("sources = %v", got.Sources)
	}
	read, _ := st.InstancesRead(ctx)
	if !read["daily"] || !read["snapshot"] {
		t.Errorf("read = %v", read)
	}
}
