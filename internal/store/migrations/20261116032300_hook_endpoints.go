package migrations

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Addresses webhooks are delivered to, as many as there are senders: each its own label, URL
// token and secret, rather than one of each in meta. A delivery says which it came through, by
// id and by the label it had then, so it still says so once the address is removed.
//
// The one address there was, and its secret, become the first of the list.
var hookEndpoints = Migration{
	Name: "20261116032300_hook_endpoints",
	Up: func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
CREATE TABLE hook_endpoints (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  label      TEXT NOT NULL,
  token      TEXT NOT NULL UNIQUE,
  -- What App Store Connect signs with; '' only for the address carried over from before.
  secret     TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
ALTER TABLE hooks ADD COLUMN endpoint_id INTEGER;
ALTER TABLE hooks ADD COLUMN via TEXT NOT NULL DEFAULT '';
CREATE INDEX hooks_endpoint ON hooks (endpoint_id);
`); err != nil {
			return err
		}

		var token, secret string
		err := tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'hook_token'`).Scan(&token)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if token == "" {
			return nil
		}
		err = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'hook_secret'`).Scan(&secret)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		const label = "Webhooks"
		res, err := tx.ExecContext(ctx,
			`INSERT INTO hook_endpoints (label, token, secret, created_at) VALUES (?, ?, ?, ?)`,
			label, token, secret, time.Now().UTC().Unix())
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hooks SET endpoint_id = ?, via = ?`, id, label); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM meta WHERE key IN ('hook_token', 'hook_secret')`)
		return err
	},
}
