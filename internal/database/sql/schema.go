package sql

import (
	"context"
	"fmt"
)

// initSchema initializes the SQLite tables, indexes, FTS5 virtual table, and synchronization metadata.
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

		// 3. High-speed FTS5 virtual table for sub-millisecond substring/trigram searches
		`CREATE VIRTUAL TABLE IF NOT EXISTS media_fts USING fts5(
			file_name,
			caption,
			content='media',
			content_rowid='rowid',
			tokenize='trigram'
		);`,

		// 4. Auto-sync triggers for FTS5
		`CREATE TRIGGER IF NOT EXISTS media_ai AFTER INSERT ON media BEGIN
			INSERT INTO media_fts(rowid, file_name, caption) VALUES (new.rowid, new.file_name, new.caption);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS media_ad AFTER DELETE ON media BEGIN
			INSERT INTO media_fts(media_fts, rowid, file_name, caption) VALUES('delete', old.rowid, old.file_name, old.caption);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS media_au AFTER UPDATE ON media BEGIN
			INSERT INTO media_fts(media_fts, rowid, file_name, caption) VALUES('delete', old.rowid, old.file_name, old.caption);
			INSERT INTO media_fts(rowid, file_name, caption) VALUES (new.rowid, new.file_name, new.caption);
		END;`,

		// 5. Synchronization state metadata table
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

	// Check if FTS needs initial rebuild from existing media rows
	var mCount, fCount int64
	_ = m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media;").Scan(&mCount)
	if mCount > 0 {
		_ = m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_fts;").Scan(&fCount)
		if fCount == 0 {
			m.logger.Info("rebuilding FTS5 trigram index for existing records...", "records", mCount)
			_, _ = m.db.ExecContext(ctx, "INSERT INTO media_fts(media_fts) VALUES('rebuild');")
			m.logger.Info("FTS5 trigram index rebuild complete")
		}
	}

	return nil
}
