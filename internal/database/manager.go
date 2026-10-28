package database

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/database/mongodb"
	"github.com/itsafxal/MYTGBOTZ/internal/database/sql"
	"go.mongodb.org/mongo-driver/mongo"
)

// DatabaseManager coordinates authoritative MongoDB connections and temporary SQLite runtime caches.
type DatabaseManager struct {
	cfg            *config.Config
	logger         *slog.Logger
	mongoManager   *mongodb.ClientManager
	sqlManager     *sql.DBManager
	userRepo       *mongodb.UserRepository
	groupRepo      *mongodb.GroupRepository
	mongoMediaRepo *mongodb.MediaRepository
	sqlMediaRepo   *sql.MediaRepository
	syncer         *sql.Syncer
	ready          bool
	syncDone       chan struct{}
	syncResult     *sql.SyncResult
	syncErr        error
	mu             sync.RWMutex
}

// NewDatabaseManager constructs a new unified DatabaseManager.
func NewDatabaseManager(cfg *config.Config, logger *slog.Logger) *DatabaseManager {
	mongoMgr := mongodb.NewClientManager(cfg, logger)
	sqlMgr := sql.NewDBManager(cfg.SQLiteDBPath, logger)

	return &DatabaseManager{
		cfg:          cfg,
		logger:       logger,
		mongoManager: mongoMgr,
		sqlManager:   sqlMgr,
		syncDone:     make(chan struct{}),
	}
}

func (m *DatabaseManager) completeSyncLocked(res *sql.SyncResult, err error) {
	m.syncResult = res
	m.syncErr = err
	select {
	case <-m.syncDone:
	default:
		close(m.syncDone)
	}
}

func (m *DatabaseManager) completeSync(res *sql.SyncResult, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completeSyncLocked(res, err)
}

