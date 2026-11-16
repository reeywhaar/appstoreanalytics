package api

import (
	"net/http"
	"regexp"
	"strings"

	"appstoreanalytics/internal/mail"
	"appstoreanalytics/internal/session"
	"appstoreanalytics/internal/store"
)

// relayBody is the relay as the form sees it. The password never comes back out; the form is
// told only whether there is one.
type relayBody struct {
	Configured  bool   `json:"configured"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"`
	Username    string `json:"username"`
	PasswordSet bool   `json:"password_set"`
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
}

func (s *Server) getRelay(w http.ResponseWriter, r *http.Request) {
	relay, err := s.store.Relay(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if relay == nil {
		writeJSON(w, http.StatusOK, relayBody{Port: 587, Security: store.SecurityStartTLS})
		return
	}
	writeJSON(w, http.StatusOK, relayBody{
		Configured:  true,
		Host:        relay.Host,
		Port:        relay.Port,
		Security:    relay.Security,
		Username:    relay.Username,
		PasswordSet: relay.Password != "",
		FromAddress: relay.FromAddress,
		FromName:    relay.FromName,
	})
}

type putRelayRequest struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
}

func (s *Server) putRelay(w http.ResponseWriter, r *http.Request) {
	var req putRelayRequest
	if !decode(w, r, &req) {
		return
	}
	err := s.store.SetRelay(r.Context(), store.Relay{
		Host:        req.Host,
		Port:        req.Port,
		Security:    req.Security,
		Username:    req.Username,
		Password:    req.Password,
		FromAddress: req.FromAddress,
		FromName:    req.FromName,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("mail relay set")
	s.getRelay(w, r)
}

func (s *Server) deleteRelay(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteRelay(r.Context()); err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("mail relay forgotten")
	w.WriteHeader(http.StatusNoContent)
}

type testRelayRequest struct {
	To string `json:"to"`
}

// testRelay sends one message through the saved relay and answers with what happened.
//
// A refusal is a 502 carrying the relay's own words, not a 500: everything on this side worked,
// and "the host was wrong", "the credentials were rejected" and "the certificate did not verify"
// are three different afternoons.
func (s *Server) testRelay(w http.ResponseWriter, r *http.Request) {
	var req testRelayRequest
	if !decode(w, r, &req) {
		return
	}
	relay, err := s.relayFor(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = mail.Send(*relay, mail.Message{
		To:      strings.TrimSpace(req.To),
		Subject: "App Store analytics test",
		Body:    "This is App Store analytics, checking that it can hand a message to your relay.\n",
	})
	if err != nil {
		refuse(w, http.StatusBadGateway, CodeRelayFailed, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// relayFor is the saved relay as the sender takes it.
func (s *Server) relayFor(r *http.Request) (*mail.Relay, error) {
	relay, err := s.store.Relay(r.Context())
	if err != nil {
		return nil, err
	}
	if relay == nil {
		return nil, store.Invalid("There is no mail relay set up.")
	}
	return &mail.Relay{
		Host:        relay.Host,
		Port:        relay.Port,
		Implicit:    relay.Security == store.SecurityImplicit,
		Username:    relay.Username,
		Password:    relay.Password,
		FromAddress: relay.FromAddress,
		FromName:    relay.FromName,
	}, nil
}

// sessionBody is one sign-in, as the person who made it sees it.
//
// Everything here is descriptive and nothing decides anything, which is what makes it
// acceptable to show a name a browser chose for itself: the question this screen asks is "do I
// recognise this", and a person judges that better than any check would.
type sessionBody struct {
	ID      string `json:"id"`
	Current bool   `json:"current"`
	// CreatedAt and LastSeenAt answer different questions, and together they separate a
	// session quietly alive for a week from one that started an hour ago.
	CreatedAt  int64 `json:"created_at"`
	LastSeenAt int64 `json:"last_seen_at"`
	ExpiresAt  int64 `json:"expires_at"`
	// Device is a guess and UserAgent is what it was guessed from. Both, because the only
	// honest thing to do with a guess is show its working.
	Device    string `json:"device"`
	UserAgent string `json:"user_agent"`
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.Sessions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	current := currentSessionID(r)
	out := make([]sessionBody, 0, len(rows))
	for _, sess := range rows {
		out = append(out, sessionBody{
			ID:         sess.ID,
			Current:    sess.ID == current,
			CreatedAt:  sess.CreatedAt.Unix(),
			LastSeenAt: sess.LastSeenAt.Unix(),
			ExpiresAt:  sess.ExpiresAt.Unix(),
			Device:     describeAgent(sess.UserAgent),
			UserAgent:  sess.UserAgent,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// revokeSession signs one session out, the current one included.
//
// Signing out the browser you are reading from is a coherent thing to want, and it lands you
// on the sign-in page. Refusing would only move the capability to a button called something else.
func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.RevokeSession(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	if id == currentSessionID(r) {
		// The cookie names a row that is gone. Cleared here, the page that follows is already
		// signed out rather than signed out one request later.
		s.sessions.Clear(w)
	}
	s.log.Info("session signed out", "session", id)
	w.WriteHeader(http.StatusNoContent)
}

// revokeOtherSessions signs out every session but this one.
func (s *Server) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.RevokeOtherSessions(r.Context(), session.Token(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("other sessions signed out", "count", n)
	w.WriteHeader(http.StatusNoContent)
}

// currentSessionID names the session this request arrived on.
func currentSessionID(r *http.Request) string {
	if token := session.Token(r); token != "" {
		return store.SessionID(token)
	}
	return ""
}

// describeAgent summarises a user agent as a browser and a platform.
//
// A guess, shown beside the string it was guessed from. Order matters: every browser claims to
// be several others, so the most specific claim is tested first.
func describeAgent(ua string) string {
	if ua == "" {
		return ""
	}
	browser := ""
	for _, c := range []struct{ match, name string }{
		{"Edg/", "Edge"},
		{"OPR/", "Opera"},
		{"Firefox/", "Firefox"},
		{"Chrome/", "Chrome"},
		{"Safari/", "Safari"},
		{"curl/", "curl"},
	} {
		if strings.Contains(ua, c.match) {
			browser = c.name
			break
		}
	}
	platform := ""
	for _, c := range []struct{ match, name string }{
		{"iPhone", "iPhone"},
		{"iPad", "iPad"},
		{"Android", "Android"},
		{"Mac OS X", "a Mac"},
		{"Windows", "Windows"},
		{"Linux", "Linux"},
	} {
		if strings.Contains(ua, c.match) {
			platform = c.name
			break
		}
	}
	switch {
	case browser != "" && platform != "":
		return browser + " on " + platform
	case browser != "":
		return browser
	case platform != "":
		return platform
	}
	return firstToken(ua)
}

var tokenRe = regexp.MustCompile(`^[^ /]+`)

func firstToken(ua string) string { return tokenRe.FindString(ua) }
