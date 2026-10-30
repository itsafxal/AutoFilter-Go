package batch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Service provides the complete Named Batch business logic and dual-persistence lifecycle.
type Service struct {
	mongoRepo *MongoRepository
	sqlRepo   *SQLRepository
	sessions  *SessionManager
	logger    *slog.Logger
}

// NewService constructs a new Named Batch Service.
func NewService(mongoProv MongoProvider, sqlProv SQLProvider, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		mongoRepo: NewMongoRepository(mongoProv),
		sqlRepo:   NewSQLRepository(sqlProv),
		sessions:  NewSessionManager(),
		logger:    logger,
	}
}

// NewServiceWithRepos constructs a Service using explicit repository instances (useful for testing).
func NewServiceWithRepos(mongoRepo *MongoRepository, sqlRepo *SQLRepository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		mongoRepo: mongoRepo,
		sqlRepo:   sqlRepo,
		sessions:  NewSessionManager(),
		logger:    logger,
	}
}

// Sessions returns the active SessionManager.
func (s *Service) Sessions() *SessionManager {
	return s.sessions
}

// Init performs the mandatory startup synchronization lifecycle:
// 1. Verifies MongoDB indexes.
// 2. Loads all authoritative finalized Batch records from MongoDB.
// 3. Loads their corresponding ordered items from MongoDB.
// 4. Synchronizes Batch records and items into SQLite (reconciling missing or stale entries).
// 5. Marks Batch indexing as ready.
func (s *Service) Init(ctx context.Context) error {
	s.logger.Info("initializing Named Batch Service & verifying indexes...")

	// 1. Ensure MongoDB indexes exist
	if err := s.mongoRepo.Init(ctx); err != nil {
		s.logger.Warn("could not ensure mongodb batch indexes (continuing)", "error", err)
	}

	// 2. Fetch authoritative finalized batch records from MongoDB
	metas, err := s.mongoRepo.GetAllFinalizedMetadata(ctx)
	if err != nil {
		s.logger.Error("failed fetching batch metadata from mongodb during startup sync", "error", err)
		// Check if SQLite already has cached records to serve requests gracefully
		cachedCount, sqlErr := s.sqlRepo.Count(ctx)
		if sqlErr == nil && cachedCount > 0 {
			s.logger.Warn("serving batch requests from local SQLite fallback cache", "cached_batches", cachedCount)
			return nil
		}
		return fmt.Errorf("batch startup synchronization failed: %w", err)
	}

	// 3. Load items for each batch from MongoDB
	itemsMap := make(map[string][]*BatchItem, len(metas))
	totalItemsCount := 0
	for _, meta := range metas {
		items, err := s.mongoRepo.GetItems(ctx, meta.BatchID)
		if err != nil {
			s.logger.Warn("failed fetching batch items for sync", "batch_id", meta.BatchID, "error", err)
			continue
		}
		itemsMap[meta.BatchID] = items
		totalItemsCount += len(items)
	}

	// 4. Atomic transactional SQLite synchronization
	if err := s.sqlRepo.SyncFromMongo(ctx, metas, itemsMap); err != nil {
		s.logger.Error("failed syncing batch records into SQLite runtime index", "error", err)
		return fmt.Errorf("batch sqlite sync failed: %w", err)
	}

	s.logger.Info("🎉 Named Batch startup synchronization completed successfully",
		"finalized_batches", len(metas),
		"synchronized_items", totalItemsCount,
	)
	return nil
}

// SaveItemDual persists a batch item to MongoDB first, then to SQLite.
func (s *Service) SaveItemDual(ctx context.Context, item *BatchItem) error {
	if item == nil {
		return errors.New("cannot persist nil batch item")
	}

	// 1. Authoritative persistence in MongoDB
	if err := s.mongoRepo.SaveItem(ctx, item); err != nil {
		return fmt.Errorf("failed persisting batch item in mongodb: %w", err)
	}

	// 2. Runtime indexing in SQLite
	if err := s.sqlRepo.SaveItem(ctx, item); err != nil {
		s.logger.Warn("sqlite item index update failed; scheduling reconciliation",
			"batch_id", item.BatchID,
			"sequence", item.Sequence,
			"error", err,
		)
	}

	return nil
}

