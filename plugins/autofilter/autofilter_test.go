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
