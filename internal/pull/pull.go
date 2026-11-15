// Package pull reads App Store Connect into the database, hourly and when asked.
//
// A pass reads the apps, then whatever days of sales reports are missing, then whatever
// analytics report instances have not been read. Each part keeps going where the last stopped,
// so a pass that is cut short — by a restart, or by Apple asking to slow down — loses nothing
// but time. See docs/app-store-connect.md.
package pull

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/store"
)

// Every is how often a pass runs on its own.
const Every = time.Hour

// Puller runs passes, one at a time.
type Puller struct {
	st     *store.Store
	asc    *asc.Client
	vendor string
	log    *slog.Logger
	// rates reads what a dollar buys; a field so a test can stand in for the web.
	rates   func(context.Context) (map[string]float64, error)
	now     func() time.Time
	wake    chan struct{}
	running atomic.Bool
	// reading is the part under way, "" between passes.
	reading atomic.Value
}

// New makes a puller for one account.
func New(st *store.Store, client *asc.Client, vendor string, log *slog.Logger) *Puller {
	return &Puller{
		st:     st,
		asc:    client,
		vendor: vendor,
		log:    log,
		rates:  fetchRates,
		now:    time.Now,
		wake:   make(chan struct{}, 1),
	}
}

// Run passes until ctx is done: one now, one every hour, and one whenever Wake asks.
func (p *Puller) Run(ctx context.Context) {
	tick := time.NewTicker(Every)
	defer tick.Stop()
	for {
		p.Pass(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-p.wake:
		}
	}
}

// Wake asks for a pass now. It reports false when one is running already, which is the pass
// that was asked for.
func (p *Puller) Wake() bool {
	if p.running.Load() {
		return false
	}
	select {
	case p.wake <- struct{}{}:
	default:
	}
	return true
}

// Running reports whether a pass is under way.
func (p *Puller) Running() bool { return p.running.Load() }

// Reading is the part of the pass under way, such as store.PartSales, and "" when none is.
func (p *Puller) Reading() string {
	s, _ := p.reading.Load().(string)
	return s
}

// Pass reads everything that is missing.
func (p *Puller) Pass(ctx context.Context) {
	if !p.running.CompareAndSwap(false, true) {
		return
	}
	defer p.running.Store(false)
	defer p.reading.Store("")
	started := time.Now()
	p.log.Info("sync started")

	var apps []store.App
	for _, part := range []struct {
		name string
		run  func() error
	}{
		{store.PartApps, func() (err error) { apps, err = p.apps(ctx); return err }},
		{store.PartRates, func() error { return p.currencies(ctx) }},
		{store.PartSales, func() error { return p.sales(ctx) }},
		{store.PartAnalytics, func() error { return p.analytics(ctx, apps) }},
	} {
		p.reading.Store(part.name)
		err := part.run()
		if ctx.Err() != nil {
			p.log.Info("sync stopped: the server is stopping", "part", part.name)
			return
		}
		p.record(ctx, part.name, err)
	}

	p.log.Info("sync finished", "took", time.Since(started).Round(time.Millisecond))
}

// record keeps how a part went, in the words the status page shows.
//
// Not for a part cut short by the server stopping, which neither went through nor failed, and
// whose last outcome stands until the next pass reads it again.
func (p *Puller) record(ctx context.Context, part string, err error) {
	why := ""
	if err != nil {
		why = reason(err)
		p.log.Warn("sync part did not go through", "part", part, "why", why, "error", err.Error())
	}
	if err := p.st.SetSyncState(ctx, part, why); err != nil {
		p.log.Error("sync state could not be kept", "part", part, "error", err.Error())
	}
}

// failure is an error with the sentence a person is shown instead of its own.
type failure struct {
	why string
	err error
}

func (f *failure) Error() string { return f.err.Error() }
func (f *failure) Unwrap() error { return f.err }

func explain(why string, err error) error { return &failure{why: why, err: err} }

func reason(err error) string {
	var f *failure
	if errors.As(err, &f) {
		return f.why
	}
	return asc.Why(err)
}

// apps refreshes the list of apps, and returns every app known, listed or not.
func (p *Puller) apps(ctx context.Context) ([]store.App, error) {
	listed, err := p.asc.Apps(ctx)
	if err == nil {
		saved := make([]store.App, 0, len(listed))
		for _, a := range listed {
			saved = append(saved, store.App{ID: a.ID, Name: a.Name, BundleID: a.BundleID, SKU: a.SKU})
		}
		err = p.st.SaveApps(ctx, saved)
	}
	known, kerr := p.st.Apps(ctx)
	if kerr != nil {
		return nil, kerr
	}
	return known, err
}

// slowDown is whether Apple asked for fewer requests, which ends a part for this pass.
func slowDown(err error) bool { return asc.StatusOf(err) == http.StatusTooManyRequests }
