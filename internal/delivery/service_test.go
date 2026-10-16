package delivery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
	"github.com/itsafxal/MYTGBOTZ/internal/database/sql"
	"go.mongodb.org/mongo-driver/bson"
)

// mockSender implements TelegramSender for isolated unit testing.
type mockSender struct {
	lastMethod      string
	lastChatID      int64
	lastInput       gotgbot.InputFileOrString
	lastCaption     string
	lastProtect     bool
	sendDocumentErr error
	sendVideoErr    error
	sendAudioErr    error
	sendPhotoErr    error
	sendAnimErr     error
	sendVoiceErr    error
	returnMsgID     int64
}

func newMockSender() *mockSender {
	return &mockSender{
		returnMsgID: 1001,
	}
}

func (m *mockSender) SendDocument(chatId int64, document gotgbot.InputFileOrString, opts *gotgbot.SendDocumentOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendDocument"
	m.lastChatID = chatId
	m.lastInput = document
	if opts != nil {
		m.lastCaption = opts.Caption
		m.lastProtect = opts.ProtectContent
	}
	if m.sendDocumentErr != nil {
		return nil, m.sendDocumentErr
	}
	return &gotgbot.Message{MessageId: m.returnMsgID, Chat: gotgbot.Chat{Id: chatId}}, nil
}

func (m *mockSender) SendVideo(chatId int64, video gotgbot.InputFileOrString, opts *gotgbot.SendVideoOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendVideo"
	m.lastChatID = chatId
	m.lastInput = video
	if opts != nil {
		m.lastCaption = opts.Caption
		m.lastProtect = opts.ProtectContent
	}
	if m.sendVideoErr != nil {
		return nil, m.sendVideoErr
	}
	return &gotgbot.Message{MessageId: m.returnMsgID, Chat: gotgbot.Chat{Id: chatId}}, nil
}

func (m *mockSender) SendAudio(chatId int64, audio gotgbot.InputFileOrString, opts *gotgbot.SendAudioOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendAudio"
	m.lastChatID = chatId
	m.lastInput = audio
	if opts != nil {
		m.lastCaption = opts.Caption
		m.lastProtect = opts.ProtectContent
	}
	if m.sendAudioErr != nil {
		return nil, m.sendAudioErr
	}
	return &gotgbot.Message{MessageId: m.returnMsgID, Chat: gotgbot.Chat{Id: chatId}}, nil
}

func (m *mockSender) SendPhoto(chatId int64, photo gotgbot.InputFileOrString, opts *gotgbot.SendPhotoOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendPhoto"
	m.lastChatID = chatId
	m.lastInput = photo
	if opts != nil {
		m.lastCaption = opts.Caption
		m.lastProtect = opts.ProtectContent
	}
	if m.sendPhotoErr != nil {
		return nil, m.sendPhotoErr
	}
	return &gotgbot.Message{MessageId: m.returnMsgID, Chat: gotgbot.Chat{Id: chatId}}, nil
}

func (m *mockSender) SendAnimation(chatId int64, animation gotgbot.InputFileOrString, opts *gotgbot.SendAnimationOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendAnimation"
	m.lastChatID = chatId
	m.lastInput = animation
	if opts != nil {
		m.lastCaption = opts.Caption
		m.lastProtect = opts.ProtectContent
	}
	if m.sendAnimErr != nil {
		return nil, m.sendAnimErr
	}
	return &gotgbot.Message{MessageId: m.returnMsgID, Chat: gotgbot.Chat{Id: chatId}}, nil
}

func (m *mockSender) SendVoice(chatId int64, voice gotgbot.InputFileOrString, opts *gotgbot.SendVoiceOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendVoice"
	m.lastChatID = chatId
	m.lastInput = voice
	if opts != nil {
		m.lastCaption = opts.Caption
		m.lastProtect = opts.ProtectContent
	}
	if m.sendVoiceErr != nil {
		return nil, m.sendVoiceErr
	}
	return &gotgbot.Message{MessageId: m.returnMsgID, Chat: gotgbot.Chat{Id: chatId}}, nil
}

