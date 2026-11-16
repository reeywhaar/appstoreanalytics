package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/mail"
)

// deliver sends a webhook as a sender would: no cookie, whatever content type it likes.
func deliver(t *testing.T, s *Server, url, contentType, body string) *http.Response {
	t.Helper()
	return deliverSigned(t, s, url, contentType, body, "")
}

// deliverSigned sends one signed as App Store Connect signs, with secret; "" sends it unsigned.
func deliverSigned(t *testing.T, s *Server, url, contentType, body, secret string) *http.Response {
	t.Helper()
	r := httptest.NewRequest("POST", url, strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		r.Header.Set("X-Apple-Signature", "hmacsha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w.Result()
}

func deliveries(t *testing.T, c *client) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, d := range body(t, c.do("GET", "/api/hooks/deliveries", ""))["deliveries"].([]any) {
		out = append(out, d.(map[string]any))
	}
	return out
}

// addEndpoint adds an address, and says its path and its secret.
func addEndpoint(t *testing.T, c *client, request string) (id, path, secret string) {
	t.Helper()
	resp := c.do("POST", "/api/hooks/endpoints", request)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add %s = %s", request, resp.Status)
	}
	got := body(t, resp)
	path = strings.TrimPrefix(got["url"].(string), "https://stats.example.com")
	if !strings.HasPrefix(path, "/hooks/") || len(path) < 30 {
		t.Fatalf("url = %q", got["url"])
	}
	return toJSON(got["id"]), path, got["secret"].(string)
}

