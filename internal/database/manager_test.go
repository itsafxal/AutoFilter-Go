package database

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDatabaseManager_StandaloneLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	sqlitePath := filepath.Join(tmpDir, "test_mgr.sqlite3")

	cfg := &config.Config{
		DatabaseURI:  "", // Standalone mode (no MongoDB)
		SQLiteDBPath: sqlitePath,
	}

	logger := newTestLogger()
	mgr := NewDatabaseManager(cfg, logger)

	ctx := context.Background()

	// Initial state
	if mgr.IsReady() {
		t.Error("expected manager not to be ready before Init")
	}

	// Init
	if err := mgr.Init(ctx); err != nil {
		t.Fatalf("failed to init DatabaseManager: %v", err)
	}

	// State after init
	if !mgr.IsReady() {
		t.Error("expected manager to be ready after Init")
	}
	if mgr.SqlMediaRepo() == nil {
		t.Error("expected SqlMediaRepo to be initialized")
	}
	if !mgr.IsSyncDone() {
		t.Error("expected IsSyncDone to be true after Init in standalone mode")
	}

	syncRes, err := mgr.WaitForSync(ctx)
	if err != nil {
		t.Errorf("expected WaitForSync not to return error, got %v", err)
	}
	if syncRes == nil || !syncRes.Skipped {
		t.Errorf("expected skipped syncResult in standalone mode, got %+v", syncRes)
	}

	// Close
	if err := mgr.Close(); err != nil {
		t.Fatalf("failed to close DatabaseManager: %v", err)
	}

	// State after close
	if mgr.IsReady() {
		t.Error("expected manager not to be ready after Close")
	}
}

func TestDatabaseManager_WaitForSync_Timeout(t *testing.T) {
	cfg := &config.Config{
		DatabaseURI:  "",
		SQLiteDBPath: ":memory:",
	}
	logger := newTestLogger()
	mgr := NewDatabaseManager(cfg, logger)

	// Before Init, sync is not completed yet
	if mgr.IsSyncDone() {
		t.Error("expected IsSyncDone to be false before Init")
	}

	timeoutCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err := mgr.WaitForSync(timeoutCtx)
	if err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
}