// setupTestDB creates an isolated SQLite test database populated with a sample media record.
func setupTestDB(t *testing.T) (*database.DatabaseManager, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_delivery.sqlite3")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		SQLiteDBPath:  dbPath,
		SyncBatchSize: 100,
	}

	dbMgr := database.NewDatabaseManager(cfg, logger)
	if err := dbMgr.Init(context.Background()); err != nil {
		t.Fatalf("failed to initialize test database manager: %v", err)
	}

	// Insert test media records directly into SQLite cache
	rawDB := dbMgr.SqlMediaRepo()
	if rawDB == nil {
		t.Fatal("SqlMediaRepo is nil")
	}

	// Populate sample records
	sqlMgr := sql.NewDBManager(dbPath, logger)
	_ = sqlMgr.Open(context.Background())
	defer sqlMgr.Close()

	records := []struct {
		id       string
		name     string
		fileType string
		caption  string
		fileSize int64
	}{
		{"doc123", "sample_document.pdf", "document", "Sample Document PDF", 1024},
		{"vid123", "sample_movie.mp4", "video", "Sample Movie Video", 104857600},
		{"aud123", "sample_track.mp3", "audio", "Sample Audio Track", 5242880},
		{"pho123", "sample_image.jpg", "photo", "Sample Photo", 204800},
		{"ani123", "sample_clip.gif", "animation", "Sample GIF Animation", 1048576},
		{"voi123", "sample_note.ogg", "voice", "Sample Voice Note", 65536},
		{"malformed_size", "bad_file.bin", "document", "Bad File", -5},
		{"empty_ref", "", "document", "No File ID", 500},
	}

	for _, r := range records {
		_, err := sqlMgr.DB().Exec(`
			INSERT OR REPLACE INTO media (file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
		`, r.id, "ref_"+r.id, r.name, r.fileSize, "1 MB", r.fileType, "application/octet-stream", r.caption, time.Now())
		if err != nil {
			t.Fatalf("failed to insert test record: %v", err)
		}
	}

	return dbMgr, dbPath
}

// 1. Test Successful lookup of an existing media record
func TestDelivery_SuccessfulLookup(t *testing.T) {
	dbMgr, _ := setupTestDB(t)
	defer dbMgr.Close()

	svc := NewService(dbMgr, nil)
	rec, err := svc.GetMedia(context.Background(), "doc123")
	if err != nil {
		t.Fatalf("expected successful lookup, got error: %v", err)
	}
	if rec == nil {
		t.Fatal("expected media record, got nil")
	}
	if rec.FileID != "doc123" {
		t.Errorf("expected FileID 'doc123', got %q", rec.FileID)
	}
	if rec.FileName != "sample_document.pdf" {
		t.Errorf("expected FileName 'sample_document.pdf', got %q", rec.FileName)
	}
	if rec.FileType != "document" {
		t.Errorf("expected FileType 'document', got %q", rec.FileType)
	}
}

// 2. Test Media record not found
func TestDelivery_RecordNotFound(t *testing.T) {
	dbMgr, _ := setupTestDB(t)
	defer dbMgr.Close()

	svc := NewService(dbMgr, nil)
	rec, err := svc.GetMedia(context.Background(), "non_existent_id")
	if !errors.Is(err, ErrMediaNotFound) {
		t.Errorf("expected ErrMediaNotFound, got %v", err)
	}
	if rec != nil {
		t.Errorf("expected nil record, got %+v", rec)
	}
}

// 3. Test Missing file_id or equivalent supported delivery reference
func TestDelivery_MissingFileID(t *testing.T) {
	svc := NewService(nil, nil)
	err := svc.ValidateMediaRecord(&MediaDetails{
		FileID:   "",
		FileName: "test.pdf",
		FileSize: 100,
	})
	if !errors.Is(err, ErrInvalidFileReference) {
		t.Errorf("expected ErrInvalidFileReference, got %v", err)
	}
}

// 4. Test Malformed or unsupported media metadata
func TestDelivery_MalformedMetadata(t *testing.T) {
	svc := NewService(nil, nil)
	err := svc.ValidateMediaRecord(&MediaDetails{
		FileID:   "doc123",
		FileName: "bad_file.bin",
		FileSize: -50,
	})
	if !errors.Is(err, ErrMalformedMetadata) {
		t.Errorf("expected ErrMalformedMetadata, got %v", err)
	}
}

