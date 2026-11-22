package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"appstoreanalytics/internal/config"
)

// warnings is what the server logged at WARN, a line each.
func warnings(buf *bytes.Buffer) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if strings.Contains(line, `"level":"WARN"`) {
			out = append(out, line)
		}
	}
	return out
}

// logged has the server log as it does when it runs, in JSON.
func logged(s *Server) *bytes.Buffer {
	var buf bytes.Buffer
	s.log = slog.New(slog.NewJSONHandler(&buf, nil))
	return &buf
}

// What fail2ban reads: every refused sign-in is one WARN line, a fixed message and then the
// address. httptest's requests come from 192.0.2.1, which is no proxy of ours.
func TestARefusedSignInIsLoggedWithTheAddress(t *testing.T) {
	s, _ := newServer(t)
	buf := logged(s)
	c := &client{t: t, server: s}

	c.do("POST", "/api/auth/login", `{"username":"misha","password":"not it"}`)
	c.do("POST", "/api/auth/login", `{"username":"nobody","password":"a good password"}`)

	got := warnings(buf)
	if len(got) != 2 {
		t.Fatalf("warnings = %q, want 2", got)
	}
	for i, line := range got {
		if !strings.Contains(line, `"msg":"sign-in refused","ip":"192.0.2.1"`) {
			t.Errorf("warning %d = %q, want the message and then the address", i, line)
		}
		// A wrong username and a wrong password read the same here too, and a username that was
		// tried is sometimes a password.
		if strings.Contains(line, "misha") || strings.Contains(line, "nobody") || strings.Contains(line, "good password") {
			t.Errorf("warning %d names what was tried: %q", i, line)
		}
	}
}

func TestASignInPastTheLimitIsLoggedAsLimited(t *testing.T) {
	s, _ := newServer(t)
	buf := logged(s)
	c := &client{t: t, server: s}

	var last *http.Response
	for range 6 {
		last = c.do("POST", "/api/auth/login", `{"username":"misha","password":"not it"}`)
	}
	if last.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("sixth sign-in = %s, want 429", last.Status)
	}
	got := warnings(buf)
	if n := len(got); n != 6 || !strings.Contains(got[n-1], `"msg":"sign-in rate limited","ip":"192.0.2.1"`) {
		t.Errorf("warnings = %q", got)
	}
}

// A sign-in that works is no refusal.
func TestASignInThatWorksIsNotCounted(t *testing.T) {
	s, _ := newServer(t)
	buf := logged(s)
	signIn(t, s)
	if got := warnings(buf); len(got) != 0 {
		t.Errorf("warnings = %q, want none", got)
	}
}

func TestTheCallerIsWhoTheProxySaw(t *testing.T) {
	for _, c := range []struct {
		name, proxies, peer, xff, xri, want string
	}{
		{"a proxy on the docker network", "private", "172.18.0.5:41000", "203.0.113.7", "", "203.0.113.7"},
		{"only the proxy's own hop counts", "private", "172.18.0.5:41000", "198.51.100.1, 203.0.113.7", "", "203.0.113.7"},
		{"ipv6 through a proxy", "private", "172.18.0.5:41000", "2001:db8::7", "", "2001:db8::7"},
		{"loopback is a proxy too", "private", "127.0.0.1:41000", "203.0.113.7", "", "203.0.113.7"},
		{"no header, the peer", "private", "172.18.0.5:41000", "", "", "172.18.0.5"},
		{"a header that is not an address, the peer", "private", "172.18.0.5:41000", "unknown", "", "172.18.0.5"},
		{"a stranger cannot name somebody else", "private", "203.0.113.9:41000", "198.51.100.1", "", "203.0.113.9"},
		{"a stranger over ipv6", "private", "[2001:db8::9]:41000", "198.51.100.1", "", "2001:db8::9"},
		{"ipv4 mapped into ipv6", "private", "[::ffff:203.0.113.9]:41000", "", "", "203.0.113.9"},
		{"X-Real-IP is the caller's own", "private", "172.18.0.5:41000", "203.0.113.7", "198.51.100.1", "203.0.113.7"},
		{"X-Real-IP from a stranger", "private", "203.0.113.9:41000", "", "198.51.100.1", "203.0.113.9"},
		{"a proxy on another host, named", "198.51.100.3", "198.51.100.3:41000", "203.0.113.7", "", "203.0.113.7"},
		{"naming one stops trusting the rest", "198.51.100.3", "172.18.0.5:41000", "203.0.113.7", "", "172.18.0.5"},
		{"a chain of named proxies", "private, 198.51.100.0/24", "172.18.0.5:41000", "192.0.2.1, 203.0.113.7, 198.51.100.3", "", "203.0.113.7"},
		{"a chain whose first hop is not named", "private", "172.18.0.5:41000", "203.0.113.7, 198.51.100.3", "", "198.51.100.3"},
		{"nothing but proxies, the furthest", "private", "172.18.0.5:41000", "10.0.0.2, 10.0.0.3", "", "10.0.0.2"},
		{"none believes nobody", "none", "172.18.0.5:41000", "203.0.113.7", "", "172.18.0.5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			proxies, err := config.ParseProxies(c.proxies)
			if err != nil {
				t.Fatal(err)
			}
			s := &Server{cfg: &config.Config{TrustedProxies: proxies}}
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.peer
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if c.xri != "" {
				r.Header.Set("X-Real-IP", c.xri)
			}
			if got := s.clientOf(r); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

// A proxy that sends the header twice means one list, in order.
func TestTwoForwardedHeadersAreOneList(t *testing.T) {
	proxies, _ := config.ParseProxies("private")
	s := &Server{cfg: &config.Config{TrustedProxies: proxies}}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.18.0.5:41000"
	r.Header.Add("X-Forwarded-For", "198.51.100.1")
	r.Header.Add("X-Forwarded-For", "203.0.113.7")
	if got := s.clientOf(r); got != "203.0.113.7" {
		t.Errorf("got %s", got)
	}
}
