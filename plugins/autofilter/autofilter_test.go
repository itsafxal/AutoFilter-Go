package autofilter

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
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
	plainMsg := &telegram.NewMessage{
		Message: &telegram.MessageObj{
			Message: "kgf",
		},
	}
	if !filter(plainMsg) {
		t.Errorf("expected filter to accept plain text 'kgf'")
	}

	// Command message
	cmdMsg := &telegram.NewMessage{
		Message: &telegram.MessageObj{
			Message: "/start",
			Entities: []telegram.MessageEntity{
				&telegram.MessageEntityBotCommand{Offset: 0, Length: 6},
			},
		},
	}
	if filter(cmdMsg) {
		t.Errorf("expected filter to reject bot command '/start'")
	}
}

func TestAutofilter_RecoverSession(t *testing.T) {
	plugin := &Plugin{}
	msg := &telegram.NewMessage{
		Chat: &telegram.ChatObj{ID: -10012345},
		Message: &telegram.MessageObj{
			Message: "𝗛𝗘𝗬 👋 User 🫶\n    \n📁 Query : kgf chapter 2\n\n🗓️ Page No › 1",
		},
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
	err := errors.New("RPCError: 400 MESSAGE_NOT_MODIFIED: message is not modified")
	if !isMessageNotModified(err) {
		t.Errorf("expected isMessageNotModified to be true")
	}
	if isMessageNotModified(nil) {
		t.Errorf("expected nil error to be false")
	}
}

func TestAutofilter_QualitiesList(t *testing.T) {
	expected := []string{"480P", "720P", "1080P", "2160P", "4K"}
	if len(qualities) != len(expected) {
		t.Fatalf("expected %d qualities, got %d", len(expected), len(qualities))
	}
	for i, q := range expected {
		if qualities[i] != q {
			t.Errorf("at index %d: expected %s, got %s", i, q, qualities[i])
		}
	}
}

func TestAutofilter_RecoverSession_WithFilters(t *testing.T) {
	plugin := &Plugin{}

	// 1. Language tag
	msgLang := &telegram.NewMessage{
		Chat: &telegram.ChatObj{ID: -10012345},
		Message: &telegram.MessageObj{
			Message: "𝗛𝗘𝗬 👋 User 🫶\n    \n📁 Query : Vikram (MALAYALAM)\n\n🗓️ Page No › 1",
		},
	}
	sessLang := plugin.recoverSessionFromMessage(msgLang, 123)
	if sessLang == nil {
		t.Fatalf("expected recovered session, got nil")
	}
	if sessLang.Query != "Vikram" || sessLang.Language != "malayalam" {
		t.Errorf("expected Query='Vikram', Language='malayalam', got Query=%q, Language=%q", sessLang.Query, sessLang.Language)
	}

	// 2. Quality tag
	msgQual := &telegram.NewMessage{
		Chat: &telegram.ChatObj{ID: -10012345},
		Message: &telegram.MessageObj{
			Message: "𝗛𝗘𝗬 👋 User 🫶\n    \n📁 Query : Vikram (1080P)\n\n🗓️ Page No › 1",
		},
	}
	sessQual := plugin.recoverSessionFromMessage(msgQual, 123)
	if sessQual == nil {
		t.Fatalf("expected recovered session, got nil")
	}
	if sessQual.Query != "Vikram" || sessQual.Quality != "1080p" {
		t.Errorf("expected Query='Vikram', Quality='1080p', got Query=%q, Quality=%q", sessQual.Query, sessQual.Quality)
	}
}

