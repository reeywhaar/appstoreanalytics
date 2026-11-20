package main

import (
	"compress/gzip"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// pacific is the zone Apple's report days are in.
var pacific = mustZone("America/Los_Angeles")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// kept is how long Apple keeps a report instance.
const kept = 35

// Server stands in for App Store Connect's API: the apps, analytics report requests and their
// reports, and webhooks, as appstoreanalytics asks for them.
type Server struct {
	Issuer, KeyID string
	Key           *ecdsa.PublicKey
	// Origin is the stand-in's own address, which report files are linked from.
	Origin string
	// DeliverTo is where webhooks are delivered in place of their address's scheme and host:
	// the demo's address is the browser's, not the stand-in's. "" delivers to the address.
	DeliverTo string
	Log       *slog.Logger
	Now       func() time.Time
	// Pause is how long apart a webhook's events go, so they arrive in their order.
	Pause time.Duration

	state *State
}

// State is what the stand-in was asked to make, kept in a file so a run finds it again.
type State struct {
	mu       sync.Mutex
	path     string
	Requests []Request `json:"requests"`
	Webhooks []Webhook `json:"webhooks"`
}

// Request is an analytics report request.
type Request struct {
	ID     string `json:"id"`
	App    string `json:"app"`
	Access string `json:"access"`
	// Made is the report day it was asked for.
	Made string `json:"made"`
}

// Webhook is an app's webhook, as made in App Store Connect.
type Webhook struct {
	ID      string   `json:"id"`
	App     string   `json:"app"`
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Enabled bool     `json:"enabled"`
	Events  []string `json:"events"`
}

// loadState reads the state kept at path, or starts an empty one there.
func loadState(path string) (*State, error) {
	st := &State{path: path}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	return st, json.Unmarshal(raw, st)
}

// save writes the state; the caller holds mu.
func (st *State) save() error {
	if st.path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

// Handler is the API.
func (s *Server) Handler(st *State) http.Handler {
	s.state = st
	mux := http.NewServeMux()
	api := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.signedIn(h)) }
	api("GET /v1/apps", s.apps)
	api("GET /v1/apps/{id}/analyticsReportRequests", s.requests)
	api("POST /v1/analyticsReportRequests", s.request)
	api("DELETE /v1/analyticsReportRequests/{id}", s.deleteRequest)
	api("GET /v1/analyticsReportRequests/{id}/reports", s.reports)
	api("GET /v1/analyticsReports/{id}/instances", s.instances)
	api("GET /v1/analyticsReportInstances/{id}/segments", s.segments)
	api("GET /v1/apps/{id}/webhooks", s.webhooks)
	api("POST /v1/webhooks", s.makeWebhook)
	api("POST /v1/webhookPings", s.ping)
	// A report file's address is signed already, and asked for without the API's token.
	mux.HandleFunc("GET /files/{instance}", s.file)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.Handle("/", s.signedIn(func(w http.ResponseWriter, r *http.Request) {
		refuse(w, http.StatusNotFound, "NOT_FOUND", "The specified resource does not exist", "The path provided does not match a defined resource type.")
	}))
	return mux
}

// signedIn refuses a request whose token App Store Connect would not take.
func (s *Server) signedIn(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		// A token lives by the wall clock, whatever day the reports are of.
		if err := checkToken(token, s.Issuer, s.KeyID, s.Key, time.Now()); err != nil {
			s.Log.Info("token refused", "path", r.URL.Path, "why", err.Error())
			refuse(w, http.StatusUnauthorized, "NOT_AUTHORIZED", "Authentication credentials are missing or invalid.",
				"Provide a properly configured and signed bearer token, and make sure that it has not expired.")
			return
		}
		next(w, r)
	})
}

// refuse answers as App Store Connect does when it will not.
func refuse(w http.ResponseWriter, status int, code, title, detail string) {
	write(w, status, map[string]any{"errors": []map[string]string{{
		"id": newID(), "status": strconv.Itoa(status), "code": code, "title": title, "detail": detail,
	}}})
}

func write(w http.ResponseWriter, status int, doc any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(doc)
}

// resource is one JSON:API resource.
type resource struct {
	Type          string         `json:"type"`
	ID            string         `json:"id"`
	Attributes    map[string]any `json:"attributes,omitempty"`
	Relationships map[string]any `json:"relationships,omitempty"`
}

