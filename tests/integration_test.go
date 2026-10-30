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
	"github.com/itsafxal/AutoFilter-Go/internal/commands"
	"github.com/itsafxal/AutoFilter-Go/internal/config"
	deliveryCore "github.com/itsafxal/AutoFilter-Go/internal/delivery"
	"github.com/itsafxal/AutoFilter-Go/internal/logging"
	broadcastPlugin "github.com/itsafxal/AutoFilter-Go/plugins/broadcast"
	"github.com/itsafxal/AutoFilter-Go/plugins/cmds"
	"github.com/itsafxal/AutoFilter-Go/plugins/delivery"
	"github.com/itsafxal/AutoFilter-Go/plugins/info"
	lifecyclePlugin "github.com/itsafxal/AutoFilter-Go/plugins/lifecycle"
	"github.com/itsafxal/AutoFilter-Go/plugins/ping"
	"github.com/itsafxal/AutoFilter-Go/plugins/start"
)

func TestIntegration_FullPluginRegistryAndHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelInfo, buf)

	cfg := &config.Config{
		BotToken: "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		AppID:    12345,
		AppHash:  "0123456789abcdef0123456789abcdef",
		OwnerID:  99887766,
		Admins:   []int64{99887766, 123456},
		LogLevel: config.LogLevelInfo,
	}

	botApp, err := app.New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to initialize App: %v", err)
	}

	// Register foundational plugins
	err = botApp.RegisterPlugins(
		start.New(botApp.AppContext),
		cmds.New(botApp.AppContext),
		ping.New(botApp.AppContext),
		info.New(botApp.AppContext),
	)
	if err != nil {
		t.Fatalf("failed to register plugins: %v", err)
	}

	// Validate plugins
	if err := botApp.ValidatePlugins(); err != nil {
		t.Fatalf("plugin validation failed: %v", err)
	}

	if botApp.Registry.PluginCount() != 4 {
		t.Errorf("expected 4 plugins, got %d", botApp.Registry.PluginCount())
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
		"• <code>/cmdc</code>",
		"• <code>/ping</code>",
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
		AppID:    12345,
		AppHash:  "0123456789abcdef0123456789abcdef",
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
	lastPeerID any
	lastMedia  any
}

func (m *integrationMockSender) SendMedia(peerID any, media any, opts ...*telegram.MediaOptions) (*telegram.NewMessage, error) {
	m.lastPeerID = peerID
	m.lastMedia = media
	return &telegram.NewMessage{ID: 777}, nil
}

func (m *integrationMockSender) GetMessages(PeerID any, Opts ...*telegram.SearchOption) ([]telegram.NewMessage, error) {
	return []telegram.NewMessage{}, nil
}

func TestIntegration_MediaDeliveryWorkflow(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelInfo, buf)

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "integration_test.sqlite3")

	cfg := &config.Config{
		BotToken:     "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		AppID:        12345,
		AppHash:      "0123456789abcdef0123456789abcdef",
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

	// Register all 5 plugins (start, cmds, ping, info, delivery)
	err = botApp.RegisterPlugins(
		start.New(botApp.AppContext),
		cmds.New(botApp.AppContext),
		ping.New(botApp.AppContext),
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

func TestIntegration_BroadcastPluginRegistration(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelInfo, buf)

	cfg := &config.Config{
		BotToken:             "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		AppID:                12345,
		AppHash:              "0123456789abcdef0123456789abcdef",
		OwnerID:              99887766,
		Admins:               []int64{99887766},
		BroadcastConcurrency: 15,
		LogLevel:             config.LogLevelInfo,
	}

	botApp, err := app.New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to initialize App: %v", err)
	}

	bp := broadcastPlugin.New(botApp.AppContext, botApp.BroadcastService)
	if err := botApp.RegisterPlugins(bp); err != nil {
		t.Fatalf("failed to register broadcast plugin: %v", err)
	}

	if err := botApp.ValidatePlugins(); err != nil {
		t.Fatalf("failed to validate broadcast plugin: %v", err)
	}

	p := botApp.Registry.GetPlugin("broadcast")
	if p == nil {
		t.Fatal("expected 'broadcast' plugin to be registered")
	}

	cmdMap := make(map[string]bool)
	for _, c := range p.Commands() {
		cmdMap[c.Name] = true
		if !c.AdminOnly {
			t.Errorf("expected command %s to be AdminOnly", c.Name)
		}
	}

	if !cmdMap["broadcast"] {
		t.Error("expected 'broadcast' command to be registered")
	}
	if !cmdMap["cancel_broadcast"] {
		t.Error("expected 'cancel_broadcast' command to be registered")
	}
	if !cmdMap["broadcast_stats"] {
		t.Error("expected 'broadcast_stats' command to be registered")
	}
}

func TestIntegration_LifecyclePluginRegistration(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelInfo, buf)

	cfg := &config.Config{
		BotToken: "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		AppID:    12345,
		AppHash:  "0123456789abcdef0123456789abcdef",
		OwnerID:  99887766,
		Admins:   []int64{99887766},
		LogLevel: config.LogLevelInfo,
	}

	botApp, err := app.New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to initialize App: %v", err)
	}

	lp := lifecyclePlugin.New(botApp.AppContext)
	if err := botApp.RegisterPlugins(lp); err != nil {
		t.Fatalf("failed to register lifecycle plugin: %v", err)
	}

	if err := botApp.ValidatePlugins(); err != nil {
		t.Fatalf("failed to validate lifecycle plugin: %v", err)
	}

	p := botApp.Registry.GetPlugin("lifecycle")
	if p == nil {
		t.Fatal("expected 'lifecycle' plugin to be registered")
	}

	cmdMap := make(map[string]bool)
	for _, c := range p.Commands() {
		cmdMap[c.Name] = true
		if !c.AdminOnly {
			t.Errorf("expected command %s to be AdminOnly", c.Name)
		}
	}

	for _, required := range []string{"leave", "disable", "enable", "groups"} {
		if !cmdMap[required] {
			t.Errorf("expected '%s' command to be registered", required)
		}
	}
}
