package autofilter

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
	"github.com/itsafxal/MYTGBOTZ/internal/search"
)

const (
	maxFilesPerPage = 8
	maxQueryLength  = 150
	minQueryLength  = 2
)

// Language definitions matching TGxCyberBot
var languages = []struct {
	Label string
	Code  string
}{
	{Label: "മലയാളം", Code: "malayalam"},
	{Label: "தமிழ்", Code: "tamil"},
	{Label: "ᴇɴɢʟɪsʜ", Code: "english"},
	{Label: "हिन्दी", Code: "hindi"},
	{Label: "తెలుగు", Code: "telugu"},
	{Label: "కన్నడ", Code: "kannada"},
	{Label: "🪄 ᴍᴜʟᴛɪ ʟᴀɴɢ 🪄", Code: "multi"},
}

// Quality definitions matching TGxCyberBot
var qualities = []string{
	"360P", "480P", "720P", "1080P", "1440P", "2160P", "4K",
}

// searchSession stores state for pagination and filtering
type searchSession struct {
	Query       string
	UserID      int64
	ChatID      int64
	Language    string
	Quality     string
	Suggestions []string
	UpdatedAt   time.Time
}

// Plugin implements the Auto-Filter Search Engine plugin.
type Plugin struct {
	appCtx   *plugins.AppContext
	sessions map[string]*searchSession
	sessMu   sync.RWMutex
}

// New creates a new autofilter plugin.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{
		appCtx:   appCtx,
		sessions: make(map[string]*searchSession),
	}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "autofilter"
}

// Commands returns the command definitions for autofilter (none required for ambient search).
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{}
}

// MessageHandlers registers the ambient movie search handler for non-command text messages.
func (p *Plugin) MessageHandlers() []plugins.MessageDefinition {
	return []plugins.MessageDefinition{
		{
			Filter: func(m *gotgbot.Message) bool {
				if m == nil || m.Text == "" {
					return false
				}
				// Reject commands
				if message.Command(m) {
					return false
				}
				return true
			},
			Handler: p.HandleTextMessage,
		},
	}
}

// CallbackHandlers registers interactive callback query handlers for pagination, spelling, and filters.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{Prefix: "next|", Handler: p.HandlePaginationCallback},
		{Prefix: "sp#", Handler: p.HandleSpellCheckCallback},
		{Prefix: "languages#", Handler: p.HandleLanguagesCallback},
		{Prefix: "fl#", Handler: p.HandleFilterLanguageCallback},
		{Prefix: "qualities#", Handler: p.HandleQualitiesCallback},
		{Prefix: "fq#", Handler: p.HandleFilterQualityCallback},
		{Prefix: "close_data", Handler: p.HandleCloseCallback},
		{Prefix: "pages", Handler: p.HandlePagesCallback},
	}
}

