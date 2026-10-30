package index

import (
	"testing"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/config"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
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
		text         string
		expectedChat string
		expectedID   int
	}{
		{
			text:         "Check out https://t.me/c/1234567890/100 for files",
			expectedChat: "-1001234567890",
			expectedID:   100,
		},
		{
			text:         "https://t.me/my_channel_username/250",
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

func TestFormatCompactDuration(t *testing.T) {
	d := 3*time.Hour + 22*time.Minute + 7*time.Second
	expected := "3h22m7s"
	if got := formatCompactDuration(d); got != expected {
		t.Errorf("formatCompactDuration() = %q; expected %q", got, expected)
	}
}

func TestFormatETA(t *testing.T) {
	d1 := 44*time.Hour + 33*time.Minute
	if got := formatETA(d1); got != "44ʜ 33ᴍ" {
		t.Errorf("formatETA(44h33m) = %q; expected '44ʜ 33ᴍ'", got)
	}

	d2 := 2*time.Hour + 15*time.Minute
	if got := formatETA(d2); got != "2ʜ 15ᴍ" {
		t.Errorf("formatETA(2h15m) = %q; expected '2ʜ 15ᴍ'", got)
	}

	d3 := 45*time.Minute + 12*time.Second
	if got := formatETA(d3); got != "45ᴍ 12ꜱ" {
		t.Errorf("formatETA(45m12s) = %q; expected '45ᴍ 12ꜱ'", got)
	}

	d4 := 25 * time.Second
	if got := formatETA(d4); got != "25ꜱ" {
		t.Errorf("formatETA(25s) = %q; expected '25ꜱ'", got)
	}
}

func TestFormatChannelDisplay(t *testing.T) {
	if got := formatChannelDisplay("-1003591797069"); got != "ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀɴɴᴇʟ 3591797069" {
		t.Errorf("formatChannelDisplay(-1003591797069) = %q; expected 'ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀɴɴᴇʟ 3591797069'", got)
	}
	if got := formatChannelDisplay("3591797069"); got != "ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀɴɴᴇʟ 3591797069" {
		t.Errorf("formatChannelDisplay(3591797069) = %q; expected 'ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀɴɴᴇʟ 3591797069'", got)
	}
	if got := formatChannelDisplay("mychannel"); got != "@mychannel" {
		t.Errorf("formatChannelDisplay(mychannel) = %q; expected '@mychannel'", got)
	}
	if got := formatChannelDisplay("@mychannel"); got != "@mychannel" {
		t.Errorf("formatChannelDisplay(@mychannel) = %q; expected '@mychannel'", got)
	}
}

func TestFormatTargetDB(t *testing.T) {
	if got := formatTargetDB("accept5"); got != "ᴅɪꜱᴛʀɪʙᴜᴛᴇ (ᴀʟᴛᴇʀɴᴀᴛᴇ →DB2↔DB3)" {
		t.Errorf("formatTargetDB(accept5) = %q; expected 'ᴅɪꜱᴛʀɪʙᴜᴛᴇ (ᴀʟᴛᴇʀɴᴀᴛᴇ →DB2↔DB3)'", got)
	}
	if got := formatTargetDB("link5"); got != "ᴅɪꜱᴛʀɪʙᴜᴛᴇ (ᴀʟᴛᴇʀɴᴀᴛᴇ →DB2↔DB3)" {
		t.Errorf("formatTargetDB(link5) = %q; expected 'ᴅɪꜱᴛʀɪʙᴜᴛᴇ (ᴀʟᴛᴇʀɴᴀᴛᴇ →DB2↔DB3)'", got)
	}
	if got := formatTargetDB("accept1"); got != "DB2 (Media Shard 1)" {
		t.Errorf("formatTargetDB(accept1) = %q; expected 'DB2 (Media Shard 1)'", got)
	}
	if got := formatTargetDB("accept2"); got != "DB3 (Media Shard 2)" {
		t.Errorf("formatTargetDB(accept2) = %q; expected 'DB3 (Media Shard 2)'", got)
	}
}

func TestFloodWaitErrorHandling(t *testing.T) {
	cases := []struct {
		errStr      string
		isFlood     bool
		expectedSec int
	}{
		{"rpc error: 420 FLOOD_WAIT_35", true, 35},
		{"FLOOD_PREMIUM_WAIT_120", true, 120},
		{"[420 FLOOD_WAIT_15: A wait of 15 seconds is required]", true, 15},
		{"A wait of 45 seconds is required (caused by channels.GetMessages)", true, 45},
		{"CHANNEL_PRIVATE", false, 0},
	}

	for _, tc := range cases {
		err := &telegram.RpcError{Message: tc.errStr, Description: tc.errStr}
		if got := isFloodWaitError(err); got != tc.isFlood {
			t.Errorf("isFloodWaitError(%q) = %v; expected %v", tc.errStr, got, tc.isFlood)
		}
		if tc.isFlood {
			if gotSec := extractFloodWaitSeconds(err); gotSec != tc.expectedSec {
				t.Errorf("extractFloodWaitSeconds(%q) = %d; expected %d", tc.errStr, gotSec, tc.expectedSec)
			}
		}
	}
}

func TestRenderProgressUpdate(t *testing.T) {
	text := renderProgressUpdate(
		"ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀɴɴᴇʟ 3591797069",
		"ᴅɪꜱᴛʀɪʙᴜᴛᴇ (ᴀʟᴛᴇʀɴᴀᴛᴇ →DB2↔DB3)",
		1, 978360, 978360,
		652239, 0, 0,
		295745, 0, 0, 4,
		2.0, 8.3, 326121,
		"3h22m7s", "44ʜ 33ᴍ", false, 0,
	)

	// Verify key styling strings match the user's specification
	expectedSubstrings := []string{
		"📊 <b>ɪɴᴅᴇxɪɴɢ ᴘʀᴏɢʀᴇꜱꜱ ᴜᴘᴅᴀᴛᴇ</b> 📊",
		"★ <b>ᴄʜᴀɴɴᴇʟ:</b> <code>ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀɴɴᴇʟ 3591797069</code>",
		"★ <b>ᴛᴀʀɢᴇᴛ ᴅʙ:</b> <code>ᴅɪꜱᴛʀɪʙᴜᴛᴇ (ᴀʟᴛᴇʀɴᴀᴛᴇ →DB2↔DB3)</code>",
		"★ <b>ᴛᴀʀɢᴇᴛ ʀᴀɴɢᴇ:</b> <code>1</code> ᴛᴏ <code>978360</code>",
		"★ <b>ᴍᴇꜱꜱᴀɢᴇꜱ ꜰᴇᴛᴄʜᴇᴅ:</b> <code>652239</code> / <code>978360</code>",
		"★ <b>ᴍᴇꜱꜱᴀɢᴇꜱ ᴘʀᴏᴄᴇꜱꜱᴇᴅ:</b> <code>0</code>",
		"★ <b>ᴍᴀx ꜱᴀᴠᴇᴅ ᴍꜱɢ ɪᴅ (ꜰɪʟᴇ ᴍᴇᴛᴀᴅᴀᴛᴀ, ɴᴏᴛ ᴄʜᴇᴄᴋᴘᴏɪɴᴛ):</b> <code>0</code>",
		"<b>ᴄᴜʀʀᴇɴᴛ ꜱᴛᴀᴛꜱ</b>",
		"✅ <b>ꜱᴀᴠᴇᴅ:</b> <code>295745</code> ꜰɪʟᴇꜱ",
		"⚠️ <b>ᴅᴜᴘʟɪᴄᴀᴛᴇꜱ:</b> <code>0</code> ꜱᴋɪᴘᴘᴇᴅ",
		"🚫 <b>ɴᴏ ᴍᴇᴅɪᴀ:</b> <code>0</code> ꜱᴋɪᴘᴘᴇᴅ",
		"❌ <b>ᴇʀʀᴏʀꜱ:</b> <code>4</code> ᴇɴᴄᴏᴜɴᴛᴇʀᴇᴅ",
		"<b>ᴘʀᴏᴄᴇꜱꜱɪɴɢ ꜱᴘᴇᴇᴅ:</b>",
		"├─ <b>ꜰᴇᴛᴄʜ:</b> <code>2.0</code> ᴍꜱɢ/ꜱ",
		"├─ <b>ɪɴᴅᴇxɪɴɢ:</b> <code>8.3</code> ꜰɪʟᴇꜱ/ꜱ",
		"└─ <b>ʀᴇᴍᴀɪɴɪɴɢ:</b> <code>326121</code> ᴍᴇꜱꜱᴀɢᴇꜱ",
		"⏱️ <b>ᴛɪᴍᴇ ᴇʟᴀᴘꜱᴇᴅ:</b> <code>3h22m7s</code>  ⏳ <b>ᴛɪᴍᴇ ʟᴇꜰᴛ:</b> ~<code>44ʜ 33ᴍ</code>",
	}

	for _, sub := range expectedSubstrings {
		if !contains(text, sub) {
			t.Errorf("renderProgressUpdate missing expected substring: %s", sub)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && findSubstr(s, substr)))
}

func findSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
