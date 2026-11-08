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
		EnablePremiumEmoji:   false,
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

	// Register AutoFilter and Delivery plugins (pure file-list filtering UI)
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

		// Multi-Audio film
		{"file_leo_multi", "Leo.2023.Tamil.Telugu.Hindi.1080p.Dual.Audio.mkv", 3200000000, "3.2 GB", "video", "Leo 2023 Multi Audio"},

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
	// 1. SEARCH NORMALIZATION: Variations resolve to the same matching individual files
	// =========================================================================
	t.Run("Normalization_Variations", func(t *testing.T) {
		queries := []string{
			"I'm Game",
			"i'm game",
			"I Am Game",
			"i am game",
			"I.Am.Game",
			"IAmGame",
			"I-Am-Game",
			"I_Am_Game",
			"I•Am•Game",
			"I@Am@Game",
			"Im Game",
			"I  Am   Game",
		}

		for _, q := range queries {
			start := time.Now()
			res, err := searchService.Search(ctx, q, 10, 0, search.SearchFilter{})
			duration := time.Since(start)

			if err != nil {
				t.Fatalf("Search failed for %q: %v", q, err)
			}
			if len(res.Files) != 6 {
				t.Fatalf("Query %q expected 6 individual files, got %d", q, len(res.Files))
			}
			if duration > 50*time.Millisecond {
				t.Errorf("Query %q exceeded latency threshold: took %v", q, duration)
			}
		}
	})

	// =========================================================================
	// 2. SEARCH MATCHING: Strict discrimination (sequels, titles, false positives)
	// =========================================================================
	t.Run("Matching_StrictDiscrimination", func(t *testing.T) {
		// Jailer 2 must return Jailer 2 files, NOT Jailer 3
		j2Res, err := searchService.Search(ctx, "Jailer 2", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(j2Res.Files) == 0 {
			t.Fatalf("Expected results for Jailer 2")
		}
		for _, f := range j2Res.Files {
			if f.FileID == "file_jailer_3" {
				t.Fatalf("REGRESSION: 'Jailer 2' matched Jailer 3 (%s)", f.FileName)
			}
		}

		// Jailer 3 must return Jailer 3 files, NOT Jailer 2
		j3Res, err := searchService.Search(ctx, "Jailer 3", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		for _, f := range j3Res.Files {
			if f.FileID == "file_jailer_2" || f.FileID == "file_jailer_2_simple" {
				t.Fatalf("REGRESSION: 'Jailer 3' matched Jailer 2 (%s)", f.FileName)
			}
		}

		// Vikram must return Vikram, NOT Vikram Vedha or Vikramadithyan
		vikramRes, err := searchService.Search(ctx, "Vikram", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(vikramRes.Files) == 0 {
			t.Fatalf("Expected results for Vikram")
		}
		for _, f := range vikramRes.Files {
			if f.FileID == "file_vikram_vedha" || f.FileID == "file_vikramadithyan" {
				t.Fatalf("REGRESSION: 'Vikram' matched distinct movie: %s", f.FileName)
			}
		}
	})

	// =========================================================================
	// 3. CANDIDATE RETRIEVAL & FALLBACK LOGIC
	// =========================================================================
	t.Run("CandidateRetrieval_Fallback", func(t *testing.T) {
		// Valid candidate retrieval
		res, err := searchService.Search(ctx, "Vikram", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(res.Files) != 1 || res.Files[0].FileID != "file_vikram" {
			t.Errorf("expected 1 file (file_vikram), got %d files", len(res.Files))
		}

		// Missing query returns 0 files without panic
		missingRes, err := searchService.Search(ctx, "UnknownNonExistentMovie9999", 10, 0, search.SearchFilter{})
		if err != nil {
			t.Fatalf("Missing search error: %v", err)
		}
		if len(missingRes.Files) != 0 {
			t.Errorf("expected 0 files, got %d", len(missingRes.Files))
		}
	})

	// =========================================================================
	// 4. LANGUAGE FILTERING: Filter individual files directly
	// =========================================================================
	t.Run("LanguageFiltering_DirectFileResults", func(t *testing.T) {
		// Filter by Malayalam
		malRes, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{Language: "malayalam"})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(malRes.Files) != 4 {
			t.Fatalf("expected 4 Malayalam files, got %d", len(malRes.Files))
		}
		for _, f := range malRes.Files {
			if !strings.Contains(f.FileName, "Malayalam") {
				t.Errorf("expected file to be Malayalam, got %s", f.FileName)
			}
		}

		// Filter by Telugu
		telRes, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{Language: "telugu"})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(telRes.Files) != 2 {
			t.Fatalf("expected 2 Telugu files, got %d", len(telRes.Files))
		}
		for _, f := range telRes.Files {
			if !strings.Contains(f.FileName, "Telugu") {
				t.Errorf("expected file to be Telugu, got %s", f.FileName)
			}
		}

		// Filter by Hindi (none exist -> 0 files)
		hinRes, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{Language: "hindi"})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(hinRes.Files) != 0 {
			t.Errorf("expected 0 files for Hindi, got %d", len(hinRes.Files))
		}

		// Multi-audio detection
		multiRes, err := searchService.Search(ctx, "Leo", 10, 0, search.SearchFilter{Language: "multi"})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(multiRes.Files) != 1 || multiRes.Files[0].FileID != "file_leo_multi" {
			t.Errorf("expected 1 multi-audio file (file_leo_multi), got %d", len(multiRes.Files))
		}
	})

	// =========================================================================
	// 5. QUALITY FILTERING: Filter individual files directly
	// =========================================================================
	t.Run("QualityFiltering_DirectFileResults", func(t *testing.T) {
		// Filter by 4K (matches 2160p / 4K)
		q4kRes, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{Quality: "4k"})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(q4kRes.Files) != 1 || q4kRes.Files[0].FileID != "file_img_mal_4k" {
			t.Errorf("expected 1 file for 4K (file_img_mal_4k), got %d", len(q4kRes.Files))
		}

		// Filter by 1080P (returns Malayalam 1080p + Telugu 1080p)
		q1080Res, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{Quality: "1080p"})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(q1080Res.Files) != 2 {
			t.Fatalf("expected 2 files for 1080p, got %d", len(q1080Res.Files))
		}

		// Filter by 720P (returns Malayalam 720p + Telugu 720p)
		q720Res, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{Quality: "720p"})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(q720Res.Files) != 2 {
			t.Fatalf("expected 2 files for 720p, got %d", len(q720Res.Files))
		}

		// Filter by 480P (returns Malayalam 480p)
		q480Res, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{Quality: "480p"})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(q480Res.Files) != 1 || q480Res.Files[0].FileID != "file_img_mal_480" {
			t.Errorf("expected 1 file for 480p (file_img_mal_480), got %d", len(q480Res.Files))
		}

		// Combined Language + Quality: Malayalam + 1080p
		comboRes, err := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{Language: "malayalam", Quality: "1080p"})
		if err != nil {
			t.Fatalf("Search error: %v", err)
		}
		if len(comboRes.Files) != 1 || comboRes.Files[0].FileID != "file_img_mal_1080" {
			t.Errorf("expected 1 file for Malayalam 1080p (file_img_mal_1080), got %d", len(comboRes.Files))
		}
	})

	// =========================================================================
	// 6. UI COMPATIBILITY: Pure File-List Filtering Keyboards and Callbacks
	// =========================================================================
	t.Run("UICompatibility_CallbacksAndKeyboards", func(t *testing.T) {
		cbHandlers := afPlugin.CallbackHandlers()
		cbMap := make(map[string]bool)
		for _, h := range cbHandlers {
			cbMap[h.Prefix] = true
		}

		// Verify existing required callbacks are present
		requiredPrefixes := []string{"next|", "sp#", "languages#", "fl#", "qualities#", "fq#", "back_all#", "back#", "close_data", "pages"}
		for _, pfx := range requiredPrefixes {
			if !cbMap[pfx] {
				t.Errorf("AutoFilter plugin missing required callback prefix: %q", pfx)
			}
		}

		// Prohibited callbacks MUST NOT be present
		prohibitedPrefixes := []string{"mv#", "back_mv#", "mv_p#", "mvpage#"}
		for _, pfx := range prohibitedPrefixes {
			if cbMap[pfx] {
				t.Errorf("REGRESSION: Prohibited movie-grouping callback prefix present: %q", pfx)
			}
		}

		// Build result search check
		res, _ := searchService.Search(ctx, "I'm Game", 10, 0, search.SearchFilter{})
		if len(res.Files) != 6 {
			t.Errorf("expected 6 files for I'm Game, got %d", len(res.Files))
		}

		// Verify Delivery plugin registered file# callback
		delHandlers := delPlugin.CallbackHandlers()
		delMap := make(map[string]bool)
		for _, h := range delHandlers {
			delMap[h.Prefix] = true
		}
		if !delMap["file#"] {
			t.Errorf("Delivery plugin missing required 'file#' callback prefix")
		}
	})

	// =========================================================================
	// 7. FILE DELIVERY: Preserves exact original media records
	// =========================================================================
	t.Run("FileDelivery_PreservesOriginalMedia", func(t *testing.T) {
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
	// 8. PERFORMANCE & STABILITY: Latency, Duplicates, and Error Logs
	// =========================================================================
	t.Run("Performance_Latency_Duplicates_ErrorLogs", func(t *testing.T) {
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
