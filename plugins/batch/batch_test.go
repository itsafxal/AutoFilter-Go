package batch_test

import (
	"testing"

	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
	batchPlugin "github.com/itsafxal/AutoFilter-Go/plugins/batch"
)

func TestBatchPluginMetadata(t *testing.T) {
	appCtx := &plugins.AppContext{}
	p := batchPlugin.New(appCtx)

	if p.Name() != "batch" {
		t.Errorf("expected plugin name 'batch', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(cmds))
	}

	cmdMap := make(map[string]bool)
	for _, cmd := range cmds {
		cmdMap[cmd.Name] = true
	}

	if !cmdMap["batch"] || !cmdMap["batchsearch"] || !cmdMap["batchlist"] {
		t.Errorf("missing expected commands; found: %+v", cmdMap)
	}

	callbacks := p.CallbackHandlers()
	if len(callbacks) < 6 {
		t.Errorf("expected at least 6 callback handlers, got %d", len(callbacks))
	}

	prefixMap := make(map[string]bool)
	for _, cb := range callbacks {
		prefixMap[cb.Prefix] = true
	}

	expectedPrefixes := []string{
		"batch_done:",
		"batch_cancel:",
		"batch_page:",
		"batch_spage:",
		"batch_view:",
		"batch_del:",
		"batch_noop",
	}

	for _, ep := range expectedPrefixes {
		if !prefixMap[ep] {
			t.Errorf("missing callback prefix %q", ep)
		}
	}

	// Verify MessageHandlers
	msgHandlers := p.MessageHandlers()
	if len(msgHandlers) != 1 {
		t.Fatalf("expected 1 message handler, got %d", len(msgHandlers))
	}
	if msgHandlers[0].Handler == nil || msgHandlers[0].Filter == nil {
		t.Error("expected non-nil filter and handler in message handler")
	}
}
