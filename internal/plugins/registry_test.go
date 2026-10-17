package plugins

import (
	"strings"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
)

type mockPlugin struct {
	name     string
	commands []CommandDefinition
}

func (m *mockPlugin) Name() string {
	return m.name
}

func (m *mockPlugin) Commands() []CommandDefinition {
	return m.commands
}

func dummyHandler(m *telegram.NewMessage) error {
	return nil
}

func TestRegistry_Success(t *testing.T) {
	r := NewRegistry(nil)

	p1 := &mockPlugin{
		name: "start",
		commands: []CommandDefinition{
			{
				Name:        "start",
				Aliases:     []string{"begin"},
				Description: "Start the bot",
				Category:    "General",
				Handler:     dummyHandler,
			},
		},
	}

	p2 := &mockPlugin{
		name: "ping",
		commands: []CommandDefinition{
			{
				Name:        "ping",
				Description: "Ping the bot",
				Category:    "General",
				Handler:     dummyHandler,
			},
		},
	}

	if err := r.Register(p1, p2); err != nil {
		t.Fatalf("failed to register plugins: %v", err)
	}

	if err := r.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if r.PluginCount() != 2 {
		t.Errorf("expected 2 plugins, got %d", r.PluginCount())
	}
	if r.CommandCount() != 2 {
		t.Errorf("expected 2 commands, got %d", r.CommandCount())
	}

	help := r.GenerateHelpText()
	if !strings.Contains(help, "/start") || !strings.Contains(help, "/ping") {
		t.Errorf("expected help text to contain /start and /ping, got:\n%s", help)
	}
	if !strings.Contains(help, "/begin") {
		t.Errorf("expected help text to mention alias /begin, got:\n%s", help)
	}
}

func TestRegistry_DuplicatePluginName(t *testing.T) {
	r := NewRegistry(nil)

	p1 := &mockPlugin{name: "ping", commands: []CommandDefinition{}}
	p2 := &mockPlugin{name: "ping", commands: []CommandDefinition{}}

	if err := r.Register(p1); err != nil {
		t.Fatalf("failed to register p1: %v", err)
	}
	err := r.Register(p2)
	if err == nil {
		t.Fatal("expected error registering duplicate plugin name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate plugin") {
		t.Errorf("expected duplicate plugin error, got: %v", err)
	}
}

func TestRegistry_DuplicateCommandName(t *testing.T) {
	r := NewRegistry(nil)

	p1 := &mockPlugin{
		name: "plugin1",
		commands: []CommandDefinition{
			{Name: "ping", Description: "Ping 1", Handler: dummyHandler},
		},
	}
	p2 := &mockPlugin{
		name: "plugin2",
		commands: []CommandDefinition{
			{Name: "ping", Description: "Ping 2", Handler: dummyHandler},
		},
	}

	_ = r.Register(p1, p2)
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate command name, got nil")
	}
	if !strings.Contains(err.Error(), "command conflict") {
		t.Errorf("expected conflict error, got: %v", err)
	}
}

func TestRegistry_ConflictingAliasWithCommand(t *testing.T) {
	r := NewRegistry(nil)

	p1 := &mockPlugin{
		name: "plugin1",
		commands: []CommandDefinition{
			{Name: "help", Description: "Help", Handler: dummyHandler},
		},
	}
	p2 := &mockPlugin{
		name: "plugin2",
		commands: []CommandDefinition{
			{Name: "cmds", Aliases: []string{"help"}, Description: "Commands", Handler: dummyHandler},
		},
	}

	_ = r.Register(p1, p2)
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for alias conflicting with existing command, got nil")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Errorf("expected conflict error, got: %v", err)
	}
}

