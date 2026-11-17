package api

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"appstoreanalytics/internal/store"
)

func (s *Server) apps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.Apps(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

// span is the days a report is asked for, and the as many days before them that it is
// compared with.
type span struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Before string `json:"previous_from"`
	App    string `json:"app"`
}

// maxDays is the longest range asked for at once: two years, which is more than Apple keeps.
const maxDays = 731

var appID = regexp.MustCompile(`^[0-9]{1,20}$`)

// spanOf reads from, to, days and app off the query.
//
// With no to, a range ends on the last day downloads were measured, so the days shown
// are whole days rather than ending in one Apple has not reported yet.
func (s *Server) spanOf(w http.ResponseWriter, r *http.Request) (span, bool) {
	q := r.URL.Query()
	var (
		sp span
		ok bool
	)
	if sp.App, ok = s.appOf(w, r); !ok {
		return sp, false
	}

	to, err := s.dayParam(q.Get("to"), "to")
	if err != nil {
		refuse(w, http.StatusBadRequest, CodeInvalid, err.Error())
		return sp, false
	}
	if to.IsZero() {
		if to, err = s.lastDay(r); err != nil {
			s.fail(w, r, err)
			return sp, false
		}
	}
	from, err := s.dayParam(q.Get("from"), "from")
	if err != nil {
		refuse(w, http.StatusBadRequest, CodeInvalid, err.Error())
		return sp, false
	}
	if from.IsZero() {
		days := 30
		if v := q.Get("days"); v != "" {
			if days, err = strconv.Atoi(v); err != nil || days < 1 || days > maxDays {
				refuse(w, http.StatusBadRequest, CodeInvalid, "days is a number of days, from 1 to 731.")
				return sp, false
			}
		}
		from = to.AddDate(0, 0, -(days - 1))
	}
	length := int(to.Sub(from).Hours()/24) + 1
	if length < 1 || length > maxDays {
		refuse(w, http.StatusBadRequest, CodeInvalid, "from has to be on or before to, and at most 731 days before it.")
		return sp, false
	}
	sp.From = from.Format(time.DateOnly)
	sp.To = to.Format(time.DateOnly)
	sp.Before = from.AddDate(0, 0, -length).Format(time.DateOnly)
	return sp, true
}

// appOf reads app off the query: an app's Apple ID, or "" for every app.
func (s *Server) appOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	app := r.URL.Query().Get("app")
	if app == "" {
		return "", true
	}
	if !appID.MatchString(app) {
		refuse(w, http.StatusBadRequest, CodeInvalid, "app is an app's Apple ID, which is a number.")
		return "", false
	}
	ok, err := s.store.AppExists(r.Context(), app)
	if err != nil {
		s.fail(w, r, err)
		return "", false
	}
	if !ok {
		refuse(w, http.StatusNotFound, CodeNotFound, "There is no app with that Apple ID here.")
		return "", false
	}
	return app, true
}

// historyChunk is how many days a history answers with at once: a year, the most a range is, so
// the first answer fills any chart, and the years before come as somebody scrolls back to them.
const historyChunk = 365

// history is the days measured, a year at a time, for charts that scroll back through them:
// the year up to the day a range ends on when none is asked for, or the year before `before`.
// `first` is the first day anything was measured, for knowing when there is nothing earlier.
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appOf(w, r)
	if !ok {
		return
	}
	to, err := s.lastDay(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if v := r.URL.Query().Get("before"); v != "" {
		before, err := time.Parse(time.DateOnly, v)
		if err != nil {
			refuse(w, http.StatusBadRequest, CodeInvalid, "before is a day, written like 2026-03-04.")
			return
		}
		if before.AddDate(0, 0, -1).Before(to) {
			to = before.AddDate(0, 0, -1)
		}
	}
	first, err := s.store.FirstDay(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from := to.AddDate(0, 0, -(historyChunk - 1)).Format(time.DateOnly)
	if first > from {
		from = first
	}
	end := to.Format(time.DateOnly)
	days := []map[string]any{}
	// Every app's own days, for the charts to tell the apps apart, when it is every app's history.
	byApp := map[string]map[string]map[string]any{}
	if first != "" && from <= end {
		if days, err = s.store.DaysIn(r.Context(), from, end, app); err != nil {
			s.fail(w, r, err)
			return
		}
		if app == "" {
			if byApp, err = s.store.DaysByApp(r.Context(), from, end); err != nil {
				s.fail(w, r, err)
				return
			}
		}
	} else {
		from = end
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from": from, "to": end, "first": first, "app": app, "days": days, "by_app": byApp,
	})
}

