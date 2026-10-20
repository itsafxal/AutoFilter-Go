package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// MediaRecord mirrors the SQLite media table row.
type MediaRecord struct {
	FileID    string
	FileRef   string
	FileName  string
	FileSize  int64
	Size      string
	FileType  string
	MimeType  string
	Caption   string
	DateSaved time.Time
}

// MediaRepository provides read access to the runtime SQLite media cache.
type MediaRepository struct {
	dbManager *DBManager
}

// NewMediaRepository creates a new SQLite MediaRepository.
func NewMediaRepository(dbManager *DBManager) *MediaRepository {
	return &MediaRepository{dbManager: dbManager}
}

// GetFileDetails retrieves a single file record by its file_id (fast indexed lookup: <0.5ms).
func (r *MediaRepository) GetFileDetails(ctx context.Context, fileID string) (*MediaRecord, error) {
	if r.dbManager == nil || r.dbManager.DB() == nil || fileID == "" {
		return nil, nil
	}

	query := `
		SELECT file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved
		FROM media
		WHERE file_id = ?
		LIMIT 1;
	`

	row := r.dbManager.DB().QueryRowContext(ctx, query, fileID)

	var rec MediaRecord
	var dateSaved sql.NullTime

	err := row.Scan(
		&rec.FileID,
		&rec.FileRef,
		&rec.FileName,
		&rec.FileSize,
		&rec.Size,
		&rec.FileType,
		&rec.MimeType,
		&rec.Caption,
		&dateSaved,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch file details for %s: %w", fileID, err)
	}

	if dateSaved.Valid {
		rec.DateSaved = dateSaved.Time
	}

	return &rec, nil
}

// Count returns the number of media rows in SQLite.
func (r *MediaRepository) Count(ctx context.Context) (int64, error) {
	if r.dbManager == nil || r.dbManager.DB() == nil {
		return 0, nil
	}
	row := r.dbManager.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM media;")
	var count int64
	err := row.Scan(&count)
	return count, err
}

// GetLatestFiles returns the most recently indexed media files up to limit.
func (r *MediaRepository) GetLatestFiles(ctx context.Context, limit int) ([]MediaRecord, error) {
	if r.dbManager == nil || r.dbManager.DB() == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved
		FROM media
		ORDER BY date_saved DESC, rowid DESC
		LIMIT ?;
	`
	rows, err := r.dbManager.DB().QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed querying latest media files: %w", err)
	}
	defer rows.Close()

	var records []MediaRecord
	for rows.Next() {
		var rec MediaRecord
		var dateSaved sql.NullTime
		if err := rows.Scan(
			&rec.FileID,
			&rec.FileRef,
			&rec.FileName,
			&rec.FileSize,
			&rec.Size,
			&rec.FileType,
			&rec.MimeType,
			&rec.Caption,
			&dateSaved,
		); err != nil {
			return nil, fmt.Errorf("failed scanning media record: %w", err)
		}
		if dateSaved.Valid {
			rec.DateSaved = dateSaved.Time
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}


// Search performs a basic substring search on the SQLite media table.
func (r *MediaRepository) Search(ctx context.Context, keyword string, limit, offset int) ([]MediaRecord, int64, error) {
	if r.dbManager == nil || r.dbManager.DB() == nil || keyword == "" {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	pattern := "%" + keyword + "%"

	// Count matching
	countRow := r.dbManager.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM media WHERE file_name LIKE ?;", pattern)
	var total int64
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved
		FROM media
		WHERE file_name LIKE ?
		ORDER BY +date_saved DESC, file_name ASC
		LIMIT ? OFFSET ?;
	`

	rows, err := r.dbManager.DB().QueryContext(ctx, query, pattern, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []MediaRecord
	for rows.Next() {
		var rec MediaRecord
		var dateSaved sql.NullTime
		if err := rows.Scan(
			&rec.FileID,
			&rec.FileRef,
			&rec.FileName,
			&rec.FileSize,
			&rec.Size,
			&rec.FileType,
			&rec.MimeType,
			&rec.Caption,
			&dateSaved,
		); err != nil {
			return nil, 0, err
		}
		if dateSaved.Valid {
			rec.DateSaved = dateSaved.Time
		}
		records = append(records, rec)
	}

	return records, total, rows.Err()
}

// UpdateFileRef updates the cached file reference for a media record in the runtime SQLite database.
func (r *MediaRepository) UpdateFileRef(ctx context.Context, fileID string, fileRef string) error {
	if r.dbManager == nil || r.dbManager.DB() == nil || fileID == "" || fileRef == "" {
		return nil
	}
	_, err := r.dbManager.DB().ExecContext(ctx, "UPDATE media SET file_ref = ? WHERE file_id = ?", fileRef, fileID)
	return err
}

// SaveMedia inserts or updates a media record in the runtime SQLite database.
func (r *MediaRepository) SaveMedia(ctx context.Context, rec MediaRecord) error {
	if r.dbManager == nil || r.dbManager.DB() == nil || rec.FileID == "" {
		return nil
	}
	if rec.DateSaved.IsZero() {
		rec.DateSaved = time.Now().UTC()
	}
	query := `INSERT OR REPLACE INTO media (file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.dbManager.DB().ExecContext(ctx, query,
		rec.FileID, rec.FileRef, rec.FileName, rec.FileSize, rec.Size, rec.FileType, rec.MimeType, rec.Caption, rec.DateSaved)
	return err
}

// BulkSaveMedia efficiently inserts or replaces a batch of media records in SQLite
// using a single atomic transaction.
func (r *MediaRepository) BulkSaveMedia(ctx context.Context, records []MediaRecord) error {
	if r.dbManager == nil || r.dbManager.DB() == nil || len(records) == 0 {
		return nil
	}

	tx, err := r.dbManager.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for BulkSaveMedia: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO media (file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement for BulkSaveMedia: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, rec := range records {
		if rec.FileID == "" {
			continue
		}
		if rec.DateSaved.IsZero() {
			rec.DateSaved = now
		}
		if _, err := stmt.ExecContext(ctx,
			rec.FileID,
			rec.FileRef,
			rec.FileName,
			rec.FileSize,
			rec.Size,
			rec.FileType,
			rec.MimeType,
			rec.Caption,
			rec.DateSaved,
		); err != nil {
			return fmt.Errorf("failed executing BulkSaveMedia insert for %s: %w", rec.FileID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit BulkSaveMedia transaction: %w", err)
	}

	return nil
}

// BulkCheckFiles checks which fileIDs exist in SQLite.
func (r *MediaRepository) BulkCheckFiles(ctx context.Context, fileIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(fileIDs))
	for _, id := range fileIDs {
		result[id] = false
	}
	if r.dbManager == nil || r.dbManager.DB() == nil || len(fileIDs) == 0 {
		return result, nil
	}

	const chunkSize = 500
	for i := 0; i < len(fileIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(fileIDs) {
			end = len(fileIDs)
		}
		chunk := fileIDs[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j] = id
		}

		query := fmt.Sprintf("SELECT file_id FROM media WHERE file_id IN (%s);", strings.Join(placeholders, ","))
		rows, err := r.dbManager.DB().QueryContext(ctx, query, args...)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var fid string
			if err := rows.Scan(&fid); err == nil && fid != "" {
				result[fid] = true
			}
		}
		rows.Close()
	}

	return result, nil
}