// 5. Test Correct selection of the Telegram send method for each implemented media type
func TestDelivery_MethodSelection(t *testing.T) {
	dbMgr, _ := setupTestDB(t)
	defer dbMgr.Close()

	svc := NewService(dbMgr, nil)
	mock := newMockSender()
	svc.SetSender(mock)

	tests := []struct {
		id             string
		expectedMethod string
	}{
		{"doc123", "SendDocument"},
		{"vid123", "SendVideo"},
		{"aud123", "SendAudio"},
		{"pho123", "SendPhoto"},
		{"ani123", "SendAnimation"},
		{"voi123", "SendVoice"},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			res, err := svc.Deliver(context.Background(), 12345, tc.id, DeliveryOptions{})
			if err != nil {
				t.Fatalf("delivery failed for %s: %v", tc.id, err)
			}
			if res.DeliveryMethod != tc.expectedMethod {
				t.Errorf("expected method %s, got %s", tc.expectedMethod, res.DeliveryMethod)
			}
			if mock.lastMethod != tc.expectedMethod {
				t.Errorf("expected sender method %s, got %s", tc.expectedMethod, mock.lastMethod)
			}
		})
	}
}

// 6. Test Successful Telegram delivery and structured result
func TestDelivery_SuccessfulDelivery(t *testing.T) {
	dbMgr, _ := setupTestDB(t)
	defer dbMgr.Close()

	svc := NewService(dbMgr, nil)
	mock := newMockSender()
	svc.SetSender(mock)

	opts := DeliveryOptions{
		UserID:         999,
		ProtectContent: true,
		CustomCaption:  "Custom Movie Caption",
	}

	res, err := svc.Deliver(context.Background(), 12345, "vid123", opts)
	if err != nil {
		t.Fatalf("unexpected error during delivery: %v", err)
	}
	if !res.Success {
		t.Error("expected Success true")
	}
	if res.MessageID != 1001 {
		t.Errorf("expected MessageID 1001, got %d", res.MessageID)
	}
	if !res.ProtectedContent {
		t.Error("expected ProtectedContent true")
	}
	if mock.lastCaption != "Custom Movie Caption" {
		t.Errorf("expected caption 'Custom Movie Caption', got %q", mock.lastCaption)
	}
	if !mock.lastProtect {
		t.Error("expected mock sender protect flag to be true")
	}
}

// 7. Test Telegram API failure
func TestDelivery_TelegramAPIFailure(t *testing.T) {
	dbMgr, _ := setupTestDB(t)
	defer dbMgr.Close()

	svc := NewService(dbMgr, nil)
	mock := newMockSender()
	mock.sendDocumentErr = &gotgbot.TelegramError{
		Code:        400,
		Description: "Bad Request: wrong file identifier",
	}
	svc.SetSender(mock)

	_, err := svc.Deliver(context.Background(), 12345, "doc123", DeliveryOptions{})
	if !errors.Is(err, ErrTelegramAPI) {
		t.Errorf("expected ErrTelegramAPI, got %v", err)
	}
	if !strings.Contains(err.Error(), "wrong file identifier") {
		t.Errorf("expected error to mention description, got %v", err)
	}
}

// 8. Test Rate-limit (FloodWait) and blocked user handling
func TestDelivery_RateLimitAndBlockedUser(t *testing.T) {
	dbMgr, _ := setupTestDB(t)
	defer dbMgr.Close()

	svc := NewService(dbMgr, nil)
	mock := newMockSender()
	svc.SetSender(mock)

	// Sub-test: Flood Wait (429)
	mock.sendDocumentErr = &gotgbot.TelegramError{
		Code: 429,
		ResponseParams: &gotgbot.ResponseParameters{
			RetryAfter: 25,
		},
	}
	_, err := svc.Deliver(context.Background(), 12345, "doc123", DeliveryOptions{})
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
	if !strings.Contains(err.Error(), "25 seconds") {
		t.Errorf("expected error message to contain retry after duration, got %v", err)
	}

	// Sub-test: Blocked by user (403)
	mock.sendDocumentErr = &gotgbot.TelegramError{
		Code:        403,
		Description: "Forbidden: bot was blocked by the user",
	}
	_, err = svc.Deliver(context.Background(), 12345, "doc123", DeliveryOptions{})
	if !errors.Is(err, ErrBlockedByUser) {
		t.Errorf("expected ErrBlockedByUser, got %v", err)
	}
}

