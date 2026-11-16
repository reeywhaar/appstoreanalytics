package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	m, err := s.store.HookMail(ctx)
	if err == nil && proof.Proven() && m.Mails(d.Event) {
		bg := context.WithoutCancel(ctx)
		s.mailing.Go(func() { s.mailHook(bg, id, d, body, m.To) })
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// mailHook hands one delivery to the relay, and records how that went on the delivery.
func (s *Server) mailHook(ctx context.Context, id int64, d hooks.Delivery, body []byte, to string) {
	why := ""
	relay, err := s.store.Relay(ctx)
	switch {
	case err != nil:
		why = "The mail relay could not be read."
	case relay == nil:
		why = "There is no mail relay set up."
	default:
		err = s.send(mail.Relay{
			Host:        relay.Host,
			Port:        relay.Port,
			Implicit:    relay.Security == store.SecurityImplicit,
			Username:    relay.Username,
			Password:    relay.Password,
			FromAddress: relay.FromAddress,
			FromName:    relay.FromName,
		}, mail.Message{
			To:      to,
			Subject: "App Store: " + d.Summary,
			Body:    hookLetter(d, body),
		})
		if err != nil {
			why = err.Error()
		}
	}
	if why != "" {
		s.log.Warn("webhook not mailed", "delivery", id, "event", d.Event, "why", why)
	} else {
		s.log.Info("webhook mailed", "delivery", id, "event", d.Event)
	}
	if err := s.store.MarkHookMailed(ctx, id, why); err != nil {
		s.log.Error("a webhook's mailing could not be recorded", "delivery", id, "error", err.Error())
	}
}

// hookLetter is what a mailed delivery says: its line, then its body opened up, or as it came.
func hookLetter(d hooks.Delivery, body []byte) string {
	var b strings.Builder
	b.WriteString(d.Summary + "\n")
	b.WriteString("Received " + time.Now().UTC().Format("2006-01-02 15:04 MST") + "\n\n")
	if d.Decoded != nil {
		pretty, err := json.MarshalIndent(d.Decoded, "", "  ")
		if err == nil {
			b.Write(pretty)
			b.WriteString("\n")
			return b.String()
		}
	}
	b.WriteString(strings.ToValidUTF8(string(body), "�"))
	return b.String()
}

// deliveryBody is a delivery as a list shows it.
type deliveryBody struct {
	ID         int64  `json:"id"`
	ReceivedAt int64  `json:"received_at"`
	Via        string `json:"via"`
	Event      string `json:"event"`
	Summary    string `json:"summary"`
	Size       int    `json:"size"`
	ProvenBy   string `json:"proven_by"`
	ProofError string `json:"proof_error"`
	MailedAt   *int64 `json:"mailed_at"`
	MailError  string `json:"mail_error"`
}

func deliveryOf(h store.Hook) deliveryBody {
	d := deliveryBody{
		ID: h.ID, ReceivedAt: h.ReceivedAt.Unix(), Via: h.Via, Event: h.Event, Summary: h.Summary,
		Size: len(h.Body), ProvenBy: h.ProvenBy, ProofError: h.ProofError, MailError: h.MailError,
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

// listHooks is the webhook settings: the addresses, who is mailed of what, and the kinds of
// event there are to mail.
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
	m, err := s.store.HookMail(ctx)
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
		"endpoints": endpoints, "mail": m, "events": events, "catalog": hooks.Catalog,
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

func (s *Server) putHookMail(w http.ResponseWriter, r *http.Request) {
	var req store.HookMail
	if !decode(w, r, &req) {
		return
	}
	if err := s.store.SetHookMail(r.Context(), req); err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := s.store.HookMail(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
