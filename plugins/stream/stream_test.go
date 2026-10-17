package stream

import (
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestStreamPlugin_Registration(t *testing.T) {
	plugin := New(&plugins.AppContext{})

	if plugin.Name() != "stream" {
		t.Errorf("expected plugin name 'stream', got %s", plugin.Name())
	}

	cmds := plugin.Commands()
	if len(cmds) == 0 {
		t.Fatal("expected at least 1 command")
	}

	hasStream := false
	for _, cmd := range cmds {
		if cmd.Name == "stream" {
			hasStream = true
			if len(cmd.Aliases) == 0 {
				t.Error("expected aliases for /stream command")
			}
			break
		}
	}
	if !hasStream {
		t.Error("expected 'stream' command to be registered")
	}

	cbs := plugin.CallbackHandlers()
	hasStreamLinksCb := false
	for _, cb := range cbs {
		if cb.Prefix == "stream_links#" {
			hasStreamLinksCb = true
			break
		}
	}
	if !hasStreamLinksCb {
		t.Error("expected 'stream_links#' callback handler to be registered")
	}
}
