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

		// 2. High-speed performance indexes (drop narrow date_saved index to avoid bad query planner choices)
		`DROP INDEX IF EXISTS idx_date_saved;`,
		`CREATE INDEX IF NOT EXISTS idx_media_file_name ON media (file_name);`,
		`CREATE INDEX IF NOT EXISTS idx_media_date_name ON media (date_saved DESC, file_name);`,

		// 3. Ultra-fast Full-Text Search (FTS5 Trigram) for instant substring queries
		`CREATE VIRTUAL TABLE IF NOT EXISTS media_fts USING fts5(
			file_name,
			content='media',
			content_rowid='rowid',
			tokenize='trigram'
		);`,

		// 4. Triggers to keep media_fts synchronized with media table writes
		`CREATE TRIGGER IF NOT EXISTS media_ai AFTER INSERT ON media BEGIN
			INSERT INTO media_fts(rowid, file_name) VALUES (new.rowid, new.file_name);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS media_ad AFTER DELETE ON media BEGIN
			INSERT INTO media_fts(media_fts, rowid, file_name) VALUES('delete', old.rowid, old.file_name);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS media_au AFTER UPDATE ON media BEGIN
			INSERT INTO media_fts(media_fts, rowid, file_name) VALUES('delete', old.rowid, old.file_name);
			INSERT INTO media_fts(rowid, file_name) VALUES (new.rowid, new.file_name);
		END;`,

		// 3. Synchronization state metadata table
		`CREATE TABLE IF NOT EXISTS sync_metadata (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at TIMESTAMP
		);`,

		// 4. Scheduled Deletions table (persists across bot restarts)
		`CREATE TABLE IF NOT EXISTS scheduled_deletions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			message_id INTEGER NOT NULL,
			msg_type TEXT NOT NULL,
			delete_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS sched_del_time_idx ON scheduled_deletions (delete_at);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS sched_del_chat_msg_idx ON scheduled_deletions (chat_id, message_id);`,

		// 5. Sent Messages table (tracks all bot messages for Weekly Trash cleanup)
		`CREATE TABLE IF NOT EXISTS sent_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			message_id INTEGER NOT NULL,
			msg_type TEXT NOT NULL,
			sent_at TIMESTAMP NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS sent_msg_time_idx ON sent_messages (sent_at);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS sent_msg_chat_msg_idx ON sent_messages (chat_id, message_id);`,

		// 6. Auto-delete runtime settings table
		`CREATE TABLE IF NOT EXISTS autodelete_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);`,
	}

	for _, query := range queries {
		if _, err := m.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed executing schema query %q: %w", query, err)
		}
	}

	// Verify and populate FTS5 index if media table has existing records
	var ftsCount int
	row := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_fts;")
	if err := row.Scan(&ftsCount); err != nil || ftsCount == 0 {
		var mediaCount int
		rowMedia := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media;")
		if errMedia := rowMedia.Scan(&mediaCount); errMedia == nil && mediaCount > 0 {
			if m.logger != nil {
				m.logger.Info("populating FTS5 trigram search index for existing records...", "total_records", mediaCount)
			}
			_, _ = m.db.ExecContext(ctx, "INSERT INTO media_fts(media_fts) VALUES('rebuild');")
		}
	}

	return nil
}
