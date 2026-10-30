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
	"github.com/itsafxal/MYTGBOTZ/internal/gfilter"
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
	if sess.UserMention != "User" {
		t.Errorf("expected user mention 'User', got %q", sess.UserMention)
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

func TestAutofilter_Pagination_PremiumEmoji(t *testing.T) {
	appCtx := &plugins.AppContext{
		Config: &config.Config{
			EnablePremiumEmoji:        true,
			PrevButtonEmojiID:         5989806303474618304,
			NextButtonEmojiID:         5992261405565260004,
			PageButtonEmojiID:         5837048052061311256,
			QualityButtonEmojiID:      5276251908774858442,
			LanguageButtonEmojiID:     5276011261757265100,
			MultiLangButtonEmojiID:    5256169350368353876,
			SelectLanguageEmojiID:     5447410659077661506,
			SelectQualityEmojiID:      5334544901428229844,
			FastDownloadButtonEmojiID: 6156513311585211842,
			GreetingPrefixEmojiID:     5472055112702629499,
			GreetingEmojiID:           5427232022252764138,
			QueryEmojiID:              5190859184312167965,
			PageTextEmojiID:           5836954069586946866,
			NotePrefixEmojiID:         4956461073550017373,
			NoteSuffixEmojiID:         4958724224962265918,
			NoteExpireEmojiID:         5440660757194744323,
			BotNameEmojiID:            6104631352190043951,
			FileSizePrefixEmojiID:     5992513524440502236,
			FileSizeSuffixEmojiID:     5992519984071315160,
		},
	}
	plugin := New(appCtx)
	files := []search.MediaFile{
		{FileID: "f1", FileName: "Vikram.2022.Tamil.1080p.mkv", Size: "1.4 GB"},
	}

	// Page 2 of 8: should have Languages/Quality row + File row (with Icon) + Previous/Page/Next row
	kb := plugin.buildResultKeyboard(files, "key1", 2, 8, false)
	if kb == nil || len(kb.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(kb.Rows))
	}

	// 1. Language & Quality buttons
	headerRow := kb.Rows[0].Buttons
	if len(headerRow) != 2 {
		t.Fatalf("expected 2 header buttons, got %d", len(headerRow))
	}
	langBtn := headerRow[0].(*telegram.KeyboardButtonCallback)
	qualBtn := headerRow[1].(*telegram.KeyboardButtonCallback)
	if langBtn.Text != "Language" {
		t.Errorf("expected 'Language', got %q", langBtn.Text)
	}
	if langBtn.Style == nil || langBtn.Style.Icon != 5276011261757265100 {
		t.Errorf("expected Language icon 5276011261757265100, got %v", langBtn.Style)
	}
	if qualBtn.Text != "Quality" {
		t.Errorf("expected 'Quality', got %q", qualBtn.Text)
	}
	if qualBtn.Style == nil || qualBtn.Style.Icon != 5276251908774858442 {
		t.Errorf("expected Quality icon 5276251908774858442, got %v", qualBtn.Style)
	}

	// 2. File button: with prefix icon and clean label
	fileBtn := kb.Rows[1].Buttons[0].(*telegram.KeyboardButtonCallback)
	if fileBtn.Style == nil || fileBtn.Style.Icon != 5992513524440502236 {
		t.Errorf("expected File icon 5992513524440502236, got %v", fileBtn.Style)
	}
	if strings.HasPrefix(fileBtn.Text, "🔸") {
		t.Errorf("expected file button with custom icon to not start with 🔸, got %q", fileBtn.Text)
	}

	// 3. Navigation row
	navRow := kb.Rows[2].Buttons
	if len(navRow) != 3 {
		t.Fatalf("expected 3 nav buttons on middle page, got %d", len(navRow))
	}
	prevBtn := navRow[0].(*telegram.KeyboardButtonCallback)
	pageBtn := navRow[1].(*telegram.KeyboardButtonCallback)
	nextBtn := navRow[2].(*telegram.KeyboardButtonCallback)

	// Previous button: "Previous", custom emoji ID, Success style (Green)
	if prevBtn.Text != "Previous" {
		t.Errorf("expected 'Previous' text for premium Previous button, got %q", prevBtn.Text)
	}
	if prevBtn.Style == nil || !prevBtn.Style.BgSuccess {
		t.Errorf("expected Previous button to be Success (Green)")
	}
	if prevBtn.Style == nil || prevBtn.Style.Icon != 5989806303474618304 {
		t.Errorf("expected Previous button Icon to be 5989806303474618304, got %v", prevBtn.Style.Icon)
	}

	// Page button: "page 2/8", custom emoji ID, Danger style
	if pageBtn.Text != "page 2/8" {
		t.Errorf("expected 'page 2/8', got %q", pageBtn.Text)
	}
	if pageBtn.Style == nil || !pageBtn.Style.BgDanger {
		t.Errorf("expected page indicator on page > 1 to be Danger (Red)")
	}
	if pageBtn.Style == nil || pageBtn.Style.Icon != 5837048052061311256 {
		t.Errorf("expected Page button Icon to be 5837048052061311256, got %v", pageBtn.Style.Icon)
	}

	// Next button: "Next", custom emoji ID, Success style (Green)
	if nextBtn.Text != "Next" {
		t.Errorf("expected 'Next' text for premium Next button, got %q", nextBtn.Text)
	}
	if nextBtn.Style == nil || !nextBtn.Style.BgSuccess {
		t.Errorf("expected Next button to be Success (Green)")
	}
	if nextBtn.Style == nil || nextBtn.Style.Icon != 5992261405565260004 {
		t.Errorf("expected Next button Icon to be 5992261405565260004, got %v", nextBtn.Style.Icon)
	}
}

