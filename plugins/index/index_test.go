package index

import (
	"testing"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestIndexPlugin_Metadata(t *testing.T) {
	appCtx := &plugins.AppContext{
		Config: &config.Config{
			OwnerID: 12345,
			Admins:  []int64{12345},
		},
	}
	p := New(appCtx)
	if p.Name() != "index" {
		t.Errorf("expected plugin name 'index', got %s", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands (/index, /indexlink, /setskip), got %d", len(cmds))
	}

	cbs := p.CallbackHandlers()
	if len(cbs) != 3 {
		t.Fatalf("expected 3 callback handlers, got %d", len(cbs))
	}

	mhs := p.MessageHandlers()
	if len(mhs) != 1 {
		t.Fatalf("expected 1 message handler for forwarded channel messages, got %d", len(mhs))
	}
}

func TestIndexPlugin_Skip(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.GetSkip() != 0 {
		t.Errorf("expected initial skip 0, got %d", p.GetSkip())
	}

	p.SetSkip(500)
	if p.GetSkip() != 500 {
		t.Errorf("expected skip 500, got %d", p.GetSkip())
	}
}

func TestIndexPlugin_ForwardedMessageFilter(t *testing.T) {
	appCtx := &plugins.AppContext{
		Config: &config.Config{
			OwnerID: 12345,
			Admins:  []int64{12345},
		},
	}
	p := New(appCtx)
	handlers := p.MessageHandlers()
	if len(handlers) == 0 {
		t.Fatal("no message handlers registered")
	}

	// 1. Non-private message should be rejected
	groupMsg := &telegram.NewMessage{
		Chat: &telegram.ChatObj{ID: -1001234567890},
	}
	if handlers[0].Filter(groupMsg) {
		t.Errorf("expected non-private message to be rejected")
	}

	// 2. Non-admin private message should be rejected
	nonAdminMsg := &telegram.NewMessage{
		Sender: &telegram.UserObj{ID: 99999},
	}
	if handlers[0].Filter(nonAdminMsg) {
		t.Errorf("expected non-admin message to be rejected")
	}

	// 3. Admin message that is NOT a forward should be rejected
	adminRegularMsg := &telegram.NewMessage{
		Sender:  &telegram.UserObj{ID: 12345},
		Message: &telegram.MessageObj{Message: "hello"},
	}
	if handlers[0].Filter(adminRegularMsg) {
		t.Errorf("expected non-forwarded admin message to be rejected")
	}

	// 4. Admin message forwarded from a channel should be accepted
	adminForwardedMsg := &telegram.NewMessage{
		Sender: &telegram.UserObj{ID: 12345},
		Message: &telegram.MessageObj{
			FwdFrom: &telegram.MessageFwdHeader{
				FromID:      &telegram.PeerChannel{ChannelID: 1234567890},
				ChannelPost: 42,
			},
		},
	}
	if !handlers[0].Filter(adminForwardedMsg) {
		t.Errorf("expected admin forwarded channel message to be accepted")
	}
}

func TestExtractChannelInfo_Links(t *testing.T) {
	cases := []struct {
		text        string
		expectedChat string
		expectedID   int
	}{
		{
			text:        "Check out https://t.me/c/1234567890/100 for files",
			expectedChat: "-1001234567890",
			expectedID:   100,
		},
		{
			text:        "https://t.me/my_channel_username/250",
			expectedChat: "my_channel_username",
			expectedID:   250,
		},
	}

	for _, tc := range cases {
		m := &telegram.NewMessage{
			Message: &telegram.MessageObj{
				Message: tc.text,
			},
		}
		chatID, msgID := extractChannelInfo(m, m)
		if chatID != tc.expectedChat || msgID != tc.expectedID {
			t.Errorf("extractChannelInfo(%q) = (%s, %d); expected (%s, %d)", tc.text, chatID, msgID, tc.expectedChat, tc.expectedID)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	d1 := 5 * time.Second
	if formatDuration(d1) != "5s" {
		t.Errorf("expected '5s', got %s", formatDuration(d1))
	}

	d2 := 2*time.Minute + 15*time.Second
	if formatDuration(d2) != "2m 15s" {
		t.Errorf("expected '2m 15s', got %s", formatDuration(d2))
	}

	d3 := 1*time.Hour + 30*time.Minute + 5*time.Second
	if formatDuration(d3) != "1h 30m 5s" {
		t.Errorf("expected '1h 30m 5s', got %s", formatDuration(d3))
	}
}
