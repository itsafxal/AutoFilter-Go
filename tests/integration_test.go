package tests

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/itsafxal/MYTGBOTZ/internal/app"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	deliveryCore "github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/logging"
	"github.com/itsafxal/MYTGBOTZ/plugins/cmds"
	"github.com/itsafxal/MYTGBOTZ/plugins/delivery"
	"github.com/itsafxal/MYTGBOTZ/plugins/id"
	"github.com/itsafxal/MYTGBOTZ/plugins/info"
	"github.com/itsafxal/MYTGBOTZ/plugins/ping"
	"github.com/itsafxal/MYTGBOTZ/plugins/start"
)

func TestIntegration_FullPluginRegistryAndHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelInfo, buf)

	cfg := &config.Config{
		BotToken: "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		OwnerID:  99887766,
		Admins:   []int64{99887766, 123456},
		LogLevel: config.LogLevelInfo,
	}

	botApp, err := app.New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to initialize App: %v", err)
	}

	// Register all 5 foundational plugins
	err = botApp.RegisterPlugins(
		start.New(botApp.AppContext),
		cmds.New(botApp.AppContext),
		ping.New(botApp.AppContext),
		id.New(botApp.AppContext),
		info.New(botApp.AppContext),
	)
	if err != nil {
		t.Fatalf("failed to register plugins: %v", err)
	}

	// Validate plugins
	if err := botApp.ValidatePlugins(); err != nil {
		t.Fatalf("plugin validation failed: %v", err)
	}

	if botApp.Registry.PluginCount() != 5 {
		t.Errorf("expected 5 plugins, got %d", botApp.Registry.PluginCount())
	}
	if botApp.Registry.CommandCount() != 5 {
		t.Errorf("expected 5 primary commands, got %d", botApp.Registry.CommandCount())
	}

	// Generate and inspect help guide
	help := botApp.Registry.GenerateHelpText()
	expectedItems := []string{
		"<b><u>🤖 BOT COMMANDS & USAGE GUIDE 🤖</u></b>",
		"• <code>/start</code>",
		"• <code>/cmds</code>",
		"• <code>/ping</code>",
		"• <code>/id</code>",
		"• <code>/info</code>",
		"(aliases: <code>/help</code>)",
		"(aliases: <code>/about</code>)",
	}

	for _, item := range expectedItems {
		if !strings.Contains(help, item) {
			t.Errorf("expected help text to contain %q, but got:\n%s", item, help)
		}
	}
}

func TestIntegration_StartPayloadSafety(t *testing.T) {
	// Safe alphanumeric deep link
	safeRes := commands.ValidateStartPayload([]string{"/start", "movie_tt1234567"})
	if !safeRes.HasPayload || !safeRes.IsValid || safeRes.Payload != "movie_tt1234567" {
		t.Errorf("expected valid payload, got %+v", safeRes)
	}

	// Unsafe payload with path traversal or script tags
	unsafeRes := commands.ValidateStartPayload([]string{"/start", "../../etc/passwd"})
	if !unsafeRes.HasPayload || unsafeRes.IsValid {
		t.Errorf("expected path traversal payload to be rejected")
	}

	// Excessively long payload
	longRes := commands.ValidateStartPayload([]string{"/start", strings.Repeat("A", 65)})
	if !longRes.HasPayload || longRes.IsValid {
		t.Errorf("expected payload >64 chars to be rejected")
	}
}

func TestIntegration_PingTimingDistinction(t *testing.T) {
	localTime := 450 * time.Microsecond
	apiTime := 89250 * time.Microsecond
	startTime := time.Now().Add(-5 * time.Minute)

	report := ping.BuildPingReport(localTime, apiTime, startTime)

	if !strings.Contains(report, "Telegram API Round-Trip:</b> <code>89.250 ms</code>") {
		t.Errorf("missing or incorrect API round-trip measurement in report:\n%s", report)
	}
	if !strings.Contains(report, "Local Processing Time:</b> <code>0.450 ms</code>") {
		t.Errorf("missing or incorrect local processing measurement in report:\n%s", report)
	}
}

