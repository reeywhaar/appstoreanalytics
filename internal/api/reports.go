package api

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"appstoreanalytics/internal/pull"
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
// With no to, a range ends on the last day whose sales report has been read, so the days shown
// are whole days rather than ending in one Apple has not reported yet.
func (s *Server) spanOf(w http.ResponseWriter, r *http.Request) (span, bool) {
	q := r.URL.Query()
	var sp span
	if sp.App = q.Get("app"); sp.App != "" {
		if !appID.MatchString(sp.App) {
			refuse(w, http.StatusBadRequest, CodeInvalid, "app is an app's Apple ID, which is a number.")
			return sp, false
		}
		ok, err := s.store.AppExists(r.Context(), sp.App)
		if err != nil {
			s.fail(w, r, err)
			return sp, false
		}
		if !ok {
			refuse(w, http.StatusNotFound, CodeNotFound, "There is no app with that Apple ID here.")
			return sp, false
		}
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

// lastDay is the last day whose sales report has been read, or yesterday in Apple's zone when
// none has.
func (s *Server) lastDay(r *http.Request) (time.Time, error) {
	_, last, _, err := s.store.SalesSpan(r.Context())
	if err != nil {
		return time.Time{}, err
	}
	if last != "" {
		return time.Parse(time.DateOnly, last)
	}
	y, m, d := time.Now().In(pull.Pacific).AddDate(0, 0, -1).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), nil
}

func (s *Server) sales(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.spanOf(w, r)
	if !ok {
		return
	}
	sales, err := s.store.SalesIn(r.Context(), sp.From, sp.To, sp.Before, sp.App)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		span
		*store.Sales
	}{sp, sales})
}

func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	sp, ok := s.spanOf(w, r)
	if !ok {
		return
	}
	measured, err := s.store.AnalyticsIn(r.Context(), sp.From, sp.To, sp.Before, sp.App)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		span
		*store.Analytics
	}{sp, measured})
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
	salesFirst, salesLast, salesDays, err := s.store.SalesSpan(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	metricsFirst, metricsLast, err := s.store.MetricsSpan(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	instances, err := s.store.InstanceCount(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	apps, err := s.store.Apps(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var ratesAt *int64
	if at, err := s.store.RatesAt(ctx); err == nil && !at.IsZero() {
		v := at.Unix()
		ratesAt = &v
	}
	var reading string
	if s.pull != nil {
		reading = s.pull.Reading()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running": s.pull != nil && s.pull.Running(),
		"reading": reading,
		"apps":    map[string]any{"count": len(apps), "part": partOf(states, store.PartApps)},
		"sales": map[string]any{
			"first": salesFirst, "last": salesLast, "days": salesDays,
			"part": partOf(states, store.PartSales),
		},
		"analytics": map[string]any{
			"first": metricsFirst, "last": metricsLast, "instances": instances,
			"part": partOf(states, store.PartAnalytics),
		},
		"rates": map[string]any{"at": ratesAt, "part": partOf(states, store.PartRates)},
	})
}

// sync starts a pass now, unless one is running, which is the pass asked for.
func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	started := s.pull != nil && s.pull.Wake()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": started})
}