// 9. Test Database timeout or database unavailability
func TestDelivery_DatabaseUnavailability(t *testing.T) {
	// Service with nil database
	svc := NewService(nil, nil)
	_, err := svc.GetMedia(context.Background(), "doc123")
	if !errors.Is(err, ErrDatabaseUnavailable) {
		t.Errorf("expected ErrDatabaseUnavailable, got %v", err)
	}

	// Context cancellation/timeout
	dbMgr, _ := setupTestDB(t)
	defer dbMgr.Close()

	svc2 := NewService(dbMgr, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	_, err = svc2.GetMedia(ctx, "doc123")
	if err == nil {
		t.Error("expected error when context cancelled, got nil")
	}
}

// 10. Test Invalid callback payload or media identifier
func TestDelivery_InvalidIdentifier(t *testing.T) {
	svc := NewService(nil, nil)

	invalidIDs := []string{
		"",
		" ",
		"file with spaces",
		"file$invalid*chars",
		"file;DROP TABLE media;",
		strings.Repeat("a", 130), // exceeds 128 limit
	}

	for _, inv := range invalidIDs {
		t.Run(fmt.Sprintf("id_%s", inv), func(t *testing.T) {
			_, err := svc.GetMedia(context.Background(), inv)
			if !errors.Is(err, ErrInvalidIdentifier) {
				t.Errorf("expected ErrInvalidIdentifier for %q, got %v", inv, err)
			}
		})
	}
}

// 11. Test In-flight deduplication (prevent rapid repeated deliveries)
func TestDelivery_DuplicateRequestHandling(t *testing.T) {
	tracker := NewInFlightTracker(5 * time.Second)

	// First acquisition succeeds
	if !tracker.Acquire(1234, "fileA") {
		t.Fatal("expected first acquire to succeed")
	}

	// Immediate duplicate acquisition fails
	if tracker.Acquire(1234, "fileA") {
		t.Error("expected second acquire to fail as duplicate")
	}

	// Different file succeeds
	if !tracker.Acquire(1234, "fileB") {
		t.Error("expected acquire for different file to succeed")
	}

	// Different user succeeds
	if !tracker.Acquire(5678, "fileA") {
		t.Error("expected acquire for different user to succeed")
	}

	// Release allows re-acquisition
	tracker.Release(1234, "fileA")
	if !tracker.Acquire(1234, "fileA") {
		t.Error("expected acquire after release to succeed")
	}
}

// 12. Test SendVideo fallback to SendDocument on format error (e.g. MKV container)
func TestDelivery_SendVideoFallback(t *testing.T) {
	dbMgr, _ := setupTestDB(t)
	defer dbMgr.Close()

	svc := NewService(dbMgr, nil)
	mock := newMockSender()
	// Simulate Telegram rejecting video format (e.g. MKV uploaded as document)
	mock.sendVideoErr = &gotgbot.TelegramError{
		Code:        400,
		Description: "Bad Request: wrong file identifier/HTTP URL specified",
	}
	svc.SetSender(mock)

	res, err := svc.Deliver(context.Background(), 12345, "vid123", DeliveryOptions{})
	if err != nil {
		t.Fatalf("expected graceful fallback to SendDocument, got error: %v", err)
	}
	if res.DeliveryMethod != "SendDocument" {
		t.Errorf("expected fallback method 'SendDocument', got %s", res.DeliveryMethod)
	}
	if mock.lastMethod != "SendDocument" {
		t.Errorf("expected mock sender lastMethod 'SendDocument', got %s", mock.lastMethod)
	}
}

// 13. Test Correct handling of optional fields in legacy BSON documents
func TestDelivery_OptionalLegacyBSONFields(t *testing.T) {
	// Raw legacy BSON document with minimal fields (no caption, no file_ref, no size string)
	rawBSON := bson.M{
		"_id":       "legacy_doc_01",
		"file_name": "legacy_file.mkv",
		"file_size": int64(52428800),
		"file_type": "video",
	}

	data, err := bson.Marshal(rawBSON)
	if err != nil {
		t.Fatalf("failed to marshal raw BSON: %v", err)
	}

	var doc struct {
		FileID   string `bson:"_id"`
		FileRef  string `bson:"file_ref,omitempty"`
		FileName string `bson:"file_name"`
		FileSize int64  `bson:"file_size"`
		Size     string `bson:"size,omitempty"`
		FileType string `bson:"file_type,omitempty"`
		Caption  string `bson:"caption,omitempty"`
	}

	if err := bson.Unmarshal(data, &doc); err != nil {
		t.Fatalf("failed to unmarshal BSON: %v", err)
	}

	if doc.FileID != "legacy_doc_01" {
		t.Errorf("expected FileID 'legacy_doc_01', got %q", doc.FileID)
	}
	if doc.FileRef != "" {
		t.Errorf("expected empty FileRef, got %q", doc.FileRef)
	}
	if doc.Caption != "" {
		t.Errorf("expected empty Caption, got %q", doc.Caption)
	}
	if doc.Size != "" {
		t.Errorf("expected empty Size, got %q", doc.Size)
	}
}
