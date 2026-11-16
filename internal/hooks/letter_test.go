package hooks

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAnEventIsTitledAsAPersonNamesIt(t *testing.T) {
	for event, want := range map[string]string{
		"TEST":                                  "Test notification",
		"SUBSCRIBED/INITIAL_BUY":                "Subscribed: initial buy",
		"appStoreVersionAppVersionStateUpdated": "App version state changed",
		"SOMETHING_NEW":                         "Something new",
		"somethingNewUpdated":                   "Something new updated",
	} {
		if got := Title(event); got != want {
			t.Errorf("Title(%q) = %q, want %q", event, got, want)
		}
	}
}

// unsigned is a JWS whose claims are claims; reading never checks the signature.
func unsigned(claims any) string {
	raw, _ := json.Marshal(claims)
	return "eyJhbGciOiJFUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(raw) + ".c2ln"
}

// A notification says what was bought, for how much and until when, before what it said.
func TestANotificationIsMailedAsWhatHappenedToWhichApp(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"signedPayload": unsigned(map[string]any{
		"notificationType": "SUBSCRIBED",
		"subtype":          "INITIAL_BUY",
		"data": map[string]any{
			"bundleId":    "com.acme.seeit",
			"environment": "Sandbox",
			"status":      1,
			"signedTransactionInfo": unsigned(map[string]any{
				"productId": "seeit.pro.yearly", "price": 29990, "currency": "USD",
				"purchaseDate": 1794801884901, "expiresDate": 1826337884901,
				"transactionId": "2000000123", "originalTransactionId": "2000000123",
				"type": "Auto-Renewable Subscription", "storefront": "USA",
			}),
			"signedRenewalInfo": unsigned(map[string]any{"autoRenewStatus": 1, "autoRenewProductId": "seeit.pro.yearly"}),
		},
	})})
	at := time.Date(2026, 11, 16, 4, 4, 0, 0, time.UTC)
	subject, text := Letter(Describe(body), body, About{App: "SeeIt", Via: "Notifications", ProvenBy: ByApple, Received: at})

	if subject != "Subscribed: initial buy · SeeIt (Sandbox)" {
		t.Errorf("subject = %q", subject)
	}
	for _, line := range []string{
		"Event: Subscribed: initial buy\n",
		"App: SeeIt (com.acme.seeit)\n",
		"Environment: Sandbox\n",
		"Product: seeit.pro.yearly\n",
		"Type: Auto-Renewable Subscription\n",
		"Price: 29.99 USD\n",
		"Purchased: 2026-11-16 04:04 UTC\n",
		"Status: active\n",
		"Auto-renew: on\n",
		"Storefront: USA\n",
		"Transaction: 2000000123\n",
		"Received: 2026-11-16 04:04 UTC\n",
		"Address: Notifications\n",
		"Signed: by Apple\n",
		"A customer subscribed",
		"with Apple's signed parts opened up",
		`"productId": "seeit.pro.yearly"`,
	} {
		if !strings.Contains(text, line) {
			t.Errorf("no %q in:\n%s", line, text)
		}
	}
	if !strings.HasPrefix(text, "Event: ") || strings.Contains(text, "Original transaction") || strings.Contains(text, "Renews to") {
		t.Errorf("text =\n%s", text)
	}
}

// A webhook's state change is said as before and after, and the subject says where it got to.
func TestAWebhookIsMailedAsTheStateItMovedTo(t *testing.T) {
	body := []byte(`{"data":{"type":"appStoreVersionAppVersionStateUpdated","id":"7c81","version":1,
		"attributes":{"newValue":"READY_FOR_SALE","oldValue":"PENDING_DEVELOPER_RELEASE","timestamp":"2026-11-16T05:00:52.745Z"},
		"relationships":{"instance":{"data":{"type":"appStoreVersions","id":"ad7e"}}}}}`)
	subject, text := Letter(Describe(body), body, About{Via: "Dots: App Store Connect", ProvenBy: BySecret, Received: time.Now()})

	if subject != "App version state changed: Ready for sale · Dots: App Store Connect" {
		t.Errorf("subject = %q", subject)
	}
	for _, line := range []string{
		"Event: App version state changed\n",
		"State: Pending developer release → Ready for sale\n",
		"At: 2026-11-16 05:00 UTC\n",
		"Signed: with the address's secret\n",
		"What it said:\n",
	} {
		if !strings.Contains(text, line) {
			t.Errorf("no %q in:\n%s", line, text)
		}
	}
	if strings.Contains(text, "App: ") || strings.Contains(text, "Environment") {
		t.Errorf("a webhook said an app or an environment:\n%s", text)
	}
}