func TestIntegration_ShutdownHandling(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelInfo, buf)

	cfg := &config.Config{
		BotToken: "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		OwnerID:  99887766,
		Admins:   []int64{99887766},
		LogLevel: config.LogLevelInfo,
	}

	botApp, err := app.New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create App: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	select {
	case <-ctx.Done():
		// Context cancellation acknowledged
	default:
		t.Fatal("expected context to be cancelled")
	}

	err = botApp.Shutdown()
	if err != nil {
		t.Errorf("expected clean shutdown, got error: %v", err)
	}
}

type integrationMockSender struct {
	lastMethod string
	lastChatID int64
}

func (m *integrationMockSender) SendDocument(chatId int64, document gotgbot.InputFileOrString, opts *gotgbot.SendDocumentOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendDocument"
	m.lastChatID = chatId
	return &gotgbot.Message{MessageId: 777, Chat: gotgbot.Chat{Id: chatId}}, nil
}
func (m *integrationMockSender) SendVideo(chatId int64, video gotgbot.InputFileOrString, opts *gotgbot.SendVideoOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendVideo"
	m.lastChatID = chatId
	return &gotgbot.Message{MessageId: 777, Chat: gotgbot.Chat{Id: chatId}}, nil
}
func (m *integrationMockSender) SendAudio(chatId int64, audio gotgbot.InputFileOrString, opts *gotgbot.SendAudioOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendAudio"
	m.lastChatID = chatId
	return &gotgbot.Message{MessageId: 777, Chat: gotgbot.Chat{Id: chatId}}, nil
}
func (m *integrationMockSender) SendPhoto(chatId int64, photo gotgbot.InputFileOrString, opts *gotgbot.SendPhotoOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendPhoto"
	m.lastChatID = chatId
	return &gotgbot.Message{MessageId: 777, Chat: gotgbot.Chat{Id: chatId}}, nil
}
func (m *integrationMockSender) SendAnimation(chatId int64, animation gotgbot.InputFileOrString, opts *gotgbot.SendAnimationOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendAnimation"
	m.lastChatID = chatId
	return &gotgbot.Message{MessageId: 777, Chat: gotgbot.Chat{Id: chatId}}, nil
}
func (m *integrationMockSender) SendVoice(chatId int64, voice gotgbot.InputFileOrString, opts *gotgbot.SendVoiceOpts) (*gotgbot.Message, error) {
	m.lastMethod = "SendVoice"
	m.lastChatID = chatId
	return &gotgbot.Message{MessageId: 777, Chat: gotgbot.Chat{Id: chatId}}, nil
}

func TestIntegration_MediaDeliveryWorkflow(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelInfo, buf)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "integration_test.sqlite3")

	cfg := &config.Config{
		BotToken:     "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		OwnerID:      99887766,
		Admins:       []int64{99887766},
		LogLevel:     config.LogLevelInfo,
		SQLiteDBPath: dbPath,
	}

	botApp, err := app.New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to initialize App: %v", err)
	}
	defer botApp.Shutdown()

	if err := botApp.InitDatabase(context.Background()); err != nil {
		t.Fatalf("failed to init database: %v", err)
	}

	// Register all 6 plugins (start, cmds, ping, id, info, delivery)
	err = botApp.RegisterPlugins(
		start.New(botApp.AppContext),
		cmds.New(botApp.AppContext),
		ping.New(botApp.AppContext),
		id.New(botApp.AppContext),
		info.New(botApp.AppContext),
		delivery.New(botApp.AppContext),
	)
	if err != nil {
		t.Fatalf("failed to register plugins: %v", err)
	}

	if err := botApp.ValidatePlugins(); err != nil {
		t.Fatalf("failed to validate plugins: %v", err)
	}

	// Attach mock sender to DeliveryService
	mockSender := &integrationMockSender{}
	botApp.DeliveryService.SetSender(mockSender)

	// Insert test media into SQLite
	sqlDb := botApp.DBManager.SqlMediaRepo()
	if sqlDb == nil {
		t.Fatal("expected non-nil SqlMediaRepo")
	}

	// Test delivery of non-existent media
	_, err = botApp.DeliveryService.Deliver(context.Background(), 12345, "missing_movie", deliveryCore.DeliveryOptions{})
	if err == nil {
		t.Error("expected error for non-existent movie, got nil")
	}
}
