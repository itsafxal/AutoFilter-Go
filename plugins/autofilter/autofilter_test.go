package autofilter

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
	"github.com/itsafxal/MYTGBOTZ/internal/search"
)

func TestAutofilterPlugin_Structure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_autofilter.sqlite3")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		SQLiteDBPath:  dbPath,
		SyncBatchSize: 100,
	}
	dbManager := database.NewDatabaseManager(cfg, logger)

	if err := dbManager.Init(context.Background()); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer dbManager.Close()

	searchService := search.NewService(dbManager, logger)
	appCtx := &plugins.AppContext{
		Config: cfg,
		Logger: logger,
		DB:     dbManager,
		Search: searchService,
	}

	plugin := New(appCtx)

	if plugin.Name() != "autofilter" {
		t.Errorf("expected plugin name 'autofilter', got %q", plugin.Name())
	}

	msgHandlers := plugin.MessageHandlers()
	if len(msgHandlers) != 1 {
		t.Fatalf("expected 1 message handler, got %d", len(msgHandlers))
	}

	cbHandlers := plugin.CallbackHandlers()
	if len(cbHandlers) == 0 {
		t.Errorf("expected callback handlers, got 0")
	}

	// Test message filter with command vs plain text
	filter := msgHandlers[0].Filter

	// Non-command text message
	plainMsg := &telegram.NewMessage{
		Message: &telegram.MessageObj{
			Message: "kgf",
		},
	}
	if !filter(plainMsg) {
		t.Errorf("expected filter to accept plain text 'kgf'")
	}

	// Command message
	cmdMsg := &telegram.NewMessage{
		Message: &telegram.MessageObj{
			Message: "/start",
			Entities: []telegram.MessageEntity{
				&telegram.MessageEntityBotCommand{Offset: 0, Length: 6},
			},
		},
	}
	if filter(cmdMsg) {
		t.Errorf("expected filter to reject bot command '/start'")
	}
}

func TestAutofilter_RecoverSession(t *testing.T) {
	plugin := &Plugin{}
	msg := &telegram.NewMessage{
		Chat: &telegram.ChatObj{ID: -10012345},
		Message: &telegram.MessageObj{
			Message: "𝗛𝗘𝗬 👋 User 🫶\n    \n📁 Query : kgf chapter 2\n\n🗓️ Page No › 1",
		},
	}

	sess := plugin.recoverSessionFromMessage(msg, 998877)
	if sess == nil {
		t.Fatalf("expected recovered session, got nil")
	}
	if sess.Query != "kgf chapter 2" {
		t.Errorf("expected query 'kgf chapter 2', got %q", sess.Query)
	}
	if sess.UserID != 998877 {
		t.Errorf("expected user id 998877, got %d", sess.UserID)
	}
	if sess.ChatID != -10012345 {
		t.Errorf("expected chat id -10012345, got %d", sess.ChatID)
	}
}

func TestAutofilter_IsMessageNotModified(t *testing.T) {
	err := errors.New("RPCError: 400 MESSAGE_NOT_MODIFIED: message is not modified")
	if !isMessageNotModified(err) {
		t.Errorf("expected isMessageNotModified to be true")
	}
	if isMessageNotModified(nil) {
		t.Errorf("expected nil error to be false")
	}
}

func TestAutofilter_QualitiesList(t *testing.T) {
	expected := []string{"480P", "720P", "1080P", "2160P", "4K"}
	if len(qualities) != len(expected) {
		t.Fatalf("expected %d qualities, got %d", len(expected), len(qualities))
	}
	for i, q := range expected {
		if qualities[i] != q {
			t.Errorf("at index %d: expected %s, got %s", i, q, qualities[i])
		}
	}
}

func TestAutofilter_RecoverSession_WithFilters(t *testing.T) {
	plugin := &Plugin{}

	// 1. Language tag
	msgLang := &telegram.NewMessage{
		Chat: &telegram.ChatObj{ID: -10012345},
		Message: &telegram.MessageObj{
			Message: "𝗛𝗘𝗬 👋 User 🫶\n    \n📁 Query : Vikram (MALAYALAM)\n\n🗓️ Page No › 1",
		},
	}
	sessLang := plugin.recoverSessionFromMessage(msgLang, 123)
	if sessLang == nil {
		t.Fatalf("expected recovered session, got nil")
	}
	if sessLang.Query != "Vikram" || sessLang.Language != "malayalam" {
		t.Errorf("expected Query='Vikram', Language='malayalam', got Query=%q, Language=%q", sessLang.Query, sessLang.Language)
	}

	// 2. Quality tag
	msgQual := &telegram.NewMessage{
		Chat: &telegram.ChatObj{ID: -10012345},
		Message: &telegram.MessageObj{
			Message: "𝗛𝗘𝗬 👋 User 🫶\n    \n📁 Query : Vikram (1080P)\n\n🗓️ Page No › 1",
		},
	}
	sessQual := plugin.recoverSessionFromMessage(msgQual, 123)
	if sessQual == nil {
		t.Fatalf("expected recovered session, got nil")
	}
	if sessQual.Query != "Vikram" || sessQual.Quality != "1080p" {
		t.Errorf("expected Query='Vikram', Quality='1080p', got Query=%q, Quality=%q", sessQual.Query, sessQual.Quality)
	}
}

