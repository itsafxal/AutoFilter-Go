package search

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
)

func setupTestDB(t *testing.T) (*database.DatabaseManager, *Service) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_search.sqlite3")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		SQLiteDBPath:  dbPath,
		SyncBatchSize: 100,
	}
	dbManager := database.NewDatabaseManager(cfg, logger)

	if err := dbManager.Init(context.Background()); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	// Insert test media records
	db := dbManager.SQLManager().DB()
	records := []struct {
		id       string
		name     string
		size     int64
		sizeStr  string
		fileType string
		caption  string
	}{
		{"file_1", "KGF.Chapter.1.2018.1080p.mkv", 2147483648, "2.0 GB", "video", "KGF Chapter 1 (2018)"},
		{"file_2", "KGF.Chapter.2.2022.1080p.mkv", 2684354560, "2.5 GB", "video", "KGF Chapter 2 (2022)"},
		{"file_3", "KGF.Chapter.2.2022.720p.Hindi.mkv", 1073741824, "1.0 GB", "video", "KGF 2 Hindi Dubbed"},
		{"file_4", "Leo.2023.1080p.Tamil.mkv", 2147483648, "2.0 GB", "video", "Leo 2023 Vijay"},
		{"file_5", "Avatar.The.Way.Of.Water.2022.mkv", 3221225472, "3.0 GB", "video", "Avatar 2"},
	}

	for _, r := range records {
		_, err := db.Exec(`
			INSERT INTO media (file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
		`, r.id, "ref", r.name, r.size, r.sizeStr, r.fileType, "video/mp4", r.caption, time.Now())
		if err != nil {
			t.Fatalf("failed to insert test record: %v", err)
		}
	}

	service := NewService(dbManager, logger)
	return dbManager, service
}

func TestSearchService_Search(t *testing.T) {
	dbManager, service := setupTestDB(t)
	defer dbManager.Close()

	ctx := context.Background()

	// 1. Exact query "kgf"
	res, err := service.Search(ctx, "kgf", 10, 0, SearchFilter{})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(res.Files) != 3 {
		t.Errorf("expected 3 files for 'kgf', got %d", len(res.Files))
	}
	if res.Total != 3 {
		t.Errorf("expected total 3, got %d", res.Total)
	}

	// 2. Multi-token query "kgf 2"
	res2, err := service.Search(ctx, "kgf 2", 10, 0, SearchFilter{})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(res2.Files) != 2 {
		t.Errorf("expected 2 files for 'kgf 2', got %d", len(res2.Files))
	}

	// 3. Filter by language "hindi"
	res3, err := service.Search(ctx, "kgf", 10, 0, SearchFilter{Language: "hindi"})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(res3.Files) != 1 {
		t.Errorf("expected 1 file for 'kgf' in hindi, got %d", len(res3.Files))
	}

	// 4. Pagination
	res4, err := service.Search(ctx, "kgf", 1, 0, SearchFilter{})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(res4.Files) != 1 {
		t.Errorf("expected 1 file with limit 1, got %d", len(res4.Files))
	}
	if !res4.HasNext {
		t.Errorf("expected HasNext true for limit 1 of 3")
	}
	if res4.NextOffset != 1 {
		t.Errorf("expected NextOffset 1, got %d", res4.NextOffset)
	}
}

func TestSearchService_GetSuggestions(t *testing.T) {
	dbManager, service := setupTestDB(t)
	defer dbManager.Close()

	ctx := context.Background()

	// Query for typo "kgff"
	sugs, err := service.GetSuggestions(ctx, "kgff", 5)
	if err != nil {
		t.Fatalf("get suggestions error: %v", err)
	}
	if len(sugs) == 0 {
		t.Errorf("expected suggestions for 'kgff', got 0")
	}
}
