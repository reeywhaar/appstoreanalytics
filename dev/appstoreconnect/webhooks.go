package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
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
			for _, e := range story(a) {
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
	go s.deliver(h, event{"webhookPingCreated", nil, "webhookPings", id})
}

// event is a webhook's delivery: its type, what changed, and what it is about.
type event struct {
	kind  string
	attrs map[string]any
	of    string
	id    string
}

// change is a state that moved, as Apple's events say it.
func change(from, to string) map[string]any {
	return map[string]any{"oldValue": from, "newValue": to}
}

// story is what happened to an app lately: a build uploaded and through beta review, a tester's
// word, and a version through App Review to sale. Knotwork's was rejected once.
func story(a *App) []event {
	build, release := newID(), newID()
	v := func(from, to string) event {
		return event{"appStoreVersionAppVersionStateUpdated", change(from, to), "appStoreVersions", release}
	}
	out := []event{
		{"buildUploadStateUpdated", change("PROCESSING", "COMPLETE"), "buildUploads", build},
		{"buildBetaDetailExternalBuildStateUpdated", change("WAITING_FOR_BETA_REVIEW", "BETA_APPROVED"), "buildBetaDetails", build},
		{"betaFeedbackScreenshotSubmissionCreated", nil, "betaFeedbackScreenshotSubmissions", newID()},
		v("PREPARE_FOR_SUBMISSION", "WAITING_FOR_REVIEW"),
		v("WAITING_FOR_REVIEW", "IN_REVIEW"),
	}
	if a.Name == "Knotwork" {
		out = append(out,
			v("IN_REVIEW", "REJECTED"),
			event{"betaFeedbackCrashSubmissionCreated", nil, "betaFeedbackCrashSubmissions", newID()},
			v("REJECTED", "WAITING_FOR_REVIEW"),
			v("WAITING_FOR_REVIEW", "IN_REVIEW"),
		)
	}
	out = append(out, v("IN_REVIEW", "PENDING_DEVELOPER_RELEASE"), v("PENDING_DEVELOPER_RELEASE", "READY_FOR_SALE"))
	return out
}

// deliver posts an event to a webhook, signed with its secret as Apple signs them.
func (s *Server) deliver(h Webhook, e event) {
	attrs := map[string]any{"timestamp": s.Now().UTC().Format("2006-01-02T15:04:05.000Z")}
	for k, v := range e.attrs {
		attrs[k] = v
	}
	body, _ := json.Marshal(map[string]any{"data": map[string]any{
		"type": e.kind, "id": newID(), "version": 1, "attributes": attrs,
		"relationships": map[string]any{"instance": map[string]any{"data": map[string]string{"type": e.of, "id": e.id}}},
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
