package autodelete

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS scheduled_deletions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			message_id INTEGER NOT NULL,
			msg_type TEXT NOT NULL,
			delete_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sent_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			message_id INTEGER NOT NULL,
			msg_type TEXT NOT NULL,
			sent_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS autodelete_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("failed to init schema: %v", err)
		}
	}

	return db
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input       string
		expected    time.Duration
		expectError bool
	}{
		{"15m", 15 * time.Minute, false},
		{"1h", 1 * time.Hour, false},
		{"30s", 30 * time.Second, false},
		{"45", 45 * time.Minute, false},
		{"0", 0, false},
		{"off", 0, false},
		{"disabled", 0, false},
		{"invalid", 0, true},
	}

	for _, tc := range tests {
		d, err := ParseDuration(tc.input)
		if tc.expectError {
			if err == nil {
				t.Errorf("expected error for %q, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tc.input, err)
			}
			if d != tc.expected {
				t.Errorf("ParseDuration(%q) = %v; expected %v", tc.input, d, tc.expected)
			}
		}
	}
}

func TestFormatDuration(t *testing.T) {
	if FormatDuration(0) != "Disabled" {
		t.Errorf("expected 'Disabled', got %q", FormatDuration(0))
	}
	if FormatDuration(15*time.Minute) != "15 minutes" {
		t.Errorf("expected '15 minutes', got %q", FormatDuration(15*time.Minute))
	}
	if FormatDuration(1*time.Minute) != "1 minute" {
		t.Errorf("expected '1 minute', got %q", FormatDuration(1*time.Minute))
	}
	if FormatDuration(1*time.Hour) != "1 hour" {
		t.Errorf("expected '1 hour', got %q", FormatDuration(1*time.Hour))
	}
	if FormatDuration(2*time.Hour) != "2 hours" {
		t.Errorf("expected '2 hours', got %q", FormatDuration(2*time.Hour))
	}
}

func TestScheduleDelete_PersistenceAndRecovery(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AutoDeleteFilterTime:  "15m",
		AutoDeleteFileTime:    "30m",
		AutoDeleteWeeklyTrash: true,
		TrashRetentionDays:    7,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(cfg, func() *sql.DB { return db }, nil, logger)

	ctx := context.Background()

	// 1. Schedule a message whose delete_at is in the PAST (simulates offline while pending)
	pastDeleteAt := time.Now().Add(-5 * time.Minute)
	_, err := db.Exec(`
		INSERT INTO scheduled_deletions (chat_id, message_id, msg_type, delete_at, created_at)
		VALUES (12345, 101, 'filter', ?, ?);
	`, pastDeleteAt, time.Now().Add(-20*time.Minute))
	if err != nil {
		t.Fatalf("failed to insert past deletion: %v", err)
	}

	// 2. Schedule a message whose delete_at is in the FUTURE
	err = svc.ScheduleDelete(12345, 102, "file", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to schedule future deletion: %v", err)
	}

	// Verify both exist before recovery
	var countBefore int
	_ = db.QueryRow("SELECT COUNT(*) FROM scheduled_deletions;").Scan(&countBefore)
	if countBefore != 2 {
		t.Fatalf("expected 2 scheduled deletions, got %d", countBefore)
	}

	// 3. Simulate bot restart recovery by calling Init
	if err := svc.Init(ctx, nil); err != nil {
		t.Fatalf("failed to init service: %v", err)
	}

	// Verify the past deletion was processed and removed, and only future deletion remains
	var countAfter int
	_ = db.QueryRow("SELECT COUNT(*) FROM scheduled_deletions;").Scan(&countAfter)
	if countAfter != 1 {
		t.Fatalf("expected 1 remaining scheduled deletion after restart purge, got %d", countAfter)
	}

	var remainingMsgID int32
	_ = db.QueryRow("SELECT message_id FROM scheduled_deletions LIMIT 1;").Scan(&remainingMsgID)
	if remainingMsgID != 102 {
		t.Fatalf("expected message 102 to remain scheduled, got %d", remainingMsgID)
	}
}