// HandleTextMessage intercepts user movie search queries in PM and group chats.
func (p *Plugin) HandleTextMessage(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	user := ctx.EffectiveSender.User
	chat := ctx.EffectiveChat

	if msg == nil || user == nil || chat == nil {
		return nil
	}

	query := strings.TrimSpace(msg.Text)
	if !search.IsValidSearchText(query) {
		return nil
	}

	if p.appCtx == nil || p.appCtx.Search == nil {
		return nil
	}

	searchKey := fmt.Sprintf("%d_%d", user.Id, msg.MessageId)

	// Execute search
	result, err := p.appCtx.Search.Search(context.Background(), query, maxFilesPerPage, 0, search.SearchFilter{})
	if err != nil {
		if p.appCtx.Logger != nil {
			p.appCtx.Logger.Error("search error", "query", query, "error", err)
		}
		return nil
	}

	userMention := formatUserMention(user)

	// If results found, display interactive file list
	if len(result.Files) > 0 {
		p.saveSession(searchKey, &searchSession{
			Query:     query,
			UserID:    user.Id,
			ChatID:    chat.Id,
			UpdatedAt: time.Now(),
		})

		text := formatResultText(userMention, query, 1)
		kb := p.buildResultKeyboard(result.Files, searchKey, 0, result.NextOffset)

		_, replyErr := msg.Reply(b, text, &gotgbot.SendMessageOpts{
			ParseMode:   "HTML",
			ReplyMarkup: kb,
		})
		return replyErr
	}

	// If no results, try to fetch spelling suggestions
	suggestions, sugErr := p.appCtx.Search.GetSuggestions(context.Background(), query, 6)
	if sugErr == nil && len(suggestions) > 0 {
		p.saveSession(searchKey, &searchSession{
			Query:       query,
			UserID:      user.Id,
			ChatID:      chat.Id,
			Suggestions: suggestions,
			UpdatedAt:   time.Now(),
		})

		text := formatSpellCheckText(userMention, query)
		kb := p.buildSpellCheckKeyboard(suggestions, searchKey)

		_, replyErr := msg.Reply(b, text, &gotgbot.SendMessageOpts{
			ParseMode:   "HTML",
			ReplyMarkup: kb,
		})
		return replyErr
	}

	// No results and no suggestions found
	notFoundText := fmt.Sprintf(
		"Hᴇʏ %s 💫,\nNᴏ Sᴇᴀʀᴄʜ Rᴇꜱᴜʟᴛ Fᴏᴜɴᴅ Fᴏʀ Yᴏᴜʀ Rᴇᴏ̨ᴜᴇsᴛᴇᴅ Mᴏᴠɪᴇ.\n\n➪ Sᴇɴᴅ Mᴇ A Cᴏʀʀᴇᴄᴛ Sᴘᴇʟʟɪɴɢ. Oʀ Pʟᴇᴀꜱᴇ Cʜᴇᴄᴋ Tʜᴀᴛ Mᴏᴠɪᴇ Oᴛᴛ Rᴇʟᴇᴇsᴇᴅ.✨",
		userMention,
	)
	_, replyErr := msg.Reply(b, notFoundText, &gotgbot.SendMessageOpts{
		ParseMode: "HTML",
	})
	return replyErr
}

// HandlePaginationCallback processes next/back navigation buttons.
// Format: next|<searchKey>|<offset>
func (p *Plugin) HandlePaginationCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.Data, "|")
	if len(parts) != 3 {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid pagination data", ShowAlert: true})
		return nil
	}

	searchKey := parts[1]
	offset, err := strconv.Atoi(parts[2])
	if err != nil {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid offset", ShowAlert: true})
		return nil
	}

	sess := p.getSession(searchKey)
	if sess == nil && cb.Message != nil {
		sess = p.recoverSessionFromMessage(cb.Message, cb.From.Id)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "🔁 Sᴇᴀʀᴄʜ ᴇxᴘɪʀᴇᴅ ᴏʀ ɴᴏᴛ ғᴏᴜɴᴅ. Pʟᴇᴀsᴇ sᴇᴀʀᴄʜ ᴀɢᴀɪɴ 😇",
			ShowAlert: true,
		})
		return nil
	}

	// Check user authorization
	if cb.From.Id != sess.UserID {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "✋ Tʜɪs ɪs ɴᴏᴛ ғᴏʀ ʏᴏᴜ! Pʟᴇᴀsᴇ ᴍᴀᴋᴇ ʏᴏᴜʀ ᴏᴡɴ sᴇᴀʀᴄʜ.",
			ShowAlert: true,
		})
		return nil
	}

	filter := search.SearchFilter{
		Language: sess.Language,
		Quality:  sess.Quality,
	}

	result, err := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, offset, filter)
	if err != nil || len(result.Files) == 0 {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "🚫 Nᴏ ᴍᴏʀᴇ ʀᴇsᴜʟᴛs ғᴏᴜɴᴅ 🚫",
			ShowAlert: true,
		})
		return nil
	}

	userMention := formatUserMention(&cb.From)
	currentPage := (offset / maxFilesPerPage) + 1
	text := formatResultText(userMention, sess.Query, currentPage)
	kb := p.buildResultKeyboard(result.Files, searchKey, offset, result.NextOffset)

	if cb.Message != nil {
		_, _, editErr := cb.Message.EditText(b, &gotgbot.EditMessageTextOpts{
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: kb,
		})
		if editErr != nil && isMessageNotModified(editErr) {
			_, _, _ = cb.Message.EditReplyMarkup(b, &gotgbot.EditMessageReplyMarkupOpts{
				ReplyMarkup: kb,
			})
		}
	}

	_, _ = cb.Answer(b, nil)
	return nil
}

