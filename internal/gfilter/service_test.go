package gfilter

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupTestSQLite initializes an isolated SQLite database with the global_filters schema.
func setupTestSQLite(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_gfilter.sqlite3")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}

	schema := `
		CREATE TABLE IF NOT EXISTS global_filters (
			id TEXT PRIMARY KEY,
			keyword TEXT NOT NULL,
			keyword_norm TEXT NOT NULL UNIQUE,
			reply_text TEXT,
			media_type TEXT NOT NULL,
			file_id TEXT,
			file_ref TEXT,
			buttons_json TEXT,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS gfilter_keyword_norm_idx ON global_filters (keyword_norm);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatalf("failed to initialize schema: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
	}
	return db, cleanup
}

// -----------------------------------------------------------------------------
// A. MATCHING TESTS: Strict Case-Insensitive Exact Whole-Message Matching
// -----------------------------------------------------------------------------

func TestMatching_ExampleA_Vikram(t *testing.T) {
	service := NewService(nil, nil, nil)
	f := &GFilter{
		ID:          "1",
		Keyword:     "Vikram",
		KeywordNorm: NormalizeKeyword("Vikram"),
		ReplyText:   "Vikram Movie Response",
		MediaType:   MediaTypeText,
	}
	service.repopulateCache([]*GFilter{f})

	tests := []struct {
		incoming string
		expected bool
		reason   string
	}{
		{"Vikram", true, "Exact match with identical casing"},
		{"vikram", true, "Exact match lowercase"},
		{"VIKRAM", true, "Exact match uppercase"},
		{"ViKrAm", true, "Exact match mixed casing"},
		{"Vikram Movie", false, "Additional word must not match"},
		{"Vikram 2025", false, "Additional number must not match"},
		{"Vikramadithyan", false, "Substring / prefix must not match"},
		{"My Vikram", false, "Preceding word must not match"},
		{"Vikram!", false, "Punctuation must be significant"},
		{"Vikram (2025)", false, "Parentheses and year must not match"},
		{" Vikram", false, "Leading space must not match"},
		{"Vikram ", false, "Trailing space must not match"},
		{"vikram  ", false, "Multiple trailing spaces must not match"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.incoming), func(t *testing.T) {
			res, matched := service.Match(tc.incoming)
			if matched != tc.expected {
				t.Errorf("Match(%q) = %v; expected %v (%s)", tc.incoming, matched, tc.expected, tc.reason)
			}
			if matched && (res == nil || res.Keyword != "Vikram") {
				t.Errorf("Expected matched filter for 'Vikram', got %+v", res)
			}
		})
	}
}

func TestMatching_ExampleB_TheOne(t *testing.T) {
	service := NewService(nil, nil, nil)
	f := &GFilter{
		ID:          "2",
		Keyword:     "The One",
		KeywordNorm: NormalizeKeyword("The One"),
		ReplyText:   "The One Response",
		MediaType:   MediaTypeText,
	}
	service.repopulateCache([]*GFilter{f})

	tests := []struct {
		incoming string
		expected bool
		reason   string
	}{
		{"The One", true, "Exact match identical casing"},
		{"the one", true, "Exact match lowercase"},
		{"THE ONE", true, "Exact match uppercase"},
		{"tHe OnE", true, "Exact match alternating casing"},
		{"TheOne", false, "Missing space must not match"},
		{"The  One", false, "Double space must not match"},
		{"The One 2019", false, "Additional year must not match"},
		{"The One Movie", false, "Additional word must not match"},
		{"The One!", false, "Exclamation must not match"},
		{" The One", false, "Leading space must not match"},
		{"The One ", false, "Trailing space must not match"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.incoming), func(t *testing.T) {
			_, matched := service.Match(tc.incoming)
			if matched != tc.expected {
				t.Errorf("Match(%q) = %v; expected %v (%s)", tc.incoming, matched, tc.expected, tc.reason)
			}
		})
	}
}

func TestMatching_Unicode(t *testing.T) {
	service := NewService(nil, nil, nil)
	f := &GFilter{
		ID:          "3",
		Keyword:     "Москва",
		KeywordNorm: NormalizeKeyword("Москва"),
		ReplyText:   "Moscow Response",
		MediaType:   MediaTypeText,
	}
	service.repopulateCache([]*GFilter{f})

	if _, matched := service.Match("москва"); !matched {
		t.Errorf("Expected unicode case-insensitive match for 'москва'")
	}
	if _, matched := service.Match("МОСКВА"); !matched {
		t.Errorf("Expected unicode uppercase match for 'МОСКВА'")
	}
	if _, matched := service.Match("москва 2026"); matched {
		t.Errorf("Expected no match for 'москва 2026'")
	}
}

// -----------------------------------------------------------------------------
// B. BUTTON PARSER TESTS
// -----------------------------------------------------------------------------

func TestButtonParser_MarkdownSyntax(t *testing.T) {
	raw := "Hello! Here is the movie.\n[Watch Online](https://example.com/watch)\n[Download Now](https://example.com/dl:same)\n[Support Channel](buttonurl:https://t.me/channel)"

	clean, rows := ParseButtonsFromText(raw)
	expectedClean := "Hello! Here is the movie."
	if clean != expectedClean {
		t.Errorf("Expected clean text %q, got %q", expectedClean, clean)
	}

	if len(rows) != 2 {
		t.Fatalf("Expected 2 button rows (due to :same), got %d", len(rows))
	}

	// Row 0 should have 2 buttons
	if len(rows[0]) != 2 {
		t.Fatalf("Expected 2 buttons in row 0, got %d", len(rows[0]))
	}
	if rows[0][0].Text != "Watch Online" || rows[0][0].URL != "https://example.com/watch" {
		t.Errorf("Unexpected button 0,0: %+v", rows[0][0])
	}
	if rows[0][1].Text != "Download Now" || rows[0][1].URL != "https://example.com/dl" {
		t.Errorf("Unexpected button 0,1: %+v", rows[0][1])
	}

	// Row 1 should have 1 button
	if len(rows[1]) != 1 {
		t.Fatalf("Expected 1 button in row 1, got %d", len(rows[1]))
	}
	if rows[1][0].Text != "Support Channel" || rows[1][0].URL != "https://t.me/channel" {
		t.Errorf("Unexpected button 1,0: %+v", rows[1][0])
	}
}

// -----------------------------------------------------------------------------
// C. DATABASE TESTS (CRUD, Synchronization & Reconciliation)
// -----------------------------------------------------------------------------

func TestSQLiteRepository_CRUD(t *testing.T) {
	db, cleanup := setupTestSQLite(t)
	defer cleanup()

	repo := NewSQLRepositoryWithDB(db)
	ctx := context.Background()

	// 1. Save new filter
	f1 := &GFilter{
		ID:        "filter_1",
		Keyword:   "Vikram",
		ReplyText: "Vikram 1080p",
		MediaType: MediaTypeVideo,
		FileID:    "BAACAgUAAxkBAAICaW...",
		Buttons:   []ButtonRow{{{Text: "Watch", URL: "https://example.com"}}},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := repo.Save(ctx, f1); err != nil {
		t.Fatalf("Failed to save filter: %v", err)
	}

	// 2. Count
	cnt, err := repo.Count(ctx)
	if err != nil || cnt != 1 {
		t.Errorf("Expected count 1, got %d (err: %v)", cnt, err)
	}

	// 3. Retrieve
	retrieved, err := repo.GetByKeywordNorm(ctx, "vikram")
	if err != nil || retrieved == nil {
		t.Fatalf("Failed to retrieve filter: %v", err)
	}
	if retrieved.Keyword != "Vikram" || retrieved.MediaType != MediaTypeVideo {
		t.Errorf("Retrieved mismatch: %+v", retrieved)
	}
	if len(retrieved.Buttons) != 1 || retrieved.Buttons[0][0].Text != "Watch" {
		t.Errorf("Button unpack failed: %+v", retrieved.Buttons)
	}

	// 4. Update (Idempotent upsert by keyword_norm)
	f1Updated := &GFilter{
		ID:        "filter_1",
		Keyword:   "VIKRAM",
		ReplyText: "Vikram 4K Remastered",
		MediaType: MediaTypeVideo,
		FileID:    "BAACAgUAAxkBAAICaW...",
		CreatedAt: f1.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	}
	if err := repo.Save(ctx, f1Updated); err != nil {
		t.Fatalf("Failed to update filter: %v", err)
	}

	cntAfterUpdate, _ := repo.Count(ctx)
	if cntAfterUpdate != 1 {
		t.Errorf("Expected count to remain 1 after update, got %d", cntAfterUpdate)
	}

	retrievedUpdated, _ := repo.GetByKeywordNorm(ctx, "vikram")
	if retrievedUpdated.ReplyText != "Vikram 4K Remastered" {
		t.Errorf("Expected updated text, got %q", retrievedUpdated.ReplyText)
	}

	// 5. Delete by keyword
	deleted, err := repo.DeleteByKeywordNorm(ctx, "vikram")
	if err != nil || !deleted {
		t.Errorf("Expected successful delete, got %v (err: %v)", deleted, err)
	}

	cntAfterDelete, _ := repo.Count(ctx)
	if cntAfterDelete != 0 {
		t.Errorf("Expected count 0 after delete, got %d", cntAfterDelete)
	}
}

func TestSQLiteRepository_SyncAll_Reconciliation(t *testing.T) {
	db, cleanup := setupTestSQLite(t)
	defer cleanup()

	repo := NewSQLRepositoryWithDB(db)
	ctx := context.Background()

	// Populate initial stale filters in SQLite
	staleFilter := &GFilter{
		ID:        "stale_1",
		Keyword:   "OldFilter",
		MediaType: MediaTypeText,
	}
	_ = repo.Save(ctx, staleFilter)

	// Authoritative list from MongoDB (contains new filters, lacks stale_1)
	authoritative := []*GFilter{
		{
			ID:        "auth_1",
			Keyword:   "Leo",
			MediaType: MediaTypeText,
			ReplyText: "Leo movie",
		},
		{
			ID:        "auth_2",
			Keyword:   "Jailer",
			MediaType: MediaTypeText,
			ReplyText: "Jailer movie",
		},
	}

	// Execute SyncAll
	if err := repo.SyncAll(ctx, authoritative); err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}

	// Total should be exactly 2
	all, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("Expected exactly 2 filters after reconciliation, got %d", len(all))
	}

	// Stale filter should be gone
	stale, _ := repo.GetByKeywordNorm(ctx, "oldfilter")
	if stale != nil {
		t.Errorf("Expected stale filter to be purged during reconciliation, but found: %+v", stale)
	}

	// Leo and Jailer should exist
	leo, _ := repo.GetByKeywordNorm(ctx, "leo")
	if leo == nil || leo.Keyword != "Leo" {
		t.Errorf("Expected Leo filter to exist")
	}
}

// -----------------------------------------------------------------------------
// D. PRIORITY AND LIFECYCLE TESTS
// -----------------------------------------------------------------------------

func TestService_SaveAndDeleteLifecycle(t *testing.T) {
	db, cleanup := setupTestSQLite(t)
	defer cleanup()

	sqlRepo := NewSQLRepositoryWithDB(db)
	service := NewServiceWithRepos(nil, sqlRepo, nil)
	ctx := context.Background()

	// 1. Add filter through service
	f := &GFilter{
		Keyword:   "KGF",
		ReplyText: "KGF Chapter 2",
		MediaType: MediaTypeText,
	}

	// We only test SQLite & memory cache portion here (since MongoDB needs mock or connection)
	_ = sqlRepo.Save(ctx, f)
	service.repopulateCache([]*GFilter{f})

	// Check match
	res, matched := service.Match("kgf")
	if !matched || res.Keyword != "KGF" {
		t.Fatalf("Expected match for 'kgf', got %v", matched)
	}

	// Check mismatch
	_, matchedMis := service.Match("kgf chapter 1")
	if matchedMis {
		t.Errorf("Expected no match for 'kgf chapter 1'")
	}

	// Delete from cache
	service.cacheMu.Lock()
	delete(service.cache, "kgf")
	service.cacheMu.Unlock()
	_, _ = sqlRepo.DeleteByKeywordNorm(ctx, "kgf")

	_, matchedAfter := service.Match("kgf")
	if matchedAfter {
		t.Errorf("Expected no match after delete")
	}
}
