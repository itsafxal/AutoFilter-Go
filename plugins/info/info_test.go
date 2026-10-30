package info

import (
	"strings"
	"testing"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/config"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

func TestFormatUserInfoCard_StandardUser(t *testing.T) {
	user := &telegram.UserObj{
		ID:        123456789,
		FirstName: "John",
		LastName:  "Doe",
		Username:  "johndoe",
		Bot:       false,
		Premium:   true,
		Verified:  true,
		Scam:      false,
		Fake:      false,
		LangCode:  "en",
		Status:    &telegram.UserStatusRecently{},
	}

	cardData := &UserCardData{
		User:           user,
		ProfilePhotoDC: 5,
		BotSessionDC:   4,
		IsGroup:        false,
	}

	card := FormatUserInfoCard(cardData)

	expectedStrings := []string{
		"👤 <b>USER INFORMATION</b>",
		"📝 <b>Name:</b> John Doe",
		"✨ <b>Username:</b> @johndoe",
		"🆔 <b>User ID:</b> <code>123456789</code>",
		"<a href=\"https://t.me/johndoe\">Open Profile</a>",
		"🤖 <b>Bot:</b> No",
		"⭐ <b>Premium:</b> Yes",
		"☑️ <b>Verified:</b> Yes",
		"🚫 <b>Scam:</b> No",
		"👁 <b>Fake:</b> No",
		"🌐 <b>Language:</b> <code>en</code>",
		"📡 <b>Last Seen:</b> Recently",
		"📍 <b>Profile Photo DC:</b> DC 5",
		"📍 <b>Bot Session DC:</b> DC 4",
	}

	for _, exp := range expectedStrings {
		if !strings.Contains(card, exp) {
			t.Errorf("expected card to contain %q, but got:\n%s", exp, card)
		}
	}
}

func TestFormatUserInfoCard_DeletedAccount(t *testing.T) {
	user := &telegram.UserObj{
		ID:      987654321,
		Deleted: true,
	}

	cardData := &UserCardData{
		User:    user,
		IsGroup: false,
	}

	card := FormatUserInfoCard(cardData)

	if !strings.Contains(card, "Deleted Account") {
		t.Errorf("expected card to report Deleted Account, got:\n%s", card)
	}
	if !strings.Contains(card, "987654321") {
		t.Errorf("expected card to contain ID, got:\n%s", card)
	}
}

func TestFormatUserInfoCard_GroupContext(t *testing.T) {
	user := &telegram.UserObj{
		ID:        555666777,
		FirstName: "Alice",
		LastName:  "Smith",
		Username:  "alicesmith",
		Status:    &telegram.UserStatusOnline{Expires: int32(time.Now().Unix() + 300)},
	}

	cardData := &UserCardData{
		User:          user,
		IsGroup:       true,
		ChatID:        -1001234567890,
		ChatTitle:     "Movie Discussions & Updates",
		GroupStatus:   "Administrator",
		GroupJoinDate: int32(time.Date(2025, 5, 12, 10, 0, 0, 0, time.UTC).Unix()),
		RepliedMsgID:  4321,
	}

	card := FormatUserInfoCard(cardData)

	expectedGroupDetails := []string{
		"👥 <b>GROUP CONTEXT</b>",
		"💬 <b>Group ID:</b> <code>-1001234567890</code>",
		"🏷️ <b>Group Title:</b> Movie Discussions &amp; Updates",
		"🎖️ <b>Membership Status:</b> Administrator",
		"📅 <b>Group Joined:</b> 12 May 2025, 10:00 UTC",
		"📨 <b>Replied Message ID:</b> <code>4321</code>",
		"📡 <b>Last Seen:</b> Online",
	}

	for _, exp := range expectedGroupDetails {
		if !strings.Contains(card, exp) {
			t.Errorf("expected card to contain %q, got:\n%s", exp, card)
		}
	}
}

func TestFormatChannelCard(t *testing.T) {
	channel := &telegram.Channel{
		ID:       1999888777,
		Title:    "Official Cinema Channel",
		Username: "cinemachannel",
	}

	data := &ChannelCardData{
		Channel:      channel,
		CurrentChat:  -100111222333,
		CurrentTitle: "Community Chat",
		RepliedMsgID: 88,
	}

	card := FormatChannelCard(data)

	expected := []string{
		"📢 <b>CHANNEL / SENDER CHAT</b>",
		"🏷️ <b>Title:</b> Official Cinema Channel",
		"✨ <b>Username:</b> @cinemachannel",
		"🆔 <b>Chat ID:</b> <code>-1001999888777</code>",
		"<a href=\"https://t.me/cinemachannel\">Open Channel</a>",
		"💬 <b>Group ID:</b> <code>-100111222333</code>",
		"🏷️ <b>Group Title:</b> Community Chat",
		"📨 <b>Replied Message ID:</b> <code>88</code>",
	}

	for _, exp := range expected {
		if !strings.Contains(card, exp) {
			t.Errorf("expected channel card to contain %q, got:\n%s", exp, card)
		}
	}
}

func TestHTMLEscaping_DangerousInput(t *testing.T) {
	user := &telegram.UserObj{
		ID:        1001,
		FirstName: "<script>alert('xss')</script>",
		LastName:  "<b>Bold</b> & <i>Italic</i>",
		Username:  "evil<tag>",
	}

	cardData := &UserCardData{
		User:    user,
		IsGroup: false,
	}

	card := FormatUserInfoCard(cardData)

	if strings.Contains(card, "<script>") {
		t.Errorf("card contains unescaped script tag: %s", card)
	}
	if !strings.Contains(card, "&lt;script&gt;") {
		t.Errorf("card does not properly escape script tag: %s", card)
	}
	if !strings.Contains(card, "&amp;") {
		t.Errorf("card does not properly escape ampersand: %s", card)
	}
}

func TestFormatLastSeen(t *testing.T) {
	tests := []struct {
		name     string
		status   telegram.UserStatus
		expected string
	}{
		{"nil", nil, "Not Available"},
		{"online", &telegram.UserStatusOnline{Expires: 100}, "Online"},
		{"recently", &telegram.UserStatusRecently{}, "Recently"},
		{"week", &telegram.UserStatusLastWeek{}, "Within a week"},
		{"month", &telegram.UserStatusLastMonth{}, "Within a month"},
		{"empty", &telegram.UserStatusEmpty{}, "Hidden / Unknown"},
		{"offline", &telegram.UserStatusOffline{WasOnline: int32(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).Unix())}, "01 Jan 2026, 12:00 UTC"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatLastSeen(tc.status)
			if got != tc.expected {
				t.Errorf("formatLastSeen() = %q, expected %q", got, tc.expected)
			}
		})
	}
}

