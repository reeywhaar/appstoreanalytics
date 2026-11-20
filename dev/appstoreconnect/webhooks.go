package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func webhookResource(h Webhook) resource {
	return resource{Type: "webhooks", ID: h.ID, Attributes: map[string]any{
		"name": h.Name, "url": h.URL, "enabled": h.Enabled, "eventTypes": h.Events,
	}}
}

func (s *Server) webhooks(w http.ResponseWriter, r *http.Request) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	var data []resource
	for _, h := range s.state.Webhooks {
		if h.App == r.PathValue("id") {
			data = append(data, webhookResource(h))
		}
	}
	page(w, r, data)
}

// makeWebhook makes an app's webhook. What has happened to the app lately is then delivered to
// it, as though the webhook had been there to hear it.
func (s *Server) makeWebhook(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Data struct {
			Attributes struct {
				Name       string   `json:"name"`
				URL        string   `json:"url"`
				Secret     string   `json:"secret"`
				Enabled    bool     `json:"enabled"`
				EventTypes []string `json:"eventTypes"`
			} `json:"attributes"`
			Relationships struct {
				App struct {
					Data struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"app"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		refuse(w, http.StatusBadRequest, "PARAMETER_ERROR.INVALID", "A parameter has an invalid value", err.Error())
		return
	}
	at := in.Data.Attributes
	a := appByID(in.Data.Relationships.App.Data.ID)
	if a == nil {
		refuse(w, http.StatusNotFound, "NOT_FOUND", "The specified resource does not exist", "There is no such app")
		return
	}
	if u, err := url.Parse(at.URL); err != nil || u.Host == "" || at.Secret == "" {
		refuse(w, http.StatusConflict, "ENTITY_ERROR.ATTRIBUTE.INVALID", "An attribute value is invalid.", "A webhook needs a url and a secret")
		return
	}
	h := Webhook{ID: newID(), App: a.ID, Name: at.Name, URL: at.URL, Secret: at.Secret, Enabled: at.Enabled, Events: at.EventTypes}
	s.state.mu.Lock()
	s.state.Webhooks = append(s.state.Webhooks, h)
	_ = s.state.save()
	s.state.mu.Unlock()
	s.Log.Info("webhook made", "app", a.ID, "webhook", h.ID)
	write(w, http.StatusCreated, map[string]any{"data": webhookResource(h)})
	if h.Enabled {
		go func() {
			for _, e := range story(a, s.Now()) {
				time.Sleep(s.Pause)
				s.deliver(h, e)
			}
		}()
	}
}

// ping sends a webhook its test event, as its Test button in App Store Connect does.
func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Data struct {
			Relationships struct {
				Webhook struct {
					Data struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"webhook"`
			} `json:"relationships"`
		} `json:"data"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	var (
		h     Webhook
		found bool
	)
	s.state.mu.Lock()
	for _, x := range s.state.Webhooks {
		if x.ID == in.Data.Relationships.Webhook.Data.ID {
			h, found = x, true
		}
	}
	s.state.mu.Unlock()
	if !found {
		refuse(w, http.StatusNotFound, "NOT_FOUND", "The specified resource does not exist", "There is no such webhook")
		return
	}
	id := newID()
	write(w, http.StatusCreated, map[string]any{"data": resource{Type: "webhookPings", ID: id}})
	go s.deliver(h, event{kind: "webhookPingCreated", of: "webhookPings", about: id, at: s.Now(), id: newID()})
}

// event is a webhook's delivery: its type, what changed, what it is about, when it happened,
// and its own id.
type event struct {
	kind  string
	attrs map[string]any
	of    string
	about string
	at    time.Time
	id    string
}

// change is a state that moved, as Apple's events say it.
func change(from, to string) map[string]any {
	return map[string]any{"oldValue": from, "newValue": to}
}

// story is what happened to an app over the days before now: a build uploaded and through beta
// review, a tester's word, and a version through App Review to sale. Knotwork's was rejected
// once. Its ids are made from the app and the day, so a story told on one day is told the same.
func story(a *App, now time.Time) []event {
	day := now.UTC().Format(time.DateOnly)
	id := func(what string) string { return idOf(a.ID, day, what) }
	build, release := id("build"), id("release")
	v := func(from, to string) event {
		return event{kind: "appStoreVersionAppVersionStateUpdated", attrs: change(from, to), of: "appStoreVersions", about: release}
	}
	out := []event{
		{kind: "buildUploadStateUpdated", attrs: change("PROCESSING", "COMPLETE"), of: "buildUploads", about: build},
		{kind: "buildBetaDetailExternalBuildStateUpdated", attrs: change("WAITING_FOR_BETA_REVIEW", "BETA_APPROVED"), of: "buildBetaDetails", about: build},
		{kind: "betaFeedbackScreenshotSubmissionCreated", of: "betaFeedbackScreenshotSubmissions", about: id("feedback")},
		v("PREPARE_FOR_SUBMISSION", "WAITING_FOR_REVIEW"),
		v("WAITING_FOR_REVIEW", "IN_REVIEW"),
	}
	// Hours before now each happened.
	hours := []float64{70.4, 63.8, 49.2, 46.1, 27.5, 25.3, 19.9}
	if a.Name == "Knotwork" {
		out = append(out,
			v("IN_REVIEW", "REJECTED"),
			event{kind: "betaFeedbackCrashSubmissionCreated", of: "betaFeedbackCrashSubmissions", about: id("crash")},
			v("REJECTED", "WAITING_FOR_REVIEW"),
			v("WAITING_FOR_REVIEW", "IN_REVIEW"),
		)
		hours = []float64{93.6, 87.9, 74.2, 71.8, 51.4, 48.7, 43.2, 39.9, 22.6, 20.1, 15.3}
	}
	out = append(out, v("IN_REVIEW", "PENDING_DEVELOPER_RELEASE"), v("PENDING_DEVELOPER_RELEASE", "READY_FOR_SALE"))
	for i := range out {
		out[i].at = now.Add(-time.Duration(hours[i] * float64(time.Hour))).Truncate(time.Millisecond)
		out[i].id = id("event " + strconv.Itoa(i))
	}
	return out
}

// deliver posts an event to a webhook, signed with its secret as Apple signs them.
func (s *Server) deliver(h Webhook, e event) {
	attrs := map[string]any{"timestamp": e.at.UTC().Format("2006-01-02T15:04:05.000Z")}
	for k, v := range e.attrs {
		attrs[k] = v
	}
	body, _ := json.Marshal(map[string]any{"data": map[string]any{
		"type": e.kind, "id": e.id, "version": 1, "attributes": attrs,
		"relationships": map[string]any{"instance": map[string]any{"data": map[string]string{"type": e.of, "id": e.about}}},
	}})
	to := h.URL
	if s.DeliverTo != "" {
		if u, err := url.Parse(h.URL); err == nil {
			to = s.DeliverTo + u.RequestURI()
		}
	}
	mac := hmac.New(sha256.New, []byte(h.Secret))
	mac.Write(body)
	req, err := http.NewRequest(http.MethodPost, to, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-apple-signature", "hmacsha256="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.Log.Warn("a webhook could not be delivered", "webhook", h.ID, "error", err.Error())
		return
	}
	resp.Body.Close()
	s.Log.Info("webhook delivered", "webhook", h.ID, "event", e.kind, "status", resp.StatusCode)
}
