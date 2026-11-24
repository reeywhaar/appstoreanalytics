package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"unicode"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/hooks"
)

// address is one of the addresses, as adding it said.
type address struct{ id, url, secret string }

func addAddress(t *testing.T, c *client, label string) address {
	t.Helper()
	id, path, secret := addEndpoint(t, c, `{"label":"`+label+`"}`)
	return address{id, "https://stats.example.com" + path, secret}
}

// standing is every app's three parts, as "state label", the label of the address each is sent
// to, or "-".
func standing(t *testing.T, c *client) map[string]string {
	t.Helper()
	labels := map[string]string{}
	for _, raw := range body(t, c.do("GET", "/api/hooks", ""))["endpoints"].([]any) {
		e := raw.(map[string]any)
		labels[toJSON(e["id"])] = e["label"].(string)
	}
	resp := c.do("GET", "/api/hooks/apple", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apple = %s", resp.Status)
	}
	out := map[string]string{}
	for _, raw := range body(t, resp)["apps"].([]any) {
		app := raw.(map[string]any)
		var parts []string
		for _, name := range []string{"webhook", "production", "sandbox"} {
			p := app[name].(map[string]any)
			at := "-"
			if p["endpoint"] != nil {
				at = labels[toJSON(p["endpoint"])]
			}
			parts = append(parts, fmt.Sprintf("%s %s", p["state"], at))
		}
		out[app["name"].(string)] = strings.Join(parts, "; ")
	}
	return out
}

// connect sends one of an app's parts where request says, and says what came of it.
func connect(t *testing.T, c *client, app, request string) string {
	t.Helper()
	resp := c.do("POST", "/api/hooks/apple/"+app+"/connect", request)
	got := body(t, resp)
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("%d %s", resp.StatusCode, got["message"])
	}
	return got["result"].(string)
}

// Each part says where it is sent: to which of the addresses; nowhere; or somewhere none of them
// is, which is a warning. A webhook to one of them turned off, or sending only some events, is a
// warning too. Somebody else's webhooks are not this page's.
func TestAnAppsPartsSayWhereTheyAreSent(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)
	hook := addAddress(t, c, "Hook")
	prod := addAddress(t, c, "Prod")
	gone := "https://stats.example.com/hooks/an-address-removed-long-ago"
	s.apple = &apple{
		apps: []asc.AppURLs{
			{ID: "1", Name: "Dots", Production: prod.url},
			{ID: "2", Name: "Lines", Production: "https://api.lines.example.com/notify", Sandbox: gone},
			{ID: "3", Name: "Rows"},
			{ID: "4", Name: "Pads"},
		},
		webhooks: map[string][]asc.Webhook{
			"1": {{ID: "w1", URL: hook.url, Enabled: true, Events: asc.WebhookEvents}},
			"2": {
				{ID: "w2", URL: hook.url, Events: asc.WebhookEvents},
				{ID: "w-ci", Name: "CI", URL: "https://ci.example.com/hook", Enabled: true},
			},
			"3": {{ID: "w3", Name: webhookName, URL: "https://elsewhere.example.com/x", Enabled: true, Events: asc.WebhookEvents}},
			"4": {{ID: "w4", URL: hook.url, Enabled: true, Events: asc.WebhookEvents[:3]}},
		},
	}
	want := map[string]string{
		"Dots":  "ok Hook; ok Prod; none -",
		"Lines": "off Hook; unknown -; unknown -",
		"Rows":  "unknown -; none -; none -",
		"Pads":  "some Hook; none -; none -",
	}
	if got := standing(t, c); !equal(got, want) {
		t.Errorf("standing = %v, want %v", got, want)
	}
}

