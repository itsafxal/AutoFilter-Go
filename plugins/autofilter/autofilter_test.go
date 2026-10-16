package autofilter

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
	"github.com/itsafxal/MYTGBOTZ/internal/search"
)

func TestAutofilterPlugin_Structure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_autofilter.sqlite3")

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

	searchService := search.NewService(dbManager, logger)
	appCtx := &plugins.AppContext{
		Config: cfg,
		Logger: logger,
		DB:     dbManager,
		Search: searchService,
	}

	plugin := New(appCtx)

	if plugin.Name() != "autofilter" {
		t.Errorf("expected plugin name 'autofilter', got %q", plugin.Name())
	}

	msgHandlers := plugin.MessageHandlers()
	if len(msgHandlers) != 1 {
		t.Fatalf("expected 1 message handler, got %d", len(msgHandlers))
	}

	cbHandlers := plugin.CallbackHandlers()
	if len(cbHandlers) == 0 {
		t.Errorf("expected callback handlers, got 0")
	}

	// Test message filter with command vs plain text
	filter := msgHandlers[0].Filter

	// Non-command text message
	plainMsg := &gotgbot.Message{Text: "kgf"}
	if !filter(plainMsg) {
		t.Errorf("expected filter to accept plain text 'kgf'")
	}

	// Command message
	cmdMsg := &gotgbot.Message{
		Text: "/start",
		Entities: []gotgbot.MessageEntity{
			{Type: gotgbot.MessageEntityTypeBotCommand, Offset: 0, Length: 6},
		},
	}
	if filter(cmdMsg) {
		t.Errorf("expected filter to reject bot command '/start'")
	}
}

func TestAutofilter_RecoverSession(t *testing.T) {
	plugin := &Plugin{}
	msg := &gotgbot.Message{
		Text: "𝗛𝗘𝗬 👋 User 🫶\n    \n📁 Query : kgf chapter 2\n\n🗓️ Page No › 1",
		Chat: gotgbot.Chat{Id: -10012345},
	}

	sess := plugin.recoverSessionFromMessage(msg, 998877)
	if sess == nil {
		t.Fatalf("expected recovered session, got nil")
	}
	if sess.Query != "kgf chapter 2" {
		t.Errorf("expected query 'kgf chapter 2', got %q", sess.Query)
	}
	if sess.UserID != 998877 {
		t.Errorf("expected user id 998877, got %d", sess.UserID)
	}
	if sess.ChatID != -10012345 {
		t.Errorf("expected chat id -10012345, got %d", sess.ChatID)
	}
}

func TestAutofilter_IsMessageNotModified(t *testing.T) {
	if !isMessageNotModified(&gotgbot.TelegramError{Description: "Bad Request: message is not modified: specified new message content and reply markup are exactly the same"}) {
		t.Errorf("expected isMessageNotModified to be true")
	}
	if isMessageNotModified(nil) {
		t.Errorf("expected nil error to be false")
	}
}
