package sql

import (
	"context"
	"fmt"
)

// initSchema initializes the SQLite tables, indexes, and synchronization metadata.
func (m *DBManager) initSchema(ctx context.Context) error {
	queries := []string{
		// 1. Authoritative media cache table matching TGxCyberBot schema
		`CREATE TABLE IF NOT EXISTS media (
			file_id TEXT PRIMARY KEY,
			file_ref TEXT,
			file_name TEXT,
			file_size INTEGER,
			size TEXT,
			file_type TEXT,
			mime_type TEXT,
			caption TEXT,
			date_saved TIMESTAMP
		);`,

		// 2. Performance indexes
		`CREATE INDEX IF NOT EXISTS idx_media_file_name ON media (file_name);`,
		`CREATE INDEX IF NOT EXISTS idx_media_caption ON media (caption);`,
		`CREATE INDEX IF NOT EXISTS idx_date_saved ON media (date_saved);`,
		`CREATE INDEX IF NOT EXISTS idx_media_date_name ON media (date_saved DESC, file_name);`,

		// 3. Synchronization state metadata table
		`CREATE TABLE IF NOT EXISTS sync_metadata (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at TIMESTAMP
		);`,
	}

	for _, query := range queries {
		if _, err := m.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed executing schema query %q: %w", query, err)
		}
	}

	return nil
}
