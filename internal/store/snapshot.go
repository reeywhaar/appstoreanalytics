package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// SnapshotTo writes a copy of the database to path: one moment of it, whole, as one file.
//
// VACUUM INTO on a reader connection, which sees one moment and blocks nobody, so a sync keeps
// writing while a large copy is made; on the one writer, every write would wait for it.
func (s *Store) SnapshotTo(ctx context.Context, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// VACUUM INTO refuses to overwrite.
	os.Remove(path)
	if _, err := s.reader.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	return nil
}
