package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// A database the first version made, with a day of sales read and an analytics measure, upgrades
// in place: what the sales report said becomes placeholder measures, in dollars, beside what
// analytics said, which it never overrides; apps, sessions, days read and rates stay.
func TestADatabaseOfSalesReportsKeepsWhatItRead(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, _ := db.Begin()
	if err := initialSchema.Up(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	for _, q := range []string{
		`INSERT INTO apps (id, name, bundle_id, sku, updated_at) VALUES ('42', 'Dots', 'com.acme.dots', 'dots', 0)`,
		`INSERT INTO sessions (id_hash, created_at, last_seen_at, expires_at) VALUES (x'00', 0, 0, 9999999999)`,
		`INSERT INTO products VALUES ('77', '42', '100 Coins', 'dots.coins', 'in_app', '2026-11-14')`,
		`INSERT INTO sales_days VALUES ('2026-11-14', 3, 0)`,
		`INSERT INTO rates VALUES ('EUR', 0.5, 0)`,
		`INSERT INTO sales VALUES ('2026-11-14', '42', '42', '1F', 'download', 'US', 'USD', 5, 0)`,
		`INSERT INTO sales VALUES ('2026-11-14', '42', '42', '7F', 'update', 'US', 'USD', 9, 0)`,
		`INSERT INTO sales VALUES ('2026-11-14', '42', '77', 'IA1', 'in_app', 'DE', 'EUR', 2, 1500000)`,
		`INSERT INTO sales VALUES ('2026-11-14', '42', '77', 'IA1', 'in_app', 'JP', 'JPY', 1, 120000000)`,
		// Analytics already said the day's updates, and a sales report never corrects it.
		`INSERT INTO metrics VALUES ('42', '2026-11-14', 'updates', 'US', '', 11)`,
		`INSERT INTO analytics_instances VALUES ('i', '42', 'App Downloads Standard', '2026-11-14', 1, 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	if err := Run(ctx, db); err != nil {
		t.Fatal(err)
	}

	value := func(metric, origin string) int64 {
		t.Helper()
		var v sql.NullInt64
		if err := db.QueryRow(`SELECT SUM(value) FROM metrics WHERE metric = ? AND origin = ?`, metric, origin).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v.Int64
	}
	if got := value("first_downloads", "sales"); got != 5 {
		t.Errorf("downloads from sales = %d, want 5", got)
	}
	if got := value("purchases", "sales"); got != 3 {
		t.Errorf("purchases from sales = %d, want 3", got)
	}
	// €1.50 at 0.5 to the dollar is $3; yen have no rate and are left out.
	if got := value("proceeds", "sales"); got != 3_000_000 {
		t.Errorf("proceeds from sales = %d millionths, want $3", got)
	}
	if got, sales := value("updates", "analytics"), value("updates", "sales"); got != 11 || sales != 0 {
		t.Errorf("updates = %d from analytics and %d from sales, want analytics' 11 alone", got, sales)
	}

	count := func(q string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	for q, want := range map[string]int{
		`SELECT COUNT(*) FROM apps WHERE sku = 'dots'`:                           1,
		`SELECT COUNT(*) FROM sessions`:                                          1,
		`SELECT COUNT(*) FROM sales_days`:                                        1,
		`SELECT COUNT(*) FROM rates`:                                             1,
		`SELECT COUNT(*) FROM items WHERE name = '100 Coins' AND in_app = 1`:     1,
		`SELECT COUNT(*) FROM analytics_instances`:                               0,
		`SELECT COUNT(*) FROM report_requests`:                                   0,
		`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('sales', 'products')`: 0,
	} {
		if got := count(q); got != want {
			t.Errorf("%s = %d, want %d", q, got, want)
		}
	}
	var version int
	db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != Count() {
		t.Errorf("version = %d, want %d", version, Count())
	}
}
