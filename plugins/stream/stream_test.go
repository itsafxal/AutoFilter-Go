package stream

import (
	"strings"
	"testing"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
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

func TestStreamPlugin_FastDownloadKeyboardStyles(t *testing.T) {
	// 1. Default style should be Green (.Success())
	pluginDefault := New(&plugins.AppContext{})
	kbDefault := pluginDefault.buildFastDownloadKeyboard("https://stream.example.com/dl/xyz")
	if kbDefault == nil {
		t.Fatal("expected keyboard to be built for valid URL")
	}
	if len(kbDefault.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(kbDefault.Rows))
	}

	dlBtn, ok := kbDefault.Rows[0].Buttons[0].(*telegram.KeyboardButtonURL)
	if !ok {
		t.Fatalf("expected *telegram.KeyboardButtonURL, got %T", kbDefault.Rows[0].Buttons[0])
	}
	if dlBtn.Style == nil || !dlBtn.Style.BgSuccess {
		t.Errorf("expected default Fast Download button to have BgSuccess (Green) style, got %v", dlBtn.Style)
	}

	closeBtn, ok := kbDefault.Rows[1].Buttons[0].(*telegram.KeyboardButtonCallback)
	if !ok {
		t.Fatalf("expected *telegram.KeyboardButtonCallback, got %T", kbDefault.Rows[1].Buttons[0])
	}
	if closeBtn.Style == nil || !closeBtn.Style.BgDanger {
		t.Errorf("expected Close button to have BgDanger (Red) style, got %v", closeBtn.Style)
	}

	// 2. Configured Blue style (.Primary())
	pluginBlue := New(&plugins.AppContext{
		Config: &config.Config{
			FastDownloadButtonStyle: "blue",
		},
	})
	kbBlue := pluginBlue.buildFastDownloadKeyboard("https://stream.example.com/dl/xyz")
	dlBtnBlue := kbBlue.Rows[0].Buttons[0].(*telegram.KeyboardButtonURL)
	if dlBtnBlue.Style == nil || !dlBtnBlue.Style.BgPrimary {
		t.Errorf("expected Fast Download button with 'blue' style to have BgPrimary style, got %v", dlBtnBlue.Style)
	}

	// 3. Premium Emoji enabled
	pluginPremium := New(&plugins.AppContext{
		Config: &config.Config{
			EnablePremiumEmoji:        true,
			FastDownloadButtonEmojiID: 5195033767969839232,
		},
	})
	kbPremium := pluginPremium.buildFastDownloadKeyboard("https://stream.example.com/dl/xyz")
	dlBtnPremium := kbPremium.Rows[0].Buttons[0].(*telegram.KeyboardButtonURL)
	if dlBtnPremium.Text != "Fast Download ⚡" {
		t.Errorf("expected button text 'Fast Download ⚡', got %q", dlBtnPremium.Text)
	}
	if dlBtnPremium.Style == nil || dlBtnPremium.Style.Icon != 5195033767969839232 {
		t.Errorf("expected custom emoji icon 5195033767969839232, got %v", dlBtnPremium.Style)
	}
}
