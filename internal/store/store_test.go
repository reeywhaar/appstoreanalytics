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

func measure(app, day, metric, territory, source, item string, value int64) Measured {
	return Measured{AppID: app, Day: day, Metric: metric, Territory: territory, Source: source, Item: item, Value: value}
}

func TestAReportSumsAndComparesWithTheDaysBefore(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	if err := st.SaveApps(ctx, []App{{ID: "1", Name: "Dots"}, {ID: "2", Name: "Lines"}}); err != nil {
		t.Fatal(err)
	}
	save := func(id, app string, yields []string, items []Item, measured ...Measured) {
		t.Helper()
		if err := st.SaveInstance(ctx, Instance{ID: id, AppID: app, Report: "r"}, yields, measured, items); err != nil {
			t.Fatal(err)
		}
	}
	downloads := []string{MetricFirstDownloads}
	purchases := []string{MetricPurchases, MetricProceeds}
	save("d1", "1", downloads, nil,
		measure("1", "2026-11-01", MetricFirstDownloads, "US", "App Store search", "", 4),
		measure("1", "2026-11-02", MetricFirstDownloads, "US", "App Store search", "", 10),
		measure("1", "2026-11-03", MetricFirstDownloads, "DE", "Web referrer", "", 1),
	)
	save("d2", "2", downloads, nil, measure("2", "2026-11-02", MetricFirstDownloads, "JP", "App Store browse", "", 7))
	save("p1", "1", purchases, []Item{{ID: "11", AppID: "1", Name: "100 Coins", InApp: true}, {ID: "1", AppID: "1", Name: "Dots", InApp: false}},
		measure("1", "2026-11-02", MetricPurchases, "US", "", "11", 2),
		measure("1", "2026-11-02", MetricProceeds, "US", "", "11", 1_400_000),
		measure("1", "2026-11-03", MetricProceeds, "DE", "", "1", 2_990_000),
	)

	got, err := st.ReportIn(ctx, "2026-11-02", "2026-11-03", "2026-10-31", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Totals[MetricFirstDownloads] != int64(18) || got.Covered[MetricFirstDownloads] != 2 {
		t.Errorf("downloads = %v over %v days", got.Totals[MetricFirstDownloads], got.Covered[MetricFirstDownloads])
	}
	// Millionths of a dollar in, dollars and cents out.
	if got.Totals[MetricProceeds] != 4.39 {
		t.Errorf("proceeds = %v, want 4.39", got.Totals[MetricProceeds])
	}
	if got.Previous[MetricFirstDownloads] != int64(4) || got.PreviousCovered[MetricFirstDownloads] != 1 {
		t.Errorf("previous = %v", got.Previous)
	}
	if len(got.Apps) != 2 || got.Apps[0]["name"] != "Dots" || got.Apps[0][MetricProceeds] != 4.39 {
		t.Errorf("apps = %v", got.Apps)
	}
	// An app bought is the app, which has its own row; only what is bought inside one is listed.
	if len(got.InApp) != 1 || got.InApp[0]["name"] != "100 Coins" || got.InApp[0]["app_name"] != "Dots" {
		t.Errorf("in-app purchases = %v", got.InApp)
	}
	if len(got.Countries) != 3 || got.Countries[0]["id"] != "US" {
		t.Errorf("countries = %v", got.Countries)
	}

	one, err := st.ReportIn(ctx, "2026-11-02", "2026-11-03", "2026-10-31", "2")
	if err != nil {
		t.Fatal(err)
	}
	if one.Totals[MetricFirstDownloads] != int64(7) || len(one.InApp) != 0 {
		t.Errorf("one app = %v, %v", one.Totals, one.InApp)
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
			measure("1", "2026-11-02", MetricImpressions, "US", "App Store search", "", impressions),
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	save("daily", 100)
	save("snapshot", 120)

	got, err := st.ReportIn(ctx, "2026-11-01", "2026-11-03", "2026-10-29", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Totals[MetricImpressions] != int64(120) || got.Covered[MetricImpressions] != 1 {
		t.Errorf("totals = %v, covered = %v", got.Totals, got.Covered)
	}
	if _, ok := got.Totals[MetricSessions]; ok {
		t.Error("a measure nothing reported has a total")
	}
	if len(got.Sources) != 1 || got.Sources[0]["id"] != "App Store search" {
		t.Errorf("sources = %v", got.Sources)
	}
	read, _ := st.InstancesRead(ctx)
	if !read["daily"] || !read["snapshot"] {
		t.Errorf("read = %v", read)
	}
}

// A sales report fills in until analytics say the same app's day, and then gives way; read after
// analytics, it fills in nothing analytics said.
func TestASalesDayGivesWayToAnalytics(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	downloads := func() any {
		t.Helper()
		r, err := st.ReportIn(ctx, "2026-11-14", "2026-11-14", "2026-11-13", "")
		if err != nil {
			t.Fatal(err)
		}
		return r.Totals[MetricFirstDownloads]
	}
	sales := []Measured{measure("1", "2026-11-14", MetricFirstDownloads, "US", "", "", 5)}
	analytics := []Measured{measure("1", "2026-11-14", MetricFirstDownloads, "US", "App Store search", "", 6)}

	if err := st.SaveSalesDay(ctx, "2026-11-14", sales, nil); err != nil {
		t.Fatal(err)
	}
	if got := downloads(); got != int64(5) {
		t.Fatalf("with sales alone = %v, want 5", got)
	}
	if err := st.SaveInstance(ctx, Instance{ID: "a", AppID: "1"}, []string{MetricFirstDownloads}, analytics, nil); err != nil {
		t.Fatal(err)
	}
	if got := downloads(); got != int64(6) {
		t.Errorf("once analytics said it = %v, want 6", got)
	}
	if err := st.SaveSalesDay(ctx, "2026-11-14", sales, nil); err != nil {
		t.Fatal(err)
	}
	if got := downloads(); got != int64(6) {
		t.Errorf("the sales report read again = %v, want analytics' 6 still", got)
	}
}

// A day whose sales report was read and sold nothing is known, at zero: not a day missing.
func TestADayOfNoSalesIsZero(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	if err := st.SaveSalesDay(ctx, "2026-11-13", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSalesDay(ctx, "2026-11-14", []Measured{measure("1", "2026-11-14", MetricFirstDownloads, "US", "", "", 5)}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReportIn(ctx, "2026-11-12", "2026-11-14", "2026-11-09", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Covered[MetricFirstDownloads] != 2 || got.Covered[MetricProceeds] != 2 {
		t.Errorf("covered = %v, want both days read", got.Covered)
	}
	if got.Totals[MetricProceeds] != 0.0 || got.Totals[MetricFirstDownloads] != int64(5) {
		t.Errorf("totals = %v", got.Totals)
	}
	if len(got.Days) != 2 || got.Days[0]["day"] != "2026-11-13" || got.Days[0][MetricFirstDownloads] != int64(0) {
		t.Errorf("days = %v", got.Days)
	}
	if _, ok := got.Covered[MetricImpressions]; ok {
		t.Error("a sales report said something of impressions")
	}
}
