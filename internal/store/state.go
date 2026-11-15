package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// The parts of a sync, each with a state of its own.
const (
	PartApps      = "apps"
	PartAnalytics = "analytics"
	PartRates     = "rates"
	PartSales     = "sales"
)

// SyncState is how a part of the sync last went.
type SyncState struct {
	RanAt time.Time
	// OKAt is when it last went through, and zero when it never has.
	OKAt time.Time
	// Error is why the last run did not go through, in words for whoever reads it, and "" when
	// it did.
	Error string
}

// SetSyncState records a run of a part. why is "" when it went through.
func (s *Store) SetSyncState(ctx context.Context, part, why string) error {
	now := unix(s.Now())
	var ok any
	if why == "" {
		ok = now
	}
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO sync_state (part, ran_at, ok_at, error) VALUES (?, ?, ?, ?)
		 ON CONFLICT (part) DO UPDATE SET ran_at = excluded.ran_at,
		   ok_at = COALESCE(excluded.ok_at, sync_state.ok_at), error = excluded.error`,
		part, now, ok, why)
	return err
}

// SyncStates is every part that has run.
func (s *Store) SyncStates(ctx context.Context) (map[string]SyncState, error) {
	rows, err := s.reader.QueryContext(ctx, `SELECT part, ran_at, ok_at, error FROM sync_state`)
	if err != nil {
		return nil, fmt.Errorf("sync states: %w", err)
	}
	defer rows.Close()
	out := map[string]SyncState{}
	for rows.Next() {
		var (
			part  string
			ran   int64
			ok    sql.NullInt64
			state SyncState
		)
		if err := rows.Scan(&part, &ran, &ok, &state.Error); err != nil {
			return nil, err
		}
		state.RanAt = fromUnix(ran)
		if ok.Valid {
			state.OKAt = fromUnix(ok.Int64)
		}
		out[part] = state
	}
	return out, rows.Err()
}
