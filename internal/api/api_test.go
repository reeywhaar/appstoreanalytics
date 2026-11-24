package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/config"
	"appstoreanalytics/internal/session"
	"appstoreanalytics/internal/store"
)

type puller struct{ woken int }

func (p *puller) Wake() bool      { p.woken++; return true }
func (p *puller) Running() bool   { return false }
func (p *puller) Reading() string { return "" }

// apple stands in for App Store Connect: what it says sends where, and what it was asked.
type apple struct {
	mu       sync.Mutex
	apps     []asc.AppURLs
	webhooks map[string][]asc.Webhook
	noIAPKey bool
	refuse   error
	pinged   []string
	tested   []string
	// asked is every change asked for, in words: made, mended and deleted webhooks, and
	// notification addresses set.
	asked []string
	// refuseChange is what every change is answered with, nil to make it.
	refuseChange error
}

func (a *apple) NotificationURLs(context.Context) ([]asc.AppURLs, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.apps), a.refuse
}

func (a *apple) Webhooks(_ context.Context, appID string) ([]asc.Webhook, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.webhooks[appID]), nil
}

func (a *apple) MakeWebhook(_ context.Context, appID, name, address, secret string) (asc.Webhook, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.refuseChange != nil {
		return asc.Webhook{}, a.refuseChange
	}
	if a.webhooks == nil {
		a.webhooks = map[string][]asc.Webhook{}
	}
	h := asc.Webhook{ID: fmt.Sprintf("made-%d", len(a.asked)), Name: name, URL: address, Enabled: true, Events: asc.WebhookEvents}
	a.webhooks[appID] = append(a.webhooks[appID], h)
	a.asked = append(a.asked, fmt.Sprintf("make %s %s %s", appID, address, secret))
	return h, nil
}

func (a *apple) MendWebhook(_ context.Context, id, address, secret string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, hooks := range a.webhooks {
		for i := range hooks {
			if hooks[i].ID == id {
				hooks[i].URL, hooks[i].Enabled, hooks[i].Events = address, true, asc.WebhookEvents
			}
		}
	}
	a.asked = append(a.asked, fmt.Sprintf("mend %s %s %s", id, address, secret))
	return nil
}

func (a *apple) DeleteWebhook(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for app, hooks := range a.webhooks {
		a.webhooks[app] = slices.DeleteFunc(hooks, func(h asc.Webhook) bool { return h.ID == id })
	}
	a.asked = append(a.asked, "delete "+id)
	return nil
}

func (a *apple) SetNotificationURL(_ context.Context, appID string, sandbox bool, address string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.refuseChange != nil {
		return a.refuseChange
	}
	for i := range a.apps {
		if a.apps[i].ID != appID {
			continue
		}
		if sandbox {
			a.apps[i].Sandbox = address
		} else {
			a.apps[i].Production = address
		}
	}
	a.asked = append(a.asked, fmt.Sprintf("notify %s sandbox=%v %q", appID, sandbox, address))
	return nil
}

func (a *apple) PingWebhook(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pinged = append(a.pinged, id)
	return nil
}

