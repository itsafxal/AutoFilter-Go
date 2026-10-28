package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver (CGO-free)
)

// DBManager manages the local, temporary SQLite runtime database.
type DBManager struct {
	dbPath             string
	logger             *slog.Logger
	db                 *sql.DB
	writeLock          sync.Mutex
	closed             bool
	maintenanceRunning bool
	stopMaintenance    chan struct{}
}

// NewDBManager creates a new SQLite DBManager.
func NewDBManager(dbPath string, logger *slog.Logger) *DBManager {
	if dbPath == "" {
		dbPath = "media.sqlite3"
	}
	return &DBManager{
		dbPath: dbPath,
		logger: logger,
	}
}

// Open opens the SQLite connection and applies high-performance PRAGMAs (WAL mode).
func (m *DBManager) Open(ctx context.Context) error {
	m.logger.Info("opening temporary SQLite runtime database...", "path", m.dbPath)

	// Open connection with pure-Go driver tuned for lean, memory-efficient VPS operation and auto-vacuum
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-8000)&_pragma=temp_store(FILE)&_pragma=mmap_size(16777216)&_pragma=auto_vacuum(INCREMENTAL)&_pragma=wal_autocheckpoint(1000)", m.dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Configure pool parameters: keep connection count small for pure-Go SQLite to conserve VPS RAM
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(1 * time.Hour)

	// Verify connection
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping SQLite database: %w", err)
	}

	m.db = db

	// Apply schema migrations
	if err := m.initSchema(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to initialize SQLite schema: %w", err)
	}

	m.logger.Info("SQLite database initialized successfully with WAL mode", "path", m.dbPath)

	// Launch background maintenance worker (WAL truncation & disk space reclamation)
	m.StartMaintenance(context.Background())

	return nil
}

// Close gracefully closes the SQLite database connection.
func (m *DBManager) Close() error {
	m.writeLock.Lock()
	defer m.writeLock.Unlock()

	if m.closed || m.db == nil {
		return nil
	}
	m.closed = true

	if m.maintenanceRunning && m.stopMaintenance != nil {
		close(m.stopMaintenance)
		m.maintenanceRunning = false
	}

	m.logger.Info("closing SQLite database connection...", "path", m.dbPath)
	return m.db.Close()
}

// DB returns the underlying sql.DB instance.
func (m *DBManager) DB() *sql.DB {
	return m.db
}

// ExecWrite executes a write operation under a mutex to serialize SQLite writes safely.
func (m *DBManager) ExecWrite(fn func(tx *sql.Tx) error) error {
	m.writeLock.Lock()
	defer m.writeLock.Unlock()

	if m.closed || m.db == nil {
		return errors.New("database is closed")
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// CheckpointWAL forces WAL checkpointing with TRUNCATE mode, flushing all journaled changes
// into the database file and shrinking media.sqlite3-wal back to 0 bytes.
func (m *DBManager) CheckpointWAL(ctx context.Context) error {
	m.writeLock.Lock()
	defer m.writeLock.Unlock()

	if m.closed || m.db == nil {
		return nil
	}

	_, err := m.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE);")
	return err
}

// IncrementalVacuum reclaims unused free pages from disk without rebuilding tables.
func (m *DBManager) IncrementalVacuum(ctx context.Context) error {
	m.writeLock.Lock()
	defer m.writeLock.Unlock()

	if m.closed || m.db == nil {
		return nil
	}

	_, err := m.db.ExecContext(ctx, "PRAGMA incremental_vacuum;")
	return err
}

// OptimizeFTS merges fragmented FTS5 B-tree index segments and purges deleted posting lists.
func (m *DBManager) OptimizeFTS(ctx context.Context) error {
	m.writeLock.Lock()
	defer m.writeLock.Unlock()

	if m.closed || m.db == nil {
		return nil
	}

	_, err := m.db.ExecContext(ctx, "INSERT INTO media_fts(media_fts) VALUES('optimize');")
	return err
}

// Vacuum defragments and reclaims all unused physical disk space from the database file.
func (m *DBManager) Vacuum(ctx context.Context) error {
	m.writeLock.Lock()
	defer m.writeLock.Unlock()

	if m.closed || m.db == nil {
		return nil
	}

	_, err := m.db.ExecContext(ctx, "VACUUM;")
	return err
}

// StartMaintenance launches a periodic background worker that controls SQLite disk size:
// - Checkpoints WAL every 15 minutes with TRUNCATE mode so .wal file stays minimal and never balloons
// - Reclaims deleted free pages back to disk via incremental_vacuum every 15 minutes without copying tables
// - Optimizes FTS5 trigram indexes periodically (every 6 hours)
// - Avoids recursive full VACUUM cycles that duplicate file storage and trigger VPS OOM kills
func (m *DBManager) StartMaintenance(ctx context.Context) {
	m.writeLock.Lock()
	if m.maintenanceRunning {
		m.writeLock.Unlock()
		return
	}
	m.maintenanceRunning = true
	m.stopMaintenance = make(chan struct{})
	m.writeLock.Unlock()

	go func() {
		// Run routine every 15 minutes
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		var cycles int
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopMaintenance:
				return
			case <-ticker.C:
				cycles++
				opCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

				// 1. Force WAL truncation to prevent .wal file from growing
				if err := m.CheckpointWAL(opCtx); err != nil {
					m.logger.Debug("wal checkpoint error", "error", err)
				}

				// 2. Reclaim free pages back to disk incrementally (CGO/pure-Go safe, zero copy)
				if err := m.IncrementalVacuum(opCtx); err != nil {
					m.logger.Debug("incremental vacuum error", "error", err)
				}

				// 3. Optimize FTS5 trigram indexes every 6 hours (cycle 24)
				if cycles%24 == 0 {
					if err := m.OptimizeFTS(opCtx); err != nil {
						m.logger.Debug("fts5 optimize error", "error", err)
					}
				}

				// 4. If WAL file grows beyond 50MB between cycles, force truncate
				if m.isWalLarge(50 * 1024 * 1024) {
					_ = m.CheckpointWAL(opCtx)
				}

				cancel()
			}
		}
	}()
}

func (m *DBManager) isWalLarge(thresholdBytes int64) bool {
	if m.dbPath == "" {
		return false
	}
	fiWal, err := os.Stat(m.dbPath + "-wal")
	if err == nil && fiWal.Size() > thresholdBytes {
		return true
	}
	return false
}