func TestBuildUserKeyboard(t *testing.T) {
	markup := BuildUserKeyboard(123456789, "testuser", 987654321)
	if markup == nil || len(markup.Rows) != 2 {
		t.Fatalf("expected 2 keyboard rows, got %v", markup)
	}

	row1 := markup.Rows[0].Buttons
	if len(row1) != 2 {
		t.Errorf("expected 2 buttons in row 1, got %d", len(row1))
	}

	row2 := markup.Rows[1].Buttons
	if len(row2) != 1 {
		t.Errorf("expected 1 button in row 2, got %d", len(row2))
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123456", true},
		{"-100123456789", true},
		{"@username", false},
		{"user123", false},
		{"", false},
		{"-", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := isNumeric(tc.input)
			if got != tc.expected {
				t.Errorf("isNumeric(%q) = %v, expected %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestPlugin_Registration(t *testing.T) {
	appCtx := &plugins.AppContext{
		Config: &config.Config{
			OwnerID: 12345,
			Admins:  []int64{12345},
		},
	}
	p := New(appCtx)

	if p.Name() != "info" {
		t.Errorf("expected name 'info', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}

	cmd := cmds[0]
	if cmd.Name != "info" {
		t.Errorf("expected command name 'info', got %q", cmd.Name)
	}
	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "about" {
		t.Errorf("expected alias 'about', got %v", cmd.Aliases)
	}

	cbs := p.CallbackHandlers()
	if len(cbs) != 1 {
		t.Fatalf("expected 1 callback handler, got %d", len(cbs))
	}
	if cbs[0].Prefix != "info_close:" {
		t.Errorf("expected prefix 'info_close:', got %q", cbs[0].Prefix)
	}
}
