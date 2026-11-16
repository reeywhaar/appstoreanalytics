package migrations

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Who is mailed of what, as many as there are people to tell, each with the kinds of event they
// want, rather than one of each in meta. A delivery says whom it was mailed to.
//
// The one there was becomes the first of the list.
var hookMails = Migration{
	Name: "20261116042948_hook_mails",
	Up: func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
CREATE TABLE hook_mails (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  to_address TEXT NOT NULL,
  -- A JSON array of kinds of event; a type covers its subtypes.
  events     TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
ALTER TABLE hooks ADD COLUMN mailed_to TEXT NOT NULL DEFAULT '';
`); err != nil {
			return err
		}

		read := func(key string) (string, error) {
			var v string
			err := tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
			if errors.Is(err, sql.ErrNoRows) {
				return "", nil
			}
			return v, err
		}
		to, err := read("hook_mail_to")
		if err != nil {
			return err
		}
		events, err := read("hook_mail_events")
		if err != nil {
			return err
		}
		if to != "" && events != "" && events != "[]" {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO hook_mails (to_address, events, created_at) VALUES (?, ?, ?)`,
				to, events, time.Now().UTC().Unix()); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM meta WHERE key IN ('hook_mail_to', 'hook_mail_events')`)
		return err
	},
}
