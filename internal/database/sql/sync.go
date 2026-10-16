package sql

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/itsafxal/MYTGBOTZ/internal/database/mongodb"
)

// Syncer handles runtime synchronization from authoritative MongoDB collections into local SQLite.
type Syncer struct {
	dbManager *DBManager
	mediaRepo *mongodb.MediaRepository
	logger    *slog.Logger
	batchSize int
}

// NewSyncer constructs a new Syncer.
func NewSyncer(dbManager *DBManager, mediaRepo *mongodb.MediaRepository, logger *slog.Logger, batchSize int) *Syncer {
	if batchSize <= 0 {
		batchSize = 10000
	}
	return &Syncer{
		dbManager: dbManager,
		mediaRepo: mediaRepo,
		logger:    logger,
		batchSize: batchSize,
	}
}

// SyncResult details the outcome of a synchronization run.
type SyncResult struct {
	Skipped       bool
	TotalInserted int64
	Duration      time.Duration
}

// Sync executes idempotent synchronization of media documents into SQLite.
func (s *Syncer) Sync(ctx context.Context) (*SyncResult, error) {
	start := time.Now()

	// 1. Check if sync was already successfully completed
	isComplete, err := s.IsSyncComplete(ctx)
	if err != nil {
		s.logger.Warn("could not check sync metadata, proceeding with synchronization check", "error", err)
	}

	hasData, err := s.HasData(ctx)
	if err != nil {
		s.logger.Warn("could not check table rows, proceeding with synchronization", "error", err)
	}

	if isComplete && hasData {
		count, _ := s.GetTotalRows(ctx)
		s.logger.Info("SQLite media table is already populated and synchronized; skipping full migration",
			"existing_records", count,
		)
		return &SyncResult{
			Skipped:       true,
			TotalInserted: count,
			Duration:      time.Since(start),
		}, nil
	}

	s.logger.Info("starting runtime media synchronization from MongoDB to temporary SQLite...",
		"batch_size", s.batchSize,
	)

	// Mark sync status as in-progress
	if err := s.setSyncMetadata(ctx, "is_complete", "false"); err != nil {
		s.logger.Warn("failed to set sync status in-progress", "error", err)
	}

	var batch []mongodb.MediaDoc
	var totalSaved int64

	insertStmt := `
		INSERT OR REPLACE INTO media (
			file_id, file_ref, file_name, file_size, size,
			file_type, mime_type, caption, date_saved
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	// Stream from authoritative MongoDB
	_, streamErr := s.mediaRepo.StreamAllDocuments(ctx, func(doc mongodb.MediaDoc) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		batch = append(batch, doc)
		if len(batch) >= s.batchSize {
			if err := s.commitBatch(ctx, insertStmt, batch); err != nil {
				return fmt.Errorf("failed to commit batch of %d records: %w", len(batch), err)
			}
			totalSaved += int64(len(batch))
			s.logger.Info("synchronized batch to SQLite", "total_saved", totalSaved)
			batch = batch[:0]
		}
		return nil
	})

	if streamErr != nil {
		s.logger.Error("synchronization stream failed", "error", streamErr)
		return nil, fmt.Errorf("synchronization failed: %w", streamErr)
	}

	// Commit any remaining records
	if len(batch) > 0 {
		if err := s.commitBatch(ctx, insertStmt, batch); err != nil {
			return nil, fmt.Errorf("failed to commit final batch of %d records: %w", len(batch), err)
		}
		totalSaved += int64(len(batch))
	}

	// Mark sync status as complete
	if err := s.setSyncMetadata(ctx, "is_complete", "true"); err != nil {
		s.logger.Warn("failed to update sync metadata to complete", "error", err)
	}
	_ = s.setSyncMetadata(ctx, "last_sync_count", fmt.Sprintf("%d", totalSaved))
	_ = s.setSyncMetadata(ctx, "last_sync_time", time.Now().UTC().Format(time.RFC3339))

	duration := time.Since(start)
	s.logger.Info("✨ synchronization completed successfully",
		"total_records", totalSaved,
		"duration", duration.String(),
	)

	return &SyncResult{
		Skipped:       false,
		TotalInserted: totalSaved,
		Duration:      duration,
	}, nil
}

func (s *Syncer) commitBatch(ctx context.Context, insertSQL string, docs []mongodb.MediaDoc) error {
	return s.dbManager.ExecWrite(func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, insertSQL)
		if err != nil {
			return fmt.Errorf("prepare error: %w", err)
		}
		defer stmt.Close()

		for _, doc := range docs {
			var dateSaved interface{}
			if !doc.Date.IsZero() {
				dateSaved = doc.Date
			} else {
				dateSaved = time.Now()
			}

			_, err := stmt.ExecContext(ctx,
				doc.FileID,
				doc.FileRef,
				doc.FileName,
				doc.FileSize,
				doc.Size,
				doc.FileType,
				doc.MimeType,
				doc.Caption,
				dateSaved,
			)
			if err != nil {
				return fmt.Errorf("insert error for file %s: %w", doc.FileID, err)
			}
		}
		return nil
	})
}

// IsSyncComplete checks the sync_metadata table to verify if the index is ready.
func (s *Syncer) IsSyncComplete(ctx context.Context) (bool, error) {
	row := s.dbManager.DB().QueryRowContext(ctx, "SELECT value FROM sync_metadata WHERE key = 'is_complete';")
	var val string
	if err := row.Scan(&val); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return val == "true", nil
}

// HasData returns true if the media table contains at least one row.
func (s *Syncer) HasData(ctx context.Context) (bool, error) {
	row := s.dbManager.DB().QueryRowContext(ctx, "SELECT 1 FROM media LIMIT 1;")
	var dummy int
	if err := row.Scan(&dummy); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetTotalRows counts all rows currently in the SQLite media table.
func (s *Syncer) GetTotalRows(ctx context.Context) (int64, error) {
	row := s.dbManager.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM media;")
	var count int64
	err := row.Scan(&count)
	return count, err
}

func (s *Syncer) setSyncMetadata(ctx context.Context, key, value string) error {
	return s.dbManager.ExecWrite(func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT OR REPLACE INTO sync_metadata (key, value, updated_at) VALUES (?, ?, ?);",
			key, value, time.Now().UTC(),
		)
		return err
	})
}
