package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The password goes in and never comes out, and a form saved without one keeps it.
func TestARelaysPasswordIsKeptAndNeverShown(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)

	if got := body(t, c.do("GET", "/api/settings/relay", "")); got["configured"] != false || got["port"] != 587.0 {
		t.Errorf("before = %v", got)
	}
	resp := c.do("PUT", "/api/settings/relay", `{"host":"smtp.example.com","port":587,"security":"starttls","username":"stats","password":"a secret","from_address":"stats@example.com","from_name":""}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put = %s", resp.Status)
	}
	got := body(t, resp)
	if got["configured"] != true || got["password_set"] != true || strings.Contains(strings.ToLower(toJSON(got)), "a secret") {
		t.Errorf("after = %v", got)
	}
	// The port corrected without the password retyped.
	resp = c.do("PUT", "/api/settings/relay", `{"host":"smtp.example.com","port":465,"security":"implicit","username":"stats","password":"","from_address":"stats@example.com","from_name":""}`)
	if got := body(t, resp); resp.StatusCode != http.StatusOK || got["password_set"] != true || got["port"] != 465.0 {
		t.Errorf("corrected = %s %v", resp.Status, got)
	}
	// No third option: a password in the clear is not a choice to make by accident.
	if resp := c.do("PUT", "/api/settings/relay", `{"host":"smtp.example.com","port":25,"security":"none","username":"","password":"","from_address":"stats@example.com","from_name":""}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("no encryption = %s", resp.Status)
	}
	if resp := c.do("DELETE", "/api/settings/relay", ""); resp.StatusCode != http.StatusNoContent {
		t.Errorf("forget = %s", resp.Status)
	}
	resp = c.do("POST", "/api/settings/relay/test", `{"to":"me@example.com"}`)
	if got := body(t, resp); resp.StatusCode != http.StatusBadRequest || !strings.Contains(got["message"].(string), "no mail relay") {
		t.Errorf("a test with no relay = %s %v", resp.Status, got)
	}
}

func TestSessionsAreListedAndSignedOut(t *testing.T) {
	s, _ := newServer(t)
	phone := signIn(t, s)
	laptop := signIn(t, s)
	tablet := signIn(t, s)

	list := func(c *client) []any {
		t.Helper()
		return body(t, c.do("GET", "/api/sessions", ""))["sessions"].([]any)
	}
	sessions := list(laptop)
	if len(sessions) != 3 {
		t.Fatalf("sessions = %v", sessions)
	}
	var current, other string
	for _, raw := range sessions {
		sess := raw.(map[string]any)
		if sess["current"] == true {
			current = sess["id"].(string)
		} else if other == "" {
			other = sess["id"].(string)
		}
	}
	if current == "" || other == "" || strings.Contains(toJSON(sessions), laptop.cookie.Value) {
		t.Fatalf("current %q, other %q, or a cookie value shown", current, other)
	}

	if resp := laptop.do("DELETE", "/api/sessions/"+other, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sign out another = %s", resp.Status)
	}
	if n := len(list(laptop)); n != 2 {
		t.Errorf("after signing one out, %d left", n)
	}
	if resp := laptop.do("DELETE", "/api/sessions/"+other, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("signing out one already out = %s", resp.Status)
	}

	if resp := laptop.do("DELETE", "/api/sessions", ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sign out everywhere else = %s", resp.Status)
	}
	if n := len(list(laptop)); n != 1 {
		t.Errorf("after everywhere else, %d left", n)
	}
	for _, c := range []*client{phone, tablet} {
		if resp := c.do("GET", "/api/auth/me", ""); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("a signed-out browser = %s", resp.Status)
		}
	}

	// Signing out this browser clears its cookie in the same answer.
	if resp := laptop.do("DELETE", "/api/sessions/"+current, ""); resp.StatusCode != http.StatusNoContent || laptop.cookie != nil {
		t.Errorf("sign out this one = %s, cookie %v", resp.Status, laptop.cookie)
	}
}

func TestADeviceIsGuessedFromItsUserAgent(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1": "Safari on iPhone",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/141.0 Safari/537.36":                       "Chrome on a Mac",
		"curl/8.7.1": "curl",
		"":           "",
	} {
		if got := describeAgent(ua); got != want {
			t.Errorf("describeAgent(%q) = %q, want %q", ua, got, want)
		}
	}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
