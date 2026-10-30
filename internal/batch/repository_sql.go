package batch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SQLProvider is a function that returns the active SQLite *sql.DB instance.
type SQLProvider func() *sql.DB

// SQLRepository manages local SQLite runtime indexing and caching of batches in media.sqlite3.
type SQLRepository struct {
	dbProvider SQLProvider
	staticDB   *sql.DB
}

// NewSQLRepository creates a new SQLRepository with a dynamic dbProvider.
func NewSQLRepository(provider SQLProvider) *SQLRepository {
	return &SQLRepository{dbProvider: provider}
}

// NewSQLRepositoryWithDB creates a new SQLRepository with a static *sql.DB.
func NewSQLRepositoryWithDB(db *sql.DB) *SQLRepository {
	return &SQLRepository{staticDB: db}
}

func (r *SQLRepository) db() *sql.DB {
	if r == nil {
		return nil
	}
	if r.dbProvider != nil {
		return r.dbProvider()
	}
	return r.staticDB
}

// SaveMetadata inserts or replaces batch metadata in SQLite.
func (r *SQLRepository) SaveMetadata(ctx context.Context, meta *BatchMetadata) error {
	if meta == nil || meta.BatchID == "" {
		return errors.New("cannot save nil or empty batch metadata in sqlite")
	}
	d := r.db()
	if d == nil {
		return errors.New("sqlite database unavailable")
	}

	meta.NormalizedName = NormalizeBatchName(meta.BatchName)
	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	query := `
		INSERT OR REPLACE INTO batch_metadata (
			batch_id, batch_name, normalized_name, owner_id,
			total_items, total_files, total_texts, total_documents,
			total_videos, total_audio, total_photos, status,
			created_at, updated_at, finalized_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	_, err := d.ExecContext(ctx, query,
		meta.BatchID,
		meta.BatchName,
		meta.NormalizedName,
		meta.OwnerID,
		meta.TotalItems,
		meta.TotalFiles,
		meta.TotalTexts,
		meta.TotalDocuments,
		meta.TotalVideos,
		meta.TotalAudio,
		meta.TotalPhotos,
		meta.Status,
		meta.CreatedAt,
		meta.UpdatedAt,
		meta.FinalizedAt,
	)
	if err != nil {
		return fmt.Errorf("failed saving batch metadata in sqlite: %w", err)
	}
	return nil
}

// SaveItem inserts or replaces a batch item in SQLite.
func (r *SQLRepository) SaveItem(ctx context.Context, item *BatchItem) error {
	if item == nil || item.BatchID == "" || item.Sequence <= 0 {
		return errors.New("cannot save invalid batch item in sqlite")
	}
	d := r.db()
	if d == nil {
		return errors.New("sqlite database unavailable")
	}

	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT OR REPLACE INTO batch_items (
			batch_id, sequence, item_type, telegram_file_id,
			telegram_file_unique_id, storage_reference, file_name,
			file_size, mime_type, caption, text_content,
			message_entities, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	_, err := d.ExecContext(ctx, query,
		item.BatchID,
		item.Sequence,
		item.ItemType,
		item.TelegramFileID,
		item.TelegramFileUniqueID,
		item.StorageReference,
		item.FileName,
		item.FileSize,
		item.MimeType,
		item.Caption,
		item.TextContent,
		item.MessageEntities,
		item.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed saving batch item in sqlite: %w", err)
	}
	return nil
}

// SaveItems inserts or replaces multiple batch items within an atomic SQLite transaction.
func (r *SQLRepository) SaveItems(ctx context.Context, items []*BatchItem) error {
	if len(items) == 0 {
		return nil
	}
	d := r.db()
	if d == nil {
		return errors.New("sqlite database unavailable")
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed beginning transaction for batch items: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT OR REPLACE INTO batch_items (
			batch_id, sequence, item_type, telegram_file_id,
			telegram_file_unique_id, storage_reference, file_name,
			file_size, mime_type, caption, text_content,
			message_entities, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed preparing statement for batch items: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, it := range items {
		if it == nil || it.BatchID == "" || it.Sequence <= 0 {
			continue
		}
		if it.CreatedAt.IsZero() {
			it.CreatedAt = now
		}
		_, err := stmt.ExecContext(ctx,
			it.BatchID,
			it.Sequence,
			it.ItemType,
			it.TelegramFileID,
			it.TelegramFileUniqueID,
			it.StorageReference,
			it.FileName,
			it.FileSize,
			it.MimeType,
			it.Caption,
			it.TextContent,
			it.MessageEntities,
			it.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed inserting batch item seq %d: %w", it.Sequence, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed committing batch items transaction: %w", err)
	}
	return nil
}

// GetMetadata retrieves batch metadata from SQLite.
func (r *SQLRepository) GetMetadata(ctx context.Context, batchID string) (*BatchMetadata, error) {
	d := r.db()
	if d == nil {
		return nil, errors.New("sqlite database unavailable")
	}

	query := `
		SELECT batch_id, batch_name, normalized_name, owner_id,
		       total_items, total_files, total_texts, total_documents,
		       total_videos, total_audio, total_photos, status,
		       created_at, updated_at, finalized_at
		FROM batch_metadata
		WHERE batch_id = ?
		LIMIT 1;
	`

	var meta BatchMetadata
	var finalizedAt sql.NullTime

	err := d.QueryRowContext(ctx, query, batchID).Scan(
		&meta.BatchID,
		&meta.BatchName,
		&meta.NormalizedName,
		&meta.OwnerID,
		&meta.TotalItems,
		&meta.TotalFiles,
		&meta.TotalTexts,
		&meta.TotalDocuments,
		&meta.TotalVideos,
		&meta.TotalAudio,
		&meta.TotalPhotos,
		&meta.Status,
		&meta.CreatedAt,
		&meta.UpdatedAt,
		&finalizedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed querying batch metadata in sqlite: %w", err)
	}

	if finalizedAt.Valid {
		meta.FinalizedAt = &finalizedAt.Time
	}
	return &meta, nil
}

// GetItems retrieves all items belonging to a batch strictly ordered by sequence ASC.
func (r *SQLRepository) GetItems(ctx context.Context, batchID string) ([]*BatchItem, error) {
	d := r.db()
	if d == nil {
		return nil, errors.New("sqlite database unavailable")
	}

	query := `
		SELECT id, batch_id, sequence, item_type, telegram_file_id,
		       telegram_file_unique_id, storage_reference, file_name,
		       file_size, mime_type, caption, text_content,
		       message_entities, created_at
		FROM batch_items
		WHERE batch_id = ?
		ORDER BY sequence ASC;
	`

	rows, err := d.QueryContext(ctx, query, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed querying batch items from sqlite: %w", err)
	}
	defer rows.Close()

	var items []*BatchItem
	for rows.Next() {
		var it BatchItem
		var tfid, tfuid, sref, fname, mtype, cap, tcont, entities sql.NullString
		err := rows.Scan(
			&it.ID,
			&it.BatchID,
			&it.Sequence,
			&it.ItemType,
			&tfid,
			&tfuid,
			&sref,
			&fname,
			&it.FileSize,
			&mtype,
			&cap,
			&tcont,
			&entities,
			&it.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed scanning batch item: %w", err)
		}
		it.TelegramFileID = tfid.String
		it.TelegramFileUniqueID = tfuid.String
		it.StorageReference = sref.String
		it.FileName = fname.String
		it.MimeType = mtype.String
		it.Caption = cap.String
		it.TextContent = tcont.String
		it.MessageEntities = entities.String
		items = append(items, &it)
	}
	return items, rows.Err()
}

// SearchMetadata performs sub-millisecond case-insensitive exact, normalized, or partial searches.
func (r *SQLRepository) SearchMetadata(ctx context.Context, query string, limit, offset int) ([]*BatchMetadata, int, error) {
	d := r.db()
	if d == nil {
		return nil, 0, errors.New("sqlite database unavailable")
	}

	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	normQuery := NormalizeBatchName(query)
	likePattern := "%" + normQuery + "%"

	countQuery := `
		SELECT COUNT(*)
		FROM batch_metadata
		WHERE status = 'finalized'
		  AND (normalized_name = ? OR normalized_name LIKE ? OR batch_name LIKE ?);
	`
	var total int
	if err := d.QueryRowContext(ctx, countQuery, normQuery, likePattern, "%"+query+"%").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed counting search results: %w", err)
	}

	selectQuery := `
		SELECT batch_id, batch_name, normalized_name, owner_id,
		       total_items, total_files, total_texts, total_documents,
		       total_videos, total_audio, total_photos, status,
		       created_at, updated_at, finalized_at
		FROM batch_metadata
		WHERE status = 'finalized'
		  AND (normalized_name = ? OR normalized_name LIKE ? OR batch_name LIKE ?)
		ORDER BY
			CASE WHEN normalized_name = ? THEN 0
			     WHEN normalized_name LIKE ? THEN 1
			     ELSE 2 END,
			created_at DESC
		LIMIT ? OFFSET ?;
	`

	prefixPattern := normQuery + "%"
	rows, err := d.QueryContext(ctx, selectQuery,
		normQuery, likePattern, "%"+query+"%",
		normQuery, prefixPattern,
		limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed executing batch search query: %w", err)
	}
	defer rows.Close()

	var results []*BatchMetadata
	for rows.Next() {
		var meta BatchMetadata
		var finalizedAt sql.NullTime
		err := rows.Scan(
			&meta.BatchID,
			&meta.BatchName,
			&meta.NormalizedName,
			&meta.OwnerID,
			&meta.TotalItems,
			&meta.TotalFiles,
			&meta.TotalTexts,
			&meta.TotalDocuments,
			&meta.TotalVideos,
			&meta.TotalAudio,
			&meta.TotalPhotos,
			&meta.Status,
			&meta.CreatedAt,
			&meta.UpdatedAt,
			&finalizedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed scanning search result row: %w", err)
		}
		if finalizedAt.Valid {
			meta.FinalizedAt = &finalizedAt.Time
		}
		results = append(results, &meta)
	}
	return results, total, rows.Err()
}

// ListMetadata returns a paginated list of finalized batches, optionally filtered by ownerID (if > 0).
func (r *SQLRepository) ListMetadata(ctx context.Context, ownerID int64, limit, offset int) ([]*BatchMetadata, int, error) {
	d := r.db()
	if d == nil {
		return nil, 0, errors.New("sqlite database unavailable")
	}

	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	whereClause := "WHERE status = 'finalized'"
	var countArgs []interface{}
	var selectArgs []interface{}

	if ownerID > 0 {
		whereClause += " AND owner_id = ?"
		countArgs = append(countArgs, ownerID)
		selectArgs = append(selectArgs, ownerID)
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM batch_metadata %s;", whereClause)
	var total int
	if err := d.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed counting batches: %w", err)
	}

	selectQuery := fmt.Sprintf(`
		SELECT batch_id, batch_name, normalized_name, owner_id,
		       total_items, total_files, total_texts, total_documents,
		       total_videos, total_audio, total_photos, status,
		       created_at, updated_at, finalized_at
		FROM batch_metadata
		%s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?;
	`, whereClause)

	selectArgs = append(selectArgs, limit, offset)
	rows, err := d.QueryContext(ctx, selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed executing batch list query: %w", err)
	}
	defer rows.Close()

	var results []*BatchMetadata
	for rows.Next() {
		var meta BatchMetadata
		var finalizedAt sql.NullTime
		err := rows.Scan(
			&meta.BatchID,
			&meta.BatchName,
			&meta.NormalizedName,
			&meta.OwnerID,
			&meta.TotalItems,
			&meta.TotalFiles,
			&meta.TotalTexts,
			&meta.TotalDocuments,
			&meta.TotalVideos,
			&meta.TotalAudio,
			&meta.TotalPhotos,
			&meta.Status,
			&meta.CreatedAt,
			&meta.UpdatedAt,
			&finalizedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed scanning list result: %w", err)
		}
		if finalizedAt.Valid {
			meta.FinalizedAt = &finalizedAt.Time
		}
		results = append(results, &meta)
	}
	return results, total, rows.Err()
}

// DeleteBatch removes a batch and its items from SQLite within a transaction.
func (r *SQLRepository) DeleteBatch(ctx context.Context, batchID string) error {
	d := r.db()
	if d == nil {
		return errors.New("sqlite database unavailable")
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed beginning delete transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM batch_items WHERE batch_id = ?;", batchID); err != nil {
		return fmt.Errorf("failed deleting batch items: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM batch_metadata WHERE batch_id = ?;", batchID); err != nil {
		return fmt.Errorf("failed deleting batch metadata: %w", err)
	}

	return tx.Commit()
}

// SyncFromMongo synchronizes authoritative batches from MongoDB into SQLite.
// Runs idempotently inside a single SQLite transaction, ensuring full cache rebuildability and reconciliation.
func (r *SQLRepository) SyncFromMongo(ctx context.Context, metas []*BatchMetadata, itemsMap map[string][]*BatchItem) error {
	d := r.db()
	if d == nil {
		return errors.New("sqlite database unavailable")
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed starting sqlite sync transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Prepare batch metadata statement
	metaStmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO batch_metadata (
			batch_id, batch_name, normalized_name, owner_id,
			total_items, total_files, total_texts, total_documents,
			total_videos, total_audio, total_photos, status,
			created_at, updated_at, finalized_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		return fmt.Errorf("failed preparing sync meta statement: %w", err)
	}
	defer metaStmt.Close()

	// 2. Prepare batch items statement
	itemStmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO batch_items (
			batch_id, sequence, item_type, telegram_file_id,
			telegram_file_unique_id, storage_reference, file_name,
			file_size, mime_type, caption, text_content,
			message_entities, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		return fmt.Errorf("failed preparing sync item statement: %w", err)
	}
	defer itemStmt.Close()

	validIDs := make(map[string]struct{}, len(metas))
	for _, meta := range metas {
		if meta == nil || meta.BatchID == "" {
			continue
		}
		validIDs[meta.BatchID] = struct{}{}

		_, err := metaStmt.ExecContext(ctx,
			meta.BatchID,
			meta.BatchName,
			meta.NormalizedName,
			meta.OwnerID,
			meta.TotalItems,
			meta.TotalFiles,
			meta.TotalTexts,
			meta.TotalDocuments,
			meta.TotalVideos,
			meta.TotalAudio,
			meta.TotalPhotos,
			meta.Status,
			meta.CreatedAt,
			meta.UpdatedAt,
			meta.FinalizedAt,
		)
		if err != nil {
			return fmt.Errorf("failed syncing metadata for batch %s: %w", meta.BatchID, err)
		}

		if items, ok := itemsMap[meta.BatchID]; ok {
			for _, it := range items {
				if it == nil || it.Sequence <= 0 {
					continue
				}
				_, err := itemStmt.ExecContext(ctx,
					it.BatchID,
					it.Sequence,
					it.ItemType,
					it.TelegramFileID,
					it.TelegramFileUniqueID,
					it.StorageReference,
					it.FileName,
					it.FileSize,
					it.MimeType,
					it.Caption,
					it.TextContent,
					it.MessageEntities,
					it.CreatedAt,
				)
				if err != nil {
					return fmt.Errorf("failed syncing item seq %d for batch %s: %w", it.Sequence, meta.BatchID, err)
				}
			}
		}
	}

	// 3. Reconcile stale records: delete batches from SQLite that do not exist in authoritative MongoDB list
	if len(validIDs) > 0 {
		placeholders := make([]string, len(validIDs))
		args := make([]interface{}, len(validIDs))
		idx := 0
		for id := range validIDs {
			placeholders[idx] = "?"
			args[idx] = id
			idx++
		}
		inClause := strings.Join(placeholders, ",")
		_, _ = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM batch_items WHERE batch_id NOT IN (%s);", inClause), args...)
		_, _ = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM batch_metadata WHERE batch_id NOT IN (%s);", inClause), args...)
	} else {
		// If MongoDB has zero finalized batches, wipe SQLite batch tables
		_, _ = tx.ExecContext(ctx, "DELETE FROM batch_items;")
		_, _ = tx.ExecContext(ctx, "DELETE FROM batch_metadata;")
	}

	return tx.Commit()
}

// Count returns the count of finalized batches in SQLite.
func (r *SQLRepository) Count(ctx context.Context) (int64, error) {
	d := r.db()
	if d == nil {
		return 0, nil
	}
	var count int64
	err := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM batch_metadata WHERE status = 'finalized';").Scan(&count)
	return count, err
}
