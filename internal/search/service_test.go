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
		{"file_6", "[CinemaCourt] Vikram (2022) 1080p WEB-DL.mkv", 2147483648, "2.0 GB", "video", "Vikram 2022 CinemaCourt"},
		{"file_7", "Cinemacourt Vikram 2022 720p.mkv", 1073741824, "1.0 GB", "video", "Vikram 2022 720p"},
		{"file_8", "Vikramadithyan (2014) Malayalam 720p HDRip.mkv", 1073741824, "1.0 GB", "video", "Vikramadithyan Dulquer"},
		{"file_9", "Vikram Vedha (2017) Tamil 1080p.mkv", 2147483648, "2.0 GB", "video", "Vikram Vedha Madhavan"},
		{"file_10", "Hargadimaju Season 3 Episode 6 Vikram Thilak.mkv", 536870912, "500 MB", "video", "Hargadimaju S03E06"},
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

	// 5. Strict movie matching for "Vikram":
	// MUST return ONLY the 2 Vikram prints (file_6, file_7).
	// MUST REJECT Vikramadithyan (file_8), Vikram Vedha (file_9), and Hargadimaju (file_10).
	resVikram, err := service.Search(ctx, "Vikram", 10, 0, SearchFilter{})
	if err != nil {
		t.Fatalf("search error for Vikram: %v", err)
	}
	if len(resVikram.Files) != 2 {
		t.Errorf("expected exactly 2 files for 'Vikram', got %d", len(resVikram.Files))
		for _, f := range resVikram.Files {
			t.Logf("unexpected returned file: %s", f.FileName)
		}
	}
	for _, f := range resVikram.Files {
		if f.FileID != "file_6" && f.FileID != "file_7" {
			t.Errorf("expected only Vikram files (file_6, file_7), got unexpected file %s (%s)", f.FileID, f.FileName)
		}
	}

	// 6. Search for "Vikram Vedha":
	// MUST return ONLY the Vikram Vedha movie (file_9), and NOT plain Vikram.
	resVV, err := service.Search(ctx, "Vikram Vedha", 10, 0, SearchFilter{})
	if err != nil {
		t.Fatalf("search error for Vikram Vedha: %v", err)
	}
	if len(resVV.Files) != 1 || resVV.Files[0].FileID != "file_9" {
		t.Errorf("expected only file_9 for 'Vikram Vedha', got %v", resVV.Files)
	}

	// 7. Search for "Vikramadithyan":
	// MUST return ONLY file_8.
	resVA, err := service.Search(ctx, "Vikramadithyan", 10, 0, SearchFilter{})
	if err != nil {
		t.Fatalf("search error for Vikramadithyan: %v", err)
	}
	if len(resVA.Files) != 1 || resVA.Files[0].FileID != "file_8" {
		t.Errorf("expected only file_8 for 'Vikramadithyan', got %v", resVA.Files)
	}

	// 8. Search for "Hargadimaju":
	// MUST return ONLY file_10.
	resHarg, err := service.Search(ctx, "Hargadimaju", 10, 0, SearchFilter{})
	if err != nil {
		t.Fatalf("search error for Hargadimaju: %v", err)
	}
	if len(resHarg.Files) != 1 || resHarg.Files[0].FileID != "file_10" {
		t.Errorf("expected only file_10 for 'Hargadimaju', got %v", resHarg.Files)
	}

	// 9. Filter for "Vikram" with Quality 720p:
	// MUST return ONLY file_7 (720p), NOT file_6 (1080p).
	resVikram720, err := service.Search(ctx, "Vikram", 10, 0, SearchFilter{Quality: "720p"})
	if err != nil {
		t.Fatalf("search error for Vikram 720p: %v", err)
	}
	if len(resVikram720.Files) != 1 || resVikram720.Files[0].FileID != "file_7" {
		t.Errorf("expected only file_7 for 'Vikram' 720p, got %d files: %v", len(resVikram720.Files), resVikram720.Files)
	}

	// 10. Filter for "Vikram" with Quality 480p (non-existent):
	// MUST return 0 files and NOT fall back to main results.
	resVikram480, err := service.Search(ctx, "Vikram", 10, 0, SearchFilter{Quality: "480p"})
	if err != nil {
		t.Fatalf("search error for Vikram 480p: %v", err)
	}
	if len(resVikram480.Files) != 0 {
		t.Errorf("expected 0 files for 'Vikram' 480p, got %d files", len(resVikram480.Files))
	}

	// 11. Filter for "Vikram" with Language "hindi" (non-existent):
	// MUST return 0 files and NOT fall back to main results.
	resVikramHindi, err := service.Search(ctx, "Vikram", 10, 0, SearchFilter{Language: "hindi"})
	if err != nil {
		t.Fatalf("search error for Vikram hindi: %v", err)
	}
	if len(resVikramHindi.Files) != 0 {
		t.Errorf("expected 0 files for 'Vikram' hindi, got %d files", len(resVikramHindi.Files))
	}

	// 12. Filter for "Leo" (Tamil 1080p) with Language "hindi":
	// MUST return 0 files.
	resLeoHindi, err := service.Search(ctx, "Leo", 10, 0, SearchFilter{Language: "hindi"})
	if err != nil {
		t.Fatalf("search error for Leo hindi: %v", err)
	}
	if len(resLeoHindi.Files) != 0 {
		t.Errorf("expected 0 files for 'Leo' hindi, got %d files", len(resLeoHindi.Files))
	}

	// 13. Filter for "Leo" (1080p) with Quality "720p":
	// MUST return 0 files.
	resLeo720, err := service.Search(ctx, "Leo", 10, 0, SearchFilter{Quality: "720p"})
	if err != nil {
		t.Fatalf("search error for Leo 720p: %v", err)
	}
	if len(resLeo720.Files) != 0 {
		t.Errorf("expected 0 files for 'Leo' 720p, got %d files", len(resLeo720.Files))
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
