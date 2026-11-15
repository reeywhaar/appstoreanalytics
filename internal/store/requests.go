package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Request is a report request made of Apple for an app.
type Request struct {
	ID         string
	AppID      string
	AccessType string
	Stopped    bool
}

// SaveRequests records report requests as Apple listed them. One seen before keeps when it was
// first seen.
func (s *Store) SaveRequests(ctx context.Context, requests []Request) error {
	now := unix(s.Now())
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, r := range requests {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO report_requests (id, app_id, access_type, stopped, since) VALUES (?, ?, ?, ?, ?)
				 ON CONFLICT (id) DO UPDATE SET stopped = excluded.stopped`,
				r.ID, r.AppID, r.AccessType, r.Stopped, now)
			if err != nil {
				return fmt.Errorf("save report request %s: %w", r.ID, err)
			}
		}
		return nil
	})
}

// SaveDelivered records what a request has delivered: how many instances of the reports read
// Apple lists for it, and the latest processing date among them.
func (s *Store) SaveDelivered(ctx context.Context, requestID string, delivered int, latest string) error {
	_, err := s.writer.ExecContext(ctx,
		`UPDATE report_requests SET delivered = ?, latest = ? WHERE id = ?`, delivered, latest, requestID)
	return err
}

// AppRequests is where an app's analytics reports stand.
type AppRequests struct {
	AppID string `json:"app_id"`
	Name  string `json:"name"`
	// Ongoing is "running", "stopped" when Apple stopped every ongoing request, or "" when
	// there is none.
	Ongoing  string `json:"ongoing"`
	Snapshot bool   `json:"snapshot"`
	// Since is the earliest request's, in Unix seconds.
	Since     int64  `json:"since"`
	Delivered int    `json:"delivered"`
	Latest    string `json:"latest"`
	Read      int    `json:"read"`
}

// RequestsByApp is each app's report requests, summed up, by app name.
func (s *Store) RequestsByApp(ctx context.Context) ([]AppRequests, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT r.app_id, COALESCE(a.name, p.title, r.app_id),
		   MAX(r.access_type = 'ONGOING' AND r.stopped = 0),
		   MAX(r.access_type = 'ONGOING'),
		   MAX(r.access_type = 'ONE_TIME_SNAPSHOT'),
		   MIN(r.since), SUM(r.delivered), MAX(r.latest),
		   (SELECT COUNT(*) FROM analytics_instances i WHERE i.app_id = r.app_id)
		 FROM report_requests r
		 LEFT JOIN apps a ON a.id = r.app_id
		 LEFT JOIN products p ON p.id = r.app_id
		 GROUP BY r.app_id
		 ORDER BY 2 COLLATE NOCASE, 1`)
	if err != nil {
		return nil, fmt.Errorf("requests by app: %w", err)
	}
	defer rows.Close()
	out := []AppRequests{}
	for rows.Next() {
		var (
			a                       AppRequests
			running, ongoing, snaps bool
		)
		if err := rows.Scan(&a.AppID, &a.Name, &running, &ongoing, &snaps, &a.Since, &a.Delivered, &a.Latest, &a.Read); err != nil {
			return nil, err
		}
		switch {
		case running:
			a.Ongoing = "running"
		case ongoing:
			a.Ongoing = "stopped"
		}
		a.Snapshot = snaps
		out = append(out, a)
	}
	return out, rows.Err()
}
