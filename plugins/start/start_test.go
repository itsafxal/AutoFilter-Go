package start

import (
	"strings"
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestStartPlugin_Metadata(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.Name() != "start" {
		t.Errorf("expected plugin name 'start', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Name != "start" {
		t.Errorf("expected command name 'start', got %q", cmds[0].Name)
	}
	if cmds[0].Handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestStartPlugin_PayloadValidation(t *testing.T) {
	valid := commands.ValidateStartPayload([]string{"/start", "file_12345"})
	if !valid.HasPayload || !valid.IsValid {
		t.Errorf("expected valid payload, got has=%v, valid=%v", valid.HasPayload, valid.IsValid)
	}

	invalid := commands.ValidateStartPayload([]string{"/start", "invalid payload!"})
	if !invalid.HasPayload || invalid.IsValid {
		t.Errorf("expected invalid payload to fail validation")
	}
	if !strings.Contains(invalid.ErrorReason, "illegal characters") {
		t.Errorf("expected illegal characters reason, got: %s", invalid.ErrorReason)
	}
}
