package autofilter

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
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
			Filter: func(m *telegram.NewMessage) bool {
				if m == nil || m.Text() == "" || m.IsChannel() || m.IsChannelPost() {
					return false
				}
				// Reject commands
				if m.IsCommand() || strings.HasPrefix(m.Text(), "/") {
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
func (p *Plugin) HandleTextMessage(m *telegram.NewMessage) error {
	if m == nil || m.IsChannel() || m.IsChannelPost() {
		return nil
	}

	user := m.Sender
	if user == nil && m.Client != nil {
		if sender, err := m.GetSender(); err == nil && sender != nil {
			user = sender
		}
	}

	var userID int64
	var userMention string
	if user != nil {
		userID = user.ID
		userMention = formatUserMention(user)
	} else {
		userID = m.SenderID()
		if userID != 0 {
			userMention = fmt.Sprintf(`<a href="tg://user?id=%d">User</a>`, userID)
		} else {
			userMention = "User"
		}
	}

	if userID == 0 {
		return nil
	}

	query := strings.TrimSpace(m.Text())
	if !search.IsValidSearchText(query) {
		return nil
	}

	if p.appCtx == nil || p.appCtx.Search == nil {
		return nil
	}

	if p.appCtx.Logger != nil {
		p.appCtx.Logger.Info("handling movie search query",
			"query", query,
			"user_id", userID,
			"chat_id", m.ChatID(),
		)
	}

	searchKey := fmt.Sprintf("%d_%d", userID, m.ID)

	// Execute search
	result, err := p.appCtx.Search.Search(context.Background(), query, maxFilesPerPage, 0, search.SearchFilter{})
	if err != nil {
		if p.appCtx.Logger != nil {
			p.appCtx.Logger.Error("search error", "query", query, "error", err)
		}
		return nil
	}

	// If results found, display interactive file list
	if len(result.Files) > 0 {
		p.saveSession(searchKey, &searchSession{
			Query:     query,
			UserID:    userID,
			ChatID:    m.ChatID(),
			UpdatedAt: time.Now(),
		})

		text := formatResultText(userMention, query, 1)
		kb := p.buildResultKeyboard(result.Files, searchKey, 0, result.NextOffset)

		replyMsg, replyErr := m.Reply(text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: kb,
		})
		if replyErr != nil {
			if isIgnorableChatError(replyErr) {
				if p.appCtx.Logger != nil {
					p.appCtx.Logger.Debug("cannot send search reply in chat without permissions", "chat_id", m.ChatID(), "error", replyErr)
				}
				return nil
			}
			if p.appCtx.Logger != nil {
				p.appCtx.Logger.Error("failed to send search results reply", "query", query, "error", replyErr)
			}
			return replyErr
		} else if p.appCtx.Logger != nil {
			p.appCtx.Logger.Info("search results reply sent", "query", query, "results_count", len(result.Files))
		}
		if replyMsg != nil && p.appCtx != nil && p.appCtx.AutoDelete != nil {
			_ = p.appCtx.AutoDelete.ScheduleFilterDelete(replyMsg.ChatID(), replyMsg.ID)
		}
		return nil
	}

	// If no results, try to fetch spelling suggestions
	suggestions, sugErr := p.appCtx.Search.GetSuggestions(context.Background(), query, 6)
	if sugErr == nil && len(suggestions) > 0 {
		p.saveSession(searchKey, &searchSession{
			Query:       query,
			UserID:      userID,
			ChatID:      m.ChatID(),
			Suggestions: suggestions,
			UpdatedAt:   time.Now(),
		})

		text := formatSpellCheckText(userMention, query)
		kb := p.buildSpellCheckKeyboard(suggestions, searchKey)

		replyMsg, replyErr := m.Reply(text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: kb,
		})
		if replyErr != nil {
			if isIgnorableChatError(replyErr) {
				if p.appCtx.Logger != nil {
					p.appCtx.Logger.Debug("cannot send suggestions in chat without permissions", "chat_id", m.ChatID(), "error", replyErr)
				}
				return nil
			}
			if p.appCtx.Logger != nil {
				p.appCtx.Logger.Error("failed to send suggestions reply", "query", query, "error", replyErr)
			}
			return replyErr
		} else if p.appCtx.Logger != nil {
			p.appCtx.Logger.Info("spelling suggestions reply sent", "query", query, "suggestions_count", len(suggestions))
		}
		if replyMsg != nil && p.appCtx != nil && p.appCtx.AutoDelete != nil {
			_ = p.appCtx.AutoDelete.ScheduleFilterDelete(replyMsg.ChatID(), replyMsg.ID)
		}
		return nil
	}

	// No results and no suggestions found
	notFoundText := fmt.Sprintf(
		"Hᴇʏ %s 💫,\nNᴏ Sᴇᴀʀᴄʜ Rᴇꜱᴜʟᴛ Fᴏᴜɴᴅ Fᴏʀ Yᴏᴜʀ Rᴇᴏ̨ᴜᴇsᴛᴇᴅ Mᴏᴠɪᴇ.\n\n➪ Sᴇɴᴅ Mᴇ A Cᴏʀʀᴇᴄᴛ Sᴘᴇʟʟɪɴɢ. Oʀ Pʟᴇᴀꜱᴇ Cʜᴇᴄᴋ Tʜᴀᴛ Mᴏᴠɪᴇ Oᴛᴛ Rᴇʟᴇᴇsᴇᴅ.✨",
		userMention,
	)
	replyMsg, replyErr := m.Reply(notFoundText, &telegram.SendOptions{
		ParseMode: "html",
	})
	if replyErr != nil {
		if isIgnorableChatError(replyErr) {
			if p.appCtx.Logger != nil {
				p.appCtx.Logger.Debug("cannot send not-found reply in chat without permissions", "chat_id", m.ChatID(), "error", replyErr)
			}
			return nil
		}
		if p.appCtx.Logger != nil {
			p.appCtx.Logger.Error("failed to send not-found reply", "query", query, "error", replyErr)
		}
		return replyErr
	} else if p.appCtx.Logger != nil {
		p.appCtx.Logger.Info("not-found reply sent", "query", query)
	}
	if replyMsg != nil && p.appCtx != nil && p.appCtx.AutoDelete != nil {
		_ = p.appCtx.AutoDelete.ScheduleFilterDelete(replyMsg.ChatID(), replyMsg.ID)
	}
	return nil
}