// Init initializes MongoDB, opens SQLite, and executes initial media synchronization.
// The bot will not report database as ready until all required initialization and sync have succeeded.
func (m *DatabaseManager) Init(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Initialize temporary SQLite runtime
	if err := m.sqlManager.Open(ctx); err != nil {
		m.completeSyncLocked(nil, err)
		return fmt.Errorf("failed to initialize runtime SQLite database: %w", err)
	}
	m.sqlMediaRepo = sql.NewMediaRepository(m.sqlManager)

	// 2. Initialize authoritative MongoDB if configured
	if m.cfg.HasDatabase() {
		m.logger.Info("initializing authoritative MongoDB connection...",
			"database", m.cfg.DatabaseName,
			"collection", m.cfg.CollectionName,
		)

		if err := m.mongoManager.Connect(ctx); err != nil {
			_ = m.sqlManager.Close()
			m.completeSyncLocked(nil, err)
			return fmt.Errorf("fatal MongoDB initialization failure: %w", err)
		}

		// Initialize repositories
		m.userRepo = mongodb.NewUserRepository(m.mongoManager.PrimaryDB())
		m.groupRepo = mongodb.NewGroupRepository(m.mongoManager.PrimaryDB())

		// Setup sharded MediaRepository exclusively on media clusters (DB2, DB3)
		// Primary DB is reserved for users, groups, and auth; it does not host media records.
		var mediaShards []mongodb.MediaShard
		if m.mongoManager.HasDB2() {
			mediaShards = append(mediaShards, mongodb.MediaShard{
				Label: "DB2",
				Col:   m.mongoManager.DB2().Collection(m.cfg.CollectionName),
			})
		}
		if m.mongoManager.HasDB3() {
			mediaShards = append(mediaShards, mongodb.MediaShard{
				Label: "DB3",
				Col:   m.mongoManager.DB3().Collection(m.cfg.CollectionName),
			})
		}

		// Fallback for single-database environments (e.g. testing or local dev)
		if len(mediaShards) == 0 && m.mongoManager.PrimaryDB() != nil {
			mediaShards = append(mediaShards, mongodb.MediaShard{
				Label: "PrimaryDB",
				Col:   m.mongoManager.PrimaryDB().Collection(m.cfg.CollectionName),
			})
		}

		m.mongoMediaRepo = mongodb.NewShardedMediaRepository(mediaShards...)

		m.logger.Info("MongoDB clusters connected successfully",
			"primary_db", m.cfg.DatabaseName+" (Users & Groups)",
			"db2_configured", m.mongoManager.HasDB2(),
			"db3_configured", m.mongoManager.HasDB3(),
			"media_shards", len(mediaShards),
		)

		// 3. Perform media synchronization from MongoDB to SQLite
		m.syncer = sql.NewSyncer(m.sqlManager, m.mongoMediaRepo, m.logger, m.cfg.SyncBatchSize)

		if !m.cfg.EnableSync {
			m.logger.Info("⚡ Media synchronization disabled (ENABLE_SYNC=false); running in high-speed zero-sync MongoDB direct mode")
			m.completeSyncLocked(&sql.SyncResult{Skipped: true}, nil)
		} else if m.cfg.BackgroundSync {
			// Check if already populated to skip immediately
			isComplete, _ := m.syncer.IsSyncComplete(ctx)
			hasData, _ := m.syncer.HasData(ctx)
			count, _ := m.syncer.GetTotalRows(ctx)
			if isComplete && hasData {
				m.logger.Info("SQLite media table is already populated and synchronized; skipping full migration",
					"existing_records", count,
				)
				m.completeSyncLocked(&sql.SyncResult{Skipped: true, TotalInserted: count}, nil)
			} else {
				m.logger.Info("⚡ Launching concurrent media migration in background to enable instant bot boot and rapid deployment...")
				go func() {
					bgCtx := context.Background()
					syncRes, err := m.syncer.Sync(bgCtx)
					if err != nil {
						m.logger.Error("background synchronization from MongoDB to SQLite failed", "error", err)
						m.completeSync(nil, err)
						return
					}
					m.logger.Info("🎉 Background media synchronization completed successfully",
						"total_records", syncRes.TotalInserted,
						"skipped", syncRes.Skipped,
						"duration", syncRes.Duration.String(),
					)
					m.completeSync(syncRes, nil)
				}()
			}
		} else {
			syncRes, err := m.syncer.Sync(ctx)
			if err != nil {
				m.logger.Error("synchronization from MongoDB to SQLite failed", "error", err)
				m.completeSyncLocked(nil, err)
				_ = m.Close()
				return fmt.Errorf("runtime database synchronization failed: %w", err)
			}

			m.logger.Info("database initialization and synchronization verified",
				"total_records", syncRes.TotalInserted,
				"skipped", syncRes.Skipped,
				"duration", syncRes.Duration.String(),
			)
			m.completeSyncLocked(syncRes, nil)
		}
	} else {
		m.logger.Warn("DATABASE_URI not configured: running in standalone mode without authoritative MongoDB")
		m.completeSyncLocked(&sql.SyncResult{Skipped: true}, nil)
	}

	m.ready = true
	return nil
}