func (s *Server) dayParam(v, name string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return time.Time{}, store.Invalid("%s is a day, written like 2026-03-04.", name)
	}
	return t, nil
}

// pacific is the zone Apple's days are in.
var pacific = func() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return loc
}()

// lastDay is the last day downloads were measured: the reports of a day arrive over a day or
// two, and downloads are the ones a range is read for. Yesterday in Apple's zone when nothing
// has been measured yet.
func (s *Server) lastDay(r *http.Request) (time.Time, error) {
	_, last, downloads, err := s.store.MetricsSpan(r.Context())
	if err != nil {
		return time.Time{}, err
	}
	if downloads != "" {
		return time.Parse(time.DateOnly, downloads)
	}
	if last != "" {
		return time.Parse(time.DateOnly, last)
	}
	y, m, d := time.Now().In(pacific).AddDate(0, 0, -1).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), nil
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.spanOf(w, r)
	if !ok {
		return
	}
	report, err := s.store.ReportIn(r.Context(), sp.From, sp.To, sp.Before, sp.App)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Every app's own days of the range, as the history has them, so the charts tell the apps
	// apart from the start rather than once the history comes.
	byApp := map[string]map[string]map[string]any{}
	if sp.App == "" {
		if byApp, err = s.store.DaysByApp(r.Context(), sp.From, sp.To); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, struct {
		span
		*store.Report
		ByApp map[string]map[string]map[string]any `json:"by_app"`
	}{sp, report, byApp})
}

// part is how one part of the sync last went, in Unix seconds.
type part struct {
	RanAt *int64 `json:"ran_at"`
	OKAt  *int64 `json:"ok_at"`
	Error string `json:"error"`
}

func partOf(states map[string]store.SyncState, name string) part {
	st, ok := states[name]
	if !ok {
		return part{}
	}
	p := part{Error: st.Error}
	ran := st.RanAt.Unix()
	p.RanAt = &ran
	if !st.OKAt.IsZero() {
		ok := st.OKAt.Unix()
		p.OKAt = &ok
	}
	return p
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	states, err := s.store.SyncStates(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	first, last, _, err := s.store.MetricsSpan(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	instances, err := s.store.InstanceCount(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	requested, since, err := s.store.Requested(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var sinceUnix *int64
	if !since.IsZero() {
		v := since.Unix()
		sinceUnix = &v
	}
	apps, err := s.store.Apps(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	salesFirst, salesLast, salesDays, err := s.store.SalesSpan(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var ratesAt *int64
	if _, at, err := s.store.Rates(ctx); err == nil && !at.IsZero() {
		v := at.Unix()
		ratesAt = &v
	}
	var reading string
	if s.pull != nil {
		reading = s.pull.Reading()
	}
	requests, err := s.store.RequestsByApp(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running":  s.pull != nil && s.pull.Running(),
		"requests": requests,
		"reading":  reading,
		"apps":     map[string]any{"count": len(apps), "part": partOf(states, store.PartApps)},
		"analytics": map[string]any{
			"first": first, "last": last, "instances": instances,
			"requested_apps": requested, "requested_since": sinceUnix,
			"part": partOf(states, store.PartAnalytics),
		},
		"sales": map[string]any{
			"on": s.cfg.Apple.VendorNumber != "", "first": salesFirst, "last": salesLast, "days": salesDays,
			"part": partOf(states, store.PartSales),
		},
		"rates": map[string]any{"at": ratesAt, "part": partOf(states, store.PartRates)},
	})
}

// sync starts a pass now, unless one is running, which is the pass asked for.
func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	started := s.pull != nil && s.pull.Wake()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": started})
}
