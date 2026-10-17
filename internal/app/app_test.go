package app

import (
	"bytes"
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/logging"
	"github.com/itsafxal/MYTGBOTZ/plugins/cmds"
	"github.com/itsafxal/MYTGBOTZ/plugins/id"
	"github.com/itsafxal/MYTGBOTZ/plugins/info"
	"github.com/itsafxal/MYTGBOTZ/plugins/ping"
	"github.com/itsafxal/MYTGBOTZ/plugins/start"
)

func TestApp_Lifecycle(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelDebug, buf)
	cfg := &config.Config{
		BotToken: "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		AppID:    12345,
		AppHash:  "0123456789abcdef0123456789abcdef",
		OwnerID:  99887766,
		Admins:   []int64{99887766},
		LogLevel: config.LogLevelDebug,
	}

	application, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create App: %v", err)
	}

	// Register all 5 core plugins
	err = application.RegisterPlugins(
		start.New(application.AppContext),
		cmds.New(application.AppContext),
		ping.New(application.AppContext),
		id.New(application.AppContext),
		info.New(application.AppContext),
	)
	if err != nil {
		t.Fatalf("failed to register plugins: %v", err)
	}

	// Validate plugins
	if err := application.ValidatePlugins(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if application.Registry.PluginCount() != 5 {
		t.Errorf("expected 5 plugins, got %d", application.Registry.PluginCount())
	}
	if application.Registry.CommandCount() != 5 {
		t.Errorf("expected 5 primary commands, got %d", application.Registry.CommandCount())
	}

	// Verify help generation includes all 5 commands
	help := application.Registry.GenerateHelpText()
	expectedCommands := []string{"/start", "/cmds", "/ping", "/id", "/info"}
	for _, expected := range expectedCommands {
		if !bytes.Contains([]byte(help), []byte(expected)) {
			t.Errorf("expected help text to contain %s, got:\n%s", expected, help)
		}
	}

	// Test shutdown
	if err := application.Shutdown(); err != nil {
		t.Errorf("unexpected error on shutdown: %v", err)
	}
}

func TestApp_NilConfigOrLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logging.NewLogger(config.LogLevelInfo, buf)

	_, err := New(nil, logger)
	if err == nil {
		t.Error("expected error when cfg is nil")
	}

	cfg := &config.Config{BotToken: "123:abc", OwnerID: 1}
	_, err = New(cfg, nil)
	if err == nil {
		t.Error("expected error when logger is nil")
	}
}
