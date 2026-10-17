package sql

import (
	"context"
	"database/sql"
	"fmt"
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
		ORDER BY date_saved DESC, file_name ASC
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
