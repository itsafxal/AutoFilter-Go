package batch_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/itsafxal/AutoFilter-Go/internal/batch"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}

	schema := `
		CREATE TABLE IF NOT EXISTS batch_metadata (
			batch_id TEXT PRIMARY KEY,
			batch_name TEXT NOT NULL,
			normalized_name TEXT NOT NULL,
			owner_id INTEGER NOT NULL,
			total_items INTEGER NOT NULL DEFAULT 0,
			total_files INTEGER NOT NULL DEFAULT 0,
			total_texts INTEGER NOT NULL DEFAULT 0,
			total_documents INTEGER NOT NULL DEFAULT 0,
			total_videos INTEGER NOT NULL DEFAULT 0,
			total_audio INTEGER NOT NULL DEFAULT 0,
			total_photos INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			finalized_at TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_batch_meta_norm_name ON batch_metadata (normalized_name);
		CREATE INDEX IF NOT EXISTS idx_batch_meta_owner ON batch_metadata (owner_id);
		CREATE INDEX IF NOT EXISTS idx_batch_meta_created ON batch_metadata (created_at DESC);

		CREATE TABLE IF NOT EXISTS batch_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			item_type TEXT NOT NULL,
			telegram_file_id TEXT,
			telegram_file_unique_id TEXT,
			storage_reference TEXT,
			file_name TEXT,
			file_size INTEGER DEFAULT 0,
			mime_type TEXT,
			caption TEXT,
			text_content TEXT,
			message_entities TEXT,
			created_at TIMESTAMP NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_batch_items_batch_id ON batch_items (batch_id);
		CREATE INDEX IF NOT EXISTS idx_batch_items_seq ON batch_items (batch_id, sequence ASC);
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_batch_item_seq ON batch_items (batch_id, sequence);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed executing test schema: %v", err)
	}

	return db
}

func TestNormalizeBatchName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Marvel Cinematic Universe", "marvel cinematic universe"},
		{"  FAST   AND    FURIOUS   ", "fast and furious"},
		{"AVATAR 2026", "avatar 2026"},
		{"", ""},
	}

	for _, tc := range tests {
		actual := batch.NormalizeBatchName(tc.input)
		if actual != tc.expected {
			t.Errorf("NormalizeBatchName(%q) = %q; expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestGenerateBatchID(t *testing.T) {
	id1 := batch.GenerateBatchID()
	id2 := batch.GenerateBatchID()

	if id1 == "" || id2 == "" {
		t.Fatal("GenerateBatchID() returned empty string")
	}
	if id1 == id2 {
		t.Fatalf("GenerateBatchID() generated duplicate IDs: %s", id1)
	}
	if len(id1) < 10 {
		t.Fatalf("GenerateBatchID() length too short: %d", len(id1))
	}
}

func TestSessionManager(t *testing.T) {
	sm := batch.NewSessionManager()

	// 1. Pending name test
	userID := int64(12345)
	chatID := int64(67890)

	if sm.IsPendingName(userID) {
		t.Error("expected IsPendingName to be false initially")
	}

	sm.SetPendingName(userID, chatID)
	if !sm.IsPendingName(userID) {
		t.Error("expected IsPendingName to be true after SetPendingName")
	}

	sm.ClearPendingName(userID)
	if sm.IsPendingName(userID) {
		t.Error("expected IsPendingName to be false after ClearPendingName")
	}

	// 2. Start session
	sess := sm.StartSession(userID, chatID, "Test Movie Collection")
	if sess == nil {
		t.Fatal("expected non-nil session from StartSession")
	}
	if sess.BatchName != "Test Movie Collection" {
		t.Errorf("unexpected BatchName: %s", sess.BatchName)
	}

	// 3. Add items and verify sequence
	seq1 := sess.AddItem(&batch.BatchItem{ItemType: batch.ItemTypeVideo, FileName: "Movie1.mp4"}, 101)
	seq2 := sess.AddItem(&batch.BatchItem{ItemType: batch.ItemTypeText, TextContent: "Movie link"}, 102)
	seq3 := sess.AddItem(&batch.BatchItem{ItemType: batch.ItemTypeDocument, FileName: "Subtitles.srt"}, 103)

	if seq1 != 1 || seq2 != 2 || seq3 != 3 {
		t.Errorf("expected sequence numbers 1, 2, 3; got %d, %d, %d", seq1, seq2, seq3)
	}

	meta, items, tempIDs := sess.GetSnapshot()
	if meta.TotalItems != 3 || meta.TotalFiles != 2 || meta.TotalTexts != 1 {
		t.Errorf("snapshot counters mismatch: TotalItems=%d, TotalFiles=%d, TotalTexts=%d",
			meta.TotalItems, meta.TotalFiles, meta.TotalTexts)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 snapshot items; got %d", len(items))
	}
	if len(tempIDs) != 3 {
		t.Errorf("expected 3 temp message IDs; got %d", len(tempIDs))
	}

	// 4. Concurrency finalize protection
	if !sess.BeginFinalize() {
		t.Error("expected BeginFinalize to succeed on first call")
	}
	if sess.BeginFinalize() {
		t.Error("expected BeginFinalize to fail on second call")
	}
	sess.MarkDone()

	// 5. Cleanup
	sm.RemoveSession(userID)
	if sm.GetSession(userID) != nil {
		t.Error("expected GetSession to return nil after RemoveSession")
	}
}

func TestSQLRepositoryOperations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := batch.NewSQLRepositoryWithDB(db)
	ctx := context.Background()

	// 1. Save and Get Metadata
	meta := &batch.BatchMetadata{
		BatchID:        "b_test001",
		BatchName:      "Lord of the Rings Trilogy",
		NormalizedName: batch.NormalizeBatchName("Lord of the Rings Trilogy"),
		OwnerID:        1001,
		TotalItems:     3,
		TotalFiles:     3,
		TotalTexts:     0,
		TotalVideos:    3,
		Status:         batch.StatusFinalized,
		CreatedAt:      time.Now().UTC().Add(-1 * time.Hour),
		UpdatedAt:      time.Now().UTC(),
	}

	if err := repo.SaveMetadata(ctx, meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	fetchedMeta, err := repo.GetMetadata(ctx, "b_test001")
	if err != nil || fetchedMeta == nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if fetchedMeta.BatchName != meta.BatchName {
		t.Errorf("metadata name mismatch: expected %q, got %q", meta.BatchName, fetchedMeta.BatchName)
	}

	// 2. Save items with strict sequential order
	items := []*batch.BatchItem{
		{
			BatchID:        "b_test001",
			Sequence:       1,
			ItemType:       batch.ItemTypeVideo,
			TelegramFileID: "FILE_001",
			FileName:       "Fellowship.of.the.Ring.mkv",
			FileSize:       1024 * 1024 * 500,
		},
		{
			BatchID:        "b_test001",
			Sequence:       2,
			ItemType:       batch.ItemTypeVideo,
			TelegramFileID: "FILE_002",
			FileName:       "The.Two.Towers.mkv",
			FileSize:       1024 * 1024 * 600,
		},
		{
			BatchID:        "b_test001",
			Sequence:       3,
			ItemType:       batch.ItemTypeVideo,
			TelegramFileID: "FILE_003",
			FileName:       "Return.of.the.King.mkv",
			FileSize:       1024 * 1024 * 700,
		},
	}

	if err := repo.SaveItems(ctx, items); err != nil {
		t.Fatalf("SaveItems failed: %v", err)
	}

	fetchedItems, err := repo.GetItems(ctx, "b_test001")
	if err != nil {
		t.Fatalf("GetItems failed: %v", err)
	}
	if len(fetchedItems) != 3 {
		t.Fatalf("expected 3 items; got %d", len(fetchedItems))
	}
	for i, it := range fetchedItems {
		expectedSeq := i + 1
		if it.Sequence != expectedSeq {
			t.Errorf("item order violated: expected sequence %d, got %d", expectedSeq, it.Sequence)
		}
	}

	// 3. Search Batches
	results, total, err := repo.SearchMetadata(ctx, "rings", 10, 0)
	if err != nil {
		t.Fatalf("SearchMetadata failed: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("expected 1 search result; got total=%d, len=%d", total, len(results))
	}
	if results[0].BatchID != "b_test001" {
		t.Errorf("search result mismatch: got %s", results[0].BatchID)
	}

	// Case-insensitive exact search
	results, total, err = repo.SearchMetadata(ctx, "LORD OF THE RINGS TRILOGY", 10, 0)
	if err != nil || total != 1 {
		t.Fatalf("case-insensitive search failed: total=%d, err=%v", total, err)
	}

	// 4. Paginated List
	list, listTotal, err := repo.ListMetadata(ctx, 0, 10, 0)
	if err != nil {
		t.Fatalf("ListMetadata failed: %v", err)
	}
	if listTotal != 1 || len(list) != 1 {
		t.Fatalf("expected 1 list item; got total=%d, len=%d", listTotal, len(list))
	}

	// 5. SyncFromMongo (Rebuilding and reconciling)
	batch2 := &batch.BatchMetadata{
		BatchID:        "b_test002",
		BatchName:      "Harry Potter Complete",
		NormalizedName: batch.NormalizeBatchName("Harry Potter Complete"),
		OwnerID:        1002,
		TotalItems:     1,
		TotalFiles:     1,
		Status:         batch.StatusFinalized,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	itemsMap := map[string][]*batch.BatchItem{
		"b_test002": {
			{
				BatchID:        "b_test002",
				Sequence:       1,
				ItemType:       batch.ItemTypeVideo,
				TelegramFileID: "HP_01",
				FileName:       "PhilosophersStone.mp4",
			},
		},
	}

	// Authoritative list now contains ONLY b_test002 (b_test001 should be reconciled/pruned)
	if err := repo.SyncFromMongo(ctx, []*batch.BatchMetadata{batch2}, itemsMap); err != nil {
		t.Fatalf("SyncFromMongo failed: %v", err)
	}

	// Verify b_test001 was pruned from SQLite
	prunedMeta, _ := repo.GetMetadata(ctx, "b_test001")
	if prunedMeta != nil {
		t.Errorf("expected b_test001 to be reconciled and pruned; still exists")
	}

	// Verify b_test002 exists
	newMeta, _ := repo.GetMetadata(ctx, "b_test002")
	if newMeta == nil || newMeta.BatchName != "Harry Potter Complete" {
		t.Errorf("expected b_test002 to be synchronized into SQLite")
	}

	newItems, _ := repo.GetItems(ctx, "b_test002")
	if len(newItems) != 1 || newItems[0].TelegramFileID != "HP_01" {
		t.Errorf("expected b_test002 item to be synchronized into SQLite")
	}

	// 6. DeleteBatch
	if err := repo.DeleteBatch(ctx, "b_test002"); err != nil {
		t.Fatalf("DeleteBatch failed: %v", err)
	}
	delMeta, _ := repo.GetMetadata(ctx, "b_test002")
	if delMeta != nil {
		t.Error("expected b_test002 to be deleted")
	}
}

func TestConcurrentSessionAdd(t *testing.T) {
	sm := batch.NewSessionManager()
	sess := sm.StartSession(999, 888, "Concurrent Uploads")

	var wg sync.WaitGroup
	count := 50

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess.AddItem(&batch.BatchItem{
				ItemType: batch.ItemTypeDocument,
				FileName: fmt.Sprintf("file_%d.pdf", idx),
			}, int32(idx+1000))
		}(i)
	}

	wg.Wait()

	meta, items, _ := sess.GetSnapshot()
	if meta.TotalItems != count {
		t.Errorf("expected %d total items; got %d", count, meta.TotalItems)
	}
	if len(items) != count {
		t.Errorf("expected %d items; got %d", count, len(items))
	}

	// Verify all sequence numbers from 1 to count exist with zero duplicates
	seenSeq := make(map[int]bool)
	for _, it := range items {
		if seenSeq[it.Sequence] {
			t.Errorf("duplicate sequence number detected: %d", it.Sequence)
		}
		seenSeq[it.Sequence] = true
	}
}
