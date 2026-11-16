// Package api is the HTTP surface: handlers, the guard, and static serving.
//
// It takes an fs.FS, so tests drive an fstest.MapFS and `go test ./...` passes with no
// frontend build present.
package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"appstoreanalytics/internal/app"
	"appstoreanalytics/internal/config"
	"appstoreanalytics/internal/mail"
	"appstoreanalytics/internal/session"
	"appstoreanalytics/internal/store"
)

// bodyMax bounds a JSON request.
const bodyMax = 1 << 20

// Puller is the sync, as far as a request can ask anything of it.
type Puller interface {
	Wake() bool
	Running() bool
	Reading() string
}

// Server holds what every handler needs.
//
// Rate limiters belong on it, not at package level: two instances in one process would
// otherwise share one budget.
type Server struct {
	cfg      *config.Config
	log      *slog.Logger
	store    *store.Store
	sessions *session.Manager
	spa      *SPA
	pull     Puller
	mux      *http.ServeMux

	loginAll  *limiter
	loginUser *limiter
	// hookLimit bounds deliveries, which anybody holding the URL can send.
	hookLimit *limiter

	// send hands a message to a relay, and check signs in to one without; fields so a test can
	// stand in for a relay.
	send  func(mail.Relay, mail.Message) error
	check func(mail.Relay) error
	// mailing is the deliveries being mailed, which a test waits for.
	mailing sync.WaitGroup
}

// New wires the routes. Authorization is decided at registration, so a handler cannot forget
// to check: one registered without its guard is visibly registered without it.
func New(cfg *config.Config, log *slog.Logger, st *store.Store, spa *SPA, pull Puller) *Server {
	s := &Server{
		cfg:      cfg,
		log:      log,
		store:    st,
		sessions: session.New(st, cfg.Secure),
		spa:      spa,
		pull:     pull,
		mux:      http.NewServeMux(),

		loginAll:  newLimiter(30, 2*time.Second),
		loginUser: newLimiter(5, 20*time.Second),
		hookLimit: newLimiter(60, time.Second),
		send:      mail.Send,
		check:     mail.Check,
	}

	s.mux.HandleFunc("GET /healthz", s.healthz)

	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.Handle("POST /api/auth/logout", s.requireSession(s.logout))
	s.mux.Handle("GET /api/auth/me", s.requireSession(s.me))

	s.mux.Handle("GET /api/apps", s.requireSession(s.apps))
	s.mux.Handle("GET /api/report", s.requireSession(s.report))
	s.mux.Handle("GET /api/history", s.requireSession(s.history))
	s.mux.Handle("GET /api/status", s.requireSession(s.status))

	s.mux.Handle("GET /api/settings/relay", s.requireSession(s.getRelay))
	s.mux.Handle("PUT /api/settings/relay", s.requireSession(s.putRelay))
	s.mux.Handle("DELETE /api/settings/relay", s.requireSession(s.deleteRelay))
	s.mux.Handle("POST /api/settings/relay/test", s.requireSession(s.testRelay))
	s.mux.Handle("POST /api/settings/relay/check", s.requireSession(s.checkRelay))

	s.mux.Handle("GET /api/hooks", s.requireSession(s.listHooks))
	s.mux.Handle("GET /api/hooks/deliveries", s.requireSession(s.listDeliveries))
	s.mux.Handle("GET /api/hooks/{id}", s.requireSession(s.getHook))
	s.mux.Handle("POST /api/hooks/endpoints", s.requireSession(s.addHookEndpoint))
	s.mux.Handle("DELETE /api/hooks/endpoints/{id}", s.requireSession(s.removeHookEndpoint))
	s.mux.Handle("PUT /api/hooks/mail", s.requireSession(s.putHookMail))

	// Public: a sender has no session, and is let in by the secret in the URL.
	s.mux.HandleFunc("POST /hooks/{token}", s.receiveHook)

	s.mux.Handle("GET /api/sessions", s.requireSession(s.listSessions))
	s.mux.Handle("DELETE /api/sessions/{id}", s.requireSession(s.revokeSession))
	s.mux.Handle("DELETE /api/sessions", s.requireSession(s.revokeOtherSessions))
	s.mux.Handle("POST /api/sync", s.requireSession(s.sync))

	// Catch-all, so a mistyped API path never falls through to the SPA and reaches a fetch as
	// an HTML document it cannot parse.
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		refuse(w, http.StatusNotFound, CodeNotFound, "There is no such endpoint. docs/api.md lists them all.")
	})

	s.mux.Handle("/", s.gate(spa))
	return s
}

// gate decides who gets which document, before the bundle loads.
//
// A signed-out visitor handed the application shell sees the whole interface draw and then
// every panel in it fail. The answer has to be here: the island cannot ask before it has loaded.
//
// Only shells are gated. A file is served to anybody, which is what lets the sign-in page load
// its own stylesheet.
func (s *Server) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sh := s.spa.shellFor(r.URL.Path)
		if sh == nil {
			next.ServeHTTP(w, r)
			return
		}
		_, err := s.sessions.Resolve(r.Context(), w, r)
		switch {
		case err != nil && !sh.public:
			redirect(w, r, signInPath)
		case err == nil && r.URL.Path == signInPath:
			redirect(w, r, "/")
		default:
			next.ServeHTTP(w, r)
		}
	})
}