func TestAutofilter_Pagination_PremiumEmoji_Disabled(t *testing.T) {
	appCtx := &plugins.AppContext{
		Config: &config.Config{
			EnablePremiumEmoji: false,
			PrevButtonEmojiID:  5989806303474618304,
			NextButtonEmojiID:  5992261405565260004,
		},
	}
	plugin := New(appCtx)
	files := []search.MediaFile{
		{FileID: "f1", FileName: "Vikram.2022.Tamil.1080p.mkv", Size: "1.4 GB"},
	}

	kb := plugin.buildResultKeyboard(files, "key1", 2, 8, false)
	// Header row
	langBtn := kb.Rows[0].Buttons[0].(*telegram.KeyboardButtonCallback)
	qualBtn := kb.Rows[0].Buttons[1].(*telegram.KeyboardButtonCallback)
	if langBtn.Text != "🌐 Languages" || (langBtn.Style != nil && langBtn.Style.Icon != 0) {
		t.Errorf("expected '🌐 Languages' without icon when disabled, got %q", langBtn.Text)
	}
	if qualBtn.Text != "🎞 Quality" || (qualBtn.Style != nil && qualBtn.Style.Icon != 0) {
		t.Errorf("expected '🎞 Quality' without icon when disabled, got %q", qualBtn.Text)
	}

	// File row
	fileBtn := kb.Rows[1].Buttons[0].(*telegram.KeyboardButtonCallback)
	if !strings.HasPrefix(fileBtn.Text, "🔸") || (fileBtn.Style != nil && fileBtn.Style.Icon != 0) {
		t.Errorf("expected file button with 🔸 and no icon when disabled, got %q", fileBtn.Text)
	}

	// Nav row
	navRow := kb.Rows[2].Buttons
	prevBtn := navRow[0].(*telegram.KeyboardButtonCallback)
	pageBtn := navRow[1].(*telegram.KeyboardButtonCallback)
	nextBtn := navRow[2].(*telegram.KeyboardButtonCallback)

	if prevBtn.Text != "◀ Previous" {
		t.Errorf("expected fallback '◀ Previous' when disabled, got %q", prevBtn.Text)
	}
	if pageBtn.Text != "📄 Page 2 / 8" {
		t.Errorf("expected fallback '📄 Page 2 / 8' when disabled, got %q", pageBtn.Text)
	}
	if nextBtn.Text != "Next ▶" {
		t.Errorf("expected fallback 'Next ▶' when disabled, got %q", nextBtn.Text)
	}
}