func (a *apple) TestNotification(_ context.Context, bundleID string, sandbox bool) (string, error) {
	if a.noIAPKey {
		return "", asc.ErrNoIAPKey
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tested = append(a.tested, fmt.Sprintf("%s %v", bundleID, sandbox))
	return "token", nil
}

func newServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	spa, err := NewSPA(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<div id="root">app`)},
		"login.html": &fstest.MapFile{Data: []byte(`<div id="root">login`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, _ := url.Parse("https://stats.example.com")
	cfg := &config.Config{PublicURL: u, Secure: true, Username: "Misha", Password: "a good password"}
	return New(cfg, slog.New(slog.DiscardHandler), st, spa, &puller{}, &apple{}), st
}

// client keeps a cookie jar of one, which is all a session is.
type client struct {
	t      *testing.T
	server *Server
	cookie *http.Cookie
}

func (c *client) do(method, path, body string) *http.Response {
	c.t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
		r.ContentLength = 0
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.server.ServeHTTP(w, r)
	resp := w.Result()
	for _, ck := range resp.Cookies() {
		if ck.Name == session.CookieName {
			if ck.MaxAge < 0 {
				c.cookie = nil
			} else {
				c.cookie = ck
			}
		}
	}
	return resp
}

func body(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func signIn(t *testing.T, s *Server) *client {
	t.Helper()
	c := &client{t: t, server: s}
	// The username without regard to case, the password exactly.
	if resp := c.do("POST", "/api/auth/login", `{"username":"misha","password":"a good password"}`); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login = %s", resp.Status)
	}
	if c.cookie == nil || !c.cookie.HttpOnly || !c.cookie.Secure {
		t.Fatalf("cookie = %+v", c.cookie)
	}
	return c
}

func TestSigningInTakesTheUsernameAndPasswordOfTheEnvironment(t *testing.T) {
	s, _ := newServer(t)
	c := &client{t: t, server: s}
	for _, wrong := range []string{
		`{"username":"misha","password":"A good password"}`,
		`{"username":"masha","password":"a good password"}`,
	} {
		resp := c.do("POST", "/api/auth/login", wrong)
		if resp.StatusCode != http.StatusUnauthorized || body(t, resp)["code"] != CodeUnauthenticated {
			t.Errorf("%s = %s", wrong, resp.Status)
		}
	}
	c = signIn(t, s)
	if me := body(t, c.do("GET", "/api/auth/me", "")); me["username"] != "Misha" {
		t.Errorf("me = %v", me)
	}
	if resp := c.do("POST", "/api/auth/logout", ""); resp.StatusCode != http.StatusNoContent || c.cookie != nil {
		t.Fatalf("logout = %s, cookie %v", resp.Status, c.cookie)
	}
}

func TestSigningInIsLimitedPerUsername(t *testing.T) {
	s, _ := newServer(t)
	c := &client{t: t, server: s}
	var last *http.Response
	for range 6 {
		last = c.do("POST", "/api/auth/login", `{"username":"misha","password":"guess"}`)
	}
	if last.StatusCode != http.StatusTooManyRequests {
		t.Errorf("the sixth guess = %s, want 429", last.Status)
	}
}

func TestEverythingButSigningInWantsASession(t *testing.T) {
	s, _ := newServer(t)
	c := &client{t: t, server: s}
	for _, path := range []string{"/api/auth/me", "/api/apps", "/api/report", "/api/history", "/api/status"} {
		resp := c.do("GET", path, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s = %s, want 401", path, resp.Status)
		}
	}
	if resp := c.do("POST", "/api/sync", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("sync = %s, want 401", resp.Status)
	}
}

// The shell is only for somebody signed in, and the sign-in page only for somebody who is not.
func TestAPageIsTheOneForWhoeverAsks(t *testing.T) {
	s, _ := newServer(t)
	stranger := &client{t: t, server: s}
	if resp := stranger.do("GET", "/apps/123", ""); resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login" {
		t.Errorf("a stranger at the app = %s to %q", resp.Status, resp.Header.Get("Location"))
	}
	if resp := stranger.do("GET", "/login", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("a stranger at the sign-in = %s", resp.Status)
	}
	c := signIn(t, s)
	if resp := c.do("GET", "/login", ""); resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		t.Errorf("signed in at the sign-in = %s to %q", resp.Status, resp.Header.Get("Location"))
	}
	if resp := c.do("GET", "/apps/123", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("signed in at the app = %s", resp.Status)
	}
}

// Falling through to the SPA would reach a fetch as HTML it cannot parse.
func TestAnUnknownAPIPathIsJSON(t *testing.T) {
	s, _ := newServer(t)
	resp := (&client{t: t, server: s}).do("GET", "/api/nope", "")
	if resp.StatusCode != http.StatusNotFound || body(t, resp)["ok"] != false {
		t.Errorf("status = %s", resp.Status)
	}
}

// Its absence is a security property: the CSRF guard leans on it.
func TestNoCORSHeaderIsEverEmitted(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)
	for _, path := range []string{"/healthz", "/api/nope", "/", "/api/report"} {
		if v := c.do("GET", path, "").Header.Get("Access-Control-Allow-Origin"); v != "" {
			t.Errorf("%s emitted Access-Control-Allow-Origin: %q", path, v)
		}
	}
}

func TestACrossSiteWriteIsRefused(t *testing.T) {
	s, _ := newServer(t)
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestAReportIsOfARangeOfDaysEndingOnTheLastDownloadsMeasured(t *testing.T) {
	s, st := newServer(t)
	ctx := context.Background()
	st.SaveApps(ctx, []store.App{{ID: "42", Name: "Dots"}})
	var measured []store.Measured
	for _, day := range []string{"2026-11-13", "2026-11-14"} {
		measured = append(measured, store.Measured{AppID: "42", Day: day, Metric: store.MetricFirstDownloads, Territory: "US", Value: 3})
	}
	// Impressions arrive a day before downloads, and do not move where a range ends.
	measured = append(measured, store.Measured{AppID: "42", Day: "2026-11-15", Metric: store.MetricImpressions, Territory: "US", Value: 100})
	st.SaveInstance(ctx, store.Instance{ID: "i", AppID: "42", Report: "r"}, nil, measured, nil)
	c := signIn(t, s)

	got := body(t, c.do("GET", "/api/report?days=7&app=42", ""))
	if got["from"] != "2026-11-08" || got["to"] != "2026-11-14" || got["previous_from"] != "2026-11-01" {
		t.Errorf("span = %v %v %v", got["from"], got["to"], got["previous_from"])
	}
	if totals := got["totals"].(map[string]any); totals[store.MetricFirstDownloads] != 6.0 {
		t.Errorf("totals = %v", totals)
	}

	for query, status := range map[string]int{
		"?app=43":                        http.StatusNotFound,
		"?app=dots":                      http.StatusBadRequest,
		"?days=0":                        http.StatusBadRequest,
		"?from=2026-11-14&to=2026-11-01": http.StatusBadRequest,
		"?from=14.11.2026":               http.StatusBadRequest,
		"?from=2026-11-01&to=2026-11-14": http.StatusOK,
	} {
		if resp := c.do("GET", "/api/report"+query, ""); resp.StatusCode != status {
			t.Errorf("%s = %s, want %d", query, resp.Status, status)
		}
	}
}

// A history is every day from the first measured to the last, so charts scroll back past the
// range the totals are of; a day between that measured nothing is known, at none.
func TestAHistoryIsEveryDayFromTheFirstMeasuredToTheLast(t *testing.T) {
	s, st := newServer(t)
	ctx := context.Background()
	st.SaveApps(ctx, []store.App{{ID: "42", Name: "Dots"}})
	var measured []store.Measured
	for _, day := range []string{"2026-01-02", "2026-11-13", "2026-11-14"} {
		measured = append(measured, store.Measured{AppID: "42", Day: day, Metric: store.MetricFirstDownloads, Territory: "US", Value: 3})
	}
	st.SaveInstance(ctx, store.Instance{ID: "i", AppID: "42", Report: "r"}, nil, measured, nil)
	c := signIn(t, s)

	got := body(t, c.do("GET", "/api/history?app=42", ""))
	days := got["days"].([]any)
	if got["from"] != "2026-01-02" || got["to"] != "2026-11-14" || len(days) != 317 {
		t.Errorf("history from %v to %v, %d days", got["from"], got["to"], len(days))
	}
	if between := days[1].(map[string]any); between["day"] != "2026-01-03" || between["first_downloads"] != 0.0 {
		t.Errorf("a day between = %v", between)
	}
	if len(got["by_app"].(map[string]any)) != 0 {
		t.Errorf("one app's history is split by app: %v", got["by_app"])
	}

	// Every app's history says each app's own days, for the charts to tell them apart.
	st.SaveApps(ctx, []store.App{{ID: "43", Name: "Lines"}})
	st.SaveInstance(ctx, store.Instance{ID: "j", AppID: "43", Report: "r"}, nil, []store.Measured{
		{AppID: "43", Day: "2026-11-14", Metric: store.MetricFirstDownloads, Territory: "US", Value: 2},
	}, nil)
	every := body(t, c.do("GET", "/api/history", ""))
	byApp := every["by_app"].(map[string]any)
	dots := byApp["42"].(map[string]any)
	lines := byApp["43"].(map[string]any)
	if len(dots) != 3 || toJSON(lines["2026-11-14"]) != `{"first_downloads":2}` {
		t.Errorf("by app = %v", byApp)
	}
	all := every["days"].([]any)
	if last := all[len(all)-1].(map[string]any); last["first_downloads"] != 5.0 {
		t.Errorf("the day of both = %v", last)
	}
	if resp := c.do("GET", "/api/history?app=dots", ""); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a bad app = %s", resp.Status)
	}

	// A year at a time: the year before a day asked for, down to the first day there is.
	earlier := body(t, c.do("GET", "/api/history?app=42&before=2026-11-13", ""))
	if earlier["from"] != "2026-01-02" || earlier["to"] != "2026-11-12" || earlier["first"] != "2026-01-02" || len(earlier["days"].([]any)) != 315 {
		t.Errorf("the year before from %v to %v, %d days", earlier["from"], earlier["to"], len(earlier["days"].([]any)))
	}
	if resp := c.do("GET", "/api/history?before=soon", ""); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a bad before = %s", resp.Status)
	}
}

func TestSyncWakesThePuller(t *testing.T) {
	s, _ := newServer(t)
	c := signIn(t, s)
	if resp := c.do("POST", "/api/sync", ""); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("sync = %s", resp.Status)
	}
	if s.pull.(*puller).woken != 1 {
		t.Error("the puller was not woken")
	}
	if got := body(t, c.do("GET", "/api/status", "")); got["running"] != false {
		t.Errorf("status = %v", got)
	}
}