func TestRegistry_ConflictingAliasWithAlias(t *testing.T) {
	r := NewRegistry(nil)

	p1 := &mockPlugin{
		name: "plugin1",
		commands: []CommandDefinition{
			{Name: "info", Aliases: []string{"about"}, Description: "Info", Handler: dummyHandler},
		},
	}
	p2 := &mockPlugin{
		name: "plugin2",
		commands: []CommandDefinition{
			{Name: "botinfo", Aliases: []string{"about"}, Description: "Bot info", Handler: dummyHandler},
		},
	}

	_ = r.Register(p1, p2)
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate alias across commands, got nil")
	}
	if !strings.Contains(err.Error(), "alias conflict") {
		t.Errorf("expected alias conflict error, got: %v", err)
	}
}

func TestRegistry_SelfAlias(t *testing.T) {
	r := NewRegistry(nil)

	p := &mockPlugin{
		name: "plugin1",
		commands: []CommandDefinition{
			{Name: "ping", Aliases: []string{"ping"}, Description: "Ping", Handler: dummyHandler},
		},
	}

	_ = r.Register(p)
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error when command aliases itself, got nil")
	}
	if !strings.Contains(err.Error(), "defines itself as an alias") {
		t.Errorf("expected self-alias error, got: %v", err)
	}
}

func TestRegistry_NilHandler(t *testing.T) {
	r := NewRegistry(nil)

	p := &mockPlugin{
		name: "plugin1",
		commands: []CommandDefinition{
			{Name: "test", Description: "Test", Handler: nil},
		},
	}

	_ = r.Register(p)
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for nil handler, got nil")
	}
	if !strings.Contains(err.Error(), "nil handler") {
		t.Errorf("expected nil handler error, got: %v", err)
	}
}

func TestRegistry_EmptyDescription(t *testing.T) {
	r := NewRegistry(nil)

	p := &mockPlugin{
		name: "plugin1",
		commands: []CommandDefinition{
			{Name: "test", Description: "", Handler: dummyHandler},
		},
	}

	_ = r.Register(p)
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for empty description, got nil")
	}
	if !strings.Contains(err.Error(), "empty description") {
		t.Errorf("expected empty description error, got: %v", err)
	}
}

func TestRegistry_CannotRegisterAfterValidation(t *testing.T) {
	r := NewRegistry(nil)
	p := &mockPlugin{
		name: "p1",
		commands: []CommandDefinition{
			{Name: "test", Description: "desc", Handler: dummyHandler},
		},
	}

	_ = r.Register(p)
	_ = r.Validate()

	err := r.Register(&mockPlugin{name: "p2"})
	if err == nil {
		t.Fatal("expected error registering after validation, got nil")
	}
}

type mockMessagePlugin struct {
	mockPlugin
	msgHandlers []MessageDefinition
	cbHandlers  []CallbackDefinition
}

func (m *mockMessagePlugin) MessageHandlers() []MessageDefinition {
	return m.msgHandlers
}

func (m *mockMessagePlugin) CallbackHandlers() []CallbackDefinition {
	return m.cbHandlers
}

func TestRegistry_AttachToClient_MessageHandlers(t *testing.T) {
	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:         12345,
		AppHash:       "0123456789abcdef0123456789abcdef",
		MemorySession: true,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	r := NewRegistry(nil)
	msgPlug := &mockMessagePlugin{
		mockPlugin: mockPlugin{
			name: "search_msg",
			commands: []CommandDefinition{
				{Name: "search", Description: "search files", Handler: dummyHandler},
			},
		},
		msgHandlers: []MessageDefinition{
			{
				Filter: func(m *telegram.NewMessage) bool {
					return m != nil && m.Text() != ""
				},
				Handler: dummyHandler,
			},
		},
		cbHandlers: []CallbackDefinition{
			{
				Prefix: "test#",
				Handler: func(cb *telegram.CallbackQuery) error {
					return nil
				},
			},
		},
	}

	if err := r.Register(msgPlug); err != nil {
		t.Fatalf("failed to register plugin: %v", err)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("failed to validate: %v", err)
	}

	if err := r.AttachToClient(client); err != nil {
		t.Fatalf("AttachToClient failed: %v", err)
	}
}