func TestAutofilter_MessageText_PremiumEmoji(t *testing.T) {
	appCtx := &plugins.AppContext{
		Config: &config.Config{
			EnablePremiumEmoji:    true,
			GreetingPrefixEmojiID: 5472055112702629499,
			GreetingEmojiID:       5427232022252764138,
			QueryEmojiID:          5190859184312167965,
			PageTextEmojiID:       5836954069586946866,
			NotePrefixEmojiID:     4956461073550017373,
			NoteSuffixEmojiID:     4958724224962265918,
			NoteExpireEmojiID:     5440660757194744323,
			BotNameEmojiID:        6104631352190043951,
			AutoDeleteFilterTime:  "15m",
		},
	}
	plugin := New(appCtx)

	text := plugin.formatResultText("User", "KGF", 1, 8, 24, "PandaBot")
	if !strings.Contains(text, `𝗛𝗘𝗬 <emoji id="5472055112702629499">👋</emoji> User <emoji id="5427232022252764138">🫶</emoji>`) {
		t.Errorf("expected greeting with emoji tag, got:\n%s", text)
	}
	if !strings.Contains(text, `<emoji id="5190859184312167965">📁</emoji> Query : KGF`) {
		t.Errorf("expected query line with emoji tag, got:\n%s", text)
	}
	if !strings.Contains(text, `<emoji id="5836954069586946866">🗓️</emoji> Page No › 1`) {
		t.Errorf("expected page line with emoji tag, got:\n%s", text)
	}
	if !strings.Contains(text, `<blockquote><emoji id="4956461073550017373">✍️</emoji> NOTE : <emoji id="4958724224962265918">⚠️</emoji> This Message Will Be Auto Deleted Within 15 Mins <emoji id="5440660757194744323">❗️</emoji></blockquote>`) {
		t.Errorf("expected note line with emoji tags, got:\n%s", text)
	}
	if !strings.Contains(text, `©PandaBot <emoji id="6104631352190043951">🗡️</emoji>`) {
		t.Errorf("expected bot name line with emoji tag, got:\n%s", text)
	}

	langText := plugin.formatLanguagesMenuText("KGF", "PandaBot")
	if !strings.Contains(langText, `<emoji id="5447410659077661506">🌐</emoji> <b>Select Desired Language</b>`) {
		t.Errorf("expected languages menu with select language header emoji tag, got:\n%s", langText)
	}
	if !strings.Contains(langText, `<emoji id="5190859184312167965">📁</emoji> Query : KGF`) {
		t.Errorf("expected languages menu with query emoji tag, got:\n%s", langText)
	}
	if !strings.Contains(langText, `<blockquote><emoji id="4956461073550017373">✍️</emoji> NOTE : <emoji id="4958724224962265918">⚠️</emoji> This Message Will Be Auto Deleted Within 15 Mins <emoji id="5440660757194744323">❗️</emoji></blockquote>`) {
		t.Errorf("expected languages menu with note emoji tags, got:\n%s", langText)
	}
	if !strings.Contains(langText, `©PandaBot <emoji id="6104631352190043951">🗡️</emoji>`) {
		t.Errorf("expected languages menu with bot emoji tag, got:\n%s", langText)
	}

	qualText := plugin.formatQualitiesMenuText("KGF", "PandaBot")
	if !strings.Contains(qualText, `<emoji id="5334544901428229844">🎞</emoji> <b>Select Desired Quality</b>`) {
		t.Errorf("expected qualities menu with select quality header emoji tag, got:\n%s", qualText)
	}
	if !strings.Contains(qualText, `<emoji id="5190859184312167965">📁</emoji> Query : KGF`) {
		t.Errorf("expected qualities menu with query emoji tag, got:\n%s", qualText)
	}
	if !strings.Contains(qualText, `<blockquote><emoji id="4956461073550017373">✍️</emoji> NOTE : <emoji id="4958724224962265918">⚠️</emoji> This Message Will Be Auto Deleted Within 15 Mins <emoji id="5440660757194744323">❗️</emoji></blockquote>`) {
		t.Errorf("expected qualities menu with note emoji tags, got:\n%s", qualText)
	}
	if !strings.Contains(qualText, `©PandaBot <emoji id="6104631352190043951">🗡️</emoji>`) {
		t.Errorf("expected qualities menu with bot emoji tag, got:\n%s", qualText)
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
			expected: "🔸 402 MB ▫️ KGF Chapter 2 Malayalam HDRip",
		},
		{
			name:     "Movie with Year, Language, and Multi-word Quality",
			file:     search.MediaFile{FileName: "Vikram.2022.Tamil.1080p.WEBRip.mkv", Size: "1.4 GB"},
			expected: "🔸 1.4 GB ▫️ Vikram Tamil 1080p WEBRip",
		},
		{
			name:     "Movie with Year and Quality (No Language)",
			file:     search.MediaFile{FileName: "Inception.2010.720p.mkv", Size: "700 MB"},
			expected: "🔸 700 MB ▫️ Inception 720p",
		},
		{
			name:     "File with Title Only (No Language or Quality)",
			file:     search.MediaFile{FileName: "Documentary.mp4", Size: "350 MB"},
			expected: "🔸 350 MB ▫️ Documentary",
		},
		{
			name:     "Movie with Language and Quality without Year",
			file:     search.MediaFile{FileName: "Premam.Malayalam.HDRip.mkv", Size: "900 MB"},
			expected: "🔸 900 MB ▫️ Premam Malayalam HDRip",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			label := formatFileButtonLabel(tc.file)
			if strings.Contains(label, "🌷") {
				t.Errorf("file label %q must not contain flower emoji 🌷", label)
			}
			if !strings.HasPrefix(label, "🔸") {
				t.Errorf("file label %q should start with 🔸", label)
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

func TestAutofilter_LanguagesKeyboard(t *testing.T) {
	kb := buildLanguagesKeyboard("testkey")
	if kb == nil || len(kb.Rows) == 0 {
		t.Fatalf("expected non-empty languages keyboard")
	}
	lastRow := kb.Rows[len(kb.Rows)-1].Buttons
	if len(lastRow) != 1 {
		t.Fatalf("expected 1 button in last row, got %d", len(lastRow))
	}
	backBtn := lastRow[0].(*telegram.KeyboardButtonCallback)
	if backBtn.Text != "↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭" {
		t.Errorf("expected back button text '↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭', got %q", backBtn.Text)
	}
	if backBtn.Style == nil || !backBtn.Style.BgDanger {
		t.Errorf("expected back button in languages menu to have Danger (Red) style")
	}

	// Test with Premium Emoji enabled
	plugin := New(&plugins.AppContext{
		Config: &config.Config{
			EnablePremiumEmoji:     true,
			MultiLangButtonEmojiID: 5256169350368353876,
		},
	})
	kbPremium := plugin.buildLanguagesKeyboard("testkey")
	if kbPremium == nil {
		t.Fatalf("expected non-empty premium languages keyboard")
	}
	var foundMultiLang bool
	for _, row := range kbPremium.Rows {
		for _, b := range row.Buttons {
			btn, ok := b.(*telegram.KeyboardButtonCallback)
			if ok && btn.Text == "Multi Lang" {
				foundMultiLang = true
				if btn.Style == nil || btn.Style.Icon != 5256169350368353876 {
					t.Errorf("expected multi lang button icon 5256169350368353876, got %v", btn.Style)
				}
			}
		}
	}
	if !foundMultiLang {
		t.Errorf("expected to find 'Multi Lang' button with premium emoji in languages keyboard")
	}
}

func TestAutofilter_QualitiesKeyboard(t *testing.T) {
	kb := buildQualitiesKeyboard("testkey")
	if kb == nil || len(kb.Rows) != 4 {
		t.Fatalf("expected 4 rows in qualities keyboard (2 qualities, 2 qualities, 1 quality, 1 back), got %d", len(kb.Rows))
	}

	// Row 1: 480P, 720P
	r1 := kb.Rows[0].Buttons
	if len(r1) != 2 || r1[0].(*telegram.KeyboardButtonCallback).Text != "480P" || r1[1].(*telegram.KeyboardButtonCallback).Text != "720P" {
		t.Errorf("row 1 expected [480P, 720P], got %+v", r1)
	}

	// Row 2: 1080P, 2160P
	r2 := kb.Rows[1].Buttons
	if len(r2) != 2 || r2[0].(*telegram.KeyboardButtonCallback).Text != "1080P" || r2[1].(*telegram.KeyboardButtonCallback).Text != "2160P" {
		t.Errorf("row 2 expected [1080P, 2160P], got %+v", r2)
	}

	// Row 3: 4K
	r3 := kb.Rows[2].Buttons
	if len(r3) != 1 || r3[0].(*telegram.KeyboardButtonCallback).Text != "4K" {
		t.Errorf("row 3 expected [4K], got %+v", r3)
	}

	// Row 4: Back button (Red / Danger)
	r4 := kb.Rows[3].Buttons
	if len(r4) != 1 {
		t.Fatalf("row 4 expected 1 button, got %d", len(r4))
	}
	backBtn := r4[0].(*telegram.KeyboardButtonCallback)
	if backBtn.Text != "↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭" {
		t.Errorf("expected back button text '↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭', got %q", backBtn.Text)
	}
	if backBtn.Style == nil || !backBtn.Style.BgDanger {
		t.Errorf("expected back button in qualities menu to have Danger (Red) style")
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
	textA := formatResultText("User", "KGF", 1, 8, 24, "PandaBot", 15)
	if !strings.Contains(textA, "𝗛𝗘𝗬 👋 User 🫶") {
		t.Errorf("expected greeting, got:\n%s", textA)
	}
	if !strings.Contains(textA, "📁 Query : KGF") {
		t.Errorf("expected query line, got:\n%s", textA)
	}
	if !strings.Contains(textA, "🗓️ Page No › 1") {
		t.Errorf("expected page line, got:\n%s", textA)
	}
	if !strings.Contains(textA, "<blockquote>✍️ NOTE : ⚠️ This Message Will Be Auto Deleted Within 15 Mins ❗️</blockquote>") {
		t.Errorf("expected auto delete blockquote note, got:\n%s", textA)
	}
	if !strings.Contains(textA, "©PandaBot 🗡️") {
		t.Errorf("expected bot signature '©PandaBot 🗡️', got:\n%s", textA)
	}

	// Bot B with single file result & custom time & user mention
	textB := formatResultText(`<a href="tg://user?id=123">Afsal</a>`, "Vikram", 2, 5, 1, "CinemaTG", 30)
	if !strings.Contains(textB, `𝗛𝗘𝗬 👋 <a href="tg://user?id=123">Afsal</a> 🫶`) {
		t.Errorf("expected mention link, got:\n%s", textB)
	}
	if !strings.Contains(textB, "📁 Query : Vikram") {
		t.Errorf("expected query line, got:\n%s", textB)
	}
	if !strings.Contains(textB, "🗓️ Page No › 2") {
		t.Errorf("expected page 2, got:\n%s", textB)
	}
	if !strings.Contains(textB, "Within 30 Mins") {
		t.Errorf("expected within 30 mins, got:\n%s", textB)
	}
	if !strings.Contains(textB, "©CinemaTG 🗡️") {
		t.Errorf("expected bot signature '©CinemaTG 🗡️', got:\n%s", textB)
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

func TestAutofilter_MaintenanceMode(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_maint.sqlite3")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		SQLiteDBPath:    dbPath,
		SyncBatchSize:   100,
		MaintenanceMode: true,
		OwnerID:         999,
		Admins:          []int64{888},
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

	if !plugin.isMaintenanceMode() {
		t.Fatalf("expected maintenance mode to be true")
	}

	// Non-admin message in group or DM should be completely ignored (return nil)
	nonAdminMsg := &telegram.NewMessage{
		Sender: &telegram.UserObj{ID: 12345},
		Chat:   &telegram.ChatObj{ID: -10012345},
		Message: &telegram.MessageObj{
			Message: "avatar",
		},
	}

	err := plugin.HandleTextMessage(nonAdminMsg)
	if err != nil {
		t.Errorf("expected nil error for non-admin message in maintenance mode, got %v", err)
	}

	// Maintenance disabled: should reflect false
	cfg.MaintenanceMode = false
	if plugin.isMaintenanceMode() {
		t.Errorf("expected maintenance mode to be false")
	}
}

func TestAutofilter_MessageHasLink(t *testing.T) {
	tests := []struct {
		name    string
		msg     *telegram.NewMessage
		wantURL bool
	}{
		{
			name: "plain text no link",
			msg: &telegram.NewMessage{
				Message: &telegram.MessageObj{Message: "KGF Chapter 2"},
			},
			wantURL: false,
		},
		{
			name: "message with https URL",
			msg: &telegram.NewMessage{
				Message: &telegram.MessageObj{Message: "check https://example.com for info"},
			},
			wantURL: true,
		},
		{
			name: "message with http URL",
			msg: &telegram.NewMessage{
				Message: &telegram.MessageObj{Message: "http://t.me/channel/123"},
			},
			wantURL: true,
		},
		{
			name: "message with URL entity",
			msg: &telegram.NewMessage{
				Message: &telegram.MessageObj{
					Message: "click here",
					Entities: []telegram.MessageEntity{
						&telegram.MessageEntityURL{Offset: 0, Length: 10},
					},
				},
			},
			wantURL: true,
		},
		{
			name: "message with TextUrl entity",
			msg: &telegram.NewMessage{
				Message: &telegram.MessageObj{
					Message: "click here",
					Entities: []telegram.MessageEntity{
						&telegram.MessageEntityTextURL{Offset: 0, Length: 10, URL: "https://example.com"},
					},
				},
			},
			wantURL: true,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantURL: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := messageHasLink(tc.msg)
			if got != tc.wantURL {
				t.Errorf("messageHasLink() = %v, want %v", got, tc.wantURL)
			}
		})
	}
}

type mockGFilterService struct {
	matchedKeywords map[string]bool
	executedCount   int
}

func (m *mockGFilterService) MatchAndExecute(msg *telegram.NewMessage) (bool, error) {
	if msg == nil {
		return false, nil
	}
	for kw := range m.matchedKeywords {
		if strings.EqualFold(msg.Text(), kw) {
			m.executedCount++
			return true, nil
		}
	}
	return false, nil
}

func (m *mockGFilterService) SaveFilter(ctx context.Context, f *gfilter.GFilter) error { return nil }
func (m *mockGFilterService) DeleteFilter(ctx context.Context, keyword string) (bool, error) { return true, nil }
func (m *mockGFilterService) DeleteAllFilters(ctx context.Context) (int64, error) { return 0, nil }
func (m *mockGFilterService) GetFilter(ctx context.Context, keyword string) (*gfilter.GFilter, error) { return nil, nil }
func (m *mockGFilterService) ListFilters(ctx context.Context, limit, offset int) ([]*gfilter.GFilter, int64, error) { return nil, 0, nil }
func (m *mockGFilterService) Count(ctx context.Context) (int64, error) { return 0, nil }

// TestAutofilter_GlobalFilterPriority tests that Global Filter takes absolute priority over Auto Filter.
// - Incoming "Vikram" -> Global Filter handles, Auto Filter is NOT executed.
// - Incoming "Vikram Movie" -> Global Filter does not match, Auto Filter continues.
func TestAutofilter_GlobalFilterPriority(t *testing.T) {
	mockGF := &mockGFilterService{
		matchedKeywords: map[string]bool{"Vikram": true},
	}

	appCtx := &plugins.AppContext{
		GFilter: mockGF,
		Config:  &config.Config{},
	}

	p := New(appCtx)

	// Test 1: Exact match "Vikram" (case variations)
	for _, text := range []string{"Vikram", "vikram", "VIKRAM", "ViKrAm"} {
		m := &telegram.NewMessage{
			Message: &telegram.MessageObj{
				Message: text,
			},
		}

		err := p.HandleTextMessage(m)
		if err != nil {
			t.Errorf("HandleTextMessage error: %v", err)
		}
	}

	if mockGF.executedCount != 4 {
		t.Errorf("Expected mock Global Filter to be executed 4 times, got %d", mockGF.executedCount)
	}

	// Test 2: Mismatch "Vikram Movie" -> Global Filter should NOT match
	priorCount := mockGF.executedCount
	mMis := &telegram.NewMessage{
		Message: &telegram.MessageObj{
			Message: "Vikram Movie",
		},
	}
	_ = p.HandleTextMessage(mMis)
	if mockGF.executedCount != priorCount {
		t.Errorf("Expected Global Filter to NOT execute for 'Vikram Movie', but count increased to %d", mockGF.executedCount)
	}
}
