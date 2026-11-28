package store

import (
	"context"
	"os"
	"strings"
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
	if _, err := st.CreateSession(ctx, "token", "Firefox", "203.0.113.9"); err != nil {
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
	if _, err := st.CreateSession(ctx, "token", "", ""); err != nil {
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

// A country's or a source's row says each app's part of it, for a bar that shows whose its length
// is: only with every app read, and not on the apps' own rows, which are each one app's already.
func TestABreakdownRowSaysEachAppsPart(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	if err := st.SaveApps(ctx, []App{{ID: "1", Name: "Dots"}, {ID: "2", Name: "Lines"}}); err != nil {
		t.Fatal(err)
	}
	yields := []string{MetricFirstDownloads}
	for _, inst := range []struct {
		id, app string
		n       int64
	}{{"a", "1", 5}, {"b", "2", 3}} {
		err := st.SaveInstance(ctx, Instance{ID: inst.id, AppID: inst.app, Report: "r"}, yields, []Measured{
			measure(inst.app, "2026-11-02", MetricFirstDownloads, "US", "App Store search", "", inst.n),
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	got, err := st.ReportIn(ctx, "2026-11-02", "2026-11-02", "2026-11-01", "")
	if err != nil {
		t.Fatal(err)
	}
	for name, rows := range map[string][]map[string]any{"countries": got.Countries, "sources": got.Sources} {
		if len(rows) != 1 || rows[0][MetricFirstDownloads] != int64(8) {
			t.Fatalf("%s = %v", name, rows)
		}
		parts, _ := rows[0]["by_app"].(map[string]map[string]any)
		if parts["1"][MetricFirstDownloads] != int64(5) || parts["2"][MetricFirstDownloads] != int64(3) {
			t.Errorf("%s by app = %v", name, rows[0]["by_app"])
		}
	}
	for _, row := range got.Apps {
		if _, ok := row["by_app"]; ok {
			t.Errorf("app %v has parts of its own", row["id"])
		}
	}

	one, err := st.ReportIn(ctx, "2026-11-02", "2026-11-02", "2026-11-01", "1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := one.Countries[0]["by_app"]; ok {
		t.Errorf("one app's countries have parts: %v", one.Countries)
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

// A sales report and an analytics report date one purchase a day apart, so a purchase on a day
// the purchases report has processed is the report's, though the report put it on the next day:
// counted from both it would be two. Past the last day the report has said anything of, a sales
// report still fills in, and once the report gets there, it gives way. Downloads are not so:
// their report leaves small counts out.
func TestASalesDayAnalyticsHasProcessedIsAnalytics(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	purchases := SameReport(MetricPurchases)
	bought := func(day string) []Measured {
		return []Measured{
			measure("1", day, MetricPurchases, "US", "", "pro", 1),
			measure("1", day, MetricProceeds, "US", "", "pro", 4_240_000),
		}
	}
	total := func() any {
		t.Helper()
		got, err := st.ReportIn(ctx, "2026-10-01", "2026-11-10", "2026-09-01", "")
		if err != nil {
			t.Fatal(err)
		}
		return got.Totals[MetricPurchases]
	}

	// The report has processed 10-01 to 11-02, and put the purchase on 11-02.
	if err := st.SaveInstance(ctx, Instance{ID: "a", AppID: "1", Report: "r"}, purchases,
		append(bought("2026-10-01"), bought("2026-11-02")...), nil); err != nil {
		t.Fatal(err)
	}
	// The sales report puts it on 11-01, and another on 11-05 the report has not reached.
	for _, day := range []string{"2026-11-01", "2026-11-05"} {
		if err := st.SaveSalesDay(ctx, day, bought(day), nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := total(); got != int64(3) {
		t.Errorf("purchases = %v, want 3: two from analytics and 11-05's from sales", got)
	}

	// The report reaches 11-06, putting 11-05's purchase there: the sales placeholder goes.
	if err := st.SaveInstance(ctx, Instance{ID: "b", AppID: "1", Report: "r"}, purchases, bought("2026-11-06"), nil); err != nil {
		t.Fatal(err)
	}
	if got := total(); got != int64(3) {
		t.Errorf("purchases = %v, want 3, each once", got)
	}

	// The downloads report leaves small counts out, so its saying nothing of a day's redownloads
	// may be one left out: a sales report's count of them stays.
	if err := st.SaveInstance(ctx, Instance{ID: "c", AppID: "1", Report: "r"}, SameReport(MetricUpdates),
		[]Measured{measure("1", "2026-11-03", MetricUpdates, "US", "", "", 4)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSalesDay(ctx, "2026-11-03", []Measured{measure("1", "2026-11-03", MetricRedownloads, "US", "", "", 2)}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReportIn(ctx, "2026-11-03", "2026-11-03", "2026-11-02", "")
	if err != nil {
		t.Fatal(err)
	}
	if v := got.Totals[MetricRedownloads]; v != int64(2) {
		t.Errorf("redownloads = %v, want the sales report's 2", v)
	}
}

// A type chosen covers its subtypes, and nothing is mailed without somebody to mail.
func TestAChosenTypeMailsItsSubtypes(t *testing.T) {
	m := HookMail{To: "me@example.com", Events: []string{"SUBSCRIBED", "REFUND/UNREPORTED"}}
	for event, want := range map[string]bool{
		"SUBSCRIBED":             true,
		"SUBSCRIBED/INITIAL_BUY": true,
		"SUBSCRIBEDX":            false,
		"REFUND":                 false,
		"REFUND/UNREPORTED":      true,
		"DID_RENEW":              false,
	} {
		if got := m.Mails(event); got != want {
			t.Errorf("Mails(%q) = %v, want %v", event, got, want)
		}
	}
	if (HookMail{Events: []string{"SUBSCRIBED"}}).Mails("SUBSCRIBED") {
		t.Error("mailed with nobody to mail")
	}
}

// A range of every app's days is summed from the day index alone, and one app's is sought by the
// primary key, so neither reads a row twice nor reads another app's rows.
func TestARangeOfDaysIsReadFromAnIndexAlone(t *testing.T) {
	st := openStore(t)
	sold := []string{MetricFirstDownloads, MetricProceeds}
	for _, c := range []struct {
		name string
		sql  func() (string, []any)
		want string
	}{
		{"every app's days", func() (string, []any) { return daysQuery("2026-01-01", "2026-12-31", "") }, "USING COVERING INDEX metrics_day"},
		{"one app's days", func() (string, []any) { return daysQuery("2026-01-01", "2026-12-31", "1234567890") }, "USING PRIMARY KEY (app_id=?"},
		{"every app by country", func() (string, []any) {
			return breakdownQuery("territory", "2026-01-01", "2026-12-31", "", sold)
		}, "USING COVERING INDEX metrics_day"},
		{"one app by country", func() (string, []any) {
			return breakdownQuery("territory", "2026-01-01", "2026-12-31", "1234567890", sold)
		}, "USING PRIMARY KEY (app_id=?"},
	} {
		query, args := c.sql()
		rows, err := st.reader.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		rows.Close()
		if !strings.Contains(strings.Join(plan, "\n"), c.want) {
			t.Errorf("%s is planned as\n%s\nand not %s", c.name, strings.Join(plan, "\n"), c.want)
		}
	}
}

// A report covers every app's day at once: a day it said something of for the account, and
// nothing of for a measure, had none of it, and the days after its last are not reported yet.
// One app's days start at the first it measured anything.
func TestADayAReportSaidNothingOfHadNone(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	save := func(id, app string, measured ...Measured) {
		t.Helper()
		if err := st.SaveInstance(ctx, Instance{ID: id, AppID: app, Report: "r"}, nil, measured, nil); err != nil {
			t.Fatal(err)
		}
	}
	save("a", "1",
		Measured{AppID: "1", Day: "2026-11-10", Metric: MetricFirstDownloads, Territory: "US", Value: 4},
		Measured{AppID: "1", Day: "2026-11-13", Metric: MetricFirstDownloads, Territory: "US", Value: 2},
		Measured{AppID: "1", Day: "2026-11-15", Metric: MetricSessions, Territory: "US", Value: 9},
	)
	save("b", "2", Measured{AppID: "2", Day: "2026-11-12", Metric: MetricFirstDownloads, Territory: "US", Value: 1})

	got, err := st.ReportIn(ctx, "2026-11-09", "2026-11-15", "2026-11-02", "")
	if err != nil {
		t.Fatal(err)
	}
	byDay := map[string]map[string]any{}
	for _, d := range got.Days {
		byDay[d["day"].(string)] = d
	}
	if d := byDay["2026-11-11"]; d == nil || d[MetricFirstDownloads] != int64(0) || d[MetricRedownloads] != int64(0) {
		t.Errorf("a day between the report's first and last = %v", d)
	}
	if _, ok := byDay["2026-11-14"][MetricFirstDownloads]; ok {
		t.Errorf("a day after the downloads report's last is known: %v", byDay["2026-11-14"])
	}
	if _, ok := byDay["2026-11-09"]; ok {
		t.Errorf("a day before anything was measured is known: %v", byDay["2026-11-09"])
	}
	if got.Covered[MetricFirstDownloads] != 4 || got.Totals[MetricFirstDownloads] != int64(7) {
		t.Errorf("downloads = %v over %d days", got.Totals[MetricFirstDownloads], got.Covered[MetricFirstDownloads])
	}

	// The second app's days start at its first, on the 12th, though the report's go back further.
	one, err := st.ReportIn(ctx, "2026-11-09", "2026-11-15", "2026-11-02", "2")
	if err != nil {
		t.Fatal(err)
	}
	if one.Covered[MetricFirstDownloads] != 2 || len(one.Days) != 3 {
		t.Errorf("the second app's days = %v", one.Days)
	}
}