func TestWeeklyTrash_PruneOldMessages(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AutoDeleteWeeklyTrash: true,
		TrashRetentionDays:    7,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(cfg, func() *sql.DB { return db }, nil, logger)
	ctx := context.Background()

	// 1. Insert recent message (1 hour old)
	recentTime := time.Now().Add(-1 * time.Hour)
	_, _ = db.Exec("INSERT INTO sent_messages (chat_id, message_id, msg_type, sent_at) VALUES (111, 1, 'start', ?);", recentTime)

	// 2. Insert old message (10 days old, older than 7 days)
	oldTime := time.Now().AddDate(0, 0, -10)
	_, _ = db.Exec("INSERT INTO sent_messages (chat_id, message_id, msg_type, sent_at) VALUES (111, 2, 'filter', ?);", oldTime)
	_, _ = db.Exec("INSERT INTO sent_messages (chat_id, message_id, msg_type, sent_at) VALUES (222, 3, 'file', ?);", oldTime)

	deleted, err := svc.RunWeeklyTrash(ctx)
	if err != nil {
		t.Fatalf("unexpected weekly trash error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 messages purged by weekly trash, got %d", deleted)
	}

	// Verify only recent message remains
	var remainingCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM sent_messages;").Scan(&remainingCount)
	if remainingCount != 1 {
		t.Fatalf("expected 1 message remaining in sent_messages, got %d", remainingCount)
	}

	var remainingMsgID int32
	_ = db.QueryRow("SELECT message_id FROM sent_messages LIMIT 1;").Scan(&remainingMsgID)
	if remainingMsgID != 1 {
		t.Fatalf("expected recent message 1 to remain, got %d", remainingMsgID)
	}
}

func TestSettingsPersistence(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AutoDeleteFilterTime:  "15m",
		AutoDeleteFileTime:    "30m",
		AutoDeleteWeeklyTrash: true,
		TrashRetentionDays:    7,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(cfg, func() *sql.DB { return db }, nil, logger)
	ctx := context.Background()

	// Update settings
	_ = svc.SetFilterDuration(ctx, 45*time.Minute)
	_ = svc.SetFileDuration(ctx, 2*time.Hour)
	_ = svc.SetFastDownloadDuration(ctx, 10*time.Minute)
	_ = svc.SetWeeklyTrash(ctx, false)
	_ = svc.SetTrashRetention(ctx, 14)

	// Recreate service to simulate restart
	svc2 := NewService(cfg, func() *sql.DB { return db }, nil, logger)
	_ = svc2.Init(ctx, nil)

	s2 := svc2.GetSettings()
	if s2.FilterDeleteDuration != 45*time.Minute {
		t.Errorf("expected FilterDeleteDuration 45m, got %v", s2.FilterDeleteDuration)
	}
	if s2.FileDeleteDuration != 2*time.Hour {
		t.Errorf("expected FileDeleteDuration 2h, got %v", s2.FileDeleteDuration)
	}
	if s2.FastDownloadDeleteDuration != 10*time.Minute {
		t.Errorf("expected FastDownloadDeleteDuration 10m, got %v", s2.FastDownloadDeleteDuration)
	}
	if s2.WeeklyTrashEnabled != false {
		t.Errorf("expected WeeklyTrashEnabled false, got %v", s2.WeeklyTrashEnabled)
	}
	if s2.TrashRetentionDays != 14 {
		t.Errorf("expected TrashRetentionDays 14, got %d", s2.TrashRetentionDays)
	}
}

func TestScheduleFastDownloadDelete(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AutoDeleteFastDownloadTime: "5m",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(cfg, func() *sql.DB { return db }, nil, logger)

	err := svc.ScheduleFastDownloadDelete(12345, 999)
	if err != nil {
		t.Fatalf("unexpected error scheduling fast download delete: %v", err)
	}

	var msgType string
	var deleteAt time.Time
	err = db.QueryRow("SELECT msg_type, delete_at FROM scheduled_deletions WHERE chat_id = 12345 AND message_id = 999;").Scan(&msgType, &deleteAt)
	if err != nil {
		t.Fatalf("expected scheduled deletion record: %v", err)
	}
	if msgType != "fast_download" {
		t.Errorf("expected msg_type 'fast_download', got %q", msgType)
	}
	if deleteAt.Before(time.Now()) {
		t.Errorf("expected delete_at to be in future, got %v", deleteAt)
	}
}
