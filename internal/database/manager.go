package database

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

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
	}
}

// Init initializes MongoDB, opens SQLite, and executes initial media synchronization.
// The bot will not report database as ready until all required initialization and sync have succeeded.
func (m *DatabaseManager) Init(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Initialize temporary SQLite runtime
	if err := m.sqlManager.Open(ctx); err != nil {
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
		} else if m.cfg.BackgroundSync {
			// Check if already populated to skip immediately
			isComplete, _ := m.syncer.IsSyncComplete(ctx)
			hasData, _ := m.syncer.HasData(ctx)
			count, _ := m.syncer.GetTotalRows(ctx)
			if (isComplete && hasData) || (hasData && count >= 500000) {
				m.logger.Info("SQLite media table is already populated and synchronized; skipping full migration",
					"existing_records", count,
				)
			} else {
				m.logger.Info("⚡ Launching concurrent media migration in background to enable instant bot boot and rapid deployment...")
				go func() {
					bgCtx := context.Background()
					syncRes, err := m.syncer.Sync(bgCtx)
					if err != nil {
						m.logger.Error("background synchronization from MongoDB to SQLite failed", "error", err)
						return
					}
					m.logger.Info("🎉 Background media synchronization completed successfully",
						"total_records", syncRes.TotalInserted,
						"skipped", syncRes.Skipped,
						"duration", syncRes.Duration.String(),
					)
				}()
			}
		} else {
			syncRes, err := m.syncer.Sync(ctx)
			if err != nil {
				m.logger.Error("synchronization from MongoDB to SQLite failed", "error", err)
				_ = m.Close()
				return fmt.Errorf("runtime database synchronization failed: %w", err)
			}

			m.logger.Info("database initialization and synchronization verified",
				"total_records", syncRes.TotalInserted,
				"skipped", syncRes.Skipped,
				"duration", syncRes.Duration.String(),
			)
		}
	} else {
		m.logger.Warn("DATABASE_URI not configured: running in standalone mode without authoritative MongoDB")
	}

	m.ready = true
	return nil
}

// Close gracefully closes SQLite and disconnects MongoDB clients.
func (m *DatabaseManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

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
