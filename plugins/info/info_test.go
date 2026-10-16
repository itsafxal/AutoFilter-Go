package info

import (
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestInfoPlugin_Metadata(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.Name() != "info" {
		t.Errorf("expected plugin name 'info', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 1 || cmds[0].Name != "info" {
		t.Fatalf("expected command 'info', got %v", cmds)
	}
	if len(cmds[0].Aliases) == 0 || cmds[0].Aliases[0] != "about" {
		t.Errorf("expected alias 'about', got %v", cmds[0].Aliases)
	}
}

func TestBuildBotInfoReport(t *testing.T) {
	botUser := &gotgbot.User{
		Id:        987654321,
		FirstName: "FilmBot",
		Username:  "FilmBot",
	}
	startTime := time.Now().Add(-1 * time.Hour)

	report := BuildBotInfoReport(botUser, startTime)
	if !strings.Contains(report, "FilmBot") {
		t.Errorf("expected bot name in report, got:\n%s", report)
	}
	if !strings.Contains(report, "@FilmBot") {
		t.Errorf("expected bot username in report, got:\n%s", report)
	}
	if !strings.Contains(report, "987654321") {
		t.Errorf("expected bot ID in report, got:\n%s", report)
	}
	if !strings.Contains(report, "gotgbot") {
		t.Errorf("expected library name in report, got:\n%s", report)
	}
	// Verify no secrets or credentials leaked
	if strings.Contains(report, "token") || strings.Contains(report, "secret") {
		t.Errorf("SECURITY: secret leaked in bot info report: %s", report)
	}
}

func TestBuildUserInfoReport(t *testing.T) {
	user := &gotgbot.User{
		Id:        12345,
		FirstName: "Alice",
		LastName:  "Smith",
		Username:  "alicesmith",
	}

	report := BuildUserInfoReport(user)
	if !strings.Contains(report, "Alice") {
		t.Errorf("expected First Name Alice, got:\n%s", report)
	}
	if !strings.Contains(report, "Smith") {
		t.Errorf("expected Last Name Smith, got:\n%s", report)
	}
	if !strings.Contains(report, "@alicesmith") {
		t.Errorf("expected Username @alicesmith, got:\n%s", report)
	}
	if !strings.Contains(report, "tg://user?id=12345") {
		t.Errorf("expected user link, got:\n%s", report)
	}
}

func TestBuildUserInfoReport_NilUser(t *testing.T) {
	report := BuildUserInfoReport(nil)
	if !strings.Contains(report, "not available") {
		t.Errorf("expected safe fallback for nil user, got: %s", report)
	}
}