// page answers a list: everything on one page, as the lists here are short.
func page(w http.ResponseWriter, r *http.Request, data []resource) {
	if data == nil {
		data = []resource{}
	}
	write(w, http.StatusOK, map[string]any{
		"data":  data,
		"links": map[string]string{"self": r.URL.String()},
		"meta":  map[string]any{"paging": map[string]int{"total": len(data), "limit": 200}},
	})
}

func (s *Server) apps(w http.ResponseWriter, r *http.Request) {
	var data []resource
	for _, a := range apps {
		data = append(data, resource{Type: "apps", ID: a.ID, Attributes: map[string]any{
			"name": a.Name, "bundleId": a.Bundle, "sku": a.SKU, "primaryLocale": "en-US",
			"subscriptionStatusUrl": nil, "subscriptionStatusUrlForSandbox": nil,
		}})
	}
	page(w, r, data)
}

func requestResource(q Request) resource {
	return resource{Type: "analyticsReportRequests", ID: q.ID, Attributes: map[string]any{
		"accessType": q.Access, "stoppedDueToInactivity": false,
	}}
}

func (s *Server) requests(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if appByID(id) == nil {
		refuse(w, http.StatusNotFound, "NOT_FOUND", "The specified resource does not exist", "There is no resource of type 'apps' with id '"+id+"'")
		return
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	var data []resource
	for _, q := range s.state.Requests {
		if q.App == id {
			data = append(data, requestResource(q))
		}
	}
	page(w, r, data)
}

// request makes an analytics report request. An app has one of each kind at a time, as Apple
// allows a snapshot about once a month.
func (s *Server) request(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Data struct {
			Attributes struct {
				AccessType string `json:"accessType"`
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
	app, access := in.Data.Relationships.App.Data.ID, in.Data.Attributes.AccessType
	if appByID(app) == nil {
		refuse(w, http.StatusNotFound, "NOT_FOUND", "The specified resource does not exist", "There is no resource of type 'apps' with id '"+app+"'")
		return
	}
	if access != "ONGOING" && access != "ONE_TIME_SNAPSHOT" {
		refuse(w, http.StatusConflict, "ENTITY_ERROR.ATTRIBUTE.INVALID", "An attribute value is invalid.", "accessType is ONGOING or ONE_TIME_SNAPSHOT")
		return
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	for _, q := range s.state.Requests {
		if q.App == app && q.Access == access {
			refuse(w, http.StatusConflict, "STATE_ERROR", "The request cannot be fulfilled because of the state of another resource.",
				"You already have such an entity")
			return
		}
	}
	q := Request{ID: newID(), App: app, Access: access, Made: s.today().Format(time.DateOnly)}
	s.state.Requests = append(s.state.Requests, q)
	if err := s.state.save(); err != nil {
		s.Log.Error("state not kept", "error", err.Error())
	}
	s.Log.Info("reports asked for", "app", app, "access", access)
	write(w, http.StatusCreated, map[string]any{"data": requestResource(q)})
}

func (s *Server) deleteRequest(w http.ResponseWriter, r *http.Request) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	id := r.PathValue("id")
	i := slices.IndexFunc(s.state.Requests, func(q Request) bool { return q.ID == id })
	if i < 0 {
		refuse(w, http.StatusNotFound, "NOT_FOUND", "The specified resource does not exist", "There is no resource of type 'analyticsReportRequests' with id '"+id+"'")
		return
	}
	s.state.Requests = slices.Delete(s.state.Requests, i, i+1)
	_ = s.state.save()
	w.WriteHeader(http.StatusNoContent)
}

// findRequest is a request by its id.
func (s *Server) findRequest(id string) (Request, bool) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	for _, q := range s.state.Requests {
		if q.ID == id {
			return q, true
		}
	}
	return Request{}, false
}

// A report's id is its request's with the report's place among reports, and an instance's is
// its report's with its processing date, so each says what it is of without a table.
func reportID(request string, i int) string { return "r" + strconv.Itoa(i) + "-" + request }

func parseReportID(id string) (request string, rep *Report, ok bool) {
	n, rest, found := strings.Cut(strings.TrimPrefix(id, "r"), "-")
	i, err := strconv.Atoi(n)
	if !found || err != nil || i < 0 || i >= len(reports) {
		return "", nil, false
	}
	return rest, &reports[i], true
}

func instanceID(report string, processed time.Time) string {
	return report + "-" + processed.Format("20060102")
}

func parseInstanceID(id string) (report string, processed time.Time, ok bool) {
	i := strings.LastIndex(id, "-")
	if i < 0 {
		return "", time.Time{}, false
	}
	t, err := time.Parse("20060102", id[i+1:])
	return id[:i], t, err == nil
}

func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	q, ok := s.findRequest(r.PathValue("id"))
	if !ok {
		refuse(w, http.StatusNotFound, "NOT_FOUND", "The specified resource does not exist", "There is no resource of type 'analyticsReportRequests' with id '"+r.PathValue("id")+"'")
		return
	}
	var data []resource
	for i, rep := range reports {
		category := "APP_USAGE"
		switch {
		case strings.Contains(rep.Name, "Purchases"):
			category = "COMMERCE"
		case strings.Contains(rep.Name, "Discovery"), strings.Contains(rep.Name, "Downloads"), strings.Contains(rep.Name, "Web Preview"):
			category = "APP_STORE_ENGAGEMENT"
		case strings.Contains(rep.Name, "Crashes"):
			category = "FRAMEWORK_USAGE"
		}
		data = append(data, resource{Type: "analyticsReports", ID: reportID(q.ID, i), Attributes: map[string]any{
			"name": rep.Name, "category": category,
		}})
	}
	page(w, r, data)
}

// processed is the processing dates a request's report has instances of, as Apple lists them:
// a snapshot's one, of the day it was asked for, and an ongoing request's every day since, each
// of the day before. None older than Apple keeps.
func (s *Server) processed(q Request) []time.Time {
	made, _ := time.Parse(time.DateOnly, q.Made)
	today := s.today()
	oldest := today.AddDate(0, 0, -kept)
	if q.Access == "ONE_TIME_SNAPSHOT" {
		if made.Before(oldest) {
			return nil
		}
		return []time.Time{made}
	}
	var out []time.Time
	for p := made; p.Before(today); p = p.AddDate(0, 0, 1) {
		if !p.Before(oldest) {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) instances(w http.ResponseWriter, r *http.Request) {
	request, rep, ok := parseReportID(r.PathValue("id"))
	q, found := s.findRequest(request)
	if !ok || !found {
		refuse(w, http.StatusNotFound, "NOT_FOUND", "The specified resource does not exist", "There is no resource of type 'analyticsReports' with id '"+r.PathValue("id")+"'")
		return
	}
	if g := r.URL.Query().Get("filter[granularity]"); rep.Lines == nil || (g != "" && g != "DAILY") {
		page(w, r, nil)
		return
	}
	var data []resource
	for _, p := range s.processed(q) {
		data = append(data, resource{Type: "analyticsReportInstances", ID: instanceID(r.PathValue("id"), p), Attributes: map[string]any{
			"granularity": "DAILY", "processingDate": p.Format(time.DateOnly),
		}})
	}
	page(w, r, data)
}

func (s *Server) segments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	report, _, ok := parseInstanceID(id)
	if ok {
		_, _, ok = parseReportID(report)
	}
	if !ok {
		refuse(w, http.StatusNotFound, "NOT_FOUND", "The specified resource does not exist", "There is no resource of type 'analyticsReportInstances' with id '"+id+"'")
		return
	}
	page(w, r, []resource{{Type: "analyticsReportSegments", ID: id + "-1", Attributes: map[string]any{
		"checksum": "", "sizeInBytes": 0, "url": s.Origin + "/files/" + id + ".tsv.gz",
	}}})
}

// file is an instance's report: a snapshot's of every day before it was asked for, an ongoing
// request's of the day before it was processed.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("instance"), ".tsv.gz")
	report, processed, ok := parseInstanceID(id)
	var (
		request string
		rep     *Report
		q       Request
	)
	if ok {
		request, rep, ok = parseReportID(report)
	}
	if ok {
		q, ok = s.findRequest(request)
	}
	if !ok || rep.Lines == nil {
		http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
		return
	}
	a := appByID(q.App)
	first, last := processed.AddDate(0, 0, -1), processed.AddDate(0, 0, -1)
	if q.Access == "ONE_TIME_SNAPSHOT" {
		first, last = history, processed.AddDate(0, 0, -2)
	}
	w.Header().Set("Content-Type", "application/gzip")
	gz := gzip.NewWriter(w)
	if err := rep.write(gz, a, first, last); err != nil {
		s.Log.Warn("a report file was cut short", "error", err.Error())
		return
	}
	_ = gz.Close()
}

// today is the report day it is in Pacific time.
func (s *Server) today() time.Time {
	y, m, d := s.Now().In(pacific).Date()
	return date(y, m, d)
}

// newID is an id as Apple makes them.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
