package store

import (
	"context"
	"database/sql"
	"fmt"
)

// App is one of the account's apps, named by Apple's id for it.
type App struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	BundleID string `json:"bundle_id"`
	// SKU is what a sales report's in-app purchase names its app by.
	SKU string `json:"-"`
}

// SaveApps records the apps App Store Connect lists. One it no longer lists is kept: its
// reports are still here, and still its.
func (s *Store) SaveApps(ctx context.Context, apps []App) error {
	now := unix(s.Now())
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, a := range apps {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO apps (id, name, bundle_id, sku, updated_at) VALUES (?, ?, ?, ?, ?)
				 ON CONFLICT (id) DO UPDATE SET name = excluded.name, bundle_id = excluded.bundle_id,
				   sku = excluded.sku, updated_at = excluded.updated_at`,
				a.ID, a.Name, a.BundleID, a.SKU, now)
			if err != nil {
				return fmt.Errorf("save app %s: %w", a.ID, err)
			}
		}
		return nil
	})
}

// Apps is every app App Store Connect has listed, by name.
func (s *Store) Apps(ctx context.Context) ([]App, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT id, name, bundle_id, sku FROM apps ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("apps: %w", err)
	}
	defer rows.Close()
	out := []App{}
	for rows.Next() {
		var a App
		if err := rows.Scan(&a.ID, &a.Name, &a.BundleID, &a.SKU); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AppExists reports whether id names an app known here.
func (s *Store) AppExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.reader.QueryRowContext(ctx, `SELECT COUNT(*) FROM apps WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}
