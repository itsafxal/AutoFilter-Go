package tests

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/app"
	"github.com/itsafxal/AutoFilter-Go/internal/config"
	deliveryCore "github.com/itsafxal/AutoFilter-Go/internal/delivery"
	"github.com/itsafxal/AutoFilter-Go/internal/logging"
	"github.com/itsafxal/AutoFilter-Go/internal/search"
	"github.com/itsafxal/AutoFilter-Go/plugins/autofilter"
	"github.com/itsafxal/AutoFilter-Go/plugins/delivery"
)

type stagingMockSender struct {
	sentMessages []stagingSentMessage
}

type stagingSentMessage struct {
	peerID any
	media  any
}

func (s *stagingMockSender) SendMedia(peerID any, media any, opts ...*telegram.MediaOptions) (*telegram.NewMessage, error) {
	s.sentMessages = append(s.sentMessages, stagingSentMessage{peerID: peerID, media: media})
	return &telegram.NewMessage{ID: 1001}, nil
}

func (s *stagingMockSender) GetMessages(peerID any, opts ...*telegram.SearchOption) ([]telegram.NewMessage, error) {
	return []telegram.NewMessage{}, nil
}

func TestStagingE2E_ControlledEnvironment(t *testing.T) {
	logBuf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelDebug, logBuf)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "staging_media.sqlite3")

	cfg := &config.Config{
		BotToken:             "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		AppID:                12345,
		AppHash:              "0123456789abcdef0123456789abcdef",
		OwnerID:              99887766,
		Admins:               []int64{99887766},
		LogLevel:             config.LogLevelDebug,
		SQLiteDBPath:         dbPath,
		SyncBatchSize:        1000,
		EnablePremiumEmoji:   true,
		AutoDeleteFilterTime: "15m",
		AutoDeleteFileTime:   "30m",
	}

	botApp, err := app.New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to initialize App: %v", err)
	}
	defer botApp.Shutdown()

	if err := botApp.InitDatabase(context.Background()); err != nil {
		t.Fatalf("failed to init database: %v", err)
	}

	// Register AutoFilter and Delivery plugins
	afPlugin := autofilter.New(botApp.AppContext)
	delPlugin := delivery.New(botApp.AppContext)

	if err := botApp.RegisterPlugins(afPlugin, delPlugin); err != nil {
		t.Fatalf("failed to register plugins: %v", err)
	}
	if err := botApp.ValidatePlugins(); err != nil {
		t.Fatalf("failed to validate plugins: %v", err)
	}

	mockSender := &stagingMockSender{}
	botApp.DeliveryService.SetSender(mockSender)

	ctx := context.Background()
	sqlDb := botApp.DBManager.SQLManager().DB()

	// Seed staging SQLite database with production-representative records
	records := []struct {
		id       string
		name     string
		size     int64
		sizeStr  string
		fileType string
		caption  string
	}{
		// I'm Game (2026) - 4 Malayalam, 2 Telugu
		{"file_img_mal_4k", "I'm.Game.2026.Malayalam.2160p.4K.HDR.mkv", 6500000000, "6.5 GB", "video", "I'm Game 2026 Malayalam 4K"},
		{"file_img_mal_1080", "I'm.Game.2026.Malayalam.1080p.WEBRip.x264.DD+5.1.mkv", 2500000000, "2.5 GB", "video", "I'm Game Malayalam 1080p"},
		{"file_img_mal_720", "I'm.Game.2026.Malayalam.720p.HEVC.x265.mkv", 1200000000, "1.2 GB", "video", "I'm Game Malayalam 720p"},
		{"file_img_mal_480", "I'm.Game.2026.Malayalam.480p.x264.mkv", 600000000, "600 MB", "video", "I'm Game Malayalam 480p"},
		{"file_img_tel_1080", "I'm.Game.2026.Telugu.1080p.WEBRip.mkv", 2300000000, "2.3 GB", "video", "I'm Game Telugu 1080p"},
		{"file_img_tel_720", "I'm.Game.2026.Telugu.720p.mkv", 1100000000, "1.1 GB", "video", "I'm Game Telugu 720p"},

		// Jailer franchise
		{"file_jailer_2", "Jailer 2 2026 Tamil HQ HDRip 1080p x264.mkv", 2147483648, "2.0 GB", "video", "Jailer 2 Rajinikanth"},
		{"file_jailer_2_simple", "Jailer 2.mkv", 1073741824, "1.0 GB", "video", "Jailer 2"},
		{"file_jailer_3", "Jailer 3 2028 Tamil.mkv", 2147483648, "2.0 GB", "video", "Jailer 3"},

		// Vikram franchise discrimination
		{"file_vikram", "[CinemaCourt] Vikram (2022) 1080p WEB-DL.mkv", 2147483648, "2.0 GB", "video", "Vikram 2022 CinemaCourt"},
		{"file_vikram_vedha", "Vikram Vedha (2017) Tamil 1080p.mkv", 2147483648, "2.0 GB", "video", "Vikram Vedha Madhavan"},
		{"file_vikramadithyan", "Vikramadithyan (2014) Malayalam 720p HDRip.mkv", 1073741824, "1.0 GB", "video", "Vikramadithyan Dulquer"},
	}

	for _, r := range records {
		_, err := sqlDb.Exec(`
			INSERT INTO media (file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
		`, r.id, "ref_"+r.id, r.name, r.size, r.sizeStr, r.fileType, "video/mp4", r.caption, time.Now())
		if err != nil {
			t.Fatalf("failed to insert staging media record: %v", err)
		}
	}

	searchService := botApp.SearchService

	// =========================================================================
	// TEST 1 & 2: Search variations of "I'm Game" -> all resolve to same canonical movie
	// =========================================================================
	t.Run("Item1_and_2_ImGameVariations", func(t *testing.T) {
		queries := []string{
			"I'm Game",
			"I Am Game",
			"I.Am.Game",
			"IAmGame",
			"I'm Game Malayalam",
		}

		for _, q := range queries {
			start := time.Now()
			res, err := searchService.Search(ctx, q, 10, 0, search.SearchFilter{})
			duration := time.Since(start)

			if err != nil {
				t.Fatalf("Search failed for %q: %v", q, err)
			}
			if len(res.Movies) != 1 {
				t.Fatalf("Query %q expected 1 canonical movie, got %d", q, len(res.Movies))
			}
			movie := res.Movies[0]
			if movie.CompactTitle != "iamgame" {
				t.Errorf("Query %q: expected CompactTitle 'iamgame', got %q", q, movie.CompactTitle)
			}
			if movie.Year != 2026 {
				t.Errorf("Query %q: expected Year 2026, got %d", q, movie.Year)
			}
			if movie.TotalFiles != 6 {
				t.Errorf("Query %q: expected TotalFiles 6, got %d", q, movie.TotalFiles)
			}
			if duration > 50*time.Millisecond {
				t.Errorf("Query %q exceeded latency threshold: took %v", q, duration)
			}
		}
	})

	// =========================================================================
	// TEST 3: Confirm Malayalam and Telugu variants are separated correctly
	// =========================================================================
	t.Run("Item3_LanguageVariantsSeparated", func(t *testing.T) {
		res, err := searchService.Search(ctx, "I'm Game Malayalam", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Movies) != 1 {
			t.Fatalf("expected 1 canonical movie, got %d", len(res.Movies))
		}
		movie := res.Movies[0]
		if len(movie.LanguageVariants) != 2 {
			t.Fatalf("expected exactly 2 variants (Malayalam & Telugu), got %d", len(movie.LanguageVariants))
		}

		// Variant 0: Malayalam (since requested)
		malVariant := movie.LanguageVariants[0]
		if malVariant.NormalizedLanguage != "malayalam" {
			t.Errorf("expected first variant to be Malayalam, got %q", malVariant.NormalizedLanguage)
		}
		if len(malVariant.Files) != 4 {
			t.Errorf("expected 4 Malayalam files, got %d", len(malVariant.Files))
		}

		// Variant 1: Telugu
		telVariant := movie.LanguageVariants[1]
		if telVariant.NormalizedLanguage != "telugu" {
			t.Errorf("expected second variant to be Telugu, got %q", telVariant.NormalizedLanguage)
		}
		if len(telVariant.Files) != 2 {
			t.Errorf("expected 2 Telugu files, got %d", len(telVariant.Files))
		}
	})

	// =========================================================================
	// TEST 4: Confirm Hindi produces missing-language notification
	// =========================================================================
	t.Run("Item4_MissingLanguageNotification_Hindi", func(t *testing.T) {
		res, err := searchService.Search(ctx, "I'm Game Hindi", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if res.RequestedLanguage != "hindi" {
			t.Errorf("expected RequestedLanguage 'hindi', got %q", res.RequestedLanguage)
		}
		if res.LanguageMatched {
			t.Errorf("expected LanguageMatched to be false")
		}
		if len(res.Movies) != 1 {
			t.Fatalf("expected 1 canonical movie found, got %d", len(res.Movies))
		}

		// Verify available languages listed
		availStr := strings.Join(res.AvailableLanguages, ",")
		if !strings.Contains(availStr, "Malayalam") || !strings.Contains(availStr, "Telugu") {
			t.Errorf("expected AvailableLanguages to contain Malayalam & Telugu, got %v", res.AvailableLanguages)
		}
		if strings.Contains(availStr, "Hindi") {
			t.Errorf("AvailableLanguages must not contain Hindi, got %v", res.AvailableLanguages)
		}
	})

	// =========================================================================
	// TEST 5: One movie appears as one grouped result (not duplicate movie entries)
	// =========================================================================
	t.Run("Item5_OneGroupedMovieResult", func(t *testing.T) {
		res, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(res.Movies) != 1 {
			t.Fatalf("expected exactly 1 grouped movie result, got %d", len(res.Movies))
		}
		if res.Movies[0].TotalFiles != 6 {
			t.Errorf("expected 6 total files grouped under the single movie, got %d", res.Movies[0].TotalFiles)
		}
	})

	// =========================================================================
	// TEST 6: Open the movie and test every available file option
	// =========================================================================
	t.Run("Item6_MovieDetailAndFileOptions", func(t *testing.T) {
		res, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{})
		if err != nil || len(res.Movies) == 0 {
			t.Fatalf("failed to search: %v", err)
		}
		movie := res.Movies[0]

		// Variant 0: Malayalam
		for _, f := range movie.LanguageVariants[0].Files {
			optLabel := search.FormatFileOptionLabel(f)
			if optLabel == "" {
				t.Errorf("file option label empty for %s", f.FileName)
			}
		}

		// Variant 1: Telugu
		for _, f := range movie.LanguageVariants[1].Files {
			optLabel := search.FormatFileOptionLabel(f)
			if optLabel == "" {
				t.Errorf("file option label empty for %s", f.FileName)
			}
		}
	})

	// =========================================================================
	// TEST 7: Selected file delivers exact original Telegram media record
	// =========================================================================
	t.Run("Item7_ActualDelivery_PreservesOriginalMedia", func(t *testing.T) {
		for _, r := range records {
			delivered, err := botApp.DeliveryService.Deliver(ctx, 99887766, r.id, deliveryCore.DeliveryOptions{})
			if err != nil {
				t.Fatalf("Delivery failed for file %s: %v", r.id, err)
			}
			if delivered.FileID != r.id {
				t.Errorf("delivered FileID mismatch: got %q, want %q", delivered.FileID, r.id)
			}
			if delivered.FileName != r.name {
				t.Errorf("delivered FileName mismatch: got %q, want %q", delivered.FileName, r.name)
			}
			if delivered.FileSize != r.size {
				t.Errorf("delivered FileSize mismatch: got %d, want %d", delivered.FileSize, r.size)
			}
			if delivered.FileType != r.fileType {
				t.Errorf("delivered FileType mismatch: got %q, want %q", delivered.FileType, r.fileType)
			}
		}

		if len(mockSender.sentMessages) != len(records) {
			t.Errorf("expected %d media sends, got %d", len(records), len(mockSender.sentMessages))
		}
	})

	// =========================================================================
	// TEST 8: Pagination, language switching, back navigation, and callback handlers
	// =========================================================================
	t.Run("Item8_Navigation_Callbacks", func(t *testing.T) {
		// Verify AutoFilter registered all required navigation and movie selection callbacks
		cbHandlers := afPlugin.CallbackHandlers()
		cbMap := make(map[string]bool)
		for _, h := range cbHandlers {
			cbMap[h.Prefix] = true
		}

		requiredPrefixes := []string{"next|", "mv#", "back_mv#", "mv_p#", "languages#", "fl#", "qualities#", "fq#", "back_all#", "back#", "close_data"}
		for _, pfx := range requiredPrefixes {
			if !cbMap[pfx] {
				t.Errorf("AutoFilter plugin missing required callback prefix: %q", pfx)
			}
		}

		// Verify Delivery registered file callback
		delHandlers := delPlugin.CallbackHandlers()
		delMap := make(map[string]bool)
		for _, h := range delHandlers {
			delMap[h.Prefix] = true
		}
		if !delMap["file#"] {
			t.Errorf("Delivery plugin missing required 'file#' callback prefix")
		}

		// Verify search result pagination offsets for navigation
		res, err := searchService.Search(ctx, "I'm Game", 2, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if !res.HasNext {
			t.Errorf("expected HasNext to be true when total files (6) > limit (2)")
		}
		if res.NextOffset != 2 {
			t.Errorf("expected NextOffset 2, got %d", res.NextOffset)
		}
	})

	// =========================================================================
	// TEST 9: Jailer 2 vs Jailer 3, and Vikram vs Vikram Vedha
	// =========================================================================
	t.Run("Item9_StrictDiscrimination", func(t *testing.T) {
		// Jailer 2 must not return Jailer 3
		j2Res, err := searchService.Search(ctx, "Jailer 2", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		for _, f := range j2Res.Files {
			if f.FileID == "file_jailer_3" {
				t.Fatalf("REGRESSION: 'Jailer 2' matched Jailer 3 (file_jailer_3: %s)", f.FileName)
			}
		}

		// Jailer 3 must not return Jailer 2
		j3Res, err := searchService.Search(ctx, "Jailer 3", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		for _, f := range j3Res.Files {
			if f.FileID == "file_jailer_2" || f.FileID == "file_jailer_2_simple" {
				t.Fatalf("REGRESSION: 'Jailer 3' matched Jailer 2 (file: %s)", f.FileName)
			}
		}

		// Vikram must not return Vikram Vedha or Vikramadithyan
		vikramRes, err := searchService.Search(ctx, "Vikram", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		for _, f := range vikramRes.Files {
			if f.FileID == "file_vikram_vedha" {
				t.Fatalf("REGRESSION: 'Vikram' matched Vikram Vedha (file_vikram_vedha: %s)", f.FileName)
			}
			if f.FileID == "file_vikramadithyan" {
				t.Fatalf("REGRESSION: 'Vikram' matched Vikramadithyan (file_vikramadithyan: %s)", f.FileName)
			}
		}
	})

	// =========================================================================
	// TEST 10: SQLite and Fallback Behavior
	// =========================================================================
	t.Run("Item10_SQLiteAndFallbackBehavior", func(t *testing.T) {
		// Fast SQLite search
		res, err := searchService.Search(ctx, "Vikram", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("SQLite search failed: %v", err)
		}
		if len(res.Files) == 0 {
			t.Fatalf("Expected files from SQLite FTS5 for 'Vikram'")
		}
		if res.Files[0].FileID != "file_vikram" {
			t.Errorf("Expected file_vikram, got %s", res.Files[0].FileID)
		}

		// Case where SQLite returns no match: search non-existent movie
		missingRes, err := searchService.Search(ctx, "NonExistentMovie9999", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search for missing movie failed with error: %v", err)
		}
		if len(missingRes.Files) != 0 || len(missingRes.Movies) != 0 {
			t.Errorf("Expected 0 files for non-existent movie, got files=%d, movies=%d", len(missingRes.Files), len(missingRes.Movies))
		}
	})

	// =========================================================================
	// TEST 11: Search latency, duplicate results, and error logs
	// =========================================================================
	t.Run("Item11_Latency_Duplicates_ErrorLogs", func(t *testing.T) {
		logBuf.Reset()
		testQueries := []string{"I'm Game", "Jailer 2", "Vikram", "I.Am.Game"}

		for _, tq := range testQueries {
			start := time.Now()
			res, err := searchService.Search(ctx, tq, 10, 0, search.SearchFilter{})
			elapsed := time.Since(start)

			if err != nil {
				t.Fatalf("query %q error: %v", tq, err)
			}
			if elapsed > 20*time.Millisecond {
				t.Errorf("search latency too high for %q: %v (limit: 20ms)", tq, elapsed)
			}

			// Check for duplicate FileIDs
			seen := make(map[string]bool)
			for _, f := range res.Files {
				if seen[f.FileID] {
					t.Fatalf("Duplicate file ID %s returned for query %q", f.FileID, tq)
				}
				seen[f.FileID] = true
			}
		}

		// Verify error logs: check that no ERROR level logs were emitted
		logOutput := logBuf.String()
		if strings.Contains(logOutput, "level=ERROR") {
			t.Errorf("detected unexpected ERROR log output:\n%s", logOutput)
		}
	})
}
