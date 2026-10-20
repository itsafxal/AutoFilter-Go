package sql

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
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
		batchSize = 20000
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

// Sync executes idempotent, high-performance synchronization of media documents into SQLite.
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

	if s.mediaRepo == nil {
		return &SyncResult{
			Skipped:       true,
			TotalInserted: 0,
			Duration:      time.Since(start),
		}, nil
	}

	// 2. Query upfront shard counts
	shardCounts, totalExpected, err := s.mediaRepo.GetShardCounts(ctx)
	if err != nil {
		s.logger.Warn("could not determine shard counts upfront, proceeding with sync", "error", err)
	}

	s.logger.Info("MongoDB media cluster status detected",
		"total_shards", len(shardCounts),
		"total_expected_files", totalExpected,
	)
	for _, sc := range shardCounts {
		s.logger.Info(fmt.Sprintf("  • Cluster [%s]: ~%d files available", sc.Label, sc.Count))
	}

	// Mark sync status as in-progress
	if err := s.setSyncMetadata(ctx, "is_complete", "false"); err != nil {
		s.logger.Warn("failed to set sync status in-progress", "error", err)
	}

	// 3. Performance optimizations: Drop secondary indexes and enable bulk pragmas
	s.logger.Info("optimizing SQLite for high-speed bulk ingestion: disabling secondary indexes and tuning pragmas...")
	if err := s.dropSecondaryIndexes(ctx); err != nil {
		s.logger.Warn("failed to drop secondary indexes", "error", err)
	}
	if err := s.setBulkPragmas(ctx); err != nil {
		s.logger.Warn("failed to set bulk pragmas", "error", err)
	}

	// Guarantee index restoration and pragma reset on exit (success or error)
	defer func() {
		_ = s.ensureIndexes(context.Background())
		_ = s.setNormalPragmas(context.Background())
	}()

	shards := s.mediaRepo.Shards()
	var totalSaved int64

	insertStmt := `
		INSERT OR REPLACE INTO media (
			file_id, file_ref, file_name, file_size, size,
			file_type, mime_type, caption, date_saved
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	// 4. Stream all media shards concurrently using a high-throughput producer-consumer pipeline
	type shardDoc struct {
		label string
		doc   mongodb.MediaDoc
	}

	docChan := make(chan shardDoc, 50000)
	errChan := make(chan error, len(shards))
	var wg sync.WaitGroup

	s.logger.Info("🚀 Launching concurrent parallel migration across all MongoDB media clusters...",
		"clusters", len(shards),
		"total_expected", totalExpected,
	)

	for i, shard := range shards {
		var shardExpected int64
		if i < len(shardCounts) {
			shardExpected = shardCounts[i].Count
		}
		s.logger.Info(fmt.Sprintf("  • Streaming cluster [%s] (approx. %d files) in parallel...", shard.Label, shardExpected),
			"cluster", shard.Label,
			"expected_files", shardExpected,
		)

		wg.Add(1)
		go func(idx int, sh mongodb.MediaShard) {
			defer wg.Done()
			shardStart := time.Now()
			streamed, err := s.mediaRepo.StreamShard(ctx, idx, func(doc mongodb.MediaDoc) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case docChan <- shardDoc{label: sh.Label, doc: doc}:
					return nil
				}
			})
			if err != nil {
				s.logger.Error(fmt.Sprintf("streaming failed for cluster [%s]", sh.Label), "error", err)
				errChan <- fmt.Errorf("sync failed on cluster %s: %w", sh.Label, err)
				return
			}
			s.logger.Info(fmt.Sprintf("✅ Cluster [%s] streaming complete: %d records in %s",
				sh.Label, streamed, time.Since(shardStart).Round(time.Millisecond).String()),
				"cluster", sh.Label,
				"records", streamed,
			)
		}(i, shard)
	}

	go func() {
		wg.Wait()
		close(docChan)
	}()

	batch := make([]mongodb.MediaDoc, 0, s.batchSize)
	shardProgress := make(map[string]int64)
	lastLog := time.Now()

	for sd := range docChan {
		batch = append(batch, sd.doc)
		shardProgress[sd.label]++

		if len(batch) >= s.batchSize {
			if err := s.commitBatch(ctx, insertStmt, batch); err != nil {
				return nil, fmt.Errorf("failed to commit batch to SQLite: %w", err)
			}
			totalSaved += int64(len(batch))
			batch = batch[:0]

			if time.Since(lastLog) >= 2*time.Second {
				var pct float64
				if totalExpected > 0 {
					pct = float64(totalSaved) / float64(totalExpected) * 100
					if pct > 100 {
						pct = 100
					}
				}
				s.logger.Info(fmt.Sprintf("📦 Synced: %d / %d (%.1f%%) | DB2: %d | DB3: %d",
					totalSaved, totalExpected, pct, shardProgress["DB2"], shardProgress["DB3"]),
					"total_saved", totalSaved,
					"progress_pct", fmt.Sprintf("%.1f%%", pct),
				)
				lastLog = time.Now()
			}
		}
	}

	if len(batch) > 0 {
		if err := s.commitBatch(ctx, insertStmt, batch); err != nil {
			return nil, fmt.Errorf("failed to commit final batch: %w", err)
		}
		totalSaved += int64(len(batch))
	}

	// Check if any shard returned an error
	select {
	case err := <-errChan:
		return nil, err
	default:
	}

	// 5. Rebuild search indexes
	indexStart := time.Now()
	s.logger.Info("⚡ Building SQLite search indexes for fast querying...")
	if err := s.ensureIndexes(ctx); err != nil {
		s.logger.Error("failed to create search indexes", "error", err)
		return nil, fmt.Errorf("failed building indexes: %w", err)
	}
	indexDuration := time.Since(indexStart)
	s.logger.Info("✨ Search indexes built successfully", "duration", indexDuration.Round(time.Millisecond).String())

	// 6. Restore normal runtime pragmas
	_ = s.setNormalPragmas(ctx)

	// 7. Update sync metadata
	if err := s.setSyncMetadata(ctx, "is_complete", "true"); err != nil {
		s.logger.Warn("failed to update sync metadata to complete", "error", err)
	}
	_ = s.setSyncMetadata(ctx, "last_sync_count", fmt.Sprintf("%d", totalSaved))
	_ = s.setSyncMetadata(ctx, "last_sync_time", time.Now().UTC().Format(time.RFC3339))

	duration := time.Since(start)
	s.logger.Info("🎉 All media clusters synchronized successfully",
		"total_records", totalSaved,
		"duration", duration.Round(time.Millisecond).String(),
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

		now := time.Now()
		for _, doc := range docs {
			var dateSaved interface{} = now
			if !doc.Date.IsZero() {
				dateSaved = doc.Date
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

func (s *Syncer) dropSecondaryIndexes(ctx context.Context) error {
	queries := []string{
		"DROP INDEX IF EXISTS idx_media_file_name;",
		"DROP INDEX IF EXISTS idx_media_caption;",
		"DROP INDEX IF EXISTS idx_date_saved;",
		"DROP INDEX IF EXISTS idx_media_date_name;",
		"DROP TRIGGER IF EXISTS media_ai;",
		"DROP TRIGGER IF EXISTS media_ad;",
		"DROP TRIGGER IF EXISTS media_au;",
	}
	for _, q := range queries {
		if _, err := s.dbManager.DB().ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (s *Syncer) ensureIndexes(ctx context.Context) error {
	queries := []string{
		"DROP INDEX IF EXISTS idx_date_saved;",
		"CREATE INDEX IF NOT EXISTS idx_media_file_name ON media (file_name);",
		"CREATE INDEX IF NOT EXISTS idx_media_date_name ON media (date_saved DESC, file_name);",
		`CREATE VIRTUAL TABLE IF NOT EXISTS media_fts USING fts5(
			file_name,
			content='media',
			content_rowid='rowid',
			tokenize='trigram'
		);`,
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
	}
	for _, q := range queries {
		if _, err := s.dbManager.DB().ExecContext(ctx, q); err != nil {
			return err
		}
	}

	// Rebuild FTS5 trigram search index after bulk sync
	if _, err := s.dbManager.DB().ExecContext(ctx, "INSERT INTO media_fts(media_fts) VALUES('rebuild');"); err != nil {
		s.logger.Warn("failed to rebuild FTS5 index", "error", err)
	}

	return nil
}

func (s *Syncer) setBulkPragmas(ctx context.Context) error {
	queries := []string{
		"PRAGMA synchronous = OFF;",
		"PRAGMA cache_size = -131072;", // 128 MB cache during bulk ingestion
		"PRAGMA temp_store = MEMORY;",
	}
	for _, q := range queries {
		if _, err := s.dbManager.DB().ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (s *Syncer) setNormalPragmas(ctx context.Context) error {
	queries := []string{
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA cache_size = -64000;",
	}
	for _, q := range queries {
		if _, err := s.dbManager.DB().ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
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