// HandleSpellCheckCallback processes clicks on spelling suggestions.
// Format: sp#<searchKey>#<index>
func (p *Plugin) HandleSpellCheckCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.Data, "#")
	if len(parts) != 3 {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid suggestion data", ShowAlert: true})
		return nil
	}

	searchKey := parts[1]
	idx, err := strconv.Atoi(parts[2])
	if err != nil {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid index", ShowAlert: true})
		return nil
	}

	sess := p.getSession(searchKey)
	if sess == nil || idx >= len(sess.Suggestions) {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "Sᴇᴀʀᴄʜ ᴇxᴘɪʀᴇᴅ ᴏʀ ɴᴏᴛ ғᴏᴜɴᴅ. Pʟᴇᴀsᴇ sᴇᴀʀᴄʜ ᴀɢᴀɪɴ. 🔰",
			ShowAlert: true,
		})
		return nil
	}

	if cb.From.Id != sess.UserID {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "✋ Tʜɪs ɪs ɴᴏᴛ ғᴏʀ ʏᴏᴜ! Pʟᴇᴀsᴇ ᴍᴀᴋᴇ ʏᴏᴜʀ ᴏᴡɴ sᴇᴀʀᴄʜ.",
			ShowAlert: true,
		})
		return nil
	}

	selectedQuery := sess.Suggestions[idx]
	sess.Query = selectedQuery
	sess.Suggestions = nil

	result, err := p.appCtx.Search.Search(context.Background(), selectedQuery, maxFilesPerPage, 0, search.SearchFilter{})
	if err != nil || len(result.Files) == 0 {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "🚫 Nᴏ ғɪʟᴇs ғᴏᴜɴᴅ.",
			ShowAlert: true,
		})
		return nil
	}

	userMention := formatUserMention(&cb.From)
	text := formatResultText(userMention, selectedQuery, 1)
	kb := p.buildResultKeyboard(result.Files, searchKey, 0, result.NextOffset)

	if cb.Message != nil {
		_, _, editErr := cb.Message.EditText(b, &gotgbot.EditMessageTextOpts{
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: kb,
		})
		if editErr != nil && isMessageNotModified(editErr) {
			_, _, _ = cb.Message.EditReplyMarkup(b, &gotgbot.EditMessageReplyMarkupOpts{
				ReplyMarkup: kb,
			})
		}
	}

	_, _ = cb.Answer(b, nil)
	return nil
}

// HandleLanguagesCallback renders language filter selection buttons.
// Format: languages#<searchKey>
func (p *Plugin) HandleLanguagesCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb == nil {
		return nil
	}

	parts := strings.SplitN(cb.Data, "#", 2)
	if len(parts) != 2 {
		return nil
	}
	searchKey := parts[1]

	sess := p.getSession(searchKey)
	if sess == nil && cb.Message != nil {
		sess = p.recoverSessionFromMessage(cb.Message, cb.From.Id)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Search expired.", ShowAlert: true})
		return nil
	}

	if cb.From.Id != sess.UserID {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "✋ This is not your search.", ShowAlert: true})
		return nil
	}

	var rows [][]gotgbot.InlineKeyboardButton
	var currentRow []gotgbot.InlineKeyboardButton

	for i, lang := range languages {
		btn := gotgbot.InlineKeyboardButton{
			Text:         lang.Label,
			CallbackData: fmt.Sprintf("fl#%s#%s", lang.Code, searchKey),
		}
		currentRow = append(currentRow, btn)
		if (i+1)%2 == 0 || i == len(languages)-1 {
			rows = append(rows, currentRow)
			currentRow = nil
		}
	}

	// Back to results button (matching TGxCyberBot)
	rows = append(rows, []gotgbot.InlineKeyboardButton{
		{Text: "↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭", CallbackData: fmt.Sprintf("next|%s|0", searchKey)},
	})

	if cb.Message != nil {
		_, _, _ = cb.Message.EditReplyMarkup(b, &gotgbot.EditMessageReplyMarkupOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
	}

	_, _ = cb.Answer(b, nil)
	return nil
}