// Close gracefully closes SQLite and disconnects MongoDB clients.
func (m *DatabaseManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-m.syncDone:
	default:
		close(m.syncDone)
	}

	m.ready = false
	var errs []error

	if m.sqlManager != nil {
		if err := m.sqlManager.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing SQLite: %w", err))
		}
	}

	if m.mongoManager != nil {
		if err := m.mongoManager.Disconnect(context.Background()); err != nil {
			errs = append(errs, fmt.Errorf("error disconnecting MongoDB: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing database manager: %v", errs)
	}
	return nil
}

// WaitForSync blocks until SQLite synchronization from MongoDB completes, or ctx is cancelled.
func (m *DatabaseManager) WaitForSync(ctx context.Context) (*sql.SyncResult, error) {
	if m == nil {
		return nil, nil
	}
	select {
	case <-m.syncDone:
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.syncResult, m.syncErr
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// IsSyncDone returns whether synchronization has completed.
func (m *DatabaseManager) IsSyncDone() bool {
	if m == nil {
		return true
	}
	select {
	case <-m.syncDone:
		return true
	default:
		return false
	}
}

// SyncResult returns the last synchronization result, if completed.
func (m *DatabaseManager) SyncResult() (*sql.SyncResult, error) {
	if m == nil {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.syncResult, m.syncErr
}

// IsReady reports whether the databases have completed initialization and synchronization.
func (m *DatabaseManager) IsReady() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ready
}

// MongoPrimaryDB returns the authoritative primary MongoDB database instance.
func (m *DatabaseManager) MongoPrimaryDB() *mongo.Database {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.mongoManager != nil {
		return m.mongoManager.PrimaryDB()
	}
	return nil
}

// UserRepo returns the authoritative MongoDB UserRepository.
func (m *DatabaseManager) UserRepo() *mongodb.UserRepository {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.userRepo
}

// GroupRepo returns the authoritative MongoDB GroupRepository.
func (m *DatabaseManager) GroupRepo() *mongodb.GroupRepository {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.groupRepo
}

// MongoMediaRepo returns the authoritative MongoDB MediaRepository.
func (m *DatabaseManager) MongoMediaRepo() *mongodb.MediaRepository {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mongoMediaRepo
}

// SqlMediaRepo returns the fast runtime SQLite MediaRepository.
func (m *DatabaseManager) SqlMediaRepo() *sql.MediaRepository {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sqlMediaRepo
}

// SQLManager returns the underlying SQLite DBManager.
func (m *DatabaseManager) SQLManager() *sql.DBManager {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sqlManager
}

// UpdateMediaFileRef updates the cached file reference for a media record in the runtime SQLite database.
func (m *DatabaseManager) UpdateMediaFileRef(ctx context.Context, fileID string, fileRef string) error {
	m.mu.RLock()
	repo := m.sqlMediaRepo
	m.mu.RUnlock()
	if repo != nil {
		return repo.UpdateFileRef(ctx, fileID, fileRef)
	}
	return nil
}

// SaveMedia saves a media record into the runtime SQLite database.
func (m *DatabaseManager) SaveMedia(ctx context.Context, rec sql.MediaRecord) error {
	m.mu.RLock()
	repo := m.sqlMediaRepo
	m.mu.RUnlock()
	if repo != nil {
		return repo.SaveMedia(ctx, rec)
	}
	return nil
}

// Syncer returns the SQLite Syncer instance.
func (m *DatabaseManager) Syncer() *sql.Syncer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.syncer
}

// MediaDocToRecord converts a MongoDB MediaDoc to a SQLite MediaRecord.
func MediaDocToRecord(doc mongodb.MediaDoc) sql.MediaRecord {
	dateSaved := doc.Date
	if dateSaved.IsZero() {
		dateSaved = time.Now().UTC()
	}
	return sql.MediaRecord{
		FileID:    doc.FileID,
		FileRef:   doc.FileRef,
		FileName:  doc.FileName,
		FileSize:  doc.FileSize,
		Size:      doc.Size,
		FileType:  doc.FileType,
		MimeType:  doc.MimeType,
		Caption:   doc.Caption,
		DateSaved: dateSaved,
	}
}

// CheckMediaExists checks if a media document already exists in SQLite or MongoDB by fileID.
func (m *DatabaseManager) CheckMediaExists(ctx context.Context, fileID string) (bool, error) {
	if fileID == "" {
		return false, nil
	}
	m.mu.RLock()
	sqlRepo := m.sqlMediaRepo
	mongoRepo := m.mongoMediaRepo
	m.mu.RUnlock()

	// 1. Fast SQLite primary-key index lookup (<0.1ms)
	if sqlRepo != nil {
		rec, err := sqlRepo.GetFileDetails(ctx, fileID)
		if err == nil && rec != nil && rec.FileID != "" {
			return true, nil
		}
	}

	// 2. Fallback to MongoDB check across shards
	if mongoRepo != nil {
		return mongoRepo.CheckFileExists(ctx, fileID)
	}

	return false, nil
}

// CheckMediaExistsByDoc checks if a media document already exists across SQLite and MongoDB shards
// using both primary file_id and secondary (file_name, file_size) metadata.
func (m *DatabaseManager) CheckMediaExistsByDoc(ctx context.Context, doc mongodb.MediaDoc) (bool, error) {
	m.mu.RLock()
	sqlRepo := m.sqlMediaRepo
	mongoRepo := m.mongoMediaRepo
	m.mu.RUnlock()

	// 1. Fast SQLite primary-key index lookup
	if sqlRepo != nil && doc.FileID != "" {
		rec, err := sqlRepo.GetFileDetails(ctx, doc.FileID)
		if err == nil && rec != nil && rec.FileID != "" {
			return true, nil
		}
	}

	// 2. Fast SQLite (file_name, file_size) index lookup
	if sqlRepo != nil && doc.FileName != "" && doc.FileSize > 0 {
		rec, err := sqlRepo.GetFileByNameAndSize(ctx, doc.FileName, doc.FileSize)
		if err == nil && rec != nil && rec.FileID != "" {
			return true, nil
		}
	}

	// 3. MongoDB primary-key check across shards
	if mongoRepo != nil && doc.FileID != "" {
		exists, err := mongoRepo.CheckFileExists(ctx, doc.FileID)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}

	// 4. MongoDB (file_name, file_size) check across shards
	if mongoRepo != nil && doc.FileName != "" && doc.FileSize > 0 {
		found, err := mongoRepo.FindByNameAndSize(ctx, doc.FileName, doc.FileSize)
		if err != nil {
			return false, err
		}
		if found != nil && found.FileID != "" {
			return true, nil
		}
	}

	return false, nil
}

// BulkCheckMediaExists checks a slice of file IDs across SQLite and MongoDB.
func (m *DatabaseManager) BulkCheckMediaExists(ctx context.Context, fileIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(fileIDs))
	for _, id := range fileIDs {
		result[id] = false
	}
	if len(fileIDs) == 0 {
		return result, nil
	}

	m.mu.RLock()
	sqlRepo := m.sqlMediaRepo
	mongoRepo := m.mongoMediaRepo
	m.mu.RUnlock()

	var remaining []string
	if sqlRepo != nil {
		sqlMap, err := sqlRepo.BulkCheckFiles(ctx, fileIDs)
		if err == nil {
			for id, exists := range sqlMap {
				if exists {
					result[id] = true
				} else {
					remaining = append(remaining, id)
				}
			}
		} else {
			remaining = fileIDs
		}
	} else {
		remaining = fileIDs
	}

	if len(remaining) > 0 && mongoRepo != nil {
		mongoMap, err := mongoRepo.BulkCheckFiles(ctx, remaining)
		if err != nil {
			return result, fmt.Errorf("mongo bulk check failed: %w", err)
		}
		for id, exists := range mongoMap {
			if exists {
				result[id] = true
			}
		}
	}

	return result, nil
}

// BulkCheckMediaExistsByDocs checks a slice of media documents across SQLite and MongoDB shards
// using both primary file_id and secondary (file_name, file_size) metadata.
func (m *DatabaseManager) BulkCheckMediaExistsByDocs(ctx context.Context, docs []mongodb.MediaDoc) (map[string]bool, error) {
	result := make(map[string]bool, len(docs))
	for _, d := range docs {
		result[d.FileID] = false
	}
	if len(docs) == 0 {
		return result, nil
	}

	m.mu.RLock()
	sqlRepo := m.sqlMediaRepo
	mongoRepo := m.mongoMediaRepo
	m.mu.RUnlock()

	// 1. Fast SQLite multi-token metadata lookup
	var remaining []mongodb.MediaDoc
	if sqlRepo != nil {
		items := make([]sql.FileCheckItem, 0, len(docs))
		for _, d := range docs {
			items = append(items, sql.FileCheckItem{
				FileID:   d.FileID,
				FileName: d.FileName,
				FileSize: d.FileSize,
			})
		}
		sqlMap, err := sqlRepo.BulkCheckMediaItems(ctx, items)
		if err == nil {
			for _, d := range docs {
				if sqlMap[d.FileID] {
					result[d.FileID] = true
				} else {
					remaining = append(remaining, d)
				}
			}
		} else {
			remaining = docs
		}
	} else {
		remaining = docs
	}

	// 2. Authoritative MongoDB metadata check across all shards
	if len(remaining) > 0 && mongoRepo != nil {
		mongoMap, err := mongoRepo.BulkCheckFilesByMetadata(ctx, remaining)
		if err != nil {
			return result, fmt.Errorf("authoritative mongo bulk check failed: %w", err)
		}
		for _, d := range remaining {
			if mongoMap[d.FileID] {
				result[d.FileID] = true
			}
		}
	}

	return result, nil
}

// SaveMediaLiveAlternating saves a media item atomically alternating across MongoDB clusters
// (DB2 and DB3) and immediately inserts it into SQLite so it is instantly searchable.
// If the document already exists in SQLite or MongoDB, it safely bypasses insertion.
func (m *DatabaseManager) SaveMediaLiveAlternating(ctx context.Context, doc mongodb.MediaDoc) (string, error) {
	m.mu.RLock()
	mongoRepo := m.mongoMediaRepo
	sqlRepo := m.sqlMediaRepo
	m.mu.RUnlock()

	// Pre-write deduplication: check if file already exists across any shard or SQLite
	exists, err := m.CheckMediaExistsByDoc(ctx, doc)
	if err != nil {
		return "", fmt.Errorf("deduplication check error: %w", err)
	}
	if exists {
		if sqlRepo != nil {
			_ = sqlRepo.SaveMedia(ctx, MediaDocToRecord(doc))
		}
		return "Existing (Deduplicated)", nil
	}

	var shardLabel string
	var mongoErr error
	if mongoRepo != nil {
		lbl, err := mongoRepo.SaveMediaAlternating(ctx, doc)
		if err != nil {
			mongoErr = err
		} else {
			shardLabel = lbl
		}
	}

	var sqlErr error
	if sqlRepo != nil {
		sqlErr = sqlRepo.SaveMedia(ctx, MediaDocToRecord(doc))
	}

	if mongoErr != nil {
		if mongodb.IsQuotaOrWriteBlockedError(mongoErr) {
			if m.logger != nil {
				m.logger.Warn("⚠️ MongoDB storage quota exceeded; media persisted into local SQLite runtime",
					"file_id", doc.FileID,
					"file_name", doc.FileName,
					"error", mongoErr,
				)
			}
			if sqlErr == nil {
				return "SQLite (Cloud Full)", nil
			}
		}
		return "", mongoErr
	}

	if sqlErr != nil {
		return shardLabel, sqlErr
	}

	return shardLabel, nil
}

// SaveMediaLive saves a media item to a specific MongoDB shard and to SQLite.
// If the document already exists, duplicate write is skipped.
func (m *DatabaseManager) SaveMediaLive(ctx context.Context, doc mongodb.MediaDoc, shardIndex int) error {
	m.mu.RLock()
	mongoRepo := m.mongoMediaRepo
	sqlRepo := m.sqlMediaRepo
	m.mu.RUnlock()

	exists, err := m.CheckMediaExistsByDoc(ctx, doc)
	if err != nil {
		return fmt.Errorf("deduplication check error: %w", err)
	}
	if exists {
		if sqlRepo != nil {
			_ = sqlRepo.SaveMedia(ctx, MediaDocToRecord(doc))
		}
		return nil
	}

	var mongoErr error
	if mongoRepo != nil {
		if err := mongoRepo.SaveMedia(ctx, doc, shardIndex); err != nil {
			mongoErr = err
		}
	}

	var sqlErr error
	if sqlRepo != nil {
		sqlErr = sqlRepo.SaveMedia(ctx, MediaDocToRecord(doc))
	}

	if mongoErr != nil {
		if mongodb.IsQuotaOrWriteBlockedError(mongoErr) {
			if m.logger != nil {
				m.logger.Warn("⚠️ MongoDB storage quota exceeded; media persisted into local SQLite runtime",
					"file_id", doc.FileID,
					"file_name", doc.FileName,
					"error", mongoErr,
				)
			}
			if sqlErr == nil {
				return nil
			}
		}
		return mongoErr
	}

	return sqlErr
}

// BulkSaveMedia saves a batch of documents to MongoDB and SQLite with strict pre-write deduplication.
func (m *DatabaseManager) BulkSaveMedia(ctx context.Context, docs []mongodb.MediaDoc, shardIndex int) (saved int, duplicates int, err error) {
	if len(docs) == 0 {
		return 0, 0, nil
	}

	m.mu.RLock()
	mongoRepo := m.mongoMediaRepo
	sqlRepo := m.sqlMediaRepo
	m.mu.RUnlock()

	// 1. Perform strict batch deduplication check across SQLite and all MongoDB shards
	existMap, checkErr := m.BulkCheckMediaExistsByDocs(ctx, docs)
	if checkErr != nil {
		return 0, 0, fmt.Errorf("pre-write deduplication check failed: %w", checkErr)
	}

	var toInsert []mongodb.MediaDoc
	for _, d := range docs {
		if existMap[d.FileID] {
			duplicates++
		} else {
			toInsert = append(toInsert, d)
		}
	}

	// Always ensure existing docs are cached in local SQLite
	if sqlRepo != nil {
		records := make([]sql.MediaRecord, 0, len(docs))
		for _, d := range docs {
			records = append(records, MediaDocToRecord(d))
		}
		_ = sqlRepo.BulkSaveMedia(ctx, records)
	}

	if len(toInsert) == 0 {
		return 0, duplicates, nil
	}

	var mongoErr error
	if mongoRepo != nil {
		s, d, mErr := mongoRepo.BulkSaveMedia(ctx, toInsert, shardIndex)
		saved = s
		duplicates += d
		mongoErr = mErr
	} else {
		saved = len(toInsert)
	}

	if mongoErr != nil {
		if mongodb.IsQuotaOrWriteBlockedError(mongoErr) {
			if m.logger != nil {
				m.logger.Warn("⚠️ MongoDB storage quota exceeded during bulk save; cached batch into SQLite runtime",
					"records", len(toInsert),
					"error", mongoErr,
				)
			}
			return saved, duplicates, nil
		}
		return saved, duplicates, mongoErr
	}

	return saved, duplicates, nil
}

// BulkSaveMediaAlternating saves docs in alternating batches with strict pre-write deduplication.
func (m *DatabaseManager) BulkSaveMediaAlternating(ctx context.Context, docs []mongodb.MediaDoc, chunkSize int) (saved int, duplicates int, err error) {
	if len(docs) == 0 {
		return 0, 0, nil
	}

	m.mu.RLock()
	mongoRepo := m.mongoMediaRepo
	sqlRepo := m.sqlMediaRepo
	m.mu.RUnlock()

	// 1. Perform strict batch deduplication check across SQLite and all MongoDB shards
	existMap, checkErr := m.BulkCheckMediaExistsByDocs(ctx, docs)
	if checkErr != nil {
		return 0, 0, fmt.Errorf("pre-write deduplication check failed: %w", checkErr)
	}

	var toInsert []mongodb.MediaDoc
	for _, d := range docs {
		if existMap[d.FileID] {
			duplicates++
		} else {
			toInsert = append(toInsert, d)
		}
	}

	// Always ensure existing docs are cached in local SQLite
	if sqlRepo != nil {
		records := make([]sql.MediaRecord, 0, len(docs))
		for _, d := range docs {
			records = append(records, MediaDocToRecord(d))
		}
		_ = sqlRepo.BulkSaveMedia(ctx, records)
	}

	if len(toInsert) == 0 {
		return 0, duplicates, nil
	}

	if chunkSize <= 0 {
		chunkSize = 100
	}

	var mongoErr error
	if mongoRepo != nil {
		s, d, mErr := mongoRepo.BulkSaveMediaAlternating(ctx, toInsert, chunkSize)
		saved = s
		duplicates += d
		mongoErr = mErr
	} else {
		saved = len(toInsert)
	}

	if mongoErr != nil {
		if mongodb.IsQuotaOrWriteBlockedError(mongoErr) {
			if m.logger != nil {
				m.logger.Warn("⚠️ MongoDB storage quota exceeded during alternating bulk save; cached batch into SQLite runtime",
					"records", len(toInsert),
					"error", mongoErr,
				)
			}
			return saved, duplicates, nil
		}
		return saved, duplicates, mongoErr
	}

	return saved, duplicates, nil
}
