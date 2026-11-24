package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"appstoreanalytics/internal/session"
)

// Seeder sets the demo up once appstoreanalytics is running: it waits for the first sync to
// read the account, then connects two apps as a person would on the Settings page, all three
// parts to the app's main automatic address: that makes the webhook and points the notifications
// in App Store Connect.
type Seeder struct {
	// Site is appstoreanalytics, and Username and Password its sign-in.
	Site, Username, Password string
	Log                      *slog.Logger

	http *http.Client
	// cookie is the session, sent by hand: a jar keeps a Secure one, as an https address makes
	// it, from the plain http appstoreanalytics is reached by on the demo's network.
	cookie string
}

// hooked are the apps connected, by name.
var hooked = []string{"Tidepool", "Knotwork"}

// Run sets the demo up, leaving alone what an earlier run did.
func (s *Seeder) Run(ctx context.Context) error {
	s.http = &http.Client{Timeout: 30 * time.Second}

	if err := until(ctx, "appstoreanalytics", time.Minute, func() (bool, error) {
		resp, err := s.http.Get(s.Site + "/healthz")
		if err != nil {
			return false, err
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK, nil
	}); err != nil {
		return err
	}
	if err := s.signIn(); err != nil {
		return fmt.Errorf("sign in: %w", err)
	}

	s.Log.Info("waiting for the first sync to read the account")
	var failed string
	// About half a minute for an account's years; a sync still going after two is stuck.
	if err := until(ctx, "the first sync", 2*time.Minute, func() (bool, error) {
		var st struct {
			Running   bool `json:"running"`
			Analytics struct {
				Last      string `json:"last"`
				Instances int    `json:"instances"`
				Part      struct {
					OKAt  *int64 `json:"ok_at"`
					Error string `json:"error"`
				} `json:"part"`
			} `json:"analytics"`
		}
		if err := s.site(http.MethodGet, "/api/status", nil, &st); err != nil {
			return false, err
		}
		if st.Analytics.Part.Error != "" && !st.Running {
			failed = st.Analytics.Part.Error
			return true, nil
		}
		return !st.Running && st.Analytics.Part.OKAt != nil && st.Analytics.Last != "", nil
	}); err != nil {
		return err
	}
	// What was read is a demo still, and the rest is on the status page.
	if failed != "" {
		s.Log.Warn("the first sync did not all go through; the demo goes on with what it read", "why", failed)
	}

	for _, name := range hooked {
		if err := s.connect(ctx, name); err != nil {
			return fmt.Errorf("connect %s: %w", name, err)
		}
	}
	// Signed out, so the sessions in Settings are the browsers'.
	return s.site(http.MethodPost, "/api/auth/logout", nil, nil)
}

// parts are what an app is connected by.
var parts = []string{"webhook", "production", "sandbox"}

// connect connects each of an app's parts to its main automatic address, as Auto, Main on the
// Settings page does, and when that made its webhook just now, waits for what the webhook is sent.
// Connected already, there is nothing to do.
func (s *Seeder) connect(ctx context.Context, name string) error {
	var app *App
	for _, a := range apps {
		if a.Name == name {
			app = a
		}
	}
	made := false
	for _, part := range parts {
		var connected struct {
			Result string `json:"result"`
		}
		if err := s.site(http.MethodPost, "/api/hooks/apple/"+app.ID+"/connect", map[string]any{"part": part, "auto": "main"}, &connected); err != nil {
			return fmt.Errorf("its %s: %w", part, err)
		}
		made = made || part == "webhook" && connected.Result == "made"
	}
	if !made {
		return nil
	}
	label := name
	want := len(story(app, time.Now()))
	return until(ctx, label+"'s deliveries", time.Minute, func() (bool, error) {
		var listed struct {
			Endpoints []struct {
				Label string `json:"label"`
				Count int    `json:"count"`
			} `json:"endpoints"`
		}
		if err := s.site(http.MethodGet, "/api/hooks", nil, &listed); err != nil {
			return false, err
		}
		for _, e := range listed.Endpoints {
			if e.Label == label {
				return e.Count >= want, nil
			}
		}
		return false, nil
	})
}

// signIn signs in to appstoreanalytics, keeping the session's cookie.
func (s *Seeder) signIn() error {
	raw, _ := json.Marshal(map[string]string{"username": s.Username, "password": s.Password})
	resp, err := s.http.Post(s.Site+"/api/auth/login", "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("appstoreanalytics answered %s", resp.Status)
	}
	for _, c := range resp.Cookies() {
		if c.Name == session.CookieName {
			s.cookie = c.Name + "=" + c.Value
		}
	}
	if s.cookie == "" {
		return errors.New("appstoreanalytics signed in without a session cookie")
	}
	return nil
}

// site asks appstoreanalytics, signed in.
func (s *Seeder) site(method, path string, in, out any) error {
	return call(s.http, method, s.Site+path, "", s.cookie, in, out)
}

func call(c *http.Client, method, url, token, cookie string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, _ := json.Marshal(in)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		said, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("%s %s answered %s: %s", method, url, resp.Status, bytes.TrimSpace(said))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// until asks again every two seconds until done says so, for as long as given.
func until(ctx context.Context, what string, wait time.Duration, done func() (bool, error)) error {
	deadline := time.Now().Add(wait)
	var last error
	for {
		ok, err := done()
		if ok {
			return nil
		}
		if err != nil {
			last = err
		}
		if time.Now().After(deadline) {
			if last != nil {
				return fmt.Errorf("%s did not answer in %s: %w", what, wait, last)
			}
			return fmt.Errorf("%s did not answer in %s", what, wait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