func TestAutofilter_Pagination_FirstPage(t *testing.T) {
	plugin := &Plugin{}
	files := []search.MediaFile{
		{FileID: "f1", FileName: "KGF.Chapter.2.2022.Malayalam.HDRip.mkv", Size: "402 MB"},
	}

	// Page 1 of 8: should have Page + Next, but NO Previous
	kb := plugin.buildResultKeyboard(files, "key1", 1, 8, false)
	if kb == nil || len(kb.Rows) != 3 {
		t.Fatalf("expected 3 rows (filter, file, nav), got %d", len(kb.Rows))
	}

	// Filter row
	if len(kb.Rows[0].Buttons) != 2 {
		t.Fatalf("expected 2 filter buttons, got %d", len(kb.Rows[0].Buttons))
	}
	f1 := kb.Rows[0].Buttons[0].(*telegram.KeyboardButtonCallback)
	f2 := kb.Rows[0].Buttons[1].(*telegram.KeyboardButtonCallback)
	if f1.Text != "🌐 Languages" || f2.Text != "🎞 Quality" {
		t.Errorf("expected filter buttons '🌐 Languages' and '🎞 Quality', got %q and %q", f1.Text, f2.Text)
	}
	if f1.Style == nil || !f1.Style.BgPrimary {
		t.Errorf("expected Languages button to be Primary (Blue)")
	}
	if f2.Style == nil || !f2.Style.BgPrimary {
		t.Errorf("expected Quality button to be Primary (Blue)")
	}

	// Nav row
	navRow := kb.Rows[2].Buttons
	if len(navRow) != 2 {
		t.Fatalf("expected 2 nav buttons on first page, got %d", len(navRow))
	}
	pBtn := navRow[0].(*telegram.KeyboardButtonCallback)
	nBtn := navRow[1].(*telegram.KeyboardButtonCallback)
	if pBtn.Text != "📄 Page 1 / 8" {
		t.Errorf("expected page indicator '📄 Page 1 / 8', got %q", pBtn.Text)
	}
	if pBtn.Style == nil || !pBtn.Style.BgSuccess {
		t.Errorf("expected page indicator on page 1 to be Success (Green)")
	}
	if nBtn.Text != "Next ▶" {
		t.Errorf("expected 'Next ▶', got %q", nBtn.Text)
	}
	if nBtn.Style == nil || !nBtn.Style.BgSuccess {
		t.Errorf("expected Next button to be Success (Green)")
	}
	if strings.Contains(pBtn.Text, "Previous") || strings.Contains(nBtn.Text, "Previous") {
		t.Errorf("first page must NOT have a Previous button")
	}
}

func TestAutofilter_Pagination_MiddlePage(t *testing.T) {
	plugin := &Plugin{}
	files := []search.MediaFile{
		{FileID: "f1", FileName: "Vikram.2022.Tamil.1080p.mkv", Size: "1.4 GB"},
	}

	// Page 2 of 8: should have Previous + Page + Next
	kb := plugin.buildResultKeyboard(files, "key1", 2, 8, false)
	if kb == nil || len(kb.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(kb.Rows))
	}

	navRow := kb.Rows[2].Buttons
	if len(navRow) != 3 {
		t.Fatalf("expected 3 nav buttons on middle page, got %d", len(navRow))
	}
	prevBtn := navRow[0].(*telegram.KeyboardButtonCallback)
	pageBtn := navRow[1].(*telegram.KeyboardButtonCallback)
	nextBtn := navRow[2].(*telegram.KeyboardButtonCallback)

	if prevBtn.Text != "◀ Previous" {
		t.Errorf("expected '◀ Previous', got %q", prevBtn.Text)
	}
	if prevBtn.Style == nil || !prevBtn.Style.BgSuccess {
		t.Errorf("expected Previous button to be Success (Green)")
	}
	if pageBtn.Text != "📄 Page 2 / 8" {
		t.Errorf("expected '📄 Page 2 / 8', got %q", pageBtn.Text)
	}
	if pageBtn.Style == nil || !pageBtn.Style.BgDanger {
		t.Errorf("expected page indicator on page > 1 to be Danger (Red)")
	}
	if nextBtn.Text != "Next ▶" {
		t.Errorf("expected 'Next ▶', got %q", nextBtn.Text)
	}
	if nextBtn.Style == nil || !nextBtn.Style.BgSuccess {
		t.Errorf("expected Next button to be Success (Green)")
	}
}

