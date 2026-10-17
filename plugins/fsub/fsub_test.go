package fsub

import (
	"io"
	"log/slog"
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
	fsubCore "github.com/itsafxal/MYTGBOTZ/internal/fsub"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestFSubPluginRegistration(t *testing.T) {
	cfg := &config.Config{
		OwnerID: 111111,
		Admins:  []int64{222222},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := fsubCore.NewRepository(nil)
	svc := fsubCore.NewService(repo, cfg, logger)

	appCtx := &plugins.AppContext{
		Config: cfg,
		Logger: logger,
		FSub:   svc,
	}

	plugin := New(appCtx, svc)
	if plugin.Name() != "fsub" {
		t.Fatalf("expected plugin name 'fsub', got %s", plugin.Name())
	}

	cmds := plugin.Commands()
	if len(cmds) != 16 {
		t.Fatalf("expected 16 fsub commands, got %d", len(cmds))
	}

	expectedTriggers := map[string]bool{
		"sub1":        true,
		"sub2":        true,
		"sub3":        true,
		"2sub1":       true,
		"2sub2":       true,
		"2sub3":       true,
		"setcount1":   true,
		"setcount2":   true,
		"setcount3":   true,
		"view_sub":    true,
		"delsub1":     true,
		"delsub2":     true,
		"delsub3":     true,
		"2delsub1":    true,
		"2delsub2":    true,
		"2delsub3":    true,
	}

	for _, cmd := range cmds {
		if !expectedTriggers[cmd.Name] {
			t.Errorf("unexpected command: %s", cmd.Name)
		}
		if !cmd.AdminOnly {
			t.Errorf("expected command %s to be AdminOnly", cmd.Name)
		}
		if cmd.Handler == nil {
			t.Errorf("command %s has nil handler", cmd.Name)
		}
	}

	// Verify JoinRequestHandlers
	jrhs := plugin.JoinRequestHandlers()
	if len(jrhs) != 1 {
		t.Fatalf("expected 1 join request handler, got %d", len(jrhs))
	}

	// Register in Registry and validate
	reg := plugins.NewRegistry(appCtx)
	if err := reg.Register(plugin); err != nil {
		t.Fatalf("failed to register fsub plugin in registry: %v", err)
	}
	if err := reg.Validate(); err != nil {
		t.Fatalf("failed to validate registry with fsub plugin: %v", err)
	}
}
