package fsub

import (
	"io"
	"log/slog"
	"testing"

	"github.com/itsafxal/AutoFilter-Go/internal/config"
	fsubCore "github.com/itsafxal/AutoFilter-Go/internal/fsub"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
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
		"sub1":      true,
		"sub2":      true,
		"sub3":      true,
		"2sub1":     true,
		"2sub2":     true,
		"2sub3":     true,
		"setcount1": true,
		"setcount2": true,
		"setcount3": true,
		"view_sub":  true,
		"delsub1":   true,
		"delsub2":   true,
		"delsub3":   true,
		"2delsub1":  true,
		"2delsub2":  true,
		"2delsub3":  true,
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

	// Verify MessageHandlers for forwarded sub commands
	mhs := plugin.MessageHandlers()
	if len(mhs) != 1 {
		t.Fatalf("expected 1 message handler for forwarded commands, got %d", len(mhs))
	}
	if mhs[0].Filter == nil || mhs[0].Handler == nil {
		t.Fatalf("expected non-nil filter and handler in message definition")
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

func TestFormatChannelChatID(t *testing.T) {
	tests := []struct {
		input    int64
		expected int64
	}{
		{input: 1234567890, expected: -1001234567890},
		{input: 987654321, expected: -100987654321},
		{input: -1001234567890, expected: -1001234567890},
		{input: -123456, expected: -123456},
	}

	for _, tc := range tests {
		actual := formatChannelChatID(tc.input)
		if actual != tc.expected {
			t.Errorf("formatChannelChatID(%d) = %d; expected %d", tc.input, actual, tc.expected)
		}
	}
}

func TestParseSubTrigger(t *testing.T) {
	tests := []struct {
		text        string
		expectedSub subCommandType
		expectedOk  bool
	}{
		// Primary channels
		{text: "/sub1", expectedSub: subCommandType{slot: 1, secondary: false}, expectedOk: true},
		{text: "sub1", expectedSub: subCommandType{slot: 1, secondary: false}, expectedOk: true},
		{text: "!sub1", expectedSub: subCommandType{slot: 1, secondary: false}, expectedOk: true},
		{text: "/sub1@MyBot", expectedSub: subCommandType{slot: 1, secondary: false}, expectedOk: true},
		{text: "/set_sub1", expectedSub: subCommandType{slot: 1, secondary: false}, expectedOk: true},
		{text: "sub2", expectedSub: subCommandType{slot: 2, secondary: false}, expectedOk: true},
		{text: "/sub3", expectedSub: subCommandType{slot: 3, secondary: false}, expectedOk: true},

		// Secondary channels
		{text: "/2sub1", expectedSub: subCommandType{slot: 1, secondary: true}, expectedOk: true},
		{text: "2sub1", expectedSub: subCommandType{slot: 1, secondary: true}, expectedOk: true},
		{text: "/setchat1_2", expectedSub: subCommandType{slot: 1, secondary: true}, expectedOk: true},
		{text: "2sub2", expectedSub: subCommandType{slot: 2, secondary: true}, expectedOk: true},
		{text: "/2sub3", expectedSub: subCommandType{slot: 3, secondary: true}, expectedOk: true},
		{text: "setchat3_2", expectedSub: subCommandType{slot: 3, secondary: true}, expectedOk: true},

		// Invalid triggers
		{text: "hello", expectedSub: subCommandType{}, expectedOk: false},
		{text: "/start", expectedSub: subCommandType{}, expectedOk: false},
		{text: "sub4", expectedSub: subCommandType{}, expectedOk: false},
	}

	for _, tc := range tests {
		sub, ok := parseSubTrigger(tc.text)
		if ok != tc.expectedOk {
			t.Errorf("parseSubTrigger(%q) ok = %v; expected %v", tc.text, ok, tc.expectedOk)
		}
		if ok && sub != tc.expectedSub {
			t.Errorf("parseSubTrigger(%q) = %+v; expected %+v", tc.text, sub, tc.expectedSub)
		}
	}
}

func TestFSubServiceDefaultDescription(t *testing.T) {
	cfg := &config.Config{
		FSubChannelDescription: "Custom Join Prompt 🌟",
	}
	svc := fsubCore.NewService(nil, cfg, nil)
	if svc.GetDefaultChannelDescription() != "Custom Join Prompt 🌟" {
		t.Errorf("expected custom description, got %s", svc.GetDefaultChannelDescription())
	}

	svcDefault := fsubCore.NewService(nil, &config.Config{}, nil)
	if svcDefault.GetDefaultChannelDescription() != "You need to request to join channel to get file 🚀" {
		t.Errorf("expected default description, got %s", svcDefault.GetDefaultChannelDescription())
	}
}
