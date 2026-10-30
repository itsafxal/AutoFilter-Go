package lifecycle

import (
	"strings"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/config"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

func TestFormatGroupWelcomeText(t *testing.T) {
	text := FormatGroupWelcomeText(
		"AutoFilterBot",
		"Cinema Lovers & Discussion",
		-1001234567890,
		"Alice Smith",
		987654321,
	)

	expectedStrings := []string{
		"🎉 <b>BOT SUCCESSFULLY ADDED!</b>",
		"Thanks for adding <b>@AutoFilterBot</b> to your group.",
		"📌 <b>GROUP INFORMATION</b>",
		"🏷 <b>Group:</b> Cinema Lovers &amp; Discussion",
		"🆔 <b>Group ID:</b> <code>-1001234567890</code>",
		"👤 <b>ADDED BY</b>",
		"📝 <b>Name:</b> Alice Smith",
		"🆔 <b>ID:</b> <code>987654321</code>",
		"✨ Your bot is now ready to use!",
	}

	for _, exp := range expectedStrings {
		if !strings.Contains(text, exp) {
			t.Errorf("expected welcome text to contain %q, but got:\n%s", exp, text)
		}
	}
}

func TestBuildGroupWelcomeKeyboard(t *testing.T) {
	kb := BuildGroupWelcomeKeyboard("AutoFilterBot", "https://github.com/itsafxal/AutoFilter-Go")
	if kb == nil || len(kb.Rows) != 2 {
		t.Fatalf("expected 2 keyboard rows, got %v", kb)
	}

	row1 := kb.Rows[0].Buttons
	btn1, ok := row1[0].(*telegram.KeyboardButtonURL)
	if !ok || btn1.Text != "🤖 @AutoFilterBot" {
		t.Errorf("unexpected row 1 buttons: %v", row1)
	}

	row2 := kb.Rows[1].Buttons
	btn2, ok := row2[0].(*telegram.KeyboardButtonURL)
	if !ok || btn2.Text != "💻 Source Code" {
		t.Errorf("unexpected row 2 buttons: %v", row2)
	}
}

func TestFormatAdminNewGroupNotification(t *testing.T) {
	notif := FormatAdminNewGroupNotification(
		"Movie Buffs Hub",
		-1009988776655,
		"@moviebuffs",
		"Bob Admin",
		1122334455,
		"@bobadmin",
		"30 Oct 2026, 15:30 UTC",
		"AutoFilterBot",
	)

	expected := []string{
		"📢 <b>#NEW_GROUP_ADDED</b>",
		"🏷️ <b>Group:</b> Movie Buffs Hub",
		"🆔 <b>Group ID:</b> <code>-1009988776655</code>",
		"🔗 <b>Group Link:</b> @moviebuffs",
		"👤 <b>ADDED BY:</b>",
		"📝 <b>Name:</b> Bob Admin",
		"🆔 <b>User ID:</b> <code>1122334455</code>",
		"✨ <b>Username:</b> @bobadmin",
		"📅 <b>Added On:</b> 30 Oct 2026, 15:30 UTC",
		"🤖 <b>Bot:</b> @AutoFilterBot",
	}

	for _, exp := range expected {
		if !strings.Contains(notif, exp) {
			t.Errorf("expected notification to contain %q, got:\n%s", exp, notif)
		}
	}
}

func TestFormatAdminRemovedNotification(t *testing.T) {
	notif := FormatAdminRemovedNotification(
		"Discontinued Group",
		-1005544332211,
		"Group Owner",
		998877,
		"30 Oct 2026, 15:35 UTC",
		"AutoFilterBot",
	)

	expected := []string{
		"🚪 <b>#BOT_REMOVED_FROM_GROUP</b>",
		"🏷️ <b>Group:</b> Discontinued Group",
		"🆔 <b>Group ID:</b> <code>-1005544332211</code>",
		"👤 <b>Action By:</b> Group Owner (<code>998877</code>)",
		"📅 <b>Date:</b> 30 Oct 2026, 15:35 UTC",
		"🤖 <b>Bot:</b> @AutoFilterBot",
	}

	for _, exp := range expected {
		if !strings.Contains(notif, exp) {
			t.Errorf("expected removed notification to contain %q, got:\n%s", exp, notif)
		}
	}
}

func TestHTMLEscaping_DangerousGroupInputs(t *testing.T) {
	text := FormatGroupWelcomeText(
		"AutoFilterBot",
		"<script>bad()</script>",
		-1001,
		"Hacker & Co <b>bold</b>",
		123,
	)

	if strings.Contains(text, "<script>") {
		t.Errorf("text contains unescaped script tag: %s", text)
	}
	if !strings.Contains(text, "&lt;script&gt;") {
		t.Errorf("text did not properly escape script tag: %s", text)
	}
	if !strings.Contains(text, "&amp; Co") {
		t.Errorf("text did not properly escape ampersand: %s", text)
	}
}

func TestPlugin_CommandsAndAdminOnly(t *testing.T) {
	p := New(&plugins.AppContext{})

	if p.Name() != "lifecycle" {
		t.Errorf("expected plugin name 'lifecycle', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(cmds))
	}

	cmdMap := make(map[string]plugins.CommandDefinition)
	for _, c := range cmds {
		cmdMap[c.Name] = c
		if !c.AdminOnly {
			t.Errorf("expected command %s to be AdminOnly: true", c.Name)
		}
	}

	for _, required := range []string{"leave", "disable", "enable", "groups"} {
		if _, ok := cmdMap[required]; !ok {
			t.Errorf("missing expected command: %s", required)
		}
	}
}

func TestGetAdminRecipients_Deduplication(t *testing.T) {
	appCtx := &plugins.AppContext{
		Config: &config.Config{
			OwnerID: 100,
			Admins:  []int64{100, 200, 300, 200}, // 100 and 200 repeated
		},
	}
	p := New(appCtx)

	admins := p.getAdminRecipients()
	if len(admins) != 3 {
		t.Fatalf("expected 3 deduplicated admins, got %d: %v", len(admins), admins)
	}

	expectedMap := map[int64]bool{100: true, 200: true, 300: true}
	for _, id := range admins {
		if !expectedMap[id] {
			t.Errorf("unexpected admin ID: %d", id)
		}
	}
}

func TestDeduplicationEventBarrier(t *testing.T) {
	p := New(&plugins.AppContext{})

	// First event for chat 12345 should not be duplicate
	if p.isDuplicateEvent("add", 12345) {
		t.Errorf("first add event should not be marked as duplicate")
	}

	// Immediate second event for chat 12345 should be identified as duplicate
	if !p.isDuplicateEvent("add", 12345) {
		t.Errorf("second add event within 30s should be marked as duplicate")
	}

	// Different chat ID should not be duplicate
	if p.isDuplicateEvent("add", 67890) {
		t.Errorf("event for different chat ID should not be duplicate")
	}

	// Different event type for same chat ID should not be duplicate
	if p.isDuplicateEvent("remove", 12345) {
		t.Errorf("different event type should not be duplicate")
	}
}
