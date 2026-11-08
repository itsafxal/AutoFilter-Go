package search

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itsafxal/AutoFilter-Go/internal/config"
	"github.com/itsafxal/AutoFilter-Go/internal/database"
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
		{"file_11", "Jailer 2 2026 Tamil HQ HDRip 1080p x264.mkv", 2147483648, "2.0 GB", "video", "Jailer 2 Rajinikanth"},
		{"file_12", "Jailer 2.mkv", 1073741824, "1.0 GB", "video", "Jailer 2"},
		{"file_13", "Jailer 3 2028 Tamil.mkv", 2147483648, "2.0 GB", "video", "Jailer 3"},
		{"file_14", "I.Am.Game.2023.1080p.mkv", 1610612736, "1.5 GB", "video", "I Am Game 2023"},
		{"file_15", "Inception.2010.1080p.mkv", 2147483648, "2.0 GB", "video", "Inception Christopher Nolan"},
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

func TestSearchService_IntelligentMovieSearch(t *testing.T) {
	dbManager, service := setupTestDB(t)
	defer dbManager.Close()

	ctx := context.Background()

	// 1. Mandatory Bug Case: "Jailer2" must return "Jailer 2" directly in normal search results
	t.Run("Jailer2 finds Jailer 2 directly", func(t *testing.T) {
		res, err := service.Search(ctx, "Jailer2", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Files) != 2 {
			t.Fatalf("expected 2 files for 'Jailer2', got %d", len(res.Files))
		}
		for _, f := range res.Files {
			if f.FileID != "file_11" && f.FileID != "file_12" {
				t.Errorf("unexpected file returned for 'Jailer2': %s (%s)", f.FileID, f.FileName)
			}
		}
	})

	// 2. Formatting variations of Jailer 2
	variations := []string{"Jailer 2", "Jailer-2", "Jailer_2", "JAILER2", "jailer2"}
	for _, v := range variations {
		t.Run("Variation: "+v, func(t *testing.T) {
			res, err := service.Search(ctx, v, 10, 0, SearchFilter{})
			if err != nil {
				t.Fatalf("Search error for %q: %v", v, err)
			}
			if len(res.Files) != 2 {
				t.Errorf("expected 2 files for %q, got %d", v, len(res.Files))
			}
		})
	}

	// 3. Strict Sequel Isolation: "Jailer2" and "Jailer 2" MUST NEVER return "Jailer 3"
	t.Run("Jailer 2 rejects Jailer 3", func(t *testing.T) {
		res, err := service.Search(ctx, "Jailer2", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		for _, f := range res.Files {
			if f.FileID == "file_13" {
				t.Fatalf("CRITICAL: Jailer2 matched Jailer 3 (file_13)!")
			}
		}
	})

	// 4. Space and symbol variations: "I Am Game"
	gameVariations := []string{
		"I Am Game",
		"IAmGame",
		"I.Am.Game",
		"I-Am-Game",
		"I_Am_Game",
		"I  Am  Game",
		"I•Am•Game",
		"I@Am@Game",
	}
	for _, gv := range gameVariations {
		t.Run("Game Variation: "+gv, func(t *testing.T) {
			res, err := service.Search(ctx, gv, 10, 0, SearchFilter{})
			if err != nil {
				t.Fatalf("Search error for %q: %v", gv, err)
			}
			if len(res.Files) != 1 || res.Files[0].FileID != "file_14" {
				t.Errorf("expected file_14 for %q, got %d files: %v", gv, len(res.Files), res.Files)
			}
		})
	}

	// 5. Acronym / Sequel matching: "KGF2" and "K.G.F"
	t.Run("KGF2 matches KGF 2 prints", func(t *testing.T) {
		res, err := service.Search(ctx, "KGF2", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Files) != 2 {
			t.Errorf("expected 2 files for 'KGF2', got %d", len(res.Files))
		}
		for _, f := range res.Files {
			if f.FileID != "file_2" && f.FileID != "file_3" {
				t.Errorf("expected file_2 or file_3, got %s (%s)", f.FileID, f.FileName)
			}
		}
	})

	t.Run("K.G.F matches KGF movies", func(t *testing.T) {
		res, err := service.Search(ctx, "K.G.F", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Files) < 2 {
			t.Errorf("expected at least 2 files for 'K.G.F', got %d", len(res.Files))
		}
	})

	// 6. Typo matching: "Inceptin" finds "Inception"
	t.Run("Inceptin finds Inception", func(t *testing.T) {
		res, err := service.Search(ctx, "Inceptin", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Files) != 1 || res.Files[0].FileID != "file_15" {
			t.Errorf("expected file_15 for 'Inceptin', got %d files: %v", len(res.Files), res.Files)
		}
	})

	// 7. Unicode Transliteration Search
	t.Run("Transliterated Malayalam query finds correct movie", func(t *testing.T) {
		resVV, err := service.Search(ctx, "വിക്രം വേധ", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(resVV.Files) != 1 || resVV.Files[0].FileID != "file_9" {
			t.Errorf("expected file_9 (Vikram Vedha) for 'വിക്രം വേധ', got %v", resVV.Files)
		}

		resV, err := service.Search(ctx, "വിക്രം", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(resV.Files) != 2 {
			t.Fatalf("expected 2 files for 'വിക്രം', got %d", len(resV.Files))
		}
		for _, f := range resV.Files {
			if f.FileID != "file_6" && f.FileID != "file_7" {
				t.Errorf("unexpected file returned for 'വിക്രം': %s (%s)", f.FileID, f.FileName)
			}
		}
	})

	// 8. Candidate Retrieval Fallback when initial FTS returns non-matching candidates
	t.Run("Caption fallback when filename FTS returns candidate that fails strict matching", func(t *testing.T) {
		db := dbManager.SQLManager().DB()
		// file_irr matches FTS trigram for "dune" in filename, but is a series episode / unrelated
		_, _ = db.Exec(`
			INSERT INTO media (file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved)
			VALUES ('file_irr', 'ref', 'DuneReviewEpisode4Discussion.mkv', 104857600, '100 MB', 'video', 'video/mp4', 'Review talk', datetime('now'));
		`)
		// file_cap has random filename, but caption contains the actual movie "Dune Part Two 2024 1080p"
		_, _ = db.Exec(`
			INSERT INTO media (file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved)
			VALUES ('file_cap', 'ref', 'VID_20240301_001.mkv', 2147483648, '2.0 GB', 'video', 'video/mp4', 'Dune Part Two 2024 1080p BluRay', datetime('now'));
		`)

		resDune, err := service.Search(ctx, "Dune Part Two", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(resDune.Files) != 1 || resDune.Files[0].FileID != "file_cap" {
			t.Errorf("expected file_cap for 'Dune Part Two' via supplementary caption fallback, got %v", resDune.Files)
		}
	})
}

func TestSearch_ProductionVerificationSuite(t *testing.T) {
	dbManager, service := setupTestDB(t)
	defer dbManager.Close()

	ctx := context.Background()

	// 1. Jailer2 = Jailer 2
	t.Run("Jailer2 = Jailer 2", func(t *testing.T) {
		res, err := service.Search(ctx, "Jailer2", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Files) != 2 {
			t.Fatalf("expected 2 files for 'Jailer2', got %d", len(res.Files))
		}
		for _, f := range res.Files {
			if f.FileID != "file_11" && f.FileID != "file_12" {
				t.Errorf("expected Jailer 2 files (file_11, file_12), got %s (%s)", f.FileID, f.FileName)
			}
		}
	})

	// 2. IAmGame = I Am Game
	t.Run("IAmGame = I Am Game", func(t *testing.T) {
		res, err := service.Search(ctx, "IAmGame", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Files) != 1 || res.Files[0].FileID != "file_14" {
			t.Errorf("expected file_14 (I Am Game), got %v", res.Files)
		}
	})

	// 3. I.Am.Game = I Am Game
	t.Run("I.Am.Game = I Am Game", func(t *testing.T) {
		res, err := service.Search(ctx, "I.Am.Game", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Files) != 1 || res.Files[0].FileID != "file_14" {
			t.Errorf("expected file_14 (I Am Game), got %v", res.Files)
		}
	})

	// 4. I@Am@Game = I Am Game
	t.Run("I@Am@Game = I Am Game", func(t *testing.T) {
		res, err := service.Search(ctx, "I@Am@Game", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Files) != 1 || res.Files[0].FileID != "file_14" {
			t.Errorf("expected file_14 (I Am Game), got %v", res.Files)
		}
	})

	// 5. KGF2 = KGF 2
	t.Run("KGF2 = KGF 2", func(t *testing.T) {
		res, err := service.Search(ctx, "KGF2", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Files) != 2 {
			t.Fatalf("expected 2 files for 'KGF2', got %d", len(res.Files))
		}
		for _, f := range res.Files {
			if f.FileID != "file_2" && f.FileID != "file_3" {
				t.Errorf("expected KGF Chapter 2 files (file_2, file_3), got %s (%s)", f.FileID, f.FileName)
			}
		}
	})

	// 6. KGF2 must not return KGF 1
	t.Run("KGF2 must not return KGF 1", func(t *testing.T) {
		res, err := service.Search(ctx, "KGF2", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		for _, f := range res.Files {
			if f.FileID == "file_1" {
				t.Fatalf("REGRESSION: KGF2 matched KGF 1 (file_1: %s)!", f.FileName)
			}
		}
	})

	// 7. Jailer 2 must not return Jailer 3
	t.Run("Jailer 2 must not return Jailer 3", func(t *testing.T) {
		for _, q := range []string{"Jailer 2", "Jailer2"} {
			res, err := service.Search(ctx, q, 10, 0, SearchFilter{})
			if err != nil {
				t.Fatalf("Search error for %q: %v", q, err)
			}
			for _, f := range res.Files {
				if f.FileID == "file_13" {
					t.Fatalf("REGRESSION: %q matched Jailer 3 (file_13: %s)!", q, f.FileName)
				}
			}
		}
	})

	// 8. Vikram must not return Vikram Vedha
	t.Run("Vikram must not return Vikram Vedha", func(t *testing.T) {
		res, err := service.Search(ctx, "Vikram", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		for _, f := range res.Files {
			if f.FileID == "file_9" {
				t.Fatalf("REGRESSION: 'Vikram' matched Vikram Vedha (file_9: %s)!", f.FileName)
			}
			if f.FileID == "file_8" {
				t.Fatalf("REGRESSION: 'Vikram' matched Vikramadithyan (file_8: %s)!", f.FileName)
			}
		}
	})
}

func TestSearchService_ImGame_LanguageAndGroupingSuite(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "imgame_test.db")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		SQLiteDBPath:  dbPath,
		SyncBatchSize: 100,
	}
	dbManager := database.NewDatabaseManager(cfg, logger)
	if err := dbManager.Init(context.Background()); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer dbManager.Close()

	db := dbManager.SQLManager().DB()
	records := []struct {
		id       string
		name     string
		size     int64
		sizeStr  string
		fileType string
		caption  string
	}{
		{"file_img_mal_4k", "I'm.Game.2026.Malayalam.2160p.4K.HDR.mkv", 6500000000, "6.5 GB", "video", "I'm Game 2026 Malayalam 4K"},
		{"file_img_mal_1080", "I'm.Game.2026.Malayalam.1080p.WEBRip.x264.DD+5.1.mkv", 2500000000, "2.5 GB", "video", "I'm Game Malayalam 1080p"},
		{"file_img_mal_720", "I'm.Game.2026.Malayalam.720p.HEVC.x265.mkv", 1200000000, "1.2 GB", "video", "I'm Game Malayalam 720p"},
		{"file_img_mal_480", "I'm.Game.2026.Malayalam.480p.x264.mkv", 600000000, "600 MB", "video", "I'm Game Malayalam 480p"},
		{"file_img_tel_1080", "I'm.Game.2026.Telugu.1080p.WEBRip.mkv", 2300000000, "2.3 GB", "video", "I'm Game Telugu 1080p"},
		{"file_img_tel_720", "I'm.Game.2026.Telugu.720p.mkv", 1100000000, "1.1 GB", "video", "I'm Game Telugu 720p"},
		{"file_jailer_2", "Jailer 2 2026 Tamil HQ HDRip 1080p x264.mkv", 2147483648, "2.0 GB", "video", "Jailer 2 Rajinikanth"},
		{"file_jailer_3", "Jailer 3 2028 Tamil.mkv", 2147483648, "2.0 GB", "video", "Jailer 3"},
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
	ctx := context.Background()

	// 1. Mandatory 14 query variations of "I'm Game"
	imGameVariations := []string{
		"I'm Game",
		"i'm game",
		"I Am Game",
		"i am game",
		"I'm Game Malayalam",
		"I Am Game Malayalam",
		"i'm game malayalam",
		"I.Am.Game",
		"I-Am-Game",
		"I_Am_Game",
		"IAmGame",
		"I@Am@Game",
		"Im Game",
		"I m Game",
	}

	for _, query := range imGameVariations {
		t.Run("QueryVariation_"+query, func(t *testing.T) {
			res, err := service.Search(ctx, query, 10, 0, SearchFilter{})
			if err != nil {
				t.Fatalf("Search failed for query %q: %v", query, err)
			}
			if len(res.Files) == 0 {
				t.Fatalf("No files returned for query %q", query)
			}
			if len(res.Movies) != 1 {
				t.Fatalf("Expected exactly 1 canonical movie for %q, got %d", query, len(res.Movies))
			}
			if res.Movies[0].CompactTitle != "iamgame" {
				t.Errorf("Expected movie CompactTitle to be 'iamgame', got %q", res.Movies[0].CompactTitle)
			}

			// Ensure Jailer is never mixed in
			for _, f := range res.Files {
				if f.FileID == "file_jailer_2" || f.FileID == "file_jailer_3" {
					t.Fatalf("Unrelated movie %s returned for query %q", f.FileName, query)
				}
			}
		})
	}

	// 2. Language prioritization when Malayalam is requested
	t.Run("LanguagePrioritization_Malayalam", func(t *testing.T) {
		res, err := service.Search(ctx, "I'm Game Malayalam", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if res.RequestedLanguage != "malayalam" {
			t.Errorf("Expected RequestedLanguage 'malayalam', got %q", res.RequestedLanguage)
		}
		if !res.LanguageMatched {
			t.Errorf("Expected LanguageMatched to be true")
		}
		if len(res.Movies) != 1 {
			t.Fatalf("Expected 1 canonical movie, got %d", len(res.Movies))
		}
		movie := res.Movies[0]
		if len(movie.LanguageVariants) < 2 {
			t.Fatalf("Expected at least 2 language variants, got %d", len(movie.LanguageVariants))
		}
		// First variant MUST be Malayalam
		if movie.LanguageVariants[0].NormalizedLanguage != "malayalam" {
			t.Errorf("Expected first variant to be Malayalam, got %q", movie.LanguageVariants[0].NormalizedLanguage)
		}
		if len(movie.LanguageVariants[0].Files) != 4 {
			t.Errorf("Expected 4 files in Malayalam variant, got %d", len(movie.LanguageVariants[0].Files))
		}
	})

	// 3. Language prioritization when Telugu is requested
	t.Run("LanguagePrioritization_Telugu", func(t *testing.T) {
		res, err := service.Search(ctx, "I'm Game Telugu", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if res.RequestedLanguage != "telugu" {
			t.Errorf("Expected RequestedLanguage 'telugu', got %q", res.RequestedLanguage)
		}
		if !res.LanguageMatched {
			t.Errorf("Expected LanguageMatched to be true")
		}
		movie := res.Movies[0]
		// First variant MUST be Telugu
		if movie.LanguageVariants[0].NormalizedLanguage != "telugu" {
			t.Errorf("Expected first variant to be Telugu, got %q", movie.LanguageVariants[0].NormalizedLanguage)
		}
		if len(movie.LanguageVariants[0].Files) != 2 {
			t.Errorf("Expected 2 files in Telugu variant, got %d", len(movie.LanguageVariants[0].Files))
		}
	})

	// 4. Missing Language notification behavior
	t.Run("LanguageMissingNotification_Hindi", func(t *testing.T) {
		res, err := service.Search(ctx, "I'm Game Hindi", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if res.RequestedLanguage != "hindi" {
			t.Errorf("Expected RequestedLanguage 'hindi', got %q", res.RequestedLanguage)
		}
		if res.LanguageMatched {
			t.Errorf("Expected LanguageMatched to be false because Hindi is not available")
		}
		if len(res.Movies) != 1 {
			t.Fatalf("Expected 1 canonical movie found, got %d", len(res.Movies))
		}
		// AvailableLanguages must contain Malayalam and Telugu, but NOT Hindi
		foundMal := false
		foundTel := false
		foundHin := false
		for _, l := range res.AvailableLanguages {
			if strings.EqualFold(l, "Malayalam") {
				foundMal = true
			}
			if strings.EqualFold(l, "Telugu") {
				foundTel = true
			}
			if strings.EqualFold(l, "Hindi") {
				foundHin = true
			}
		}
		if !foundMal || !foundTel {
			t.Errorf("Expected AvailableLanguages to list Malayalam and Telugu, got %v", res.AvailableLanguages)
		}
		if foundHin {
			t.Errorf("AvailableLanguages should NOT contain Hindi, got %v", res.AvailableLanguages)
		}
	})

	// 5. Preserving original file IDs and filenames
	t.Run("OriginalMetadataPreservation", func(t *testing.T) {
		res, err := service.Search(ctx, "I'm Game", 10, 0, SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		fileMap := make(map[string]string)
		for _, f := range res.Files {
			fileMap[f.FileID] = f.FileName
		}

		expectedFiles := map[string]string{
			"file_img_mal_4k":   "I'm.Game.2026.Malayalam.2160p.4K.HDR.mkv",
			"file_img_mal_1080": "I'm.Game.2026.Malayalam.1080p.WEBRip.x264.DD+5.1.mkv",
			"file_img_mal_720":  "I'm.Game.2026.Malayalam.720p.HEVC.x265.mkv",
			"file_img_mal_480":  "I'm.Game.2026.Malayalam.480p.x264.mkv",
			"file_img_tel_1080": "I'm.Game.2026.Telugu.1080p.WEBRip.mkv",
			"file_img_tel_720":  "I'm.Game.2026.Telugu.720p.mkv",
		}

		for id, expectedName := range expectedFiles {
			actualName, ok := fileMap[id]
			if !ok {
				t.Errorf("Missing file ID %s in search results", id)
			} else if actualName != expectedName {
				t.Errorf("File ID %s has modified filename: got %q, want %q", id, actualName, expectedName)
			}
		}
	})
}
