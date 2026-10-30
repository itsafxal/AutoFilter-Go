package cmds

import (
	"strings"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
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
	hasHelp := false
	hasCmd := false
	for _, a := range cmds[0].Aliases {
		if a == "help" {
			hasHelp = true
		}
		if a == "cmd" {
			hasCmd = true
		}
	}
	if !hasHelp || !hasCmd {
		t.Errorf("expected aliases ['help', 'cmd'], got %v", cmds[0].Aliases)
	}

	if cmds[1].Name != "cmdc" {
		t.Errorf("expected command name 'cmdc', got %q", cmds[1].Name)
	}
	if !cmds[1].AdminOnly {
		t.Errorf("expected cmdc to be AdminOnly")
	}

	// Verify callback handlers
	cbs := p.CallbackHandlers()
	cbMap := make(map[string]bool)
	for _, cb := range cbs {
		cbMap[cb.Prefix] = true
	}
	expectedPrefixes := []string{"cmd_cat#", "cmdc_cat#", "cmd_home", "cmdc_home", "close_data", "noop"}
	for _, prefix := range expectedPrefixes {
		if !cbMap[prefix] {
			t.Errorf("missing expected callback prefix %q in %v", prefix, cbMap)
		}
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

type dummyPlugin struct {
	name string
	cmds []plugins.CommandDefinition
}

func (d *dummyPlugin) Name() string                          { return d.name }
func (d *dummyPlugin) Commands() []plugins.CommandDefinition { return d.cmds }

func TestCmdsPlugin_InteractiveMenus(t *testing.T) {
	appCtx := &plugins.AppContext{}
	reg := plugins.NewRegistry(appCtx)

	// Register dummy commands across different categories
	dummy := &dummyPlugin{
		name: "test_cmds",
		cmds: []plugins.CommandDefinition{
			{Name: "start", Description: "Start bot", Category: "General", Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "info", Description: "User info", Category: "User Commands", Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "getfile", Description: "Get file", Category: "File & Media", Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "batch", Description: "Batch tool", Category: "Batch Management", Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "sub1", Description: "Set sub 1", Category: "FCB Commands", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "sub2", Description: "Set sub 2", Category: "FCB Commands", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "sub3", Description: "Set sub 3", Category: "FCB Commands", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "delsub1", Description: "Del sub 1", Category: "FCB Commands", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "delsub2", Description: "Del sub 2", Category: "FCB Commands", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "delsub3", Description: "Del sub 3", Category: "FCB Commands", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "2sub1", Description: "Set 2sub 1", Category: "FCB Commands", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "gfilter", Description: "Global filter", Category: "Global Filters", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "index", Description: "Index channel", Category: "Indexing", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "leave", Description: "Leave group", Category: "Group Management", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "broadcast", Description: "Broadcast msg", Category: "Admin Commands", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
			{Name: "vacuum", Description: "Vacuum DB", Category: "Developer Tools", AdminOnly: true, Handler: func(*telegram.NewMessage) error { return nil }},
		},
	}
	if err := reg.Register(dummy); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := reg.Validate(); err != nil {
		t.Fatalf("validate failed: %v", err)
	}

	p := New(appCtx)

	// 1. Admin Main Menu
	adminText, adminMarkup := p.buildMainMenu(true)
	if !strings.Contains(adminText, "AutoFilter-Go COMMAND CENTER") {
		t.Errorf("unexpected menu text: %s", adminText)
	}
	if !strings.Contains(adminText, "Total Commands:") {
		t.Errorf("expected 'Total Commands:' in admin menu")
	}
	// Admin has 10 categories -> 5 rows of 2 buttons + 1 row for Close = 6 rows
	if adminMarkup == nil || len(adminMarkup.Rows) != 6 {
		t.Fatalf("expected 6 rows in admin markup, got %d", len(adminMarkup.Rows))
	}

	// Verify Close button is Danger (Red)
	closeRow := adminMarkup.Rows[len(adminMarkup.Rows)-1].Buttons
	if len(closeRow) != 1 {
		t.Fatalf("expected 1 close button, got %d", len(closeRow))
	}
	closeBtn, ok := closeRow[0].(*telegram.KeyboardButtonCallback)
	if !ok || closeBtn.Style == nil || !closeBtn.Style.BgDanger {
		t.Errorf("expected Close button to have BgDanger=true (Red)")
	}

	// 2. Non-Admin Main Menu
	userText, userMarkup := p.buildMainMenu(false)
	if !strings.Contains(userText, "Available Commands:") {
		t.Errorf("expected 'Available Commands:' in user menu")
	}
	// Non-admin has 4 public categories -> 2 rows of 2 buttons + 1 row for Close = 3 rows
	if userMarkup == nil || len(userMarkup.Rows) != 3 {
		t.Fatalf("expected 3 rows in user markup, got %d", len(userMarkup.Rows))
	}

	// 3. Category Detail View with Pagination (> 6 items)
	fcbMeta := findCategoryMeta("fcb")
	if fcbMeta == nil {
		t.Fatalf("fcb category metadata not found")
	}

	// Page 1 of FCB (7 items total -> 2 pages)
	// Alphabetical order: 2sub1, delsub1, delsub2, delsub3, sub1, sub2 on page 1; sub3 on page 2
	p1Text, p1Markup := p.buildCategoryView(fcbMeta, 1, true)
	if !strings.Contains(p1Text, "Page 1/2") {
		t.Errorf("expected 'Page 1/2' in FCB page 1 header, got:\n%s", p1Text)
	}
	if !strings.Contains(p1Text, "/2sub1") {
		t.Errorf("expected /2sub1 on page 1")
	}
	// FCB markup should have 2 rows: 1 pagination row + 1 back/close row
	if len(p1Markup.Rows) != 2 {
		t.Fatalf("expected 2 rows in paginated category markup, got %d", len(p1Markup.Rows))
	}

	// Page 2 of FCB
	p2Text, p2Markup := p.buildCategoryView(fcbMeta, 2, true)
	if !strings.Contains(p2Text, "Page 2/2") {
		t.Errorf("expected 'Page 2/2' in FCB page 2 header, got:\n%s", p2Text)
	}
	if !strings.Contains(p2Text, "/sub3") {
		t.Errorf("expected /sub3 on page 2")
	}
	if p2Markup == nil || len(p2Markup.Rows) != 2 {
		t.Fatalf("expected 2 rows in page 2 markup, got %d", len(p2Markup.Rows))
	}

	// 4. Single-Page Category View
	genMeta := findCategoryMeta("gen")
	genText, genMarkup := p.buildCategoryView(genMeta, 1, true)
	if strings.Contains(genText, "Page 1/") {
		t.Errorf("single page category should not display pagination header")
	}
	// Single page category has only 1 row (Back to Menu & Close)
	if genMarkup == nil || len(genMarkup.Rows) != 1 {
		t.Fatalf("expected 1 row in single-page category markup, got %d", len(genMarkup.Rows))
	}
}

func TestCmdsPlugin_FindCategoryMeta(t *testing.T) {
	for _, cat := range CanonicalCategories {
		byID := findCategoryMeta(cat.ID)
		if byID == nil || byID.ID != cat.ID {
			t.Errorf("failed to find category by ID %s", cat.ID)
		}
		byName := findCategoryMeta(cat.Name)
		if byName == nil || byName.Name != cat.Name {
			t.Errorf("failed to find category by Name %s", cat.Name)
		}
	}

	if findCategoryMeta("unknown_category") != nil {
		t.Errorf("expected nil for nonexistent category")
	}
}
