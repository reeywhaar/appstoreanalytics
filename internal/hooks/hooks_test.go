package hooks

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// sign makes a compact JWS of claims, with a signature nothing checks.
func sign(t *testing.T, claims any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "x5c": "…"})
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(body) + ".c2lnbmF0dXJl"
}

// The signed transaction inside the signed payload is opened too, so the whole of it reads.
func TestAServerNotificationIsReadThroughItsSignatures(t *testing.T) {
	transaction := sign(t, map[string]any{"productId": "dots.pro.yearly", "price": 20990})
	body, _ := json.Marshal(map[string]string{"signedPayload": sign(t, map[string]any{
		"notificationType": "SUBSCRIBED",
		"subtype":          "INITIAL_BUY",
		"data": map[string]any{
			"bundleId":              "com.acme.dots",
			"environment":           "Production",
			"signedTransactionInfo": transaction,
		},
	})})
	d := Describe(body)
	if d.Event != "SUBSCRIBED/INITIAL_BUY" {
		t.Errorf("event = %q", d.Event)
	}
	if d.Summary != "SUBSCRIBED/INITIAL_BUY · com.acme.dots · Production" {
		t.Errorf("summary = %q", d.Summary)
	}
	got := d.Decoded.(map[string]any)["signedPayload"].(map[string]any)["data"].(map[string]any)["signedTransactionInfo"]
	if tx, ok := got.(map[string]any); !ok || tx["productId"] != "dots.pro.yearly" {
		t.Errorf("transaction = %v, want it opened", got)
	}
}

func TestAnAppStoreConnectWebhookIsNamedByItsType(t *testing.T) {
	d := Describe([]byte(`{"data":{"type":"appStoreVersionAppVersionStateUpdated","id":"abc","attributes":{"newValue":"READY_FOR_SALE"}}}`))
	if d.Event != "appStoreVersionAppVersionStateUpdated" || d.Summary != "appStoreVersionAppVersionStateUpdated · abc" {
		t.Errorf("delivery = %+v", d)
	}
}

func TestAnythingElseIsKeptAndNamedWhereItCanBe(t *testing.T) {
	for body, event := range map[string]string{
		`{"event":"ping"}`:  "ping",
		`{"hello":"there"}`: Unknown,
		`[1,2]`:             Unknown,
		`not json at all`:   Unknown,
		``:                  Unknown,
	} {
		if d := Describe([]byte(body)); d.Event != event {
			t.Errorf("Describe(%q).Event = %q, want %q", body, d.Event, event)
		}
	}
}