// A part is connected to one of the addresses, to one typed in, or to nothing. A webhook is made
// when the app has none here, else the one here is moved and set right: every event, turned on,
// signed with the secret. Nothing deletes it. Notifications are pointed, or cleared.
func TestAPartIsConnectedToAnAddressOneTypedInOrNothing(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)
	one := addAddress(t, c, "One")
	two := addAddress(t, c, "Two")
	fake := &apple{apps: []asc.AppURLs{{ID: "1", Name: "Dots"}}}
	s.apple = fake
	asked := func() string {
		defer func() { fake.asked = nil }()
		return strings.Join(fake.asked, "\n")
	}

	if got := connect(t, c, "1", `{"part":"webhook","endpoint":`+one.id+`}`); got != "made" {
		t.Errorf("webhook to One = %s", got)
	}
	if got := asked(); got != "make 1 "+one.url+" "+one.secret {
		t.Errorf("asked %q", got)
	}
	if got := connect(t, c, "1", `{"part":"webhook","endpoint":`+one.id+`}`); got != "already" || asked() != "" {
		t.Errorf("webhook to One again = %s", got)
	}
	if got := connect(t, c, "1", `{"part":"webhook","endpoint":`+two.id+`}`); got != "changed" {
		t.Errorf("webhook to Two = %s", got)
	}
	if got := asked(); got != "mend made-0 "+two.url+" "+two.secret {
		t.Errorf("asked %q", got)
	}
	if got := connect(t, c, "1", `{"part":"webhook","url":"https://ci.example.com/hook","secret":"theirs"}`); got != "changed" {
		t.Errorf("webhook typed in = %s", got)
	}
	if got := asked(); got != "mend made-0 https://ci.example.com/hook theirs" {
		t.Errorf("asked %q", got)
	}
	if got := standing(t, c)["Dots"]; got != "unknown -; none -; none -" {
		t.Errorf("sent elsewhere = %s", got)
	}
	if got := connect(t, c, "1", `{"part":"webhook"}`); got != "changed" || asked() != "delete made-0" {
		t.Errorf("webhook to nothing = %s", got)
	}
	if got := connect(t, c, "1", `{"part":"webhook"}`); got != "already" || asked() != "" {
		t.Errorf("webhook to nothing again = %s", got)
	}

	if got := connect(t, c, "1", `{"part":"production","endpoint":`+two.id+`}`); got != "changed" {
		t.Errorf("production to Two = %s", got)
	}
	if got := connect(t, c, "1", `{"part":"sandbox","url":"https://api.dots.example.com/apple"}`); got != "changed" {
		t.Errorf("sandbox typed in = %s", got)
	}
	if got := standing(t, c)["Dots"]; got != "none -; ok Two; unknown -" {
		t.Errorf("notifications = %s", got)
	}
	if got := connect(t, c, "1", `{"part":"production"}`); got != "changed" {
		t.Errorf("production to nothing = %s", got)
	}
	if got := asked(); got != fmt.Sprintf("notify 1 sandbox=false %q\nnotify 1 sandbox=true %q\nnotify 1 sandbox=false \"\"", two.url, "https://api.dots.example.com/apple") {
		t.Errorf("asked %q", got)
	}

	for request, want := range map[string]string{
		`{"part":"builds","endpoint":` + one.id + `}`:                                "400 A part is",
		`{"part":"webhook","url":"https://ci.example.com/hook"}`:                     "400 A webhook is signed with a secret",
		`{"part":"sandbox","url":"http://api.dots.example.com/apple"}`:               "400 Apple sends only to an https address",
		`{"part":"sandbox","endpoint":` + one.id + `,"url":"https://a.example.com"}`: "400 Send it to its automatic address, one of the addresses or one typed in",
		`{"part":"sandbox","auto":"separate","endpoint":` + one.id + `}`:             "400 Send it to its automatic address",
		`{"part":"sandbox","endpoint":999}`:                                          "404",
	} {
		if got := connect(t, c, "1", request); !strings.HasPrefix(got, want) {
			t.Errorf("%s = %s, want %s…", request, got, want)
		}
	}
	if got := connect(t, c, "9", `{"part":"sandbox"}`); !strings.HasPrefix(got, "404") {
		t.Errorf("no such app = %s", got)
	}
}

// A part connected to its automatic address has one made for it the first time, labelled with the
// app and the part and numbered when that label is taken by another, and marked as made for it;
// the next time it is the same one. One Apple would not take is not kept.
func TestAPartIsConnectedToItsAutomaticAddress(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)
	addAddress(t, c, "Dots: Sandbox")
	fake := &apple{apps: []asc.AppURLs{{ID: "1", Name: "Dots"}}}
	s.apple = fake

	resp := c.do("POST", "/api/hooks/apple/1/connect", `{"part":"webhook","auto":"separate"}`)
	got := body(t, resp)
	made, _ := got["endpoint"].(map[string]any)
	if resp.StatusCode != http.StatusOK || got["result"] != "made" || made["label"] != "Dots: Webhook" {
		t.Fatalf("new webhook address = %s %v", resp.Status, got)
	}
	if !strings.Contains(strings.Join(fake.asked, "\n"), "make 1 "+made["url"].(string)+" ") {
		t.Errorf("asked %v", fake.asked)
	}
	if made["app"] != "1" || made["part"] != "webhook" {
		t.Errorf("made = %v, want it marked as Dots' webhook's", made)
	}
	connect(t, c, "1", `{"part":"sandbox","auto":"separate"}`)
	if got := standing(t, c)["Dots"]; got != "ok Dots: Webhook; none -; ok Dots: Sandbox 2" {
		t.Errorf("standing = %s", got)
	}
	// Moved away and back, the sandbox's own address is the one it had.
	connect(t, c, "1", `{"part":"sandbox"}`)
	connect(t, c, "1", `{"part":"sandbox","auto":"separate"}`)
	if got := standing(t, c)["Dots"]; got != "ok Dots: Webhook; none -; ok Dots: Sandbox 2" {
		t.Errorf("standing again = %s", got)
	}
	if n := len(body(t, c.do("GET", "/api/hooks", ""))["endpoints"].([]any)); n != 3 {
		t.Errorf("%d addresses, want 3", n)
	}

	fake.refuseChange = &asc.Error{Status: http.StatusForbidden}
	if got := connect(t, c, "1", `{"part":"production","auto":"separate"}`); !strings.HasPrefix(got, "502") {
		t.Errorf("refused = %s", got)
	}
	for _, raw := range body(t, c.do("GET", "/api/hooks", ""))["endpoints"].([]any) {
		if l := raw.(map[string]any)["label"]; l == "Dots: Production" {
			t.Errorf("an address made for a part Apple would not take was kept")
		}
	}
}

