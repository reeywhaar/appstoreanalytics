package asc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// What sends to an address is read from the apps and their webhooks, and a ping is asked for
// with the webhook named as App Store Connect wants it.
func TestWhatSendsWhereIsReadAndAWebhookPinged(t *testing.T) {
	var pinged string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/apps":
			if !strings.Contains(r.URL.Query().Get("fields[apps]"), "subscriptionStatusUrlForSandbox") {
				t.Errorf("fields = %q", r.URL.Query().Get("fields[apps]"))
			}
			w.Write([]byte(`{"data":[{"id":"42","attributes":{"name":"Dots","bundleId":"com.acme.dots","subscriptionStatusUrl":"https://a/hooks/x","subscriptionStatusUrlForSandbox":"https://a/hooks/y"}}],"links":{}}`))
		case r.Method == "GET" && r.URL.Path == "/v1/apps/42/webhooks":
			if !strings.Contains(r.URL.Query().Get("fields[webhooks]"), "eventTypes") {
				t.Errorf("fields = %q", r.URL.Query().Get("fields[webhooks]"))
			}
			w.Write([]byte(`{"data":[{"id":"w1","attributes":{"name":"Builds","url":"https://a/hooks/z","enabled":true,"eventTypes":["BUILD_UPLOAD_STATE_UPDATED"]}}],"links":{}}`))
		case r.Method == "POST" && r.URL.Path == "/v1/webhookPings":
			var doc struct {
				Data struct {
					Type          string `json:"type"`
					Relationships struct {
						Webhook struct {
							Data struct{ Type, ID string } `json:"data"`
						} `json:"webhook"`
					} `json:"relationships"`
				} `json:"data"`
			}
			json.NewDecoder(r.Body).Decode(&doc)
			if doc.Data.Type != "webhookPings" || doc.Data.Relationships.Webhook.Data.Type != "webhooks" {
				t.Errorf("ping = %+v", doc)
			}
			pinged = doc.Data.Relationships.Webhook.Data.ID
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data":{"type":"webhookPings","id":"p1"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	cred := credentials(t)
	cred.BaseURL = srv.URL
	c := New(cred)
	ctx := context.Background()

	apps, err := c.NotificationURLs(ctx)
	if err != nil || len(apps) != 1 || apps[0] != (AppURLs{ID: "42", Name: "Dots", BundleID: "com.acme.dots", Production: "https://a/hooks/x", Sandbox: "https://a/hooks/y"}) {
		t.Fatalf("apps = %+v, %v", apps, err)
	}
	hooks, err := c.Webhooks(ctx, "42")
	if err != nil || len(hooks) != 1 || !reflect.DeepEqual(hooks[0], Webhook{ID: "w1", Name: "Builds", URL: "https://a/hooks/z", Enabled: true, Events: []string{"BUILD_UPLOAD_STATE_UPDATED"}}) || hooks[0].Every() {
		t.Fatalf("webhooks = %+v, %v", hooks, err)
	}
	if err := c.PingWebhook(ctx, "w1"); err != nil || pinged != "w1" {
		t.Fatalf("ping = %v, pinged %q", err, pinged)
	}
}

