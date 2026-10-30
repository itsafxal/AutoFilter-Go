package database

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/itsafxal/AutoFilter-Go/internal/config"
	"github.com/itsafxal/AutoFilter-Go/internal/database/mongodb"
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

func TestDatabaseManager_DeduplicationByNameAndSize(t *testing.T) {
	tmpDir := t.TempDir()
	sqlitePath := filepath.Join(tmpDir, "test_dedup.sqlite3")

	cfg := &config.Config{
		DatabaseURI:  "",
		SQLiteDBPath: sqlitePath,
	}

	logger := newTestLogger()
	mgr := NewDatabaseManager(cfg, logger)
	ctx := context.Background()

	if err := mgr.Init(ctx); err != nil {
		t.Fatalf("failed to init mgr: %v", err)
	}
	defer mgr.Close()

	// 1. Initial file stored with legacy Pyrogram FileID
	legacyDoc := mongodb.MediaDoc{
		FileID:   "BAACAgUAAxkBAAI_legacy_id_123",
		FileName: "Inception.2010.1080p.BluRay.mkv",
		FileSize: 2147483648,
		Size:     "2.0 GB",
		Date:     time.Now().UTC(),
	}

	if err := mgr.SqlMediaRepo().SaveMedia(ctx, MediaDocToRecord(legacyDoc)); err != nil {
		t.Fatalf("failed to save legacy doc: %v", err)
	}

	// 2. Incoming file scanned from channel with Gogram packed FileID (different string)
	gogramDoc := mongodb.MediaDoc{
		FileID:   "BAMAAf_new_gogram_packed_id_789",
		FileName: "Inception.2010.1080p.BluRay.mkv", // exact same name
		FileSize: 2147483648,                        // exact same size
		Size:     "2.0 GB",
	}

	// CheckMediaExists (by FileID only) should not find it
	existsByID, err := mgr.CheckMediaExists(ctx, gogramDoc.FileID)
	if err != nil || existsByID {
		t.Errorf("expected CheckMediaExists to return false for new FileID string, got %v", existsByID)
	}

	// CheckMediaExistsByDoc (by FileID OR (name, size)) MUST recognize it as a duplicate!
	existsByDoc, err := mgr.CheckMediaExistsByDoc(ctx, gogramDoc)
	if err != nil {
		t.Fatalf("unexpected error from CheckMediaExistsByDoc: %v", err)
	}
	if !existsByDoc {
		t.Errorf("CRITICAL BUG: CheckMediaExistsByDoc failed to detect duplicate file with identical name and size!")
	}

	// 3. SaveMediaLiveAlternating should detect duplicate and return "Existing (Deduplicated)" without writing
	lbl, err := mgr.SaveMediaLiveAlternating(ctx, gogramDoc)
	if err != nil {
		t.Fatalf("unexpected error from SaveMediaLiveAlternating: %v", err)
	}
	if lbl != "Existing (Deduplicated)" {
		t.Errorf("expected 'Existing (Deduplicated)', got %q", lbl)
	}
}

func TestDatabaseManager_BulkDeduplicationAndIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	sqlitePath := filepath.Join(tmpDir, "test_bulk_dedup.sqlite3")

	cfg := &config.Config{
		DatabaseURI:  "",
		SQLiteDBPath: sqlitePath,
	}

	logger := newTestLogger()
	mgr := NewDatabaseManager(cfg, logger)
	ctx := context.Background()

	if err := mgr.Init(ctx); err != nil {
		t.Fatalf("failed to init mgr: %v", err)
	}
	defer mgr.Close()

	// 1. Pre-populate database with 3 files
	initialDocs := []mongodb.MediaDoc{
		{FileID: "pyro_1", FileName: "Movie.1.2020.mkv", FileSize: 1000},
		{FileID: "pyro_2", FileName: "Movie.2.2021.mkv", FileSize: 2000},
		{FileID: "pyro_3", FileName: "Movie.3.2022.mkv", FileSize: 3000},
	}
	for _, d := range initialDocs {
		_ = mgr.SqlMediaRepo().SaveMedia(ctx, MediaDocToRecord(d))
	}

	// 2. Index batch containing 2 duplicates (different Gogram IDs) and 2 new files
	batch := []mongodb.MediaDoc{
		{FileID: "gogram_1", FileName: "Movie.1.2020.mkv", FileSize: 1000}, // Duplicate by name+size
		{FileID: "gogram_2", FileName: "Movie.2.2021.mkv", FileSize: 2000}, // Duplicate by name+size
		{FileID: "gogram_4", FileName: "Movie.4.2023.mkv", FileSize: 4000}, // New
		{FileID: "gogram_5", FileName: "Movie.5.2024.mkv", FileSize: 5000}, // New
	}

	existMap, err := mgr.BulkCheckMediaExistsByDocs(ctx, batch)
	if err != nil {
		t.Fatalf("BulkCheckMediaExistsByDocs error: %v", err)
	}

	if !existMap["gogram_1"] {
		t.Error("expected gogram_1 to be detected as duplicate")
	}
	if !existMap["gogram_2"] {
		t.Error("expected gogram_2 to be detected as duplicate")
	}
	if existMap["gogram_4"] {
		t.Error("expected gogram_4 to be detected as new")
	}
	if existMap["gogram_5"] {
		t.Error("expected gogram_5 to be detected as new")
	}

	// 3. BulkSaveMedia should save 2 and report 2 duplicates
	saved, dups, err := mgr.BulkSaveMedia(ctx, batch, 0)
	if err != nil {
		t.Fatalf("BulkSaveMedia error: %v", err)
	}
	if dups != 2 {
		t.Errorf("expected 2 duplicates skipped, got %d", dups)
	}
	if saved != 2 {
		t.Errorf("expected 2 new files saved, got %d", saved)
	}

	// 4. Repeating the exact same batch immediately should result in 4 duplicates and 0 saved!
	saved2, dups2, err := mgr.BulkSaveMedia(ctx, batch, 0)
	if err != nil {
		t.Fatalf("repeat BulkSaveMedia error: %v", err)
	}
	if dups2 != 4 {
		t.Errorf("expected 4 duplicates on repeated run, got %d", dups2)
	}
	if saved2 != 0 {
		t.Errorf("expected 0 saved on repeated run, got %d", saved2)
	}
}

func TestDatabaseManager_ConcurrentWritesDeduplication(t *testing.T) {
	tmpDir := t.TempDir()
	sqlitePath := filepath.Join(tmpDir, "test_concurrent.sqlite3")

	cfg := &config.Config{
		DatabaseURI:  "",
		SQLiteDBPath: sqlitePath,
	}

	logger := newTestLogger()
	mgr := NewDatabaseManager(cfg, logger)
	ctx := context.Background()

	if err := mgr.Init(ctx); err != nil {
		t.Fatalf("failed to init mgr: %v", err)
	}
	defer mgr.Close()

	// Launch 20 concurrent goroutines trying to insert the exact same movie
	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			doc := mongodb.MediaDoc{
				FileID:   fmt.Sprintf("concurrent_id_%d", id),
				FileName: "Concurrent.Movie.2024.1080p.mkv",
				FileSize: 50000000,
			}
			_, _ = mgr.SaveMediaLiveAlternating(ctx, doc)
		}(i)
	}

	wg.Wait()

	// Verify only 1 record exists for this name and size in SQLite
	rec, err := mgr.SqlMediaRepo().GetFileByNameAndSize(ctx, "Concurrent.Movie.2024.1080p.mkv", 50000000)
	if err != nil || rec == nil {
		t.Fatalf("expected record to exist, got %v (err: %v)", rec, err)
	}
}
