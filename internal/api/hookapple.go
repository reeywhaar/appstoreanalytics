package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/store"
)

// An app sends to up to three addresses: its App Store Connect webhook, and its App Store Server
// Notifications, for production and for sandbox. Each is connected to one of the addresses by
// hand, so whoever is mailed can be mailed of what comes through one and not another: sandbox
// kept out of the inbox of somebody who wants only what is real.
const (
	partWebhook    = "webhook"
	partProduction = "production"
	partSandbox    = "sandbox"
)

// An app's main automatic address is one for all three of its parts, labelled with its name
// alone: the one Auto connects to unless asked for the part's own, so four apps are four
// addresses rather than twelve.
const (
	autoMain     = "main"
	autoSeparate = "separate"
)

// suffix is what an address made for a part is labelled, after the app's name.
var suffix = map[string]string{
	partWebhook:    "Webhook",
	partProduction: "Production",
	partSandbox:    "Sandbox",
}

// webhookName is what a webhook made here is called in App Store Connect, under its app.
const webhookName = "App Store analytics"

// How a part stands.
const (
	// stateOK is sending to one of the addresses: a webhook turned on and sending every event.
	stateOK = "ok"
	// stateOff and stateSome are a webhook to one of the addresses turned off, or sending only
	// some events.
	stateOff  = "off"
	stateSome = "some"
	// stateNone is sending nowhere: no notification address, or no webhook here.
	stateNone = "none"
	// stateUnknown is sending to an address that is none of these: notifications sent to another
	// server, or a webhook to this instance at an address since removed.
	stateUnknown = "unknown"
)

// eachApp runs f for every app, a few at once: one list of webhooks per app is the slow part.
func eachApp[T any](apps []asc.AppURLs, f func(asc.AppURLs) T) []T {
	out := make([]T, len(apps))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i, app := range apps {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			out[i] = f(app)
		})
	}
	wg.Wait()
	return out
}

// here reports whether an address is one of this instance's: its scheme and host, and a webhook
// path, whether or not an address there is kept now.
func (s *Server) here(address string) bool {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || address == "" {
		return false
	}
	p := s.cfg.PublicURL
	return strings.EqualFold(u.Scheme, p.Scheme) && strings.EqualFold(u.Host, p.Host) && strings.HasPrefix(u.Path, "/hooks/")
}

// ours is which of the addresses a URL is, or nil.
func (s *Server) ours(endpoints []store.HookEndpoint, address string) *store.HookEndpoint {
	for i, e := range endpoints {
		if sameURL(address, s.hookURL(e.Token)) {
			return &endpoints[i]
		}
	}
	return nil
}

// sender is how one of an app's three parts stands: where Apple sends it, and which of the
// addresses that is.
type sender struct {
	State string `json:"state"`
	// URL is where Apple sends it, "" for nowhere.
	URL string `json:"url"`
	// Endpoint is the address it is sent to, or none.
	Endpoint *int64 `json:"endpoint"`
	// Name is the webhook's in App Store Connect.
	Name string `json:"name,omitempty"`
}

// webhookOf is the app's webhook that is its part here, and how it stands: one to the addresses
// at its best; else one to this instance at an address since removed, or the one named as made
// here, sent somewhere else since. Webhooks to anywhere else are somebody else's.
func (s *Server) webhookOf(hooks []asc.Webhook, endpoints []store.HookEndpoint) (*asc.Webhook, sender) {
	var of *asc.Webhook
	out := sender{State: stateNone}
	rank := map[string]int{stateNone: 0, stateUnknown: 1, stateOff: 2, stateSome: 3, stateOK: 4}
	for i, h := range hooks {
		e := s.ours(endpoints, h.URL)
		state := stateNone
		switch {
		case e != nil && h.Enabled && h.Every():
			state = stateOK
		case e != nil && h.Enabled:
			state = stateSome
		case e != nil:
			state = stateOff
		case s.here(h.URL) || h.Name == webhookName:
			state = stateUnknown
		}
		if rank[state] <= rank[out.State] {
			continue
		}
		of, out = &hooks[i], sender{State: state, URL: h.URL, Name: h.Name}
		if e != nil {
			out.Endpoint = &e.ID
		}
	}
	return of, out
}

// notificationsSender is how an app's server notifications for one environment stand.
func (s *Server) notificationsSender(address string, endpoints []store.HookEndpoint) sender {
	out := sender{State: stateUnknown, URL: address}
	switch e := s.ours(endpoints, address); {
	case address == "":
		out.State = stateNone
	case e != nil:
		out.State, out.Endpoint = stateOK, &e.ID
	}
	return out
}

// appleAppBody is an app as App Store Connect has it send, part by part.
type appleAppBody struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Webhook    sender `json:"webhook"`
	Production sender `json:"production"`
	Sandbox    sender `json:"sandbox"`
}

