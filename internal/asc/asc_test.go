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

func gz(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
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

// Proceeds are in dollars already; an in-app purchase is counted, and the app bought is not.
func TestAPurchasesReportKeepsProceedsAndWhatWasBought(t *testing.T) {
	file := "Date\tApp Name\tApp Apple Identifier\tPurchase Type\tContent Name\tContent Apple Identifier\tPayment Method\tDevice\tPlatform Version\tSource Type\tPage Type\tApp Download Date\tPre-Order\tTerritory\tPurchases\tProceeds in USD\tSales in USD\tPaying Users\n" +
		"2026-11-14\tDots\t1234567890\tIn-app purchases\t100 Coins\t2222222222\tApple ID\tiPhone\tiOS 26.1\tApp Store search\t\t2026-11-01\tNo\tUS\t3\t2.10\t2.97\t3\n" +
		"2026-11-14\tDots\t1234567890\tIn-app purchases\t100 Coins\t2222222222\tApple ID\tiPhone\tiOS 26.1\tApp Store search\t\t2026-11-01\tNo\tUS\t-1\t-0.70\t-0.99\t1\n" +
		"2026-11-14\tDots\t1234567890\tApp purchase\tDots\t1234567890\tApple ID\tiPad\tiOS 26.1\tWeb referrer\t\t2026-11-14\tNo\tDE\t1\t2.99\t3.99\t1\n"
	measures, err := ParseReport("App Store Purchases Standard", bytes.NewReader(gz(t, file)))
	if err != nil {
		t.Fatal(err)
	}
	var proceeds, purchases int64
	for _, m := range measures {
		switch m.Metric {
		case store.MetricProceeds:
			proceeds += m.Value
		case store.MetricPurchases:
			purchases += m.Value
			if m.Item != "2222222222" || m.ItemName != "100 Coins" || !m.InApp {
				t.Errorf("purchase = %+v", m)
			}
		}
	}
	if proceeds != 4_390_000 || purchases != 2 {
		t.Errorf("proceeds = %d, purchases = %d; want $4.39 and 2, the refund taken back", proceeds, purchases)
	}
}

func TestARefusedKeyIsSaidInWords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("a request went without its token")
		}
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"status": "401", "detail": "Provide a properly configured and signed bearer token."}}})
	}))
	defer srv.Close()
	cred := credentials(t)
	cred.BaseURL = srv.URL
	_, err := New(cred).Apps(context.Background())
	if StatusOf(err) != http.StatusUnauthorized || !strings.Contains(Why(err), "refused the key") {
		t.Errorf("err = %v, why = %q", err, Why(err))
	}
}
