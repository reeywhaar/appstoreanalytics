package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/hooks"
	"appstoreanalytics/internal/mail"
	"appstoreanalytics/internal/store"
)

// hookMax bounds a delivery. Apple's are a few kilobytes.
const hookMax = 1 << 20

// receiveHook keeps whatever a webhook delivers, checks where it came from, and mails it where
// its event is one to mail and its origin is proven.
//
// Let in by the token in its URL, which names one of the addresses, so a wrong one, or one of
// an address removed, is the same 404 as a path that was never there. One that fails its signature is still kept, marked so, and never mailed: what was sent
// to this address is worth seeing either way. Answered before any mail is sent, because a sender
// that waits too long gives up and sends again.
func (s *Server) receiveHook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	endpoint, err := s.store.HookEndpointByToken(ctx, r.PathValue("token"))
	if errors.Is(err, store.ErrNotFound) {
		refuse(w, http.StatusNotFound, CodeNotFound, "There is no such endpoint.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.hookLimit.allow("") {
		refuse(w, http.StatusTooManyRequests, CodeRateLimited, "Too many deliveries. Send again in a moment.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, hookMax))
	var over *http.MaxBytesError
	if errors.As(err, &over) {
		refuse(w, http.StatusRequestEntityTooLarge, CodeBodyTooLarge, fmt.Sprintf("A delivery is at most %d MB.", hookMax>>20))
		return
	}
	if err != nil {
		refuse(w, http.StatusBadRequest, CodeInvalid, "The delivery could not be read.")
		return
	}
	d := hooks.Describe(body)
	proof := hooks.Check(body, r.Header.Get("X-Apple-Signature"), endpoint.Secret)
	id, err := s.store.SaveHook(ctx, store.Hook{
		EndpointID:  endpoint.ID,
		Via:         endpoint.Label,
		Event:       d.Event,
		Summary:     d.Summary,
		ContentType: truncate(r.Header.Get("Content-Type"), 200),
		Body:        body,
		ProvenBy:    proof.By,
		ProofError:  proof.Failed,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if proof.Failed != "" {
		s.log.Warn("webhook received, and its signature failed", "delivery", id, "via", endpoint.Label, "event", d.Event, "bytes", len(body), "why", proof.Failed)
	} else {
		s.log.Info("webhook received", "delivery", id, "via", endpoint.Label, "event", d.Event, "bytes", len(body), "proven_by", proof.By)
	}

	to, err := s.store.Recipients(ctx, d.Event)
	if err == nil && proof.Proven() && len(to) > 0 {
		subject, text := hooks.Letter(d, body, hooks.About{
			App:      s.appNamed(ctx, d.BundleID),
			Via:      endpoint.Label,
			ProvenBy: proof.By,
			Received: s.store.Now(),
		})
		bg := context.WithoutCancel(ctx)
		s.mailing.Go(func() {
			s.mailHook(bg, id, d.Event, to, mail.Message{Subject: subject, Body: text})
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// mailHook hands one delivery's letter to the relay for each of its recipients, and records
// whom it went to and what the relay refused.
func (s *Server) mailHook(ctx context.Context, id int64, event string, to []string, msg mail.Message) {
	var why []string
	relay, err := s.store.Relay(ctx)
	switch {
	case err != nil:
		why = append(why, "The mail relay could not be read.")
	case relay == nil:
		why = append(why, "There is no mail relay set up.")
	default:
		r := mail.Relay{
			Host:        relay.Host,
			Port:        relay.Port,
			Implicit:    relay.Security == store.SecurityImplicit,
			Username:    relay.Username,
			Password:    relay.Password,
			FromAddress: relay.FromAddress,
			FromName:    relay.FromName,
		}
		// One message each: a recipient sees nobody else's address, and one refused leaves the
		// rest sent.
		for _, recipient := range to {
			msg.To = recipient
			if err := s.send(r, msg); err != nil {
				why = append(why, recipient+": "+err.Error())
			}
		}
	}
	if len(why) > 0 {
		s.log.Warn("webhook not mailed to every recipient", "delivery", id, "event", event, "why", strings.Join(why, "; "))
	} else {
		s.log.Info("webhook mailed", "delivery", id, "event", event, "recipients", len(to))
	}
	if err := s.store.MarkHookMailed(ctx, id, to, strings.Join(why, "; ")); err != nil {
		s.log.Error("a webhook's mailing could not be recorded", "delivery", id, "error", err.Error())
	}
}

// appNamed is the name of the app with a bundle id, as the last sync listed it, "" for none.
func (s *Server) appNamed(ctx context.Context, bundleID string) string {
	if bundleID == "" {
		return ""
	}
	apps, err := s.store.Apps(ctx)
	if err != nil {
		return ""
	}
	for _, a := range apps {
		if a.BundleID == bundleID {
			return a.Name
		}
	}
	return ""
}

// deliveryBody is a delivery as a list shows it.
type deliveryBody struct {
	ID         int64    `json:"id"`
	ReceivedAt int64    `json:"received_at"`
	Via        string   `json:"via"`
	Event      string   `json:"event"`
	Summary    string   `json:"summary"`
	Size       int      `json:"size"`
	ProvenBy   string   `json:"proven_by"`
	ProofError string   `json:"proof_error"`
	MailedAt   *int64   `json:"mailed_at"`
	MailedTo   []string `json:"mailed_to"`
	MailError  string   `json:"mail_error"`
}

func deliveryOf(h store.Hook) deliveryBody {
	d := deliveryBody{
		ID: h.ID, ReceivedAt: h.ReceivedAt.Unix(), Via: h.Via, Event: h.Event, Summary: h.Summary,
		Size: len(h.Body), ProvenBy: h.ProvenBy, ProofError: h.ProofError,
		MailedTo: h.MailedTo, MailError: h.MailError,
	}
	if !h.MailedAt.IsZero() {
		at := h.MailedAt.Unix()
		d.MailedAt = &at
	}
	return d
}

// hookURL is where a sender delivers to.
func (s *Server) hookURL(token string) string {
	return s.cfg.PublicURL.String() + "/hooks/" + token
}

// endpointBody is an address as the list shows it. Its secret never comes back out once it was
// shown on adding it.
type endpointBody struct {
	ID        int64  `json:"id"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"created_at"`
	Count     int    `json:"count"`
	LastAt    *int64 `json:"last_at"`
	SecretSet bool   `json:"secret_set"`
}

func (s *Server) endpointOf(e store.HookEndpoint) endpointBody {
	b := endpointBody{
		ID: e.ID, Label: e.Label, URL: s.hookURL(e.Token), CreatedAt: e.CreatedAt.Unix(),
		Count: e.Count, SecretSet: e.Secret != "",
	}
	if !e.LastAt.IsZero() {
		at := e.LastAt.Unix()
		b.LastAt = &at
	}
	return b
}

// listHooks is the webhook settings: the addresses, everybody mailed and of what, and the kinds
// of event there are to mail.
func (s *Server) listHooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.store.HookEndpoints(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	endpoints := make([]endpointBody, 0, len(rows))
	for _, e := range rows {
		endpoints = append(endpoints, s.endpointOf(e))
	}
	mails, err := s.store.HookMails(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	events, err := s.store.HookEvents(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoints": endpoints, "mails": mails, "events": events, "catalog": hooks.Catalog,
	})
}

type addHookEndpointRequest struct {
	Label  string `json:"label"`
	Secret string `json:"secret"`
}

// addHookEndpoint makes a new address, and answers with its secret: the one time it is shown.
func (s *Server) addHookEndpoint(w http.ResponseWriter, r *http.Request) {
	var req addHookEndpointRequest
	if !decode(w, r, &req) {
		return
	}
	e, err := s.store.AddHookEndpoint(r.Context(), req.Label, req.Secret)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("webhook address added", "endpoint", e.ID, "label", e.Label)
	writeJSON(w, http.StatusCreated, struct {
		endpointBody
		Secret string `json:"secret"`
	}{s.endpointOf(*e), e.Secret})
}

// removeHookEndpoint removes an address. It answers nothing from this moment; what came
// through it is kept.
func (s *Server) removeHookEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		refuse(w, http.StatusBadRequest, CodeInvalid, "An address's id is a number.")
		return
	}
	if err := s.store.RemoveHookEndpoint(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("webhook address removed", "endpoint", id)
	w.WriteHeader(http.StatusNoContent)
}

// listDeliveries is the newest deliveries, for the dashboard.
func (s *Server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.Hooks(r.Context(), 50)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	deliveries := make([]deliveryBody, 0, len(rows))
	for _, h := range rows {
		deliveries = append(deliveries, deliveryOf(h))
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": deliveries})
}

// getHook is one delivery with its body, as it came and opened up.
func (s *Server) getHook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		refuse(w, http.StatusBadRequest, CodeInvalid, "A delivery's id is a number.")
		return
	}
	h, err := s.store.Hook(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		deliveryBody
		ContentType string `json:"content_type"`
		Body        string `json:"body"`
		Decoded     any    `json:"decoded"`
	}{deliveryOf(*h), h.ContentType, strings.ToValidUTF8(string(h.Body), "�"), hooks.Describe(h.Body).Decoded})
}

// addHookMail adds somebody to mail of the kinds of event they choose.
func (s *Server) addHookMail(w http.ResponseWriter, r *http.Request) {
	var req store.HookMail
	if !decode(w, r, &req) {
		return
	}
	m, err := s.store.AddHookMail(r.Context(), req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("webhook mail added", "mail", m.ID, "events", len(m.Events))
	writeJSON(w, http.StatusCreated, m)
}

// putHookMail changes whom an entry mails, and of what.
func (s *Server) putHookMail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req store.HookMail
	if !decode(w, r, &req) {
		return
	}
	req.ID = id
	m, err := s.store.SetHookMail(r.Context(), req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("webhook mail changed", "mail", m.ID, "events", len(m.Events))
	writeJSON(w, http.StatusOK, m)
}

// removeHookMail stops mailing somebody.
func (s *Server) removeHookMail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.RemoveHookMail(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("webhook mail removed", "mail", id)
	w.WriteHeader(http.StatusNoContent)
}

// pathID is the {id} of a path, refused when it is not one.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		refuse(w, http.StatusBadRequest, CodeInvalid, "An id is a number.")
		return 0, false
	}
	return id, true
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// Apple is what testing an address asks of Apple: what sends to it, and to send it something.
// *asc.Client is one; a test stands in for it.
type Apple interface {
	NotificationURLs(ctx context.Context) ([]asc.AppURLs, error)
	Webhooks(ctx context.Context, appID string) ([]asc.Webhook, error)
	PingWebhook(ctx context.Context, webhookID string) error
	TestNotification(ctx context.Context, bundleID string, sandbox bool) (string, error)
}

// Kinds of sender, and what asking one for a test came to.
const (
	senderNotifications = "notifications"
	senderWebhook       = "webhook"

	testAsked = "asked"
	testOff   = "off"
	testNoKey = "no_key"
	testError = "failed"
)

// senderBody is one thing in App Store Connect that sends to an address, and what asking it to
// send a test came to.
type senderBody struct {
	Kind string `json:"kind"`
	App  string `json:"app"`
	// Name is a webhook's, or Production or Sandbox for an app's server notifications.
	Name    string `json:"name"`
	Result  string `json:"result"`
	Message string `json:"message"`
}

// testHookEndpoint finds what in App Store Connect sends to an address, and asks Apple to send
// each a test: a webhook its ping, with the team key; an app's server notifications their test
// notification, with the In-App Purchase key when there is one. What arrives arrives as any
// delivery does; this says only what was found and asked.
func (s *Server) testHookEndpoint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		refuse(w, http.StatusBadRequest, CodeInvalid, "An address's id is a number.")
		return
	}
	e, err := s.store.HookEndpoint(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	address := s.hookURL(e.Token)
	apps, err := s.apple.NotificationURLs(ctx)
	if err != nil {
		refuse(w, http.StatusBadGateway, CodeAppleFailed, asc.Why(err))
		return
	}

	// An app at a time, a few at once: one webhooks list per app is the slow part.
	found := make([][]senderBody, len(apps))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i, app := range apps {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			found[i] = s.testSenders(ctx, app, address)
		})
	}
	wg.Wait()

	senders := []senderBody{}
	for _, f := range found {
		senders = append(senders, f...)
	}
	s.log.Info("webhook address tested", "endpoint", e.ID, "senders", len(senders))
	writeJSON(w, http.StatusOK, map[string]any{"senders": senders})
}

// testSenders asks for a test of whatever of one app sends to address.
func (s *Server) testSenders(ctx context.Context, app asc.AppURLs, address string) []senderBody {
	var out []senderBody
	for _, env := range []struct {
		name    string
		url     string
		sandbox bool
	}{{"Production", app.Production, false}, {"Sandbox", app.Sandbox, true}} {
		if !sameURL(env.url, address) {
			continue
		}
		b := senderBody{Kind: senderNotifications, App: app.Name, Name: env.name, Result: testAsked}
		_, err := s.apple.TestNotification(ctx, app.BundleID, env.sandbox)
		switch {
		case errors.Is(err, asc.ErrNoIAPKey):
			b.Result = testNoKey
		case err != nil:
			b.Result, b.Message = testError, err.Error()
		}
		out = append(out, b)
	}

	hooks, err := s.apple.Webhooks(ctx, app.ID)
	if err != nil {
		return append(out, senderBody{Kind: senderWebhook, App: app.Name, Result: testError, Message: "Its webhooks could not be read. " + asc.Why(err)})
	}
	for _, h := range hooks {
		if !sameURL(h.URL, address) {
			continue
		}
		b := senderBody{Kind: senderWebhook, App: app.Name, Name: h.Name, Result: testAsked}
		switch {
		case !h.Enabled:
			b.Result = testOff
		default:
			if err := s.apple.PingWebhook(ctx, h.ID); err != nil {
				b.Result, b.Message = testError, asc.Why(err)
			}
		}
		out = append(out, b)
	}
	return out
}

// sameURL compares addresses as a server sees them: the scheme and host in any case, and a
// trailing slash or none.
func sameURL(a, b string) bool {
	ua, errA := url.Parse(strings.TrimSpace(a))
	ub, errB := url.Parse(strings.TrimSpace(b))
	if errA != nil || errB != nil || a == "" || b == "" {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host) &&
		strings.TrimSuffix(ua.Path, "/") == strings.TrimSuffix(ub.Path, "/")
}
