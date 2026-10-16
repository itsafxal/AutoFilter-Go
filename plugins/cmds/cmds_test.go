package cmds

import (
	"strings"
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestCmdsPlugin_Metadata(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.Name() != "cmds" {
		t.Errorf("expected plugin name 'cmds', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Name != "cmds" {
		t.Errorf("expected command name 'cmds', got %q", cmds[0].Name)
	}
	if len(cmds[0].Aliases) == 0 || cmds[0].Aliases[0] != "help" {
		t.Errorf("expected alias 'help', got %v", cmds[0].Aliases)
	}
}

func TestCmdsPlugin_DynamicHelpContent(t *testing.T) {
	appCtx := &plugins.AppContext{}
	reg := plugins.NewRegistry(appCtx)
	p := New(appCtx)

	_ = reg.Register(p)
	_ = reg.Validate()

	help := reg.GenerateHelpText()
	if !strings.Contains(help, "/cmds") {
		t.Errorf("expected help text to contain /cmds, got: %s", help)
	}
	if !strings.Contains(help, "/help") {
		t.Errorf("expected help text to contain alias /help, got: %s", help)
	}
}