// listApple says, for every app, how its three parts stand.
func (s *Server) listApple(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	endpoints, err := s.store.HookEndpoints(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	apps, err := s.apple.NotificationURLs(ctx)
	if err != nil {
		refuse(w, http.StatusBadGateway, CodeAppleFailed, asc.Why(err))
		return
	}
	type found struct {
		hooks []asc.Webhook
		err   error
	}
	all := eachApp(apps, func(app asc.AppURLs) found {
		hooks, err := s.apple.Webhooks(ctx, app.ID)
		return found{hooks, err}
	})
	out := make([]appleAppBody, 0, len(apps))
	for i, app := range apps {
		if all[i].err != nil {
			refuse(w, http.StatusBadGateway, CodeAppleFailed, fmt.Sprintf("%s's webhooks could not be read. %s", app.Name, asc.Why(all[i].err)))
			return
		}
		_, webhook := s.webhookOf(all[i].hooks, endpoints)
		out = append(out, appleAppBody{
			ID: app.ID, Name: app.Name,
			Webhook:    webhook,
			Production: s.notificationsSender(app.Production, endpoints),
			Sandbox:    s.notificationsSender(app.Sandbox, endpoints),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out})
}

// What connecting a part came to.
const (
	connectMade    = "made"
	connectChanged = "changed"
	connectAlready = "already"
)

// connectRequest is where one of an app's parts is to send: an automatic address, made the first
// time, the app's main one or the part's separate one; one of the addresses, by its id; an
// address typed in, with the secret a webhook to it is signed with; or, with none of these,
// nowhere.
type connectRequest struct {
	Part     string `json:"part"`
	Auto     string `json:"auto"`
	Endpoint *int64 `json:"endpoint"`
	URL      string `json:"url"`
	Secret   string `json:"secret"`
}

// connectApple has one of an app's parts send somewhere else: to one of the addresses, to one
// typed in, or nowhere.
//
// A webhook already sending there is set right if it needs to be: turned on, every event, the
// secret, which also sets right one typed in wrongly. Else the app's webhook here is moved there
// and set right the same way, and with none, one is made. Nowhere deletes the webhook here.
//
// Server notifications are pointed there, replacing whatever was, or at nothing: the page says
// what it replaces before asking, since an address of the app's own server would stop hearing of
// its subscriptions.
func (s *Server) connectApple(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req connectRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Part != partWebhook && req.Part != partProduction && req.Part != partSandbox {
		refuse(w, http.StatusBadRequest, CodeInvalid, "A part is webhook, production or sandbox.")
		return
	}
	if req.Auto != "" && req.Auto != autoMain && req.Auto != autoSeparate {
		refuse(w, http.StatusBadRequest, CodeInvalid, "An automatic address is the app's main one or the part's separate one.")
		return
	}
	address, secret := strings.TrimSpace(req.URL), strings.TrimSpace(req.Secret)
	if n := btoi(req.Auto != "") + btoi(req.Endpoint != nil) + btoi(address != ""); n > 1 {
		refuse(w, http.StatusBadRequest, CodeInvalid, "Send it to its automatic address, one of the addresses or one typed in: one of them.")
		return
	}
	switch {
	case req.Endpoint != nil:
		e, err := s.store.HookEndpoint(ctx, *req.Endpoint)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		address, secret = s.hookURL(e.Token), e.Secret
		if req.Part == partWebhook && secret == "" {
			refuse(w, http.StatusBadRequest, CodeInvalid, "That address has no secret, and a webhook is signed with one: add another address for it.")
			return
		}
	case address != "":
		if u, err := url.Parse(address); err != nil || u.Scheme != "https" || u.Host == "" {
			refuse(w, http.StatusBadRequest, CodeInvalid, "Apple sends only to an https address, such as https://example.com/apple.")
			return
		}
		if req.Part == partWebhook && secret == "" {
			refuse(w, http.StatusBadRequest, CodeInvalid, "A webhook is signed with a secret: type the one its server checks.")
			return
		}
	}
	apps, err := s.apple.NotificationURLs(ctx)
	if err != nil {
		refuse(w, http.StatusBadGateway, CodeAppleFailed, asc.Why(err))
		return
	}
	var app *asc.AppURLs
	for i := range apps {
		if apps[i].ID == r.PathValue("app") {
			app = &apps[i]
		}
	}
	if app == nil {
		refuse(w, http.StatusNotFound, CodeNotFound, "App Store Connect lists no app with that id.")
		return
	}
	var made *store.HookEndpoint
	if req.Auto != "" {
		e, isNew, err := s.automatic(ctx, *app, req.Part, req.Auto == autoMain)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if isNew {
			made = e
		}
		address, secret = s.hookURL(e.Token), e.Secret
	}

	var result string
	if req.Part == partWebhook {
		result, err = s.connectWebhook(ctx, app.ID, address, secret)
	} else {
		now := app.Production
		if req.Part == partSandbox {
			now = app.Sandbox
		}
		result = connectAlready
		if !(now == "" && address == "" || sameURL(now, address)) {
			result = connectChanged
			err = s.apple.SetNotificationURL(ctx, app.ID, req.Part == partSandbox, address)
		}
	}
	if err != nil {
		// An address made for it that nothing sends to is not wanted.
		if made != nil {
			_ = s.store.RemoveHookEndpoint(ctx, made.ID)
		}
		refuse(w, http.StatusBadGateway, CodeAppleFailed, asc.Why(err))
		return
	}
	s.log.Info("app connected", "app", app.ID, "part", req.Part, "to", address, "result", result)
	body := map[string]any{"result": result}
	if made != nil {
		s.log.Info("webhook address added", "endpoint", made.ID, "label", made.Label)
		body["endpoint"] = s.endpointOf(*made)
	}
	writeJSON(w, http.StatusOK, body)
}

// automatic is an automatic address, and whether it was made just now: the app's main one, or
// the part's separate one; the one made for it before, or else one made now. The main one is
// labelled with the app's name, "Dots", and a separate one with both, "Dots: Production"; either
// with a number after when that is taken by another, "Dots 2".
func (s *Server) automatic(ctx context.Context, app asc.AppURLs, part string, main bool) (*store.HookEndpoint, bool, error) {
	endpoints, err := s.store.HookEndpoints(ctx)
	if err != nil {
		return nil, false, err
	}
	of, base := part, app.Name+": "+suffix[part]
	if main {
		of, base = autoMain, app.Name
	}
	for i, e := range endpoints {
		if e.App == app.ID && e.Part == of {
			return &endpoints[i], false, nil
		}
	}
	label := base
	for n := 2; slices.ContainsFunc(endpoints, func(e store.HookEndpoint) bool { return strings.EqualFold(e.Label, label) }); n++ {
		label = fmt.Sprintf("%s %d", base, n)
	}
	e, err := s.store.AddAppEndpoint(ctx, label, app.ID, of)
	return e, err == nil, err
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// connectWebhook leaves, sets right, moves, makes or deletes an app's webhook, as connectApple
// says.
func (s *Server) connectWebhook(ctx context.Context, appID, address, secret string) (string, error) {
	hooks, err := s.apple.Webhooks(ctx, appID)
	if err != nil {
		return "", err
	}
	endpoints, err := s.store.HookEndpoints(ctx)
	if err != nil {
		return "", err
	}
	of, _ := s.webhookOf(hooks, endpoints)
	if address == "" {
		if of == nil {
			return connectAlready, nil
		}
		return connectChanged, s.apple.DeleteWebhook(ctx, of.ID)
	}
	for _, h := range hooks {
		if sameURL(h.URL, address) {
			if h.Enabled && h.Every() && s.ours(endpoints, address) != nil {
				return connectAlready, nil
			}
			return connectChanged, s.apple.MendWebhook(ctx, h.ID, address, secret)
		}
	}
	if of != nil {
		return connectChanged, s.apple.MendWebhook(ctx, of.ID, address, secret)
	}
	_, err = s.apple.MakeWebhook(ctx, appID, webhookName, address, secret)
	return connectMade, err
}

// unhook deletes the webhooks App Store Connect sends to an address being removed, and says what
// it could not: a webhook Apple would not delete, and server notifications still pointed there,
// which are left for a person to point elsewhere.
func (s *Server) unhook(ctx context.Context, address string) []string {
	apps, err := s.apple.NotificationURLs(ctx)
	if err != nil {
		return []string{"App Store Connect could not be asked about its webhooks to it. " + asc.Why(err)}
	}
	left := eachApp(apps, func(app asc.AppURLs) []string {
		var out []string
		for _, env := range []struct{ name, url string }{{partProduction, app.Production}, {partSandbox, app.Sandbox}} {
			if sameURL(env.url, address) {
				out = append(out, fmt.Sprintf("%s's App Store Server Notifications for %s still go to it: connect them to another address.", app.Name, env.name))
			}
		}
		hooks, err := s.apple.Webhooks(ctx, app.ID)
		if err != nil {
			return append(out, fmt.Sprintf("%s's webhooks could not be read. %s", app.Name, asc.Why(err)))
		}
		for _, h := range hooks {
			if !sameURL(h.URL, address) {
				continue
			}
			if err := s.apple.DeleteWebhook(ctx, h.ID); err != nil {
				out = append(out, fmt.Sprintf("%s's webhook “%s” could not be deleted. %s", app.Name, h.Name, asc.Why(err)))
				continue
			}
			s.log.Info("webhook deleted at Apple", "app", app.ID, "webhook", h.ID)
		}
		return out
	})
	var out []string
	for _, l := range left {
		out = append(out, l...)
	}
	return out
}
