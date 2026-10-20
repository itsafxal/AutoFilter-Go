package cmds

import (
	"strings"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestCmdsPlugin_Metadata(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.Name() != "cmds" {
		t.Errorf("expected plugin name 'cmds', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands (cmds, cmdc), got %d", len(cmds))
	}
	if cmds[0].Name != "cmds" {
		t.Errorf("expected command name 'cmds', got %q", cmds[0].Name)
	}
	if len(cmds[0].Aliases) == 0 || cmds[0].Aliases[0] != "help" {
		t.Errorf("expected alias 'help', got %v", cmds[0].Aliases)
	}
	if cmds[1].Name != "cmdc" {
		t.Errorf("expected command name 'cmdc', got %q", cmds[1].Name)
	}
	if !cmds[1].AdminOnly {
		t.Errorf("expected cmdc to be AdminOnly")
	}

	// Verify callback handlers
	cbs := p.CallbackHandlers()
	if len(cbs) < 3 {
		t.Fatalf("expected at least 3 callback handlers, got %d", len(cbs))
	}

	cbMap := make(map[string]bool)
	for _, cb := range cbs {
		cbMap[cb.Prefix] = true
	}
	if !cbMap["cmdc_cat#"] || !cbMap["cmdc_home"] || !cbMap["close_data"] {
		t.Errorf("missing expected callback prefix in %v", cbMap)
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

	adminHelp := reg.GenerateAdminHelpText()
	if !strings.Contains(adminHelp, "/cmdc") {
		t.Errorf("expected admin help text to contain /cmdc, got: %s", adminHelp)
	}
}

func TestCmdsPlugin_InteractiveMenus(t *testing.T) {
	p := New(&plugins.AppContext{})

	// 1. Main Admin Menu
	menuText, menuMarkup := p.buildCmdcMainMenu()
	if !strings.Contains(menuText, "BOT COMMANDS MASTER PANEL (ADMIN)") {
		t.Errorf("unexpected menu text: %s", menuText)
	}
	if menuMarkup == nil || len(menuMarkup.Rows) < 4 {
		t.Fatalf("expected at least 4 rows in menuMarkup, got %v", menuMarkup)
	}

	// Check Close button is Danger (Red) in main menu
	lastRow := menuMarkup.Rows[len(menuMarkup.Rows)-1].Buttons
	if len(lastRow) != 1 {
		t.Fatalf("expected 1 button in last row, got %d", len(lastRow))
	}
	closeBtn, ok := lastRow[0].(*telegram.KeyboardButtonCallback)
	if !ok || closeBtn.Style == nil || !closeBtn.Style.BgDanger {
		t.Errorf("expected Close button to have BgDanger=true (Red)")
	}

	// 2. Category Views
	categories := []string{"fsub", "autodel", "caption", "media", "general"}
	for _, cat := range categories {
		catText, catMarkup := p.buildCmdcCategoryView(cat)
		if len(catText) == 0 {
			t.Errorf("expected non-empty text for category %s", cat)
		}
		if catMarkup == nil || len(catMarkup.Rows) != 1 {
			t.Fatalf("expected 1 row in category markup for %s", cat)
		}

		// Ensure Back button and Close button (Red)
		row := catMarkup.Rows[0].Buttons
		if len(row) != 2 {
			t.Fatalf("expected 2 buttons (Back, Close) for %s, got %d", cat, len(row))
		}
		catCloseBtn, okC := row[1].(*telegram.KeyboardButtonCallback)
		if !okC || catCloseBtn.Style == nil || !catCloseBtn.Style.BgDanger {
			t.Errorf("expected Close button in category %s to have BgDanger=true", cat)
		}
	}
}
