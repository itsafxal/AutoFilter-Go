package gfilter

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

// SQLRepository manages local SQLite runtime indexing and caching of Global Filters.
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

// Save inserts or replaces a Global Filter in SQLite.
func (r *SQLRepository) Save(ctx context.Context, f *GFilter) error {
	if f == nil {
		return errors.New("cannot save nil global filter")
	}
	d := r.db()
	if d == nil {
		return errors.New("sqlite database unavailable")
	}

	f.KeywordNorm = NormalizeKeyword(f.Keyword)
	f.PackButtons()

	now := time.Now().UTC()
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	if f.UpdatedAt.IsZero() {
		f.UpdatedAt = now
	}

	query := `
		INSERT OR REPLACE INTO global_filters
		(id, keyword, keyword_norm, reply_text, media_type, file_id, file_ref, buttons_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	_, err := d.ExecContext(ctx, query,
		f.ID,
		f.Keyword,
		f.KeywordNorm,
		f.ReplyText,
		string(f.MediaType),
		f.FileID,
		f.FileRef,
		f.ButtonsJSON,
		f.CreatedAt,
		f.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save global filter in sqlite: %w", err)
	}

	return nil
}

// GetByKeywordNorm retrieves a Global Filter from SQLite by its normalized keyword.
func (r *SQLRepository) GetByKeywordNorm(ctx context.Context, norm string) (*GFilter, error) {
	d := r.db()
	if d == nil {
		return nil, errors.New("sqlite database unavailable")
	}

	query := `
		SELECT id, keyword, keyword_norm, reply_text, media_type, file_id, file_ref, buttons_json, created_at, updated_at
		FROM global_filters
		WHERE keyword_norm = ?
		LIMIT 1;
	`

	var f GFilter
	var mediaTypeStr string
	var replyText, fileID, fileRef, buttonsJSON sql.NullString
	var createdAt, updatedAt time.Time

	err := d.QueryRowContext(ctx, query, norm).Scan(
		&f.ID,
		&f.Keyword,
		&f.KeywordNorm,
		&replyText,
		&mediaTypeStr,
		&fileID,
		&fileRef,
		&buttonsJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query global filter by norm %q: %w", norm, err)
	}

	f.MediaType = MediaType(mediaTypeStr)
	if replyText.Valid {
		f.ReplyText = replyText.String
	}
	if fileID.Valid {
		f.FileID = fileID.String
	}
	if fileRef.Valid {
		f.FileRef = fileRef.String
	}
	if buttonsJSON.Valid {
		f.ButtonsJSON = buttonsJSON.String
		f.UnpackButtons()
	}
	f.CreatedAt = createdAt
	f.UpdatedAt = updatedAt

	return &f, nil
}

// GetAll retrieves all Global Filters cached in SQLite ordered by keyword.
func (r *SQLRepository) GetAll(ctx context.Context) ([]*GFilter, error) {
	d := r.db()
	if d == nil {
		return nil, errors.New("sqlite database unavailable")
	}

	query := `
		SELECT id, keyword, keyword_norm, reply_text, media_type, file_id, file_ref, buttons_json, created_at, updated_at
		FROM global_filters
		ORDER BY keyword ASC;
	`

	rows, err := d.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all global filters from sqlite: %w", err)
	}
	defer rows.Close()

	var filters []*GFilter
	for rows.Next() {
		var f GFilter
		var mediaTypeStr string
		var replyText, fileID, fileRef, buttonsJSON sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(
			&f.ID,
			&f.Keyword,
			&f.KeywordNorm,
			&replyText,
			&mediaTypeStr,
			&fileID,
			&fileRef,
			&buttonsJSON,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed scanning global filter row: %w", err)
		}

		f.MediaType = MediaType(mediaTypeStr)
		if replyText.Valid {
			f.ReplyText = replyText.String
		}
		if fileID.Valid {
			f.FileID = fileID.String
		}
		if fileRef.Valid {
			f.FileRef = fileRef.String
		}
		if buttonsJSON.Valid {
			f.ButtonsJSON = buttonsJSON.String
			f.UnpackButtons()
		}
		f.CreatedAt = createdAt
		f.UpdatedAt = updatedAt

		filters = append(filters, &f)
	}

	return filters, rows.Err()
}

// DeleteByKeywordNorm removes a Global Filter from SQLite by its normalized keyword.
func (r *SQLRepository) DeleteByKeywordNorm(ctx context.Context, norm string) (bool, error) {
	d := r.db()
	if d == nil {
		return false, errors.New("sqlite database unavailable")
	}

	res, err := d.ExecContext(ctx, "DELETE FROM global_filters WHERE keyword_norm = ?;", norm)
	if err != nil {
		return false, fmt.Errorf("failed deleting global filter %q from sqlite: %w", norm, err)
	}

	rowsAffected, _ := res.RowsAffected()
	return rowsAffected > 0, nil
}

// DeleteAll removes all Global Filters from SQLite.
func (r *SQLRepository) DeleteAll(ctx context.Context) (int64, error) {
	d := r.db()
	if d == nil {
		return 0, errors.New("sqlite database unavailable")
	}

	res, err := d.ExecContext(ctx, "DELETE FROM global_filters;")
	if err != nil {
		return 0, fmt.Errorf("failed truncating global_filters table in sqlite: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	return rowsAffected, nil
}

// Count returns the count of Global Filters cached in SQLite.
func (r *SQLRepository) Count(ctx context.Context) (int64, error) {
	d := r.db()
	if d == nil {
		return 0, nil
	}

	var count int64
	err := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM global_filters;").Scan(&count)
	return count, err
}

// SyncAll synchronizes an authoritative slice of Global Filters into SQLite inside a single atomic transaction.
// It inserts or updates all records and purges any stale SQLite records not present in the authoritative slice.
func (r *SQLRepository) SyncAll(ctx context.Context, authoritative []*GFilter) error {
	d := r.db()
	if d == nil {
		return errors.New("sqlite database unavailable")
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin sqlite transaction for global filters sync: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO global_filters
		(id, keyword, keyword_norm, reply_text, media_type, file_id, file_ref, buttons_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare sync statement: %w", err)
	}
	defer stmt.Close()

	authIDs := make(map[string]struct{}, len(authoritative))
	for _, f := range authoritative {
		if f == nil || f.ID == "" {
			continue
		}
		authIDs[f.ID] = struct{}{}
		f.KeywordNorm = NormalizeKeyword(f.Keyword)
		f.PackButtons()

		if _, err := stmt.ExecContext(ctx,
			f.ID,
			f.Keyword,
			f.KeywordNorm,
			f.ReplyText,
			string(f.MediaType),
			f.FileID,
			f.FileRef,
			f.ButtonsJSON,
			f.CreatedAt,
			f.UpdatedAt,
		); err != nil {
			return fmt.Errorf("failed syncing filter %q to sqlite: %w", f.Keyword, err)
		}
	}

	// Reconcile: delete any SQLite records whose ID is not present in the authoritative list
	if len(authoritative) == 0 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM global_filters;"); err != nil {
			return fmt.Errorf("failed clearing stale global filters in sqlite: %w", err)
		}
	} else {
		// Collect existing IDs
		rows, err := tx.QueryContext(ctx, "SELECT id FROM global_filters;")
		if err == nil {
			var staleIDs []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err == nil {
					if _, exists := authIDs[id]; !exists {
						staleIDs = append(staleIDs, id)
					}
				}
			}
			rows.Close()

			if len(staleIDs) > 0 {
				placeholders := make([]string, len(staleIDs))
				args := make([]any, len(staleIDs))
				for i, id := range staleIDs {
					placeholders[i] = "?"
					args[i] = id
				}
				delQuery := fmt.Sprintf("DELETE FROM global_filters WHERE id IN (%s);", strings.Join(placeholders, ","))
				if _, err := tx.ExecContext(ctx, delQuery, args...); err != nil {
					return fmt.Errorf("failed deleting stale global filters: %w", err)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit global filters sync transaction: %w", err)
	}

	return nil
}
