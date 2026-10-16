package sql

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/database/mongodb"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSQLite_InitAndPragmas(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_media.sqlite3")

	mgr := NewDBManager(dbPath, newTestLogger())
	ctx := context.Background()

	if err := mgr.Open(ctx); err != nil {
		t.Fatalf("failed to open SQLite: %v", err)
	}
	defer mgr.Close()

	// Verify journal_mode is WAL
	var journalMode string
	err := mgr.DB().QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode wal, got %s", journalMode)
	}

	// Verify media table exists
	var tableName string
	err = mgr.DB().QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='media';").Scan(&tableName)
	if err != nil || tableName != "media" {
		t.Errorf("media table was not created: %v", err)
	}

	// Verify sync_metadata table exists
	err = mgr.DB().QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='sync_metadata';").Scan(&tableName)
	if err != nil || tableName != "sync_metadata" {
		t.Errorf("sync_metadata table was not created: %v", err)
	}
}

func TestSQLite_DuplicateHandling(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_dup.sqlite3")

	mgr := NewDBManager(dbPath, newTestLogger())
	ctx := context.Background()

	if err := mgr.Open(ctx); err != nil {
		t.Fatalf("failed to open SQLite: %v", err)
	}
	defer mgr.Close()

	// Insert record 1
	err := mgr.ExecWrite(func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO media (file_id, file_name, file_size) VALUES (?, ?, ?);
		`, "file_001", "Old Movie Name", 1000)
		return err
	})
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// Insert duplicate record with updated title
	err = mgr.ExecWrite(func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO media (file_id, file_name, file_size) VALUES (?, ?, ?);
		`, "file_001", "New Updated Movie Name", 2000)
		return err
	})
	if err != nil {
		t.Fatalf("duplicate replace failed: %v", err)
	}

	// Verify count is still 1
	var count int64
	_ = mgr.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM media;").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 record after duplicate replace, got %d", count)
	}

	// Verify name was updated
	var name string
	_ = mgr.DB().QueryRowContext(ctx, "SELECT file_name FROM media WHERE file_id = 'file_001';").Scan(&name)
	if name != "New Updated Movie Name" {
		t.Errorf("expected updated title, got %s", name)
	}
}

func TestSQLite_SyncAndIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_sync.sqlite3")

	mgr := NewDBManager(dbPath, newTestLogger())
	ctx := context.Background()

	if err := mgr.Open(ctx); err != nil {
		t.Fatalf("failed to open SQLite: %v", err)
	}
	defer mgr.Close()

	syncer := NewSyncer(mgr, nil, newTestLogger(), 2)

	// Manually insert sample test records using commitBatch
	sampleDocs := []mongodb.MediaDoc{
		{FileID: "m1", FileName: "Inception 2010 1080p", FileSize: 2000000, Size: "2.0 GB", Caption: "Inception Movie"},
		{FileID: "m2", FileName: "Interstellar 2014 720p", FileSize: 1500000, Size: "1.5 GB", Caption: "Interstellar Movie"},
		{FileID: "m3", FileName: "Oppenheimer 2023 2160p", FileSize: 8000000, Size: "8.0 GB", Caption: "Oppenheimer 4K"},
	}

	insertSQL := `INSERT OR REPLACE INTO media (file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`
	err := syncer.commitBatch(ctx, insertSQL, sampleDocs)
	if err != nil {
		t.Fatalf("failed to commit sample batch: %v", err)
	}

	_ = syncer.setSyncMetadata(ctx, "is_complete", "true")

	// Verify sync is marked complete
	complete, err := syncer.IsSyncComplete(ctx)
	if err != nil || !complete {
		t.Errorf("expected sync to be complete, got %v (err: %v)", complete, err)
	}

	// Verify row count
	count, err := syncer.GetTotalRows(ctx)
	if err != nil || count != 3 {
		t.Errorf("expected 3 rows, got %d (err: %v)", count, err)
	}

	// Test repository lookups
	repo := NewMediaRepository(mgr)
	rec, err := repo.GetFileDetails(ctx, "m2")
	if err != nil || rec == nil {
		t.Fatalf("expected to find record m2: %v", err)
	}
	if rec.FileName != "Interstellar 2014 720p" {
		t.Errorf("unexpected file name: %s", rec.FileName)
	}

	// Test repository search
	results, total, err := repo.Search(ctx, "Interstellar", 10, 0)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if total != 1 || len(results) != 1 || results[0].FileID != "m2" {
		t.Errorf("expected search match for Interstellar, got %d matches", total)
	}
}

func TestSQLite_RollbackOnError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_rollback.sqlite3")

	mgr := NewDBManager(dbPath, newTestLogger())
	ctx := context.Background()

	if err := mgr.Open(ctx); err != nil {
		t.Fatalf("failed to open SQLite: %v", err)
	}
	defer mgr.Close()

	// Attempt write that returns an error
	err := mgr.ExecWrite(func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO media (file_id, file_name, file_size) VALUES (?, ?, ?);", "good_001", "Movie", 100)
		if err != nil {
			return err
		}
		// Trigger an intentional error to force rollback
		return errors.New("simulated network or disk failure")
	})

	if err == nil {
		t.Fatal("expected error from ExecWrite, got nil")
	}

	// Verify good_001 was NOT committed due to rollback
	var count int64
	_ = mgr.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM media WHERE file_id = 'good_001';").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}
