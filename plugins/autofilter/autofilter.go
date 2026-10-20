package autofilter

import (
	"context"
	"fmt"
	"html"
	"regexp"
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
	"480P", "720P", "1080P", "2160P", "4K",
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
		{Prefix: "back_all#", Handler: p.HandleBackToAllFilesCallback},
		{Prefix: "back#", Handler: p.HandleBackToFilesCallback},
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

	searchKey := fmt.Sprintf("%d_%d_%d", m.ChatID(), userID, m.ID)

	startSearch := time.Now()

	// Execute search
	result, err := p.appCtx.Search.Search(context.Background(), query, maxFilesPerPage, 0, search.SearchFilter{})
	searchDuration := time.Since(startSearch)
	if err != nil {
		if p.appCtx.Logger != nil {
			p.appCtx.Logger.Error("search error", "query", query, "error", err, "search_ms", searchDuration.Milliseconds())
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

		botName := p.getBotName(m.Client)
		text := p.formatResultText(query, result.Page, result.TotalPages, result.Total, botName)
		kb := p.buildResultKeyboard(result.Files, searchKey, result.Page, result.TotalPages, false)

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
			p.appCtx.Logger.Info("search results reply sent",
				"query", query,
				"results_count", len(result.Files),
				"search_ms", searchDuration.Milliseconds(),
				"total_ms", time.Since(startSearch).Milliseconds(),
			)
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
			p.appCtx.Logger.Info("spelling suggestions reply sent",
				"query", query,
				"suggestions_count", len(suggestions),
				"search_ms", searchDuration.Milliseconds(),
				"total_ms", time.Since(startSearch).Milliseconds(),
			)
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
		p.appCtx.Logger.Info("not-found reply sent",
			"query", query,
			"search_ms", searchDuration.Milliseconds(),
			"total_ms", time.Since(startSearch).Milliseconds(),
		)
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

	botName := p.getBotName(cb.Client)
	displayQuery := p.getDisplayQuery(sess)
	text := p.formatResultText(displayQuery, result.Page, result.TotalPages, result.Total, botName)
	hasFilter := sess.Language != "" || sess.Quality != ""
	kb := p.buildResultKeyboard(result.Files, searchKey, result.Page, result.TotalPages, hasFilter)

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

	botName := p.getBotName(cb.Client)
	text := p.formatResultText(selectedQuery, result.Page, result.TotalPages, result.Total, botName)
	kb := p.buildResultKeyboard(result.Files, searchKey, result.Page, result.TotalPages, false)

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

	// Back to results button
	kb.AddRow(telegram.Button.Data("↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭", fmt.Sprintf("back#%s", searchKey)))
	markup := kb.Build()

	botName := p.getBotName(cb.Client)
	menuText := formatLanguagesMenuText(p.getDisplayQuery(sess), botName)
	_, editErr := cb.Edit(menuText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	if editErr != nil && isMessageNotModified(editErr) {
		_, _ = cb.Edit(menuText, &telegram.SendOptions{
			ReplyMarkup: markup,
		})
	}

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

	if cb.SenderID != sess.UserID {
		_, _ = cb.Answer("✋ This is not your search.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	filter := search.SearchFilter{
		Language: langCode,
		Quality:  sess.Quality,
	}

	result, err := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, 0, filter)
	if err != nil || len(result.Files) == 0 {
		_, _ = cb.Answer(fmt.Sprintf("🚫 Nᴏ Fɪʟᴇ Fᴏᴜɴᴅ Wɪᴛʜ Tʜɪs Lᴀɴɢᴜᴀɢᴇ (%s) 🚫", strings.ToUpper(langCode)), &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	sess.Language = langCode
	sess.UpdatedAt = time.Now()
	p.saveSession(searchKey, sess)

	botName := p.getBotName(cb.Client)
	displayQuery := p.getDisplayQuery(sess)
	text := p.formatResultText(displayQuery, result.Page, result.TotalPages, result.Total, botName)
	kb := p.buildResultKeyboard(result.Files, searchKey, result.Page, result.TotalPages, true)

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

	// Back button
	kb.AddRow(telegram.Button.Data("↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭", fmt.Sprintf("back#%s", searchKey)))
	markup := kb.Build()

	botName := p.getBotName(cb.Client)
	menuText := formatQualitiesMenuText(p.getDisplayQuery(sess), botName)
	_, editErr := cb.Edit(menuText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	if editErr != nil && isMessageNotModified(editErr) {
		_, _ = cb.Edit(menuText, &telegram.SendOptions{
			ReplyMarkup: markup,
		})
	}

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

	if cb.SenderID != sess.UserID {
		_, _ = cb.Answer("✋ This is not your search.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	filter := search.SearchFilter{
		Language: sess.Language,
		Quality:  quality,
	}

	result, err := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, 0, filter)
	if err != nil || len(result.Files) == 0 {
		_, _ = cb.Answer(fmt.Sprintf("🚫 Nᴏ Fɪʟᴇ Fᴏᴜɴᴅ Wɪᴛʜ Tʜɪs Qᴜᴀʟɪᴛʏ (%s) 🚫", strings.ToUpper(quality)), &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	sess.Quality = quality
	sess.UpdatedAt = time.Now()
	p.saveSession(searchKey, sess)

	botName := p.getBotName(cb.Client)
	displayQuery := p.getDisplayQuery(sess)
	text := p.formatResultText(displayQuery, result.Page, result.TotalPages, result.Total, botName)
	kb := p.buildResultKeyboard(result.Files, searchKey, result.Page, result.TotalPages, true)

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

// HandleBackToFilesCallback returns from language/quality menu back to the active file list.
// Format: back#<searchKey>
func (p *Plugin) HandleBackToFilesCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.DataString(), "#")
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

	filter := search.SearchFilter{
		Language: sess.Language,
		Quality:  sess.Quality,
	}

	result, err := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, 0, filter)
	if err != nil || len(result.Files) == 0 {
		sess.Language = ""
		sess.Quality = ""
		p.saveSession(searchKey, sess)
		result, err = p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, 0, search.SearchFilter{})
	}

	if err != nil || len(result.Files) == 0 {
		_, _ = cb.Answer("🚫 No files found.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	botName := p.getBotName(cb.Client)
	displayQuery := p.getDisplayQuery(sess)
	hasFilter := sess.Language != "" || sess.Quality != ""
	text := p.formatResultText(displayQuery, result.Page, result.TotalPages, result.Total, botName)
	kb := p.buildResultKeyboard(result.Files, searchKey, result.Page, result.TotalPages, hasFilter)

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

// HandleBackToAllFilesCallback resets all language/quality filters and returns to unfiltered results.
// Format: back_all#<searchKey>
func (p *Plugin) HandleBackToAllFilesCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.DataString(), "#")
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

	sess.Language = ""
	sess.Quality = ""
	sess.UpdatedAt = time.Now()
	p.saveSession(searchKey, sess)

	result, err := p.appCtx.Search.Search(context.Background(), sess.Query, maxFilesPerPage, 0, search.SearchFilter{})
	if err != nil || len(result.Files) == 0 {
		_, _ = cb.Answer("🚫 No files found.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	botName := p.getBotName(cb.Client)
	text := p.formatResultText(sess.Query, result.Page, result.TotalPages, result.Total, botName)
	kb := p.buildResultKeyboard(result.Files, searchKey, result.Page, result.TotalPages, false)

	_, editErr := cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: kb,
	})
	if editErr != nil && isMessageNotModified(editErr) {
		_, _ = cb.Edit(text, &telegram.SendOptions{
			ReplyMarkup: kb,
		})
	}

	_, _ = cb.Answer("🔄 Showing all files")
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
		_, _ = cb.Answer("📄 You are viewing search results", &telegram.CallbackOptions{
			Alert: false,
		})
	}
	return nil
}

// --- Keyboards and Formatting ---

var (
	reResolution   = regexp.MustCompile(`(?i)\b(2160p|4k|1080p|720p|480p|360p)\b`)
	reRip          = regexp.MustCompile(`(?i)\b(hdrip|webrip|web-dl|webdl|bluray|brrip|dvdrip|hdtc|camrip|hd)\b`)
	reLangMatch    = regexp.MustCompile(`(?i)\b(malayalam|tamil|hindi|telugu|kannada|english|multi|dual[\s._-]*audio|bengali|marathi|punjabi|gujarati|mal|tam|hin|tel|kan|eng)\b`)
	reFileExtStrip = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|webm|flv|mov|m4v|3gp|zip|rar)$`)
	reBrackets     = regexp.MustCompile(`\[[^\]]*\]|\([^\)]*\)|\{[^\}]*\}`)
	reCleanChars   = regexp.MustCompile(`[._]+`)
	reYearFind     = regexp.MustCompile(`\b(19\d\d|20\d\d)\b`)
)

func canonicalLanguage(raw string) string {
	rawLower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(rawLower, "mal"):
		return "Malayalam"
	case strings.HasPrefix(rawLower, "tam"):
		return "Tamil"
	case strings.HasPrefix(rawLower, "hin"):
		return "Hindi"
	case strings.HasPrefix(rawLower, "tel"):
		return "Telugu"
	case strings.HasPrefix(rawLower, "kan"):
		return "Kannada"
	case strings.HasPrefix(rawLower, "eng"):
		return "English"
	case strings.Contains(rawLower, "multi") || strings.Contains(rawLower, "dual"):
		return "Multi"
	case strings.HasPrefix(rawLower, "ben"):
		return "Bengali"
	case strings.HasPrefix(rawLower, "mar"):
		return "Marathi"
	case strings.HasPrefix(rawLower, "pun"):
		return "Punjabi"
	case strings.HasPrefix(rawLower, "guj"):
		return "Gujarati"
	default:
		if len(raw) > 0 {
			return strings.ToUpper(raw[:1]) + strings.ToLower(raw[1:])
		}
		return raw
	}
}

func canonicalQuality(raw string) string {
	rawLower := strings.ToLower(strings.TrimSpace(raw))
	switch rawLower {
	case "4k":
		return "4K"
	case "2160p":
		return "2160p"
	case "1080p":
		return "1080p"
	case "720p":
		return "720p"
	case "480p":
		return "480p"
	case "360p":
		return "360p"
	case "hdrip":
		return "HDRip"
	case "webrip":
		return "WEBRip"
	case "web-dl", "webdl":
		return "WEB-DL"
	case "bluray":
		return "BluRay"
	case "brrip":
		return "BRRip"
	case "dvdrip":
		return "DVDRip"
	case "hdtc":
		return "HDTC"
	case "camrip":
		return "CAM"
	case "hd":
		return "HD"
	default:
		return strings.ToUpper(raw)
	}
}

func extractFileMetadata(fileName, caption string) (lang string, qual string, cleanTitle string) {
	text := fileName + " " + caption

	// 1. Language
	if match := reLangMatch.FindString(text); match != "" {
		lang = canonicalLanguage(match)
	}

	// 2. Quality
	res := reResolution.FindString(text)
	rip := reRip.FindString(text)
	if res != "" && rip != "" {
		qual = canonicalQuality(res) + " " + canonicalQuality(rip)
	} else if res != "" {
		qual = canonicalQuality(res)
	} else if rip != "" {
		qual = canonicalQuality(rip)
	}

	// 3. Clean Title
	clean := reFileExtStrip.ReplaceAllString(fileName, "")
	clean = reBrackets.ReplaceAllString(clean, " ")
	if loc := reYearFind.FindStringIndex(clean); loc != nil {
		clean = clean[:loc[0]]
	} else if loc := reResolution.FindStringIndex(clean); loc != nil {
		clean = clean[:loc[0]]
	} else if loc := reRip.FindStringIndex(clean); loc != nil {
		clean = clean[:loc[0]]
	} else if loc := reLangMatch.FindStringIndex(clean); loc != nil {
		clean = clean[:loc[0]]
	}
	clean = reCleanChars.ReplaceAllString(clean, " ")
	clean = strings.TrimSpace(clean)
	cleanTitle = clean

	return lang, qual, cleanTitle
}

func formatFileButtonLabel(f search.MediaFile) string {
	size := strings.TrimSpace(f.Size)
	if size == "" && f.FileSize > 0 {
		size = formatBytes(f.FileSize)
	}
	if size == "" {
		size = "N/A"
	}

	lang, qual, cleanTitle := extractFileMetadata(f.FileName, f.Caption)

	var parts []string
	parts = append(parts, "🎬 "+size)

	if lang != "" && qual != "" {
		parts = append(parts, lang, qual)
	} else if lang != "" {
		parts = append(parts, lang)
		if cleanTitle != "" && len(cleanTitle) <= 15 {
			parts = append(parts, cleanTitle)
		}
	} else if qual != "" {
		if cleanTitle != "" && len(cleanTitle) <= 15 {
			parts = append(parts, cleanTitle)
		}
		parts = append(parts, qual)
	} else if cleanTitle != "" {
		parts = append(parts, cleanTitle)
	} else {
		name := strings.TrimSpace(f.FileName)
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		parts = append(parts, name)
	}

	label := strings.Join(parts, " · ")
	if len(label) > 60 {
		label = label[:57] + "..."
	}
	return label
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

func (p *Plugin) buildResultKeyboard(files []search.MediaFile, searchKey string, currentPage, totalPages int, hasFilter bool) *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()

	// 1. Languages and Quality header row
	kb.AddRow(
		telegram.Button.Data("🌐 Languages", fmt.Sprintf("languages#%s", searchKey)),
		telegram.Button.Data("🎞 Quality", fmt.Sprintf("qualities#%s", searchKey)),
	)

	// 2. File list rows (1 button per file)
	for _, f := range files {
		fileLabel := formatFileButtonLabel(f)
		kb.AddRow(telegram.Button.Data(fileLabel, fmt.Sprintf("file#%s", f.FileID)))
	}

	// 3. Navigation row
	if totalPages < 1 {
		totalPages = 1
	}
	if currentPage < 1 {
		currentPage = 1
	}

	var navButtons []telegram.KeyboardButton

	// Previous button: only when currentPage > 1
	if currentPage > 1 {
		prevOffset := (currentPage - 2) * maxFilesPerPage
		if prevOffset < 0 {
			prevOffset = 0
		}
		navButtons = append(navButtons, telegram.Button.Data("◀ Previous", fmt.Sprintf("next|%s|%d", searchKey, prevOffset)))
	}

	// Page indicator button: always present
	pageLabel := fmt.Sprintf("📄 Page %d / %d", currentPage, totalPages)
	navButtons = append(navButtons, telegram.Button.Data(pageLabel, "pages"))

	// Next button: only when currentPage < totalPages
	if currentPage < totalPages {
		nextOffset := currentPage * maxFilesPerPage
		navButtons = append(navButtons, telegram.Button.Data("Next ▶", fmt.Sprintf("next|%s|%d", searchKey, nextOffset)))
	}

	if len(navButtons) > 0 {
		kb.AddRow(navButtons...)
	}

	// 4. Filter reset (if active filter)
	if hasFilter {
		kb.AddRow(telegram.Button.Data("↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭", fmt.Sprintf("back_all#%s", searchKey)))
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

func (p *Plugin) getBotName(client *telegram.Client) string {
	if p.appCtx != nil && p.appCtx.BotUser != nil {
		if name := strings.TrimSpace(p.appCtx.BotUser.FirstName); name != "" {
			return name
		}
		if username := strings.TrimSpace(p.appCtx.BotUser.Username); username != "" {
			return "@" + username
		}
	}
	if client != nil {
		if me, err := client.GetMe(); err == nil && me != nil {
			if name := strings.TrimSpace(me.FirstName); name != "" {
				return name
			}
			if username := strings.TrimSpace(me.Username); username != "" {
				return "@" + username
			}
		}
	}
	return "Bot"
}

func (p *Plugin) formatResultText(query string, page, totalPages int, totalResults int64, botName string) string {
	return formatResultText(query, page, totalPages, totalResults, botName)
}

func formatResultText(query string, page, totalPages int, totalResults int64, botName string) string {
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}

	resultsStr := "files"
	if totalResults == 1 {
		resultsStr = "file"
	}

	title := strings.TrimSpace(query)
	if openIdx := strings.LastIndex(title, "("); openIdx != -1 && strings.HasSuffix(title, ")") {
		title = strings.TrimSpace(title[:openIdx])
	}
	if title == "" {
		title = strings.TrimSpace(query)
	}

	return fmt.Sprintf(
		"🎬 <b>%s</b>\n\n"+
			"🔎 <b>Query:</b> %s\n"+
			"📄 <b>Page:</b> %d / %d\n"+
			"📦 <b>Results:</b> %d %s\n"+
			"⏳ <i>This result expires in 15 minutes.</i>\n\n"+
			"© %s",
		html.EscapeString(title),
		html.EscapeString(query),
		page,
		totalPages,
		totalResults,
		resultsStr,
		html.EscapeString(botName),
	)
}

func formatLanguagesMenuText(query string, botName string) string {
	return fmt.Sprintf(
		"🌐 <b>Select Desired Language</b>\n\n"+
			"🔎 <b>Query:</b> %s\n\n"+
			"⏳ <i>This message expires in 15 minutes.</i>\n\n"+
			"© %s",
		html.EscapeString(query),
		html.EscapeString(botName),
	)
}

func formatQualitiesMenuText(query string, botName string) string {
	return fmt.Sprintf(
		"🎞 <b>Select Desired Quality</b>\n\n"+
			"🔎 <b>Query:</b> %s\n\n"+
			"⏳ <i>This message expires in 15 minutes.</i>\n\n"+
			"© %s",
		html.EscapeString(query),
		html.EscapeString(botName),
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

func (p *Plugin) getDisplayQuery(sess *searchSession) string {
	if sess == nil {
		return ""
	}
	if sess.Language != "" && sess.Quality != "" {
		return fmt.Sprintf("%s (%s - %s)", sess.Query, strings.ToUpper(sess.Language), strings.ToUpper(sess.Quality))
	} else if sess.Language != "" {
		return fmt.Sprintf("%s (%s)", sess.Query, strings.ToUpper(sess.Language))
	} else if sess.Quality != "" {
		return fmt.Sprintf("%s (%s)", sess.Query, strings.ToUpper(sess.Quality))
	}
	return sess.Query
}

func isKnownQuality(q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	switch q {
	case "480p", "720p", "1080p", "2160p", "4k":
		return true
	}
	return false
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
		var queryPart string
		if strings.Contains(line, "Query:") {
			idx := strings.Index(line, "Query:")
			queryPart = strings.TrimSpace(line[idx+len("Query:"):])
		} else if strings.Contains(line, "Query :") {
			idx := strings.Index(line, "Query :")
			queryPart = strings.TrimSpace(line[idx+len("Query :"):])
		}
		if queryPart != "" {
			queryPart = strings.TrimPrefix(queryPart, "</b>")
			queryPart = strings.TrimSpace(queryPart)
			if queryPart == "" {
				continue
			}

			// Check for filter tags like (MALAYALAM), (1080P), or (MALAYALAM - 1080P)
			var lang, qual string
			if strings.HasSuffix(queryPart, ")") && strings.Contains(queryPart, "(") {
				openIdx := strings.LastIndex(queryPart, "(")
				tag := queryPart[openIdx+1 : len(queryPart)-1]
				cleanQuery := strings.TrimSpace(queryPart[:openIdx])

				tagLower := strings.ToLower(tag)
				if strings.Contains(tagLower, " - ") {
					parts := strings.SplitN(tagLower, " - ", 2)
					lang = strings.TrimSpace(parts[0])
					qual = strings.TrimSpace(parts[1])
				} else {
					if isKnownQuality(tagLower) {
						qual = tagLower
					} else {
						lang = tagLower
					}
				}
				queryPart = cleanQuery
			}

			if queryPart != "" {
				return &searchSession{
					Query:     queryPart,
					Language:  lang,
					Quality:   qual,
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
