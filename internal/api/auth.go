package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"appstoreanalytics/internal/session"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// login checks the username and password the environment names, and issues a cookie.
//
// Two buckets, with two jobs: the global one bounds how fast anybody guesses at all, and the
// per-username one stops a list of passwords worked through against the one username.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if !decode(w, r, &req) {
		return
	}
	if !s.loginAll.allow("") {
		s.refused(r, "sign-in rate limited")
		refuse(w, http.StatusTooManyRequests, CodeRateLimited, "Too many sign-in attempts. Wait a minute.")
		return
	}
	if !s.loginUser.allow(strings.ToLower(req.Username)) {
		s.refused(r, "sign-in rate limited")
		refuse(w, http.StatusTooManyRequests, CodeRateLimited, "Too many sign-in attempts for that username. Wait a minute.")
		return
	}
	// Both compared whatever the first says, so how long a refusal takes says nothing of which
	// one was wrong.
	user := same(strings.ToLower(req.Username), strings.ToLower(s.cfg.Username))
	pass := same(req.Password, s.cfg.Password)
	if !user || !pass {
		s.refused(r, "sign-in refused")
		refuse(w, http.StatusUnauthorized, CodeUnauthenticated, "That username and password do not match.")
		return
	}
	if err := s.sessions.Issue(r.Context(), w, r); err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("signed in")
	w.WriteHeader(http.StatusNoContent)
}

// same compares in constant time, over digests so the length of neither is told either.
func same(a, b string) bool {
	x := sha256.Sum256([]byte(a))
	y := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(x[:], y[:]) == 1
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.sessions.Revoke(r.Context(), w, r); err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("signed out")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"username": s.cfg.Username})
}

// requireSession admits a request that carries a live session, and nothing else.
//
// Never a redirect: a 302 to an HTML page is the least useful thing a fetch can receive. The
// island reads the 401 and navigates itself.
func (s *Server) requireSession(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := s.sessions.Resolve(r.Context(), w, r)
		if errors.Is(err, session.ErrNoSession) {
			refuse(w, http.StatusUnauthorized, CodeUnauthenticated, "Sign in first.")
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
