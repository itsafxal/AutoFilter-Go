package start

import (
	"strings"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
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

	validOldBot := commands.ValidateStartPayload([]string{"/start", "from_oldbot"})
	if !validOldBot.HasPayload || !validOldBot.IsValid {
		t.Errorf("expected valid from_oldbot payload, got has=%v, valid=%v", validOldBot.HasPayload, validOldBot.IsValid)
	}

	validSearch := commands.ValidateStartPayload([]string{"/start", "q_S0dG"})
	if !validSearch.HasPayload || !validSearch.IsValid {
		t.Errorf("expected valid q_ search payload, got has=%v, valid=%v", validSearch.HasPayload, validSearch.IsValid)
	}

	invalid := commands.ValidateStartPayload([]string{"/start", "invalid payload!"})
	if !invalid.HasPayload || invalid.IsValid {
		t.Errorf("expected invalid payload to fail validation")
	}
	if !strings.Contains(invalid.ErrorReason, "illegal characters") {
		t.Errorf("expected illegal characters reason, got: %s", invalid.ErrorReason)
	}
}

func TestStartPlugin_CallbacksAndContent(t *testing.T) {
	appCtx := &plugins.AppContext{}
	p := New(appCtx)

	// Check CallbackHandlers
	cbs := p.CallbackHandlers()
	if len(cbs) < 3 {
		t.Fatalf("expected at least 3 callback handlers, got %d", len(cbs))
	}

	prefixes := make(map[string]bool)
	for _, cb := range cbs {
		prefixes[cb.Prefix] = true
	}
	if !prefixes["start_help"] || !prefixes["start_about"] || !prefixes["start_home"] {
		t.Errorf("missing expected start callbacks, got %+v", prefixes)
	}

	// Test Welcome Content
	wText, wMarkup := p.buildWelcomeContent(nil, "TestBot")
	if !strings.Contains(wText, "𝙃𝙚𝙮 𝙩𝙝𝙚𝙧𝙚") || !strings.Contains(wText, "𝙖𝙪𝙩𝙤-𝙛𝙞𝙡𝙩𝙚𝙧 𝙗𝙤𝙩") {
		t.Errorf("unexpected welcome text: %s", wText)
	}

	userObj := &telegram.UserObj{ID: 123456, FirstName: "Alex"}
	uText, _ := p.buildWelcomeContent(userObj, "TestBot")
	if !strings.Contains(uText, "<a href=\"tg://user?id=123456\">Alex</a>") {
		t.Errorf("expected mention with user link, got: %s", uText)
	}

	if wMarkup == nil || len(wMarkup.Rows) != 2 {
		t.Fatalf("expected 2 keyboard rows in start menu, got %v", wMarkup)
	}
	// Row 1: Add me to group
	if len(wMarkup.Rows[0].Buttons) != 1 {
		t.Errorf("expected 1 button in row 1, got %d", len(wMarkup.Rows[0].Buttons))
	}
	// Row 2: Help, About
	if len(wMarkup.Rows[1].Buttons) != 2 {
		t.Errorf("expected 2 buttons in row 2 (Help, About), got %d", len(wMarkup.Rows[1].Buttons))
	}

	// Test Help Content
	hText, hMarkup := p.buildHelpContent()
	if !strings.Contains(hText, "/ping") || !strings.Contains(hText, "/latest") || !strings.Contains(hText, "/request") {
		t.Errorf("expected help text to contain commands, got: %s", hText)
	}
	if hMarkup == nil || len(hMarkup.Rows) != 1 || len(hMarkup.Rows[0].Buttons) != 2 {
		t.Errorf("expected 1 row with 2 buttons in help markup, got %v", hMarkup)
	}

	cleanText, tags, err := telegram.ParseHTMLToTags(hText)
	if err != nil {
		t.Fatalf("ParseHTMLToTags failed on helpText: %v", err)
	}
	t.Logf("Clean text: %s, tags count: %d", cleanText, len(tags))

	// Test About Content
	aText, aMarkup := p.buildAboutContent()
	if !strings.Contains(aText, "cybertg.vercel.app") || !strings.Contains(aText, "ᴍᴏɴɢᴏ ᴅʙ") || !strings.Contains(aText, "ǫᴜɪᴄᴋ ꜰᴀsᴛ") {
		t.Errorf("expected about text to contain custom about info, got: %s", aText)
	}
	if aMarkup == nil || len(aMarkup.Rows) != 1 || len(aMarkup.Rows[0].Buttons) != 2 {
		t.Errorf("expected 1 row with 2 buttons in about markup, got %v", aMarkup)
	}
}