// FinalizeBatch persists final metadata and all items to MongoDB, then updates SQLite.
func (s *Service) FinalizeBatch(ctx context.Context, meta *BatchMetadata, items []*BatchItem) error {
	if meta == nil || meta.BatchID == "" {
		return errors.New("cannot finalize invalid batch")
	}

	now := time.Now().UTC()
	meta.Status = StatusFinalized
	meta.FinalizedAt = &now
	meta.UpdatedAt = now

	// 1. Authoritative write to MongoDB
	if err := s.mongoRepo.SaveMetadata(ctx, meta); err != nil {
		return fmt.Errorf("failed persisting finalized metadata in mongodb: %w", err)
	}
	if err := s.mongoRepo.SaveItems(ctx, items); err != nil {
		return fmt.Errorf("failed persisting finalized items in mongodb: %w", err)
	}

	// 2. Synchronize to SQLite
	if err := s.sqlRepo.SaveMetadata(ctx, meta); err != nil {
		s.logger.Warn("sqlite metadata save failed during finalization", "batch_id", meta.BatchID, "error", err)
	}
	if err := s.sqlRepo.SaveItems(ctx, items); err != nil {
		s.logger.Warn("sqlite items save failed during finalization", "batch_id", meta.BatchID, "error", err)
	}

	s.logger.Info("batch successfully finalized and dual-persisted",
		"batch_id", meta.BatchID,
		"name", meta.BatchName,
		"total_items", meta.TotalItems,
	)
	return nil
}

// GetBatch retrieves batch metadata, checking SQLite first and falling back to MongoDB.
func (s *Service) GetBatch(ctx context.Context, batchID string) (*BatchMetadata, error) {
	// 1. SQLite runtime cache lookup (<0.1ms)
	meta, err := s.sqlRepo.GetMetadata(ctx, batchID)
	if err == nil && meta != nil {
		return meta, nil
	}

	// 2. Authoritative MongoDB fallback
	meta, err = s.mongoRepo.GetMetadata(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if meta != nil {
		// Re-populate SQLite index in background
		_ = s.sqlRepo.SaveMetadata(ctx, meta)
	}
	return meta, nil
}

// GetBatchItems retrieves items strictly ordered by sequence ASC, checking SQLite first.
func (s *Service) GetBatchItems(ctx context.Context, batchID string) ([]*BatchItem, error) {
	// 1. Fast SQLite retrieval
	items, err := s.sqlRepo.GetItems(ctx, batchID)
	if err == nil && len(items) > 0 {
		return items, nil
	}

	// 2. Authoritative MongoDB fallback
	items, err = s.mongoRepo.GetItems(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		// Re-populate SQLite index
		_ = s.sqlRepo.SaveItems(ctx, items)
	}
	return items, nil
}

// SearchBatches queries SQLite for fast case-insensitive exact, normalized, or partial matches.
func (s *Service) SearchBatches(ctx context.Context, query string, limit, offset int) ([]*BatchMetadata, int, error) {
	return s.sqlRepo.SearchMetadata(ctx, query, limit, offset)
}

// ListBatches returns a paginated list of batches from SQLite.
func (s *Service) ListBatches(ctx context.Context, ownerID int64, limit, offset int) ([]*BatchMetadata, int, error) {
	return s.sqlRepo.ListMetadata(ctx, ownerID, limit, offset)
}

// DeleteBatch deletes a batch and all its sequential items from both MongoDB and SQLite.
func (s *Service) DeleteBatch(ctx context.Context, batchID string) error {
	// 1. Remove from authoritative MongoDB
	if err := s.mongoRepo.DeleteBatch(ctx, batchID); err != nil {
		return fmt.Errorf("failed deleting batch from mongodb: %w", err)
	}

	// 2. Remove from runtime SQLite
	if err := s.sqlRepo.DeleteBatch(ctx, batchID); err != nil {
		s.logger.Warn("failed deleting batch from sqlite index", "batch_id", batchID, "error", err)
	}

	return nil
}

// Count returns the total count of finalized batches.
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.sqlRepo.Count(ctx)
	if err == nil && count > 0 {
		return count, nil
	}
	return s.mongoRepo.Count(ctx)
}
