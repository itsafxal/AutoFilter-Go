package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver (CGO-free)
)

// DBManager manages the local, temporary SQLite runtime database.
type DBManager struct {
	dbPath    string
	logger    *slog.Logger
	db        *sql.DB
	writeLock sync.Mutex
	closed    bool
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

	// Open connection with pure-Go driver
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-64000)&_pragma=temp_store(MEMORY)&_pragma=foreign_keys(ON)", m.dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Configure pool parameters
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
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