const signInPath = "/login"

// redirect sends a navigation elsewhere, uncached, or a browser would keep redirecting after
// the sign-in that fixed it.
func redirect(w http.ResponseWriter, r *http.Request, to string) {
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, to, http.StatusFound)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(context.WithValue(r.Context(), ctxStart, time.Now()))
	sv := &served{ResponseWriter: w}
	defer s.logRequest(r, sv)
	s.guard(s.mux).ServeHTTP(sv, r)
}

// served is what a request's line in the log says: the status it was answered with.
type served struct {
	http.ResponseWriter
	status int
}

func (sv *served) WriteHeader(code int) {
	if sv.status == 0 {
		sv.status = code
	}
	sv.ResponseWriter.WriteHeader(code)
}

func (sv *served) Write(b []byte) (int, error) {
	if sv.status == 0 {
		sv.status = http.StatusOK
	}
	return sv.ResponseWriter.Write(b)
}

func (sv *served) Unwrap() http.ResponseWriter { return sv.ResponseWriter }

// logRequest says what was asked and how it was answered. Not the query, and not the bundle's
// files or the health check, which say nothing anybody looks for.
func (s *Server) logRequest(r *http.Request, sv *served) {
	path := r.URL.Path
	page := s.spa != nil && s.spa.shellFor(path) != nil
	if path == "/healthz" || (!strings.HasPrefix(path, "/api/") && !page) {
		return
	}
	status := cmp.Or(sv.status, http.StatusOK)
	level := slog.LevelInfo
	if status >= 500 {
		level = slog.LevelWarn
	}
	s.log.Log(r.Context(), level, "request answered",
		"method", r.Method, "path", path, "status", status, "took", elapsed(r))
}

type ctxKey int

const ctxStart ctxKey = iota

func elapsed(r *http.Request) time.Duration {
	if at, ok := r.Context().Value(ctxStart).(time.Time); ok {
		return time.Since(at).Round(100 * time.Microsecond)
	}
	return 0
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": app.Version})
}

// guard is the CSRF defense: SameSite on the cookie, Sec-Fetch-Site, and a declared content
// type. It leans on this origin being the only one the browser talks to, which a test pins by
// asserting no Access-Control-Allow-Origin is ever emitted.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		// A webhook is sent by a server, in whatever it sends, and the secret in its URL is what
		// lets it in.
		if strings.HasPrefix(r.URL.Path, "/hooks/") {
			next.ServeHTTP(w, r)
			return
		}
		// A browser sets this; a script cannot forge it.
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			refuse(w, http.StatusForbidden, CodeForbidden, "That request came from somewhere else.")
			return
		}
		// Only when a body is present: a write with nothing to say carries none.
		if r.ContentLength != 0 && !isJSON(r.Header.Get("Content-Type")) {
			refuse(w, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "Send application/json.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// decode reads a JSON body into v, refusing anything it does not recognize: a caller's typo'd
// field is a refusal saying so rather than an ignored intention.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	// MaxBytesReader rather than LimitReader: a limit that cuts the body short silently reads as
	// JSON that ends early. Hitting this limit says so.
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var over *http.MaxBytesError
		if errors.As(err, &over) {
			refuse(w, http.StatusRequestEntityTooLarge, CodeBodyTooLarge, fmt.Sprintf("That request body is over %d MB.", bodyMax>>20))
			return false
		}
		refuse(w, http.StatusBadRequest, CodeInvalid, "That request body is not the JSON this expects: "+err.Error())
		return false
	}
	return true
}

// fail maps a store error onto a status and a code, in the one place that does.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		refuse(w, http.StatusNotFound, CodeNotFound, sentence(err, "There is no such thing."))
	case errors.Is(err, store.ErrConflict):
		refuse(w, http.StatusConflict, CodeConflict, sentence(err, "That already exists."))
	case errors.Is(err, store.ErrInvalid):
		refuse(w, http.StatusBadRequest, CodeInvalid, sentence(err, "That request is not valid."))
	case errors.Is(err, context.Canceled):
		// The caller left, or the process is stopping: nobody reads an answer.
	default:
		// An unclassified error is a bug, and the message that reaches the caller deliberately
		// does not say what it was.
		s.log.Error("request failed with an error nothing here explains", "method", r.Method, "path", r.URL.Path, "error", err.Error())
		refuse(w, http.StatusInternalServerError, CodeInternal, "Something went wrong here.")
	}
}

// sentence is the error's own text when it carries one written for a reader.
func sentence(err error, fallback string) string {
	switch msg := err.Error(); msg {
	case "", store.ErrNotFound.Error(), store.ErrConflict.Error(), store.ErrInvalid.Error():
		return fallback
	default:
		return msg
	}
}

func isJSON(ct string) bool {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.TrimSpace(ct) == "application/json"
}
