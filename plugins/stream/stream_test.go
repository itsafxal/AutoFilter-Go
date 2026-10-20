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

	hasFastDL := false
	for _, cmd := range cmds {
		if cmd.Name == "fastdl" || cmd.Name == "stream" {
			hasFastDL = true
			if len(cmd.Aliases) == 0 {
				t.Error("expected aliases for command")
			}
			break
		}
	}
	if !hasFastDL {
		t.Error("expected 'fastdl' command to be registered")
	}

	cbs := plugin.CallbackHandlers()
	hasFastDLCb := false
	hasStreamLinksCb := false
	for _, cb := range cbs {
		if cb.Prefix == "fast_download#" {
			hasFastDLCb = true
		}
		if cb.Prefix == "stream_links#" {
			hasStreamLinksCb = true
		}
	}
	if !hasFastDLCb {
		t.Error("expected 'fast_download#' callback handler to be registered")
	}
	if !hasStreamLinksCb {
		t.Error("expected 'stream_links#' callback handler to be registered")
	}
}
