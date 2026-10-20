package stream

import (
	"strings"
	"testing"
	"time"

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
	hasCloseCb := false
	for _, cb := range cbs {
		if cb.Prefix == "fast_download#" {
			hasFastDLCb = true
		}
		if cb.Prefix == "stream_links#" {
			hasStreamLinksCb = true
		}
		if cb.Prefix == "close_data" {
			hasCloseCb = true
		}
	}
	if !hasFastDLCb {
		t.Error("expected 'fast_download#' callback handler to be registered")
	}
	if !hasStreamLinksCb {
		t.Error("expected 'stream_links#' callback handler to be registered")
	}
	if !hasCloseCb {
		t.Error("expected 'close_data' callback handler to be registered")
	}
}

func TestStreamPlugin_FastDownloadDurationsAndFormat(t *testing.T) {
	plugin := New(&plugins.AppContext{})

	// 1. Defaults verification
	linkExpiry := plugin.getFastDownloadLinkExpiry()
	if linkExpiry != 30*time.Minute {
		t.Errorf("expected default link expiry 30m, got %v", linkExpiry)
	}

	msgDur := plugin.getFastDownloadMsgDeleteDuration()
	if msgDur != 5*time.Minute {
		t.Errorf("expected default message auto-delete duration 5m, got %v", msgDur)
	}

	// 2. Formatting verification
	text := formatFastDownloadText("sample_movie.mkv", "1.2 GB", "http://localhost:8080/dl/xyz", msgDur, linkExpiry)
	if !strings.Contains(text, "sample_movie.mkv") {
		t.Error("expected text to contain file name")
	}
	if !strings.Contains(text, "1.2 GB") {
		t.Error("expected text to contain size")
	}
	if !strings.Contains(text, "Message Auto-Delete In:</b> <code>5 minutes</code>") {
		t.Errorf("expected text to specify 5 minutes message auto-delete, got:\n%s", text)
	}
	if !strings.Contains(text, "Link Validity:</b> <code>30 minutes</code>") {
		t.Errorf("expected text to specify 30 minutes link validity, got:\n%s", text)
	}
	if !strings.Contains(text, "http://localhost:8080/dl/xyz") {
		t.Error("expected text to contain download URL")
	}
}
