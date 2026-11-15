package asc

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"appstoreanalytics/internal/config"
	"appstoreanalytics/internal/store"
)

func credentials(t *testing.T) config.Apple {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return config.Apple{IssuerID: "00000000-0000-0000-0000-000000000000", KeyID: "TESTKEY000", Key: key}
}

// A signature Apple can check: ES256 over the first two parts, r and s side by side.
func TestATokenIsSignedAsAppleChecksIt(t *testing.T) {
	cred := credentials(t)
	now := time.Unix(1_800_000_000, 0)
	token, err := sign(cred, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}
	var header, claims map[string]any
	decodePart(t, parts[0], &header)
	decodePart(t, parts[1], &claims)
	if header["alg"] != "ES256" || header["kid"] != cred.KeyID || header["typ"] != "JWT" {
		t.Errorf("header = %v", header)
	}
	if claims["iss"] != cred.IssuerID || claims["aud"] != "appstoreconnect-v1" {
		t.Errorf("claims = %v", claims)
	}
	if life := claims["exp"].(float64) - claims["iat"].(float64); life <= 0 || life > 20*60 {
		t.Errorf("the token lives %vs, and Apple takes at most twenty minutes", life)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("signature is %d bytes, err %v", len(sig), err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(&cred.Key.PublicKey, digest[:], r, s) {
		t.Error("the signature does not verify")
	}
}

func decodePart(t *testing.T, part string, v any) {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatal(err)
	}
}

// The header of a real daily summary, tab-separated, and lines of an app, its update, its
// in-app purchase and a refund.
const summary = "Provider\tProvider Country\tSKU\tDeveloper\tTitle\tVersion\tProduct Type Identifier\tUnits\tDeveloper Proceeds\tBegin Date\tEnd Date\tCustomer Currency\tCountry Code\tCurrency of Proceeds\tApple Identifier\tCustomer Price\tPromo Code\tParent Identifier\tSubscription\tPeriod\tCategory\tCMB\tDevice\tSupported Platforms\tProceeds Reason\tPreserved Pricing\tClient\tOrder Type\n" +
	"APPLE\tUS\tdots.app\tAcme\tDots \"Pro\"\t1.2\t1F\t12\t0\t11/14/2026\t11/14/2026\tUSD\tUS\tUSD\t1234567890\t0\t \t \t \t \tGames\t \tiPhone\tiOS\t \t \t \t \n" +
	"APPLE\tUS\tdots.app\tAcme\tDots \"Pro\"\t1.2\t7F\t40\t0\t11/14/2026\t11/14/2026\tUSD\tDE\tUSD\t1234567890\t0\t \t \t \t \tGames\t \tiPad\tiOS\t \t \t \t \n" +
	"APPLE\tUS\tdots.coins\tAcme\t100 Coins\t \tIA1\t3\t0.70\t11/14/2026\t11/14/2026\tEUR\tDE\tEUR\t2222222222\t0.99\t \tdots.app\t \t \tGames\t \tiPhone\tiOS\t \t \t \t \n" +
	"APPLE\tUS\tdots.coins\tAcme\t100 Coins\t \tIA1\t-1\t0.70\t11/14/2026\t11/14/2026\tEUR\tDE\tEUR\t2222222222\t-0.99\t \tdots.app\t \t \tGames\t \tiPhone\tiOS\t \t \t \t \n" +
	"\n"

func gz(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
}

func TestASalesSummaryIsReadByItsColumnNames(t *testing.T) {
	lines, err := ParseSales(bytes.NewReader(gz(t, summary)))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 4 {
		t.Fatalf("read %d lines, want 4", len(lines))
	}
	app, purchase, refund := lines[0], lines[2], lines[3]
	if app.AppleID != "1234567890" || app.ProductType != "1F" || app.Units != 12 || app.Title != `Dots "Pro"` {
		t.Errorf("app line = %+v", app)
	}
	if purchase.ParentSKU != "dots.app" || purchase.Currency != "EUR" || purchase.Proceeds != 2_100_000 {
		t.Errorf("purchase line = %+v, want 3 × €0.70 under dots.app", purchase)
	}
	if refund.Units != -1 || refund.Proceeds != -700_000 {
		t.Errorf("refund line = %+v, want it to take its proceeds back", refund)
	}
}

// A comma-separated file with quotes reads the same as a tab-separated one.
func TestAnAnalyticsReportKeepsTheLinesItMeasures(t *testing.T) {
	file := "Date\tApp Name\tApp Apple Identifier\tEvent\tPage Type\tSource Type\tEngagement Type\tDevice\tPlatform Version\tTerritory\tCounts\tUnique Counts\n" +
		"2026-11-14\tDots\t1234567890\tImpression\tProduct page\tApp Store search\t\tiPhone\tiOS 26.1\tUS\t120\t80\n" +
		"2026-11-14\tDots\t1234567890\tPage view\tProduct page\tApp Store search\t\tiPhone\tiOS 26.1\tUS\t30\t20\n" +
		"2026-11-14\tDots\t1234567890\tTap\tProduct page\tApp Store search\tGet\tiPhone\tiOS 26.1\tUS\t9\t9\n"
	measures, err := ParseReport("App Store Discovery and Engagement Standard", bytes.NewReader(gz(t, file)))
	if err != nil {
		t.Fatal(err)
	}
	want := []Measure{
		{Day: "2026-11-14", Metric: store.MetricImpressions, Territory: "US", Source: "App Store search", Value: 120},
		{Day: "2026-11-14", Metric: store.MetricPageViews, Territory: "US", Source: "App Store search", Value: 30},
	}
	if len(measures) != len(want) || measures[0] != want[0] || measures[1] != want[1] {
		t.Errorf("measures = %+v, want %+v", measures, want)
	}

	csv := "\"Date\",\"App Name\",\"Download Type\",\"Source Type\",\"Territory\",\"Counts\"\n" +
		"\"2026-11-14\",\"Dots, the game\",\"First-time Download\",\"Web referrer\",\"DE\",\"7\"\n"
	measures, err = ParseReport("app downloads standard", strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(measures) != 1 || measures[0].Metric != store.MetricFirstDownloads || measures[0].Value != 7 {
		t.Errorf("measures = %+v", measures)
	}
}

// Apple says which of the two a missing report is only in words.
func TestAMissingSalesReportIsToldApartByWhatAppleSays(t *testing.T) {
	answers := map[string]struct {
		status int
		detail string
	}{
		"2026-11-01": {http.StatusNotFound, "There were no sales for the date specified."},
		"2026-11-02": {http.StatusNotFound, "Report is not available yet. Daily reports for the Americas are available by 5 am Pacific Time; Japan, Australia, and New Zealand by 5 am Japan Standard Time; and 5 am Central European Time for all other territories."},
		"2026-11-03": {http.StatusGone, "Gone"},
		"2026-11-04": {http.StatusUnauthorized, "Authentication credentials are missing or invalid."},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("a request went without its token")
		}
		if r.URL.Query().Get("filter[vendorNumber]") != "00000000" {
			t.Errorf("vendor = %q", r.URL.Query().Get("filter[vendorNumber]"))
		}
		a := answers[r.URL.Query().Get("filter[reportDate]")]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(a.status)
		json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"status": a.status, "code": "NOT_FOUND", "detail": a.detail}}})
	}))
	defer srv.Close()
	cred := credentials(t)
	cred.BaseURL = srv.URL
	c := New(cred)

	ctx := context.Background()
	if _, err := c.SalesReport(ctx, "00000000", "2026-11-01"); !errors.Is(err, ErrNoSales) {
		t.Errorf("no sales: err = %v", err)
	}
	if _, err := c.SalesReport(ctx, "00000000", "2026-11-02"); !errors.Is(err, ErrNotYet) {
		t.Errorf("not yet: err = %v", err)
	}
	if _, err := c.SalesReport(ctx, "00000000", "2026-11-03"); !errors.Is(err, ErrGone) {
		t.Errorf("gone: err = %v", err)
	}
	_, err := c.SalesReport(ctx, "00000000", "2026-11-04")
	if StatusOf(err) != http.StatusUnauthorized || !strings.Contains(Why(err), "refused the key") {
		t.Errorf("refused: err = %v, why = %q", err, Why(err))
	}
}