func TestAutofilter_Pagination_LastPage(t *testing.T) {
	plugin := &Plugin{}
	files := []search.MediaFile{
		{FileID: "f1", FileName: "Inception.2010.mkv", Size: "700 MB"},
	}

	// Page 8 of 8: should have Previous + Page, but NO Next
	kb := plugin.buildResultKeyboard(files, "key1", 8, 8, false)
	if kb == nil || len(kb.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(kb.Rows))
	}

	navRow := kb.Rows[2].Buttons
	if len(navRow) != 2 {
		t.Fatalf("expected 2 nav buttons on last page, got %d", len(navRow))
	}
	prevBtn := navRow[0].(*telegram.KeyboardButtonCallback)
	pageBtn := navRow[1].(*telegram.KeyboardButtonCallback)

	if prevBtn.Text != "◀ Previous" {
		t.Errorf("expected '◀ Previous', got %q", prevBtn.Text)
	}
	if prevBtn.Style == nil || !prevBtn.Style.BgSuccess {
		t.Errorf("expected Previous button to be Success (Green)")
	}
	if pageBtn.Text != "📄 Page 8 / 8" {
		t.Errorf("expected '📄 Page 8 / 8', got %q", pageBtn.Text)
	}
	if pageBtn.Style == nil || !pageBtn.Style.BgDanger {
		t.Errorf("expected page indicator on page > 1 to be Danger (Red)")
	}
	if strings.Contains(prevBtn.Text, "Next") || strings.Contains(pageBtn.Text, "Next") {
		t.Errorf("last page must NOT have a Next button")
	}
}

func TestAutofilter_Pagination_SinglePage(t *testing.T) {
	plugin := &Plugin{}
	files := []search.MediaFile{
		{FileID: "f1", FileName: "ShortFilm.mp4", Size: "50 MB"},
	}

	// Page 1 of 1: only Page indicator button
	kb := plugin.buildResultKeyboard(files, "key1", 1, 1, false)
	if kb == nil || len(kb.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(kb.Rows))
	}

	navRow := kb.Rows[2].Buttons
	if len(navRow) != 1 {
		t.Fatalf("expected 1 nav button on single page, got %d", len(navRow))
	}
	pageBtn := navRow[0].(*telegram.KeyboardButtonCallback)
	if pageBtn.Text != "📄 Page 1 / 1" {
		t.Errorf("expected '📄 Page 1 / 1', got %q", pageBtn.Text)
	}
	if pageBtn.Style == nil || !pageBtn.Style.BgSuccess {
		t.Errorf("expected single page indicator to be Success (Green)")
	}
}

func TestAutofilter_FileButtonLabel_Format(t *testing.T) {
	testCases := []struct {
		name     string
		file     search.MediaFile
		expected string
	}{
		{
			name:     "Movie with Year, Language, and Quality",
			file:     search.MediaFile{FileName: "KGF.Chapter.2.2022.Malayalam.HDRip.x264.mkv", Size: "402 MB"},
			expected: "🎬 402 MB · KGF Chapter 2 · Malayalam · HDRip",
		},
		{
			name:     "Movie with Year, Language, and Multi-word Quality",
			file:     search.MediaFile{FileName: "Vikram.2022.Tamil.1080p.WEBRip.mkv", Size: "1.4 GB"},
			expected: "🎬 1.4 GB · Vikram · Tamil · 1080p WEBRip",
		},
		{
			name:     "Movie with Year and Quality (No Language)",
			file:     search.MediaFile{FileName: "Inception.2010.720p.mkv", Size: "700 MB"},
			expected: "🎬 700 MB · Inception · 720p",
		},
		{
			name:     "File with Title Only (No Language or Quality)",
			file:     search.MediaFile{FileName: "Documentary.mp4", Size: "350 MB"},
			expected: "🎬 350 MB · Documentary",
		},
		{
			name:     "Movie with Language and Quality without Year",
			file:     search.MediaFile{FileName: "Premam.Malayalam.HDRip.mkv", Size: "900 MB"},
			expected: "🎬 900 MB · Premam · Malayalam · HDRip",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			label := formatFileButtonLabel(tc.file)
			if strings.Contains(label, "🌷") {
				t.Errorf("file label %q must not contain flower emoji 🌷", label)
			}
			if !strings.HasPrefix(label, "🎬") {
				t.Errorf("file label %q should start with 🎬", label)
			}
			cleanLabel := strings.TrimRight(label, "\u2800")
			if cleanLabel != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, cleanLabel)
			}
			if len([]rune(label)) < 50 {
				t.Errorf("expected button label to be padded to at least 50 runes for left-alignment, got %d", len([]rune(label)))
			}
		})
	}
}

