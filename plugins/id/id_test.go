package id

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestIDPlugin_Metadata(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.Name() != "id" {
		t.Errorf("expected plugin name 'id', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 1 || cmds[0].Name != "id" {
		t.Fatalf("expected command 'id', got %v", cmds)
	}
}

func TestFormatPrivateID_NormalUser(t *testing.T) {
	user := &gotgbot.User{
		Id:        12345678,
		FirstName: "John",
		LastName:  "Doe",
		Username:  "johndoe",
	}

	report := FormatPrivateID(user, 12345678)
	if !strings.Contains(report, "John") {
		t.Errorf("expected First Name John, got:\n%s", report)
	}
	if !strings.Contains(report, "Doe") {
		t.Errorf("expected Last Name Doe, got:\n%s", report)
	}
	if !strings.Contains(report, "@johndoe") {
		t.Errorf("expected Username @johndoe, got:\n%s", report)
	}
	if !strings.Contains(report, "12345678") {
		t.Errorf("expected Telegram ID 12345678, got:\n%s", report)
	}
}

func TestFormatPrivateID_MissingUser(t *testing.T) {
	// Should not panic when user is nil
	report := FormatPrivateID(nil, 9999)
	if !strings.Contains(report, "N/A") {
		t.Errorf("expected N/A for missing user info, got:\n%s", report)
	}
	if !strings.Contains(report, "9999") {
		t.Errorf("expected Chat ID 9999, got:\n%s", report)
	}
}

func TestFormatGroupID_NormalAndReplied(t *testing.T) {
	chat := &gotgbot.Chat{
		Id:    -1001234567890,
		Title: "Test Movie Group",
	}
	user := &gotgbot.User{
		Id: 111222333,
	}
	replyMsg := &gotgbot.Message{
		MessageId: 42,
		From: &gotgbot.User{
			Id: 444555666,
		},
	}

	report := FormatGroupID(chat, user, replyMsg)
	if !strings.Contains(report, "-1001234567890") {
		t.Errorf("expected Chat ID in report, got:\n%s", report)
	}
	if !strings.Contains(report, "Test Movie Group") {
		t.Errorf("expected Chat Title in report, got:\n%s", report)
	}
	if !strings.Contains(report, "111222333") {
		t.Errorf("expected Your User ID in report, got:\n%s", report)
	}
	if !strings.Contains(report, "444555666") {
		t.Errorf("expected Replied User ID in report, got:\n%s", report)
	}
	if !strings.Contains(report, "42") {
		t.Errorf("expected Replied Message ID in report, got:\n%s", report)
	}
}

func TestFormatGroupID_MissingUserAndAnonymousReply(t *testing.T) {
	// Anonymous sender (user is nil) in group, replying to a channel post
	chat := &gotgbot.Chat{
		Id:    -1009876543210,
		Title: "Anonymous Group",
	}
	replyMsg := &gotgbot.Message{
		MessageId: 101,
		SenderChat: &gotgbot.Chat{
			Id: -1005555555555,
		},
	}

	report := FormatGroupID(chat, nil, replyMsg)
	if !strings.Contains(report, "Anonymous") {
		t.Errorf("expected 'Anonymous' for missing user, got:\n%s", report)
	}
	if !strings.Contains(report, "<code>-1005555555555</code> (Channel)") {
		t.Errorf("expected channel sender id for replied message, got:\n%s", report)
	}
}