func isIgnorableChatError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "CHAT_ADMIN_REQUIRED") ||
		strings.Contains(msg, "CHAT_WRITE_FORBIDDEN") ||
		strings.Contains(msg, "USER_BANNED_IN_CHANNEL") ||
		strings.Contains(msg, "INPUT_USER_DEACTIVATED") ||
		strings.Contains(msg, "BOT_METHOD_INVALID") ||
		strings.Contains(msg, "PEER_ID_INVALID")
}

// HandlePaginationCallback processes next/back navigation buttons.
// Format: next|<searchKey>|<offset>
func (p *Plugin) HandlePaginationCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.DataString(), "|")
	if len(parts) != 3 {
		_, _ = cb.Answer("Invalid pagination data", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	searchKey := parts[1]
	offset, err := strconv.Atoi(parts[2])
	if err != nil {
		_, _ = cb.Answer("Invalid offset", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	msg, _ := cb.GetMessage()
	sess := p.getSession(searchKey)
	if sess == nil && msg != nil {
		sess = p.recoverSessionFromMessage(msg, cb.SenderID)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer("🔁 Sᴇᴀʀᴄʜ ᴇxᴘɪʀᴇᴅ ᴏʀ ɴᴏᴛ ғᴏᴜɴᴅ. Pʟᴇᴀsᴇ sᴇᴀʀᴄʜ ᴀɢᴀɪɴ 😇", &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	// Check user authorization
	if cb.SenderID != sess.UserID {
		_, _ = cb.Answer("✋ Tʜɪs ɪs ɴᴏᴛ ғᴏʀ ʏᴏᴜ! Pʟᴇᴀsᴇ ᴍᴀᴋᴇ ʏᴏᴜʀ ᴏᴡɴ sᴇᴀʀᴄʜ.", &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	filter := search.SearchFilter{
		Language: sess.Language,
		Quality:  sess.Quality,
	}

	result, err := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, offset, filter)
	if err != nil || len(result.Files) == 0 {
		_, _ = cb.Answer("🚫 Nᴏ ᴍᴏʀᴇ ʀᴇsᴜʟᴛs ғᴏᴜɴᴅ 🚫", &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	userMention := formatUserMention(cb.Sender)
	currentPage := (offset / maxFilesPerPage) + 1
	text := formatResultText(userMention, sess.Query, currentPage)
	kb := p.buildResultKeyboard(result.Files, searchKey, offset, result.NextOffset)

	_, editErr := cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: kb,
	})
	if editErr != nil && isMessageNotModified(editErr) {
		_, _ = cb.Edit(text, &telegram.SendOptions{
			ReplyMarkup: kb,
		})
	}

	_, _ = cb.Answer("")
	return nil
}

// HandleSpellCheckCallback processes clicks on spelling suggestions.
// Format: sp#<searchKey>#<index>
func (p *Plugin) HandleSpellCheckCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.DataString(), "#")
	if len(parts) != 3 {
		_, _ = cb.Answer("Invalid suggestion data", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	searchKey := parts[1]
	idx, err := strconv.Atoi(parts[2])
	if err != nil {
		_, _ = cb.Answer("Invalid index", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	sess := p.getSession(searchKey)
	if sess == nil || idx >= len(sess.Suggestions) {
		_, _ = cb.Answer("Sᴇᴀʀᴄʜ ᴇxᴘɪʀᴇᴅ ᴏʀ ɴᴏᴛ ғᴏᴜɴᴅ. Pʟᴇᴀsᴇ sᴇᴀʀᴄʜ ᴀɢᴀɪɴ. 🔰", &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	if cb.SenderID != sess.UserID {
		_, _ = cb.Answer("✋ Tʜɪs ɪs ɴᴏᴛ ғᴏʀ ʏᴏᴜ! Pʟᴇᴀsᴇ ᴍᴀᴋᴇ ʏᴏᴜʀ ᴏᴡɴ sᴇᴀʀᴄʜ.", &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	selectedQuery := sess.Suggestions[idx]
	sess.Query = selectedQuery
	sess.Suggestions = nil

	result, err := p.appCtx.Search.Search(context.Background(), selectedQuery, maxFilesPerPage, 0, search.SearchFilter{})
	if err != nil || len(result.Files) == 0 {
		_, _ = cb.Answer("🚫 Nᴏ ғɪʟᴇs ғᴏᴜɴᴅ.", &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	userMention := formatUserMention(cb.Sender)
	text := formatResultText(userMention, selectedQuery, 1)
	kb := p.buildResultKeyboard(result.Files, searchKey, 0, result.NextOffset)

	_, editErr := cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: kb,
	})
	if editErr != nil && isMessageNotModified(editErr) {
		_, _ = cb.Edit(text, &telegram.SendOptions{
			ReplyMarkup: kb,
		})
	}

	_, _ = cb.Answer("")
	return nil
}

// HandleLanguagesCallback renders language filter selection buttons.
// Format: languages#<searchKey>
func (p *Plugin) HandleLanguagesCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	parts := strings.SplitN(cb.DataString(), "#", 2)
	if len(parts) != 2 {
		return nil
	}
	searchKey := parts[1]

	msg, _ := cb.GetMessage()
	sess := p.getSession(searchKey)
	if sess == nil && msg != nil {
		sess = p.recoverSessionFromMessage(msg, cb.SenderID)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer("Search expired.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	if cb.SenderID != sess.UserID {
		_, _ = cb.Answer("✋ This is not your search.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	kb := telegram.NewKeyboard()
	var currentRow []telegram.KeyboardButton

	for i, lang := range languages {
		btn := telegram.Button.Data(lang.Label, fmt.Sprintf("fl#%s#%s", lang.Code, searchKey))
		currentRow = append(currentRow, btn)
		if (i+1)%2 == 0 || i == len(languages)-1 {
			kb.AddRow(currentRow...)
			currentRow = nil
		}
	}

	// Back to results button (matching TGxCyberBot)
	kb.AddRow(telegram.Button.Data("↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭", fmt.Sprintf("next|%s|0", searchKey)))
	markup := kb.Build()

	currentText := ""
	if msg != nil {
		currentText = msg.Text()
	}
	_, _ = cb.Edit(currentText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})

	_, _ = cb.Answer("")
	return nil
}

// HandleFilterLanguageCallback filters current search by selected language.
// Format: fl#<langCode>#<searchKey>
func (p *Plugin) HandleFilterLanguageCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.DataString(), "#")
	if len(parts) != 3 {
		return nil
	}

	langCode := parts[1]
	searchKey := parts[2]

	msg, _ := cb.GetMessage()
	sess := p.getSession(searchKey)
	if sess == nil && msg != nil {
		sess = p.recoverSessionFromMessage(msg, cb.SenderID)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer("Search expired.", &telegram.CallbackOptions{Alert: true})
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
		currentText := ""
		if msg != nil {
			currentText = msg.Text()
		}
		_, _ = cb.Edit(currentText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: origKb,
		})
		_, _ = cb.Answer(fmt.Sprintf("🚫 Nᴏ Fɪʟᴇ Fᴏᴜɴᴅ Wɪᴛʜ Tʜɪs Lᴀɴɢᴜᴀɢᴇ (%s). Bᴀᴄᴋ ᴛᴏ ᴏʀɪɢɪɴᴀʟ sᴇᴀʀᴄʜ ʀᴇsᴜʟᴛs.", strings.ToUpper(langCode)), &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	userMention := formatUserMention(cb.Sender)
	text := formatResultText(userMention, fmt.Sprintf("%s (%s)", sess.Query, strings.ToUpper(langCode)), 1)
	kb := p.buildResultKeyboard(result.Files, searchKey, 0, result.NextOffset)

	_, editErr := cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: kb,
	})
	if editErr != nil && isMessageNotModified(editErr) {
		_, _ = cb.Edit(text, &telegram.SendOptions{
			ReplyMarkup: kb,
		})
	}

	_, _ = cb.Answer("")
	return nil
}

// HandleQualitiesCallback renders quality filter selection buttons.
// Format: qualities#<searchKey>
func (p *Plugin) HandleQualitiesCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	parts := strings.SplitN(cb.DataString(), "#", 2)
	if len(parts) != 2 {
		return nil
	}
	searchKey := parts[1]

	msg, _ := cb.GetMessage()
	sess := p.getSession(searchKey)
	if sess == nil && msg != nil {
		sess = p.recoverSessionFromMessage(msg, cb.SenderID)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer("Search expired.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	if cb.SenderID != sess.UserID {
		_, _ = cb.Answer("✋ This is not your search.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	kb := telegram.NewKeyboard()
	var currentRow []telegram.KeyboardButton

	for i, q := range qualities {
		btn := telegram.Button.Data(q, fmt.Sprintf("fq#%s#%s", strings.ToLower(q), searchKey))
		currentRow = append(currentRow, btn)
		if (i+1)%3 == 0 || i == len(qualities)-1 {
			kb.AddRow(currentRow...)
			currentRow = nil
		}
	}

	// Back button (matching TGxCyberBot)
	kb.AddRow(telegram.Button.Data("↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭", fmt.Sprintf("next|%s|0", searchKey)))
	markup := kb.Build()

	currentText := ""
	if msg != nil {
		currentText = msg.Text()
	}
	_, _ = cb.Edit(currentText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})

	_, _ = cb.Answer("")
	return nil
}

// HandleFilterQualityCallback filters current search by selected quality.
// Format: fq#<quality>#<searchKey>
func (p *Plugin) HandleFilterQualityCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.DataString(), "#")
	if len(parts) != 3 {
		return nil
	}

	quality := parts[1]
	searchKey := parts[2]

	msg, _ := cb.GetMessage()
	sess := p.getSession(searchKey)
	if sess == nil && msg != nil {
		sess = p.recoverSessionFromMessage(msg, cb.SenderID)
		if sess != nil {
			p.saveSession(searchKey, sess)
		}
	}

	if sess == nil {
		_, _ = cb.Answer("Search expired.", &telegram.CallbackOptions{Alert: true})
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
		currentText := ""
		if msg != nil {
			currentText = msg.Text()
		}
		_, _ = cb.Edit(currentText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: origKb,
		})
		_, _ = cb.Answer(fmt.Sprintf("🚫 Nᴏ Fɪʟᴇ Fᴏᴜɴᴅ Wɪᴛʜ Tʜɪs Qᴜᴀʟɪᴛʏ (%s). Bᴀᴄᴋ ᴛᴏ ᴏʀɪɢɪɴᴀʟ sᴇᴀʀᴄʜ ʀᴇsᴜʟᴛs.", strings.ToUpper(quality)), &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	userMention := formatUserMention(cb.Sender)
	text := formatResultText(userMention, fmt.Sprintf("%s (%s)", sess.Query, strings.ToUpper(quality)), 1)
	kb := p.buildResultKeyboard(result.Files, searchKey, 0, result.NextOffset)

	_, editErr := cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: kb,
	})
	if editErr != nil && isMessageNotModified(editErr) {
		_, _ = cb.Edit(text, &telegram.SendOptions{
			ReplyMarkup: kb,
		})
	}

	_, _ = cb.Answer("")
	return nil
}

// HandleCloseCallback deletes the search message.
func (p *Plugin) HandleCloseCallback(cb *telegram.CallbackQuery) error {
	if cb != nil {
		_, _ = cb.Delete()
		_, _ = cb.Answer("")
	}
	return nil
}

// HandlePagesCallback answers non-clickable page indicator button.
func (p *Plugin) HandlePagesCallback(cb *telegram.CallbackQuery) error {
	if cb != nil {
		_, _ = cb.Answer("↭ ɴᴏ ᴍᴏʀᴇ ᴘᴀɢᴇꜱ ᴀᴠᴀɪʟᴀʙʟᴇ ↭", &telegram.CallbackOptions{
			Alert: false,
		})
	}
	return nil
}

// --- Keyboards and Formatting ---

func (p *Plugin) buildResultKeyboard(files []search.MediaFile, searchKey string, offset, nextOffset int) *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()

	// 1. Languages and Quality header row
	kb.AddRow(
		telegram.Button.Data("Lᴀɴɢᴜᴀɢᴇs", fmt.Sprintf("languages#%s", searchKey)),
		telegram.Button.Data("Qᴜᴀʟɪᴛʏ", fmt.Sprintf("qualities#%s", searchKey)),
	)

	// 2. File list rows (1 button per file)
	for _, f := range files {
		fileLabel := fmt.Sprintf("%s 🌷 %s", f.Size, f.FileName)
		kb.AddRow(telegram.Button.Data(fileLabel, fmt.Sprintf("file#%s", f.FileID)))
	}

	// 3. Navigation row
	var navButtons []telegram.KeyboardButton
	if offset > 0 {
		prevOffset := offset - maxFilesPerPage
		if prevOffset < 0 {
			prevOffset = 0
		}
		navButtons = append(navButtons, telegram.Button.Data("◀️ 𝗕𝗮𝗰𝗸", fmt.Sprintf("next|%s|%d", searchKey, prevOffset)))
	}

	if nextOffset > 0 {
		navButtons = append(navButtons, telegram.Button.Data("𝗡𝗲𝘅𝘁 ▶️", fmt.Sprintf("next|%s|%d", searchKey, nextOffset)))
	}

	if len(navButtons) > 0 {
		kb.AddRow(navButtons...)
	} else {
		kb.AddRow(telegram.Button.Data("↭ ɴᴏ ᴍᴏʀᴇ ᴘᴀɢᴇꜱ ᴀᴠᴀɪʟᴀʙʟᴇ ↭", "pages"))
	}

	return kb.Build()
}

func (p *Plugin) buildSpellCheckKeyboard(suggestions []string, searchKey string) *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()

	var currentRow []telegram.KeyboardButton
	for i, s := range suggestions {
		btn := telegram.Button.Data(fmt.Sprintf("🎬 %s", s), fmt.Sprintf("sp#%s#%d", searchKey, i))
		currentRow = append(currentRow, btn)

		// 2 per row unless text is long
		if len(s) > 22 || len(currentRow) == 2 || i == len(suggestions)-1 {
			kb.AddRow(currentRow...)
			currentRow = nil
		}
	}

	// Close button
	kb.AddRow(telegram.Button.Data("❌ Cʟᴏsᴇ", "close_data"))

	return kb.Build()
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

func formatUserMention(user *telegram.UserObj) string {
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
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, user.ID, html.EscapeString(name))
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

func (p *Plugin) recoverSessionFromMessage(msg *telegram.NewMessage, userID int64) *searchSession {
	if msg == nil || msg.Text() == "" {
		return nil
	}
	chatID := msg.ChatID()
	if chatID == 0 {
		if msg.Chat != nil && msg.Chat.ID != 0 {
			chatID = msg.Chat.ID
		} else if msg.Channel != nil && msg.Channel.ID != 0 {
			chatID = msg.Channel.ID
		}
	}
	lines := strings.Split(msg.Text(), "\n")
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
					ChatID:    chatID,
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