// HandleFilterLanguageCallback filters current search by selected language.
// Format: fl#<langCode>#<searchKey>
func (p *Plugin) HandleFilterLanguageCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.Data, "#")
	if len(parts) != 3 {
		return nil
	}

	langCode := parts[1]
	searchKey := parts[2]

	sess := p.getSession(searchKey)
	if sess == nil && cb.Message != nil {
		sess = p.recoverSessionFromMessage(cb.Message, cb.From.Id)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Search expired.", ShowAlert: true})
		return nil
	}

	sess.Language = langCode

	filter := search.SearchFilter{
		Language: langCode,
		Quality:  sess.Quality,
	}

	result, err := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, 0, filter)
	if err != nil || len(result.Files) == 0 {
		// Revert to original unfiltered results matching TGxCyberBot behavior
		sess.Language = ""
		origRes, _ := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, 0, search.SearchFilter{Quality: sess.Quality})
		var origFiles []search.MediaFile
		origNext := 0
		if origRes != nil {
			origFiles = origRes.Files
			origNext = origRes.NextOffset
		}
		origKb := p.buildResultKeyboard(origFiles, searchKey, 0, origNext)
		if cb.Message != nil {
			_, _, _ = cb.Message.EditReplyMarkup(b, &gotgbot.EditMessageReplyMarkupOpts{
				ReplyMarkup: origKb,
			})
		}
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      fmt.Sprintf("🚫 Nᴏ Fɪʟᴇ Fᴏᴜɴᴅ Wɪᴛʜ Tʜɪs Lᴀɴɢᴜᴀɢᴇ (%s). Bᴀᴄᴋ ᴛᴏ ᴏʀɪɢɪɴᴀʟ sᴇᴀʀᴄʜ ʀᴇsᴜʟᴛs.", strings.ToUpper(langCode)),
			ShowAlert: true,
		})
		return nil
	}

	userMention := formatUserMention(&cb.From)
	text := formatResultText(userMention, fmt.Sprintf("%s (%s)", sess.Query, strings.ToUpper(langCode)), 1)
	kb := p.buildResultKeyboard(result.Files, searchKey, 0, result.NextOffset)

	if cb.Message != nil {
		_, _, editErr := cb.Message.EditText(b, &gotgbot.EditMessageTextOpts{
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: kb,
		})
		if editErr != nil && isMessageNotModified(editErr) {
			_, _, _ = cb.Message.EditReplyMarkup(b, &gotgbot.EditMessageReplyMarkupOpts{
				ReplyMarkup: kb,
			})
		}
	}

	_, _ = cb.Answer(b, nil)
	return nil
}

