package pull

import (
	"context"
	"net/http"
	"sort"
	"time"

	"appstoreanalytics/internal/asc"
	"appstoreanalytics/internal/store"
)

// analytics reads every report instance not read yet, of every app, asking Apple to make the
// reports first where nobody has: an ongoing request for every day from now, and a snapshot
// for the history before it.
func (p *Puller) analytics(ctx context.Context, apps []store.App) error {
	read, err := p.st.InstancesRead(ctx)
	if err != nil {
		return err
	}
	var first error
	var instances int
	for _, app := range apps {
		requests, err := p.requests(ctx, app.ID)
		if asc.StatusOf(err) == http.StatusForbidden {
			return explain("Asking Apple for analytics reports takes an API key with the Admin role: sales are read, analytics are not.", err)
		}
		if slowDown(err) || ctx.Err() != nil {
			return err
		}
		if err != nil {
			first = keep(first, err)
			continue
		}
		n, err := p.readApp(ctx, app.ID, requests, read)
		instances += n
		if slowDown(err) || ctx.Err() != nil {
			return err
		}
		first = keep(first, err)
	}
	if instances > 0 {
		p.log.Info("analytics reports read", "instances", instances)
	}
	return first
}

func keep(first, err error) error {
	if first == nil {
		return err
	}
	return first
}

// requests is an app's report requests, after asking for whichever of the two is missing.
// An ongoing request Apple stopped for want of reading is asked for again.
func (p *Puller) requests(ctx context.Context, appID string) ([]asc.ReportRequest, error) {
	requests, err := p.asc.ReportRequests(ctx, appID)
	if err != nil {
		return nil, err
	}
	var ongoing, snapshot bool
	for _, r := range requests {
		switch r.AccessType {
		case asc.Ongoing:
			ongoing = ongoing || !r.Stopped
		case asc.Snapshot:
			snapshot = true
		}
	}
	for kind, has := range map[string]bool{asc.Ongoing: ongoing, asc.Snapshot: snapshot} {
		if has {
			continue
		}
		r, err := p.asc.RequestReports(ctx, appID, kind)
		if asc.StatusOf(err) == http.StatusConflict {
			continue
		}
		if err != nil {
			return nil, err
		}
		p.log.Info("analytics reports asked for", "app", appID, "access", kind)
		requests = append(requests, r)
	}
	kept := make([]store.Request, 0, len(requests))
	for _, r := range requests {
		kept = append(kept, store.Request{ID: r.ID, AppID: appID, AccessType: r.AccessType, Stopped: r.Stopped})
	}
	return requests, p.st.SaveRequests(ctx, kept)
}

// emptyFor is how long a snapshot may go without delivering anything before it is taken as one
// whose reports are gone. Apple delivers the first within a day or two, and deletes instances
// after 35 days, so a week of nothing is a snapshot that has nothing left to give: one made
// before the database was, most likely.
const emptyFor = 7 * 24 * time.Hour

// renew asks for history again in place of a snapshot that has delivered nothing for a week.
//
// Apple allows a snapshot about once a month, deleted ones included. When it refuses, there is
// no snapshot left, and every pass asks again until it allows one.
func (p *Puller) renew(ctx context.Context, appID string, old asc.ReportRequest) error {
	stale, err := p.st.Undelivered(ctx, old.ID, p.st.Now().Add(-emptyFor))
	if err != nil || !stale {
		return err
	}
	if err := p.asc.DeleteRequest(ctx, old.ID); err != nil && asc.StatusOf(err) != http.StatusNotFound {
		return err
	}
	if err := p.st.ForgetRequest(ctx, old.ID); err != nil {
		return err
	}
	made, err := p.asc.RequestReports(ctx, appID, asc.Snapshot)
	if asc.StatusOf(err) == http.StatusConflict {
		p.log.Info("analytics history asked for again, and Apple wants to wait: its snapshots are monthly", "app", appID)
		return nil
	}
	if err != nil {
		return err
	}
	p.log.Info("analytics history asked for again: the snapshot had delivered nothing in a week", "app", appID)
	return p.st.SaveRequests(ctx, []store.Request{{ID: made.ID, AppID: appID, AccessType: asc.Snapshot}})
}

// unread is an instance to read, and the report it is of.
type unread struct {
	report   string
	instance asc.ReportInstance
}

// readApp reads the instances of an app's reports that are not read yet, from every request,
// and says how many it read.
//
// Oldest first: an instance processed later corrects the days an earlier one said, and is the
// one that has to be read last.
func (p *Puller) readApp(ctx context.Context, appID string, requests []asc.ReportRequest, read map[string]bool) (int, error) {
	var todo []unread
	for _, r := range requests {
		reports, err := p.asc.Reports(ctx, r.ID)
		if err != nil {
			return 0, err
		}
		var delivered int
		var latest string
		for _, report := range reports {
			if _, ok := asc.ReadingOf(report.Name); !ok {
				continue
			}
			instances, err := p.asc.Instances(ctx, report.ID)
			if err != nil {
				return 0, err
			}
			for _, inst := range instances {
				delivered++
				latest = max(latest, inst.ProcessingDate)
				if !read[inst.ID] {
					todo = append(todo, unread{report.Name, inst})
				}
			}
		}
		if err := p.st.SaveDelivered(ctx, r.ID, delivered, latest); err != nil {
			return 0, err
		}
		if r.AccessType == asc.Snapshot && delivered == 0 {
			if err := p.renew(ctx, appID, r); err != nil {
				return 0, err
			}
		}
	}
	sort.SliceStable(todo, func(i, j int) bool {
		return todo[i].instance.ProcessingDate < todo[j].instance.ProcessingDate
	})
	for n, u := range todo {
		reading, _ := asc.ReadingOf(u.report)
		measured, err := p.readInstance(ctx, appID, u.report, u.instance.ID)
		if err != nil {
			return n, err
		}
		err = p.st.SaveInstance(ctx, store.Instance{
			ID: u.instance.ID, AppID: appID, Report: u.report, ProcessingDate: u.instance.ProcessingDate,
		}, reading.Yields, measured)
		if err != nil {
			return n, err
		}
		read[u.instance.ID] = true
	}
	return len(todo), nil
}

// readInstance sums every file of an instance by day, measure, territory and source.
func (p *Puller) readInstance(ctx context.Context, appID, report, instanceID string) ([]store.Measured, error) {
	segments, err := p.asc.Segments(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	type key struct{ day, metric, territory, source string }
	sums := map[key]int64{}
	var order []key
	for _, address := range segments {
		body, err := p.asc.Segment(ctx, address)
		if err != nil {
			return nil, err
		}
		measures, err := asc.ParseReport(report, body)
		body.Close()
		if err != nil {
			return nil, err
		}
		for _, m := range measures {
			k := key{m.Day, m.Metric, m.Territory, m.Source}
			if _, ok := sums[k]; !ok {
				order = append(order, k)
			}
			sums[k] += m.Value
		}
	}
	out := make([]store.Measured, 0, len(order))
	for _, k := range order {
		out = append(out, store.Measured{AppID: appID, Day: k.day, Metric: k.metric, Territory: k.territory, Source: k.source, Value: sums[k]})
	}
	return out, nil
}
