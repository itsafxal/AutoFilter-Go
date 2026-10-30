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

		// 2. High-speed performance indexes (drop narrow date_saved and redundant idx_media_file_name in favor of composite idx_media_name_size)
		`DROP INDEX IF EXISTS idx_date_saved;`,
		`DROP INDEX IF EXISTS idx_media_file_name;`,
		`CREATE INDEX IF NOT EXISTS idx_media_date_name ON media (date_saved DESC, file_name);`,
		`CREATE INDEX IF NOT EXISTS idx_media_name_size ON media (file_name, file_size);`,

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

		// 7. Global Filters table for instant case-insensitive exact matching
		`CREATE TABLE IF NOT EXISTS global_filters (
			id TEXT PRIMARY KEY,
			keyword TEXT NOT NULL,
			keyword_norm TEXT NOT NULL UNIQUE,
			reply_text TEXT,
			media_type TEXT NOT NULL,
			file_id TEXT,
			file_ref TEXT,
			buttons_json TEXT,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS gfilter_keyword_norm_idx ON global_filters (keyword_norm);`,

		// 8. Named Batch Metadata table for high-speed local indexing and search
		`CREATE TABLE IF NOT EXISTS batch_metadata (
			batch_id TEXT PRIMARY KEY,
			batch_name TEXT NOT NULL,
			normalized_name TEXT NOT NULL,
			owner_id INTEGER NOT NULL,
			total_items INTEGER NOT NULL DEFAULT 0,
			total_files INTEGER NOT NULL DEFAULT 0,
			total_texts INTEGER NOT NULL DEFAULT 0,
			total_documents INTEGER NOT NULL DEFAULT 0,
			total_videos INTEGER NOT NULL DEFAULT 0,
			total_audio INTEGER NOT NULL DEFAULT 0,
			total_photos INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			finalized_at TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS batch_idx_norm_name ON batch_metadata (normalized_name);`,
		`CREATE INDEX IF NOT EXISTS batch_idx_owner ON batch_metadata (owner_id);`,
		`CREATE INDEX IF NOT EXISTS batch_idx_created ON batch_metadata (created_at DESC);`,

		// 9. Named Batch Items table with strict sequential ordering and unique constraint
		`CREATE TABLE IF NOT EXISTS batch_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			item_type TEXT NOT NULL,
			telegram_file_id TEXT,
			telegram_file_unique_id TEXT,
			storage_reference TEXT,
			file_name TEXT,
			file_size INTEGER DEFAULT 0,
			mime_type TEXT,
			caption TEXT,
			text_content TEXT,
			message_entities TEXT,
			created_at TIMESTAMP NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS batch_idx_items_batch_id ON batch_items (batch_id);`,
		`CREATE INDEX IF NOT EXISTS batch_idx_items_seq ON batch_items (batch_id, sequence ASC);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_batch_item_seq ON batch_items (batch_id, sequence);`,
	}

	for _, query := range queries {
		if _, err := m.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed executing schema query %q: %w", query, err)
		}
	}

	// Verify and populate FTS5 index only if media table has records and media_fts is empty
	var dummy int
	errFts := m.db.QueryRowContext(ctx, "SELECT 1 FROM media_fts LIMIT 1;").Scan(&dummy)
	if errFts != nil {
		// media_fts has no rows yet; populate only if media table has existing rows
		var mediaCount int
		rowMedia := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media;")
		if errMedia := rowMedia.Scan(&mediaCount); errMedia == nil && mediaCount > 0 {
			if m.logger != nil {
				m.logger.Info("populating initial FTS5 trigram search index for existing records...", "total_records", mediaCount)
			}
			_, _ = m.db.ExecContext(ctx, "INSERT INTO media_fts(media_fts) VALUES('rebuild');")
		}
	}

	return nil
}