// HandleQualitiesCallback renders quality filter selection buttons.
// Format: qualities#<searchKey>
func (p *Plugin) HandleQualitiesCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb == nil {
		return nil
	}

	parts := strings.SplitN(cb.Data, "#", 2)
	if len(parts) != 2 {
		return nil
	}
	searchKey := parts[1]

	sess := p.getSession(searchKey)
	if sess == nil && cb.Message != nil {
		sess = p.recoverSessionFromMessage(cb.Message, cb.From.Id)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Search expired.", ShowAlert: true})
		return nil
	}

	if cb.From.Id != sess.UserID {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "✋ This is not your search.", ShowAlert: true})
		return nil
	}

	var rows [][]gotgbot.InlineKeyboardButton
	var currentRow []gotgbot.InlineKeyboardButton

	for i, q := range qualities {
		btn := gotgbot.InlineKeyboardButton{
			Text:         q,
			CallbackData: fmt.Sprintf("fq#%s#%s", strings.ToLower(q), searchKey),
		}
		currentRow = append(currentRow, btn)
		if (i+1)%3 == 0 || i == len(qualities)-1 {
			rows = append(rows, currentRow)
			currentRow = nil
		}
	}

	// Back button (matching TGxCyberBot)
	rows = append(rows, []gotgbot.InlineKeyboardButton{
		{Text: "↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭", CallbackData: fmt.Sprintf("next|%s|0", searchKey)},
	})

	if cb.Message != nil {
		_, _, _ = cb.Message.EditReplyMarkup(b, &gotgbot.EditMessageReplyMarkupOpts{
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
	}

	_, _ = cb.Answer(b, nil)
	return nil
}

// HandleFilterQualityCallback filters current search by selected quality.
// Format: fq#<quality>#<searchKey>
func (p *Plugin) HandleFilterQualityCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.Data, "#")
	if len(parts) != 3 {
		return nil
	}

	quality := parts[1]
	searchKey := parts[2]

	sess := p.getSession(searchKey)
	if sess == nil && cb.Message != nil {
		sess = p.recoverSessionFromMessage(cb.Message, cb.From.Id)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Search expired.", ShowAlert: true})
		return nil
	}

	sess.Quality = quality

	filter := search.SearchFilter{
		Language: sess.Language,
		Quality:  quality,
	}

	result, err := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, 0, filter)
	if err != nil || len(result.Files) == 0 {
		// Revert to original unfiltered results matching TGxCyberBot behavior
		sess.Quality = ""
		origRes, _ := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, 0, search.SearchFilter{Language: sess.Language})
		var origFiles []search.MediaFile
		origNext := 0
		if origRes != nil {
			origFiles = origRes.Files
			origNext = origRes.NextOffset
		}
		origKb := p.buildResultKeyboard(origFiles, searchKey, 0, origNext)
		if cb.Message != nil {
			_, _, _ = cb.Message.EditReplyMarkup(b, &gotgbot.EditMessageReplyMarkupOpts{
				ReplyMarkup: origKb,
			})
		}
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      fmt.Sprintf("🚫 Nᴏ Fɪʟᴇ Fᴏᴜɴᴅ Wɪᴛʜ Tʜɪs Qᴜᴀʟɪᴛʏ (%s). Bᴀᴄᴋ ᴛᴏ ᴏʀɪɢɪɴᴀʟ sᴇᴀʀᴄʜ ʀᴇsᴜʟᴛs.", strings.ToUpper(quality)),
			ShowAlert: true,
		})
		return nil
	}

	userMention := formatUserMention(&cb.From)
	text := formatResultText(userMention, fmt.Sprintf("%s (%s)", sess.Query, strings.ToUpper(quality)), 1)
	kb := p.buildResultKeyboard(result.Files, searchKey, 0, result.NextOffset)

	if cb.Message != nil {
		_, _, editErr := cb.Message.EditText(b, &gotgbot.EditMessageTextOpts{
			Text:        text,
			ParseMode:   "HTML",
			ReplyMarkup: kb,
		})
		if editErr != nil && isMessageNotModified(editErr) {
			_, _, _ = cb.Message.EditReplyMarkup(b, &gotgbot.EditMessageReplyMarkupOpts{
				ReplyMarkup: kb,
			})
		}
	}

	_, _ = cb.Answer(b, nil)
	return nil
}

// HandleCloseCallback deletes the search message.
func (p *Plugin) HandleCloseCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb != nil && cb.Message != nil {
		_, _ = cb.Message.Delete(b, nil)
		_, _ = cb.Answer(b, nil)
	}
	return nil
}

// HandlePagesCallback answers non-clickable page indicator button.
func (p *Plugin) HandlePagesCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb != nil {
		_, _ = cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "↭ ɴᴏ ᴍᴏʀᴇ ᴘᴀɢᴇꜱ ᴀᴠᴀɪʟᴀʙʟᴇ ↭",
			ShowAlert: false,
		})
	}
	return nil
}

// --- Keyboards and Formatting ---