// A test notification is asked for of the sandbox or production App Store Server API, signed with
// the In-App Purchase key and naming the app; without that key it is not asked for at all.
func TestATestNotificationIsAskedForWithTheInAppPurchaseKey(t *testing.T) {
	iap, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	var asked []string
	serve := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" || r.URL.Path != "/inApps/v1/notifications/test" {
				t.Errorf("unexpected %s %s", r.Method, r.URL)
			}
			parts := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
			var header, claims map[string]any
			decodePart(t, parts[0], &header)
			decodePart(t, parts[1], &claims)
			if header["kid"] != "IAPKEY0000" || claims["bid"] != "com.acme.dots" || claims["aud"] != "appstoreconnect-v1" {
				t.Errorf("header %v, claims %v", header, claims)
			}
			io.Copy(io.Discard, r.Body)
			asked = append(asked, name)
			if name == "refusing" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"errorCode":4000006,"errorMessage":"Invalid request identifier."}`))
				return
			}
			w.Write([]byte(`{"testNotificationToken":"t-` + name + `"}`))
		}))
	}
	production, sandbox := serve("production"), serve("sandbox")
	defer production.Close()
	defer sandbox.Close()
	cred := credentials(t)
	cred.StoreKitURL, cred.StoreKitSandboxURL = production.URL, sandbox.URL
	ctx := context.Background()

	if _, err := New(cred).TestNotification(ctx, "com.acme.dots", true); !errors.Is(err, ErrNoIAPKey) || len(asked) != 0 {
		t.Fatalf("without a key = %v, asked %v", err, asked)
	}
	cred.IAPKeyID, cred.IAPKey = "IAPKEY0000", iap
	c := New(cred)
	if token, err := c.TestNotification(ctx, "com.acme.dots", true); err != nil || token != "t-sandbox" {
		t.Errorf("sandbox = %q, %v", token, err)
	}
	if token, err := c.TestNotification(ctx, "com.acme.dots", false); err != nil || token != "t-production" {
		t.Errorf("production = %q, %v", token, err)
	}

	refusing := serve("refusing")
	defer refusing.Close()
	cred.StoreKitURL = refusing.URL
	if _, err := New(cred).TestNotification(ctx, "com.acme.dots", false); err == nil || !strings.Contains(err.Error(), "Invalid request identifier") {
		t.Errorf("a refusal = %v", err)
	}
}

// A webhook is made sending every event, signed, for its app; mended in place; deleted; and an
// app's server notifications are pointed at an address, version 2, or at nothing, one
// environment at a time.
func TestAWebhookIsMadeMendedAndDeletedAndNotificationsPointed(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var doc struct {
			Data struct {
				Type          string         `json:"type"`
				ID            string         `json:"id"`
				Attributes    map[string]any `json:"attributes"`
				Relationships struct {
					App struct {
						Data struct{ Type, ID string } `json:"data"`
					} `json:"app"`
				} `json:"relationships"`
			} `json:"data"`
		}
		json.Unmarshal(raw, &doc)
		d := doc.Data
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/webhooks":
			if d.Type != "webhooks" || d.Relationships.App.Data != (struct{ Type, ID string }{"apps", "42"}) ||
				d.Attributes["secret"] != "s3cret" || d.Attributes["enabled"] != true || len(d.Attributes["eventTypes"].([]any)) != len(WebhookEvents) {
				t.Errorf("make = %s", raw)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data":{"type":"webhooks","id":"w9"}}`))
		case r.Method == "PATCH" && r.URL.Path == "/v1/webhooks/w9":
			if d.ID != "w9" || d.Attributes["url"] != "https://a/hooks/new" || d.Attributes["enabled"] != true || len(d.Attributes["eventTypes"].([]any)) != len(WebhookEvents) {
				t.Errorf("mend = %s", raw)
			}
			w.Write([]byte(`{"data":{"type":"webhooks","id":"w9"}}`))
		case r.Method == "DELETE" && r.URL.Path == "/v1/webhooks/w9":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "PATCH" && r.URL.Path == "/v1/apps/42":
			if d.Type != "apps" || d.ID != "42" {
				t.Errorf("notify = %s", raw)
			}
			w.Write([]byte(`{"data":{"type":"apps","id":"42"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		asked = append(asked, fmt.Sprintf("%s %s %v", r.Method, r.URL.Path, d.Attributes))
	}))
	defer srv.Close()
	cred := credentials(t)
	cred.BaseURL = srv.URL
	c := New(cred)
	ctx := context.Background()

	made, err := c.MakeWebhook(ctx, "42", "App Store analytics", "https://a/hooks/x", "s3cret")
	if err != nil || made.ID != "w9" || !made.Every() {
		t.Fatalf("made = %+v, %v", made, err)
	}
	if err := c.MendWebhook(ctx, "w9", "https://a/hooks/new", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteWebhook(ctx, "w9"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetNotificationURL(ctx, "42", true, "https://a/hooks/sandbox"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetNotificationURL(ctx, "42", false, ""); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 5 ||
		asked[3] != "PATCH /v1/apps/42 map[subscriptionStatusUrlForSandbox:https://a/hooks/sandbox subscriptionStatusUrlVersionForSandbox:V2]" ||
		asked[4] != "PATCH /v1/apps/42 map[subscriptionStatusUrl:<nil>]" {
		t.Errorf("asked = %q", asked)
	}
}