func TestAWebhookIsKeptByTheAddressItCameTo(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)
	if got := body(t, c.do("GET", "/api/hooks", ""))["endpoints"].([]any); len(got) != 0 {
		t.Fatalf("a new instance has addresses: %v", got)
	}
	if resp := c.do("POST", "/api/hooks/endpoints", `{"label":"  "}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an address without a label = %s", resp.Status)
	}

	connectID, connect, generated := addEndpoint(t, c, `{"label":"Dots: App Store Connect"}`)
	if len(generated) != 32 || strings.Trim(generated, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789") != "" {
		t.Errorf("generated secret = %q", generated)
	}
	_, notifications, given := addEndpoint(t, c, `{"label":"Dots: notifications","secret":"This is my secret"}`)
	if given != "This is my secret" || connect == notifications {
		t.Errorf("secret %q, paths %q %q", given, connect, notifications)
	}

	ping := `{"data":{"type":"webhookPingCreated","id":"p"}}`
	if resp := deliverSigned(t, s, connect, "application/json", ping, generated); resp.StatusCode != http.StatusOK {
		t.Fatalf("a delivery = %s", resp.Status)
	}
	// A sender sends what it sends, and one without a signature to an address with a secret has
	// failed, but is kept.
	if resp := deliver(t, s, notifications, "text/plain", "hello"); resp.StatusCode != http.StatusOK {
		t.Errorf("a delivery of text = %s", resp.Status)
	}
	if resp := deliver(t, s, "/hooks/wrong", "application/json", `{}`); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a wrong token = %s", resp.Status)
	}

	list := deliveries(t, c)
	if len(list) != 2 {
		t.Fatalf("deliveries = %v", list)
	}
	if text := list[0]; text["via"] != "Dots: notifications" || text["proven_by"] != "" || !strings.Contains(text["proof_error"].(string), "no signature") {
		t.Errorf("the text = %v", text)
	}
	signed := list[1]
	if signed["via"] != "Dots: App Store Connect" || signed["event"] != "webhookPingCreated" || signed["proven_by"] != "secret" {
		t.Errorf("the ping = %v", signed)
	}
	one := body(t, c.do("GET", "/api/hooks/"+toJSON(signed["id"]), ""))
	if one["body"] != ping || one["decoded"] == nil {
		t.Errorf("one delivery = %v", one)
	}

	got := body(t, c.do("GET", "/api/hooks", ""))
	endpoints := got["endpoints"].([]any)
	first := endpoints[0].(map[string]any)
	if len(endpoints) != 2 || first["label"] != "Dots: App Store Connect" || first["count"] != 1.0 || first["last_at"] == nil || first["secret_set"] != true {
		t.Errorf("endpoints = %v", endpoints)
	}
	if _, ok := first["secret"]; ok {
		t.Error("a secret came back out after it was shown")
	}
	if len(got["catalog"].([]any)) != 2 || got["events"].([]any)[0].(map[string]any)["count"] != 1.0 {
		t.Errorf("catalog or events = %v %v", got["catalog"], got["events"])
	}

	// Removed: it answers nothing, and what came through it still says so.
	if resp := c.do("DELETE", "/api/hooks/endpoints/"+connectID, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove = %s", resp.Status)
	}
	if resp := deliverSigned(t, s, connect, "application/json", ping, generated); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a removed address = %s", resp.Status)
	}
	if resp := c.do("DELETE", "/api/hooks/endpoints/"+connectID, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("removed twice = %s", resp.Status)
	}
	if list := deliveries(t, c); len(list) != 2 || list[1]["via"] != "Dots: App Store Connect" {
		t.Errorf("after removing = %v", list)
	}
	if got := body(t, c.do("GET", "/api/hooks", ""))["endpoints"].([]any); len(got) != 1 {
		t.Errorf("endpoints after removing = %v", got)
	}
}

// Only the events chosen, from a sender proven by its signature, are mailed, and how the mailing
// went is kept on the delivery.
func TestAChosenEventIsMailed(t *testing.T) {
	s, _ := newServer(t)
	var (
		mu   sync.Mutex
		sent []mail.Message
	)
	s.send = func(_ mail.Relay, m mail.Message) error {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, m)
		return nil
	}
	c := signIn(t, s)
	_, path, secret := addEndpoint(t, c, `{"label":"Dots","secret":"This is my secret"}`)
	if resp := c.do("PUT", "/api/hooks/mail", `{"to":"me@example.com","events":["webhookPingCreated"]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("set mail = %s", resp.Status)
	}

	// Chosen, and no relay to send it with.
	deliverSigned(t, s, path, "application/json", `{"data":{"type":"webhookPingCreated","id":"1"}}`, secret)
	s.mailing.Wait()
	first := deliveries(t, c)[0]
	if first["mailed_at"] == nil || !strings.Contains(first["mail_error"].(string), "no mail relay") {
		t.Errorf("without a relay = %v", first)
	}

	c.do("PUT", "/api/settings/relay", `{"host":"smtp.example.com","port":587,"security":"starttls","username":"","password":"","from_address":"stats@example.com","from_name":""}`)
	deliverSigned(t, s, path, "application/json", `{"data":{"type":"webhookPingCreated","id":"2"}}`, secret)
	deliverSigned(t, s, path, "application/json", `{"data":{"type":"buildUploadStateUpdated","id":"3"}}`, secret)
	s.mailing.Wait()

	if len(sent) != 1 || sent[0].To != "me@example.com" || !strings.HasPrefix(sent[0].Subject, "App Store: webhookPingCreated") {
		t.Fatalf("sent = %+v", sent)
	}
	list := deliveries(t, c)
	if mailed := list[1]; mailed["mailed_at"] == nil || mailed["mail_error"] != "" || mailed["proven_by"] != "secret" {
		t.Errorf("the mailed one = %v", mailed)
	}
	if skipped := list[0]; skipped["mailed_at"] != nil {
		t.Errorf("one not chosen was mailed: %v", skipped)
	}
	if resp := c.do("PUT", "/api/hooks/mail", `{"to":"nobody","events":[]}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a To that is not an address = %s", resp.Status)
	}

	// Chosen, and unproven: kept and marked, and never mailed.
	before := len(sent)
	deliver(t, s, path, "application/json", `{"data":{"type":"webhookPingCreated","id":"4"}}`)
	deliverSigned(t, s, path, "application/json", `{"data":{"type":"webhookPingCreated","id":"5"}}`, "Another secret")
	s.mailing.Wait()
	if len(sent) != before {
		t.Errorf("an unproven delivery was mailed: %+v", sent[before:])
	}
	for _, d := range deliveries(t, c)[:2] {
		if d["proven_by"] != "" || d["proof_error"] == "" || d["mailed_at"] != nil {
			t.Errorf("an unproven one = %v", d)
		}
	}
}

// Testing an address finds what in App Store Connect sends to it, and asks each for a test: the
// webhooks pinged, the server notifications sent a test where there is a key to ask with.
func TestAnAddressIsTestedByWhatSendsToIt(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)
	id, path, _ := addEndpoint(t, c, `{"label":"Dots"}`)
	here := "https://stats.example.com" + path
	fake := &apple{
		apps: []asc.AppURLs{
			// Production here, as Apple might keep it, with a slash and its host in capitals.
			{ID: "42", Name: "Dots", BundleID: "com.acme.dots", Production: "https://Stats.Example.com" + path + "/", Sandbox: here},
			{ID: "43", Name: "Lines", BundleID: "com.acme.lines", Production: "https://elsewhere.example.com/hooks/x"},
		},
		webhooks: map[string][]asc.Webhook{
			"42": {{ID: "w1", Name: "Builds", URL: here, Enabled: true}, {ID: "w2", Name: "Old", URL: here}},
			"43": {{ID: "w3", Name: "Elsewhere", URL: "https://elsewhere.example.com/x", Enabled: true}},
		},
	}
	s.apple = fake

	test := func() []any {
		t.Helper()
		resp := c.do("POST", "/api/hooks/endpoints/"+id+"/test", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("test = %s", resp.Status)
		}
		return body(t, resp)["senders"].([]any)
	}
	senders := test()
	results := []string{}
	for _, raw := range senders {
		b := raw.(map[string]any)
		results = append(results, fmt.Sprint(b["kind"], " ", b["app"], " ", b["name"], " ", b["result"]))
	}
	want := []string{
		"notifications Dots Production asked",
		"notifications Dots Sandbox asked",
		"webhook Dots Builds asked",
		"webhook Dots Old off",
	}
	if strings.Join(results, "; ") != strings.Join(want, "; ") {
		t.Errorf("senders = %v, want %v", results, want)
	}
	if strings.Join(fake.pinged, ",") != "w1" || strings.Join(fake.tested, ",") != "com.acme.dots false,com.acme.dots true" {
		t.Errorf("pinged %v, tested %v", fake.pinged, fake.tested)
	}

	// Without an In-App Purchase key, the server notifications are found and not asked.
	fake.noIAPKey = true
	if b := test()[0].(map[string]any); b["result"] != "no_key" {
		t.Errorf("without a key = %v", b)
	}

	// App Store Connect refusing is said in its words.
	fake.refuse = &asc.Error{Status: http.StatusUnauthorized}
	resp := c.do("POST", "/api/hooks/endpoints/"+id+"/test", "")
	if got := body(t, resp); resp.StatusCode != http.StatusBadGateway || !strings.Contains(got["message"].(string), "refused the key") {
		t.Errorf("a refusal = %s %v", resp.Status, got)
	}
	if resp := c.do("POST", "/api/hooks/endpoints/999/test", ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("no such address = %s", resp.Status)
	}
}