func (p *Plugin) buildResultKeyboard(files []search.MediaFile, searchKey string, offset, nextOffset int) gotgbot.InlineKeyboardMarkup {
	var rows [][]gotgbot.InlineKeyboardButton

	// 1. Languages and Quality header row
	rows = append(rows, []gotgbot.InlineKeyboardButton{
		{Text: "Lᴀɴɢᴜᴀɢᴇs", CallbackData: fmt.Sprintf("languages#%s", searchKey)},
		{Text: "Qᴜᴀʟɪᴛʏ", CallbackData: fmt.Sprintf("qualities#%s", searchKey)},
	})

	// 2. File list rows (1 button per file)
	for _, f := range files {
		fileLabel := fmt.Sprintf("%s 🌷 %s", f.Size, f.FileName)
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{
				Text:         fileLabel,
				CallbackData: fmt.Sprintf("file#%s", f.FileID),
			},
		})
	}

	// 3. Navigation row
	var navRow []gotgbot.InlineKeyboardButton
	if offset > 0 {
		prevOffset := offset - maxFilesPerPage
		if prevOffset < 0 {
			prevOffset = 0
		}
		navRow = append(navRow, gotgbot.InlineKeyboardButton{
			Text:         "◀️ 𝗕𝗮𝗰𝗸",
			CallbackData: fmt.Sprintf("next|%s|%d", searchKey, prevOffset),
		})
	}

	if nextOffset > 0 {
		navRow = append(navRow, gotgbot.InlineKeyboardButton{
			Text:         "𝗡𝗲𝘅𝘁 ▶️",
			CallbackData: fmt.Sprintf("next|%s|%d", searchKey, nextOffset),
		})
	}

	if len(navRow) > 0 {
		rows = append(rows, navRow)
	} else {
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{Text: "↭ ɴᴏ ᴍᴏʀᴇ ᴘᴀɢᴇꜱ ᴀᴠᴀɪʟᴀʙʟᴇ ↭", CallbackData: "pages"},
		})
	}

	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (p *Plugin) buildSpellCheckKeyboard(suggestions []string, searchKey string) gotgbot.InlineKeyboardMarkup {
	var rows [][]gotgbot.InlineKeyboardButton
	var currentRow []gotgbot.InlineKeyboardButton

	for i, s := range suggestions {
		btn := gotgbot.InlineKeyboardButton{
			Text:         fmt.Sprintf("🎬 %s", s),
			CallbackData: fmt.Sprintf("sp#%s#%d", searchKey, i),
		}
		currentRow = append(currentRow, btn)

		// 2 per row unless text is long
		if len(s) > 22 || len(currentRow) == 2 || i == len(suggestions)-1 {
			rows = append(rows, currentRow)
			currentRow = nil
		}
	}

	// Close button
	rows = append(rows, []gotgbot.InlineKeyboardButton{
		{Text: "❌ Cʟᴏsᴇ", CallbackData: "close_data"},
	})

	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func formatResultText(userMention, search string, page int) string {
	return fmt.Sprintf(
		"𝗛𝗘𝗬 👋 %s 🫶\n    \n📁 <b>Query :</b> %s\n\n🗓️ <b><i>Page No ›</i></b> <code>%d</code>\n\n✍️ NOTE : ⚠️ This Message Will Be Auto Deleted Within 15 Mins ❗️\n©ᶜʸᵇᵉʳᵗᵍ 🗡️",
		userMention,
		html.EscapeString(search),
		page,
	)
}

func formatSpellCheckText(userMention, search string) string {
	return fmt.Sprintf(
		"Hᴇʏ %s 💫,\n🔎 <b>Nᴏ ᴇxᴀᴄᴛ ʀᴇsᴜʟᴛ ғᴏᴜɴᴅ ғᴏʀ:</b>\n<code>%s</code>\n\n✨ <b>Cʟᴏsᴇsᴛ ᴍᴀᴛᴄʜᴇs:</b>",
		userMention,
		html.EscapeString(search),
	)
}

func formatUserMention(user *gotgbot.User) string {
	if user == nil {
		return "User"
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name == "" {
		name = user.FirstName
	}
	if name == "" {
		name = "User"
	}
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, user.Id, html.EscapeString(name))
}

// Session helpers
func (p *Plugin) saveSession(key string, sess *searchSession) {
	p.sessMu.Lock()
	defer p.sessMu.Unlock()
	// Prune old sessions if map grows too large
	if len(p.sessions) > 5000 {
		for k := range p.sessions {
			delete(p.sessions, k)
			break
		}
	}
	p.sessions[key] = sess
}

func (p *Plugin) getSession(key string) *searchSession {
	p.sessMu.RLock()
	defer p.sessMu.RUnlock()
	return p.sessions[key]
}

func (p *Plugin) recoverSessionFromMessage(maybeMsg gotgbot.MaybeInaccessibleMessage, userID int64) *searchSession {
	if maybeMsg == nil {
		return nil
	}
	msg, ok := maybeMsg.(*gotgbot.Message)
	if !ok || msg == nil || msg.Text == "" {
		return nil
	}
	lines := strings.Split(msg.Text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Query :") {
			idx := strings.Index(line, "Query :")
			queryPart := strings.TrimSpace(line[idx+len("Query :"):])
			queryPart = strings.TrimPrefix(queryPart, "</b>")
			queryPart = strings.TrimSpace(queryPart)
			if queryPart != "" {
				return &searchSession{
					Query:     queryPart,
					UserID:    userID,
					ChatID:    msg.Chat.Id,
					UpdatedAt: time.Now(),
				}
			}
		}
	}
	return nil
}

func isMessageNotModified(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "message is not modified") || strings.Contains(msg, "message_not_modified")
}
