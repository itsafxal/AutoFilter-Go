package botstats

import (
	"context"
	"strings"
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestStatsPlugin_Metadata(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.Name() != "stats" {
		t.Errorf("expected plugin name 'stats', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Name != "stats" {
		t.Errorf("expected command name 'stats', got %q", cmds[0].Name)
	}
	if len(cmds[0].Aliases) == 0 || cmds[0].Aliases[0] != "status" {
		t.Errorf("expected alias 'status', got %+v", cmds[0].Aliases)
	}
	if cmds[0].Handler == nil {
		t.Error("expected non-nil handler")
	}

	cbs := p.CallbackHandlers()
	if len(cbs) != 2 {
		t.Errorf("expected 2 callback handlers, got %d", len(cbs))
	}
}

func TestStatsPlugin_FormatMessage(t *testing.T) {
	sampleStats := BotStats{
		TotalFiles: 295745,
		TotalUsers: 1420,
		TotalChats: 85,
		DB1SizeMB:  12.45,
		DB2SizeMB:  158.20,
		DB3SizeMB:  162.80,
		SQLSizeMB:  84.50,
	}

	msg := FormatStatsMessage(sampleStats)

	// Check header
	if !strings.Contains(msg, "📈 𝐁𝐨𝐭 𝐒𝐭𝐚𝐭𝐢𝐬𝐭𝐢𝐜𝐬 𝐎𝐯𝐞𝐫𝐯𝐢𝐞𝐰 📊") {
		t.Errorf("expected header in message, got: %s", msg)
	}

	// Check totals
	if !strings.Contains(msg, "📚 𝐓𝐨𝐭𝐚𝐥 𝐅𝐢𝐥𝐞𝐬: 295745") {
		t.Errorf("expected files count, got: %s", msg)
	}
	if !strings.Contains(msg, "👥 𝐓𝐨𝐭𝐚𝐥 𝐔𝐬𝐞𝐫𝐬: 1420") {
		t.Errorf("expected users count, got: %s", msg)
	}
	if !strings.Contains(msg, "💬 𝐓𝐨𝐭𝐚𝐥 𝐂𝐡𝐚𝐭𝐬: 85") {
		t.Errorf("expected chats count, got: %s", msg)
	}

	// Check database sizes
	if !strings.Contains(msg, "💾 𝐃𝐚𝐭𝐚𝐛𝐚𝐬𝐞 𝐒𝐢𝐳𝐞𝐬:") {
		t.Errorf("expected database sizes section, got: %s", msg)
	}
	if !strings.Contains(msg, "• 𝐃𝐁𝟏: 12.45 𝐌𝐁") {
		t.Errorf("expected DB1 size, got: %s", msg)
	}
	if !strings.Contains(msg, "• 𝐃𝐁𝟐: 158.20 𝐌𝐁") {
		t.Errorf("expected DB2 size, got: %s", msg)
	}
	if !strings.Contains(msg, "• 𝐃𝐁𝟑: 162.80 𝐌𝐁") {
		t.Errorf("expected DB3 size, got: %s", msg)
	}
	if !strings.Contains(msg, "• 𝐒𝐐𝐋: 84.50 𝐌𝐁") {
		t.Errorf("expected SQL size, got: %s", msg)
	}
}

func TestStatsPlugin_BuildStatsContent(t *testing.T) {
	appCtx := &plugins.AppContext{}
	p := New(appCtx)

	text, markup := p.buildStatsContent(context.Background())
	if !strings.Contains(text, "📈 𝐁𝐨𝐭 𝐒𝐭𝐚𝐭𝐢𝐬𝐭𝐢𝐜𝐬 𝐎𝐯𝐞𝐫𝐯𝐢𝐞𝐰 📊") {
		t.Errorf("unexpected stats text: %s", text)
	}
	if markup == nil || len(markup.Rows) != 1 || len(markup.Rows[0].Buttons) != 2 {
		t.Fatalf("expected 1 row with 2 buttons, got %v", markup)
	}
}