func TestAutofilter_FileButtonLabel_LongTitlePreservesMetadata(t *testing.T) {
	file := search.MediaFile{
		FileName: "Spider-Man.Across.the.Spider-Verse.2023.English.1080p.mkv",
		Size:     "2.4 GB",
	}
	label := formatFileButtonLabel(file)
	if !strings.Contains(label, "English") {
		t.Errorf("label %q should retain language 'English'", label)
	}
	if !strings.Contains(label, "1080p") {
		t.Errorf("label %q should retain quality '1080p'", label)
	}
	if !strings.Contains(label, "Spider-Man") {
		t.Errorf("label %q should retain movie title prefix", label)
	}
	if len([]rune(label)) > 60 {
		t.Errorf("label length %d exceeds max 60 runes: %q", len([]rune(label)), label)
	}
}

func TestAutofilter_MessageText_FormattingAndBotIdentity(t *testing.T) {
	// Bot A
	textA := formatResultText("KGF", 1, 8, 24, "PandaBot")
	if !strings.Contains(textA, "🎬 <b>KGF</b>") {
		t.Errorf("expected header '🎬 <b>KGF</b>', got:\n%s", textA)
	}
	if !strings.Contains(textA, "🔎 <b>Query:</b> KGF") {
		t.Errorf("expected query line, got:\n%s", textA)
	}
	if !strings.Contains(textA, "📄 <b>Page:</b> 1 / 8") {
		t.Errorf("expected page line, got:\n%s", textA)
	}
	if !strings.Contains(textA, "📦 <b>Results:</b> 24 files") {
		t.Errorf("expected results line, got:\n%s", textA)
	}
	if !strings.Contains(textA, "⏳ <i>This result expires in 15 minutes.</i>") {
		t.Errorf("expected expiry note, got:\n%s", textA)
	}
	if !strings.Contains(textA, "© PandaBot") {
		t.Errorf("expected bot signature '© PandaBot', got:\n%s", textA)
	}

	// Bot B with single file result
	textB := formatResultText("Vikram", 2, 5, 1, "CinemaTG")
	if !strings.Contains(textB, "📦 <b>Results:</b> 1 file") {
		t.Errorf("expected singular '1 file', got:\n%s", textB)
	}
	if !strings.Contains(textB, "© CinemaTG") {
		t.Errorf("expected bot signature '© CinemaTG', got:\n%s", textB)
	}

	// Verify no hardcoded Panda or CyberTG
	if strings.Contains(textB, "CyberTG") || strings.Contains(textB, "PandaBot") {
		t.Errorf("textB contains hardcoded bot name")
	}
}

func TestAutofilter_DynamicBotName(t *testing.T) {
	// 1. Bot with FirstName
	pluginA := &Plugin{
		appCtx: &plugins.AppContext{
			BotUser: &telegram.UserObj{
				ID:        12345,
				FirstName: "CustomBot",
				Username:  "custom_bot",
			},
		},
	}
	if name := pluginA.getBotName(nil); name != "CustomBot" {
		t.Errorf("expected 'CustomBot', got %q", name)
	}

	// 2. Bot with Username only
	pluginB := &Plugin{
		appCtx: &plugins.AppContext{
			BotUser: &telegram.UserObj{
				ID:       54321,
				Username: "movie_bot",
			},
		},
	}
	if name := pluginB.getBotName(nil); name != "@movie_bot" {
		t.Errorf("expected '@movie_bot', got %q", name)
	}

	// 3. Fallback when BotUser is nil
	pluginC := &Plugin{}
	if name := pluginC.getBotName(nil); name != "Bot" {
		t.Errorf("expected fallback 'Bot', got %q", name)
	}
}

func TestAutofilter_GroupSession_Isolation(t *testing.T) {
	plugin := &Plugin{
		sessions: make(map[string]*searchSession),
	}

	// Session 1: User 100 in Group -1001
	key1 := "-1001_100_50"
	plugin.saveSession(key1, &searchSession{
		Query:    "KGF",
		UserID:   100,
		ChatID:   -1001,
		Language: "malayalam",
	})

	// Session 2: User 200 in Group -1001
	key2 := "-1001_200_55"
	plugin.saveSession(key2, &searchSession{
		Query:    "Vikram",
		UserID:   200,
		ChatID:   -1001,
		Language: "tamil",
	})

	sess1 := plugin.getSession(key1)
	sess2 := plugin.getSession(key2)

	if sess1 == nil || sess2 == nil {
		t.Fatalf("expected both sessions to exist")
	}
	if sess1.Query != "KGF" || sess1.UserID != 100 || sess1.Language != "malayalam" {
		t.Errorf("session 1 corrupted: %+v", sess1)
	}
	if sess2.Query != "Vikram" || sess2.UserID != 200 || sess2.Language != "tamil" {
		t.Errorf("session 2 corrupted: %+v", sess2)
	}
}