// The app's main automatic address is one for all three parts, labelled with its name, and
// numbered when that is taken by an address added by hand.
func TestThePartsShareTheAppsMainAddress(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)
	addAddress(t, c, "Dots")
	fake := &apple{apps: []asc.AppURLs{{ID: "1", Name: "Dots"}}}
	s.apple = fake
	for _, part := range []string{"webhook", "production", "sandbox"} {
		if got := connect(t, c, "1", `{"part":"`+part+`","auto":"main"}`); got == "" || strings.HasPrefix(got, "4") || strings.HasPrefix(got, "5") {
			t.Errorf("%s = %s", part, got)
		}
	}
	if got := standing(t, c)["Dots"]; got != "ok Dots 2; ok Dots 2; ok Dots 2" {
		t.Errorf("standing = %s", got)
	}
	if n := len(body(t, c.do("GET", "/api/hooks", ""))["endpoints"].([]any)); n != 2 {
		t.Errorf("%d addresses, want the one by hand and the main one", n)
	}
	if got := connect(t, c, "1", `{"part":"sandbox","auto":"either"}`); !strings.HasPrefix(got, "400") {
		t.Errorf("auto either = %s", got)
	}
}

// Removing an address deletes the webhooks sent to it, and says which notifications still are.
func TestRemovingAnAddressDeletesItsWebhooksAtApple(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)
	hook := addAddress(t, c, "Hook")
	prod := addAddress(t, c, "Prod")
	fake := &apple{apps: []asc.AppURLs{{ID: "1", Name: "Dots"}}}
	s.apple = fake
	connect(t, c, "1", `{"part":"webhook","endpoint":`+hook.id+`}`)
	connect(t, c, "1", `{"part":"production","endpoint":`+prod.id+`}`)

	if resp := c.do("DELETE", "/api/hooks/endpoints/"+hook.id, ""); resp.StatusCode != http.StatusNoContent {
		t.Errorf("remove the webhook's = %s", resp.Status)
	}
	if len(fake.webhooks["1"]) != 0 {
		t.Errorf("webhooks left = %v", fake.webhooks["1"])
	}
	resp := c.do("DELETE", "/api/hooks/endpoints/"+prod.id, "")
	left := body(t, resp)["left"]
	if resp.StatusCode != http.StatusOK || !strings.Contains(fmt.Sprint(left), "Dots's App Store Server Notifications for production still go to it") {
		t.Errorf("remove production's = %s %v", resp.Status, left)
	}
	if got := standing(t, c)["Dots"]; got != "none -; unknown -; none -" {
		t.Errorf("after = %s", got)
	}
}

// The webhook events asked of Apple are the catalog's, in the names its API takes, the Test
// button's ping aside, which a webhook is sent whatever it was made for.
func TestAWebhookIsMadeForEveryKindInTheCatalog(t *testing.T) {
	var want []string
	for _, svc := range hooks.Catalog {
		if svc.Name != "App Store Connect webhooks" {
			continue
		}
		for _, g := range svc.Groups {
			for _, k := range g.Kinds {
				if k.Name == "webhookPingCreated" {
					continue
				}
				var b strings.Builder
				for _, r := range k.Name {
					if unicode.IsUpper(r) {
						b.WriteByte('_')
					}
					b.WriteRune(unicode.ToUpper(r))
				}
				want = append(want, b.String())
			}
		}
	}
	got := slices.Sorted(slices.Values(asc.WebhookEvents))
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func equal(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
