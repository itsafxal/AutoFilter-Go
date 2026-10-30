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
	"github.com/itsafxal/AutoFilter-Go/internal/autodelete"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
	"github.com/itsafxal/AutoFilter-Go/internal/search"
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
	UserMention string
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
				// In DMs: skip any forwarded file/document silently.
				// Admin-forwarded files are handled by the index plugin; non-admin forwarded
				// files must receive no reply at all.
				if m.IsPrivate() {
					if (m.IsForward() || (m.Message != nil && m.Message.FwdFrom != nil)) &&
						(m.Document() != nil || m.Video() != nil || m.Audio() != nil) {
						return false
					}
				}
				// In groups: skip messages containing URLs/links to prevent link-triggered replies.
				if !m.IsPrivate() {
					if messageHasLink(m) {
						return false
					}
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

	// 1. GLOBAL FILTER PRIORITY: Check Global Filter BEFORE Auto Filter
	if p.appCtx != nil && p.appCtx.GFilter != nil {
		handled, err := p.appCtx.GFilter.MatchAndExecute(m)
		if err != nil && p.appCtx.Logger != nil {
			p.appCtx.Logger.Error("global filter processing error", "error", err)
		}
		if handled {
			// Global filter matched and responded! Stop further filter processing.
			// Do NOT execute Auto Filter.
			return nil
		}
	}

	// 2. Not matched by Global Filter: continue to Auto Filter search
	return p.ExecuteSearch(m, m.Text())
}

// ExecuteSearch performs search for rawQuery and replies to message m with interactive filter results.
func (p *Plugin) ExecuteSearch(m *telegram.NewMessage, rawQuery string) error {
	if m == nil || m.IsChannel() || m.IsChannelPost() {
		return nil
	}

	user := m.Sender
	if (user == nil || user.ID == 0) && m.Client != nil {
		if sender, err := m.GetSender(); err == nil && sender != nil && sender.ID != 0 {
			user = sender
			m.Sender = sender
		}
	}

	var userID int64
	var userMention string
	if user != nil && user.ID != 0 {
		userID = user.ID
		userMention = formatUserMention(user)
	} else {
		userID = m.SenderID()
		if userID == 0 {
			userID = m.ChatID()
		}
		if userID != 0 {
			userMention = fmt.Sprintf(`<a href="tg://user?id=%d">User</a>`, userID)
		} else {
			userMention = "User"
		}
	}

	if userID == 0 {
		return nil
	}

	rawQuery = search.ExtractQueryTitle(rawQuery)
	query := strings.TrimSpace(rawQuery)
	if !search.IsValidSearchText(query) {
		return nil
	}

	// In group chats, verify the group is not disabled
	if !m.IsPrivate() && p.appCtx != nil && p.appCtx.DB != nil && p.appCtx.DB.GroupRepo() != nil {
		group, err := p.appCtx.DB.GroupRepo().GetGroup(context.Background(), m.ChatID())
		if err == nil && group != nil && group.ChatStatus.IsDisabled {
			if p.appCtx.Logger != nil {
				p.appCtx.Logger.Debug("autofilter skipped: group is disabled", "chat_id", m.ChatID(), "reason", group.ChatStatus.Reason)
			}
			return nil
		}
	}

	// In maintenance mode, only admins receive search results (both in group and PM).
	// Groups and non-admin users receive no search responses.
	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(userID)
		if !isAdmin {
			if p.appCtx != nil && p.appCtx.Logger != nil {
				p.appCtx.Logger.Info("autofilter query blocked: maintenance mode active",
					"query", query,
					"user_id", userID,
					"chat_id", m.ChatID(),
				)
			}
			return nil
		}
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
			Query:       query,
			UserID:      userID,
			UserMention: userMention,
			ChatID:      m.ChatID(),
			UpdatedAt:   time.Now(),
		})

		botName := p.getBotName(m.Client)
		text := p.formatResultText(userMention, query, result.Page, result.TotalPages, result.Total, botName)
		kb := p.buildResultKeyboard(result.Files, searchKey, result.Page, result.TotalPages, false)

		replyMsg, replyErr := m.Reply(text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: kb,
		})
		if replyErr != nil && m.Client != nil && m.Peer != nil {
			replyMsg, replyErr = m.Client.SendMessage(m.Peer, text, &telegram.SendOptions{
				ParseMode:   "html",
				ReplyMarkup: kb,
				ReplyID:     m.ID,
			})
		}
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
			UserMention: userMention,
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
		if replyErr != nil && m.Client != nil && m.Peer != nil {
			replyMsg, replyErr = m.Client.SendMessage(m.Peer, text, &telegram.SendOptions{
				ParseMode:   "html",
				ReplyMarkup: kb,
				ReplyID:     m.ID,
			})
		}
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
	if replyErr != nil && m.Client != nil && m.Peer != nil {
		replyMsg, replyErr = m.Client.SendMessage(m.Peer, notFoundText, &telegram.SendOptions{
			ParseMode: "html",
			ReplyID:   m.ID,
		})
	}
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

// messageHasLink returns true if the message contains a URL/link, either detected via
// Telegram message entities (MessageEntityURL, MessageEntityTextURL) or by regex scanning the text.
func messageHasLink(m *telegram.NewMessage) bool {
	if m == nil || m.Message == nil {
		return false
	}
	// Check Telegram message entities for URL types
	for _, entity := range m.Message.Entities {
		switch entity.(type) {
		case *telegram.MessageEntityURL,
			*telegram.MessageEntityTextURL:
			return true
		}
	}
	// Also check plain text for URL patterns
	if urlRegex.MatchString(m.Message.Message) {
		return true
	}
	return false
}

// HandlePaginationCallback processes next/back navigation buttons.
// Format: next|<searchKey>|<offset>
func (p *Plugin) HandlePaginationCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(cb.SenderID)
		if !isAdmin {
			_, _ = cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return nil
		}
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
	userMention := p.getUserMentionForSession(sess, cb.Sender)
	text := p.formatResultText(userMention, displayQuery, result.Page, result.TotalPages, result.Total, botName)
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

	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(cb.SenderID)
		if !isAdmin {
			_, _ = cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return nil
		}
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
	userMention := p.getUserMentionForSession(sess, cb.Sender)
	text := p.formatResultText(userMention, selectedQuery, result.Page, result.TotalPages, result.Total, botName)
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

	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(cb.SenderID)
		if !isAdmin {
			_, _ = cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return nil
		}
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

	markup := p.buildLanguagesKeyboard(searchKey)

	botName := p.getBotName(cb.Client)
	menuText := p.formatLanguagesMenuText(p.getDisplayQuery(sess), botName)
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

	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(cb.SenderID)
		if !isAdmin {
			_, _ = cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return nil
		}
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
	userMention := p.getUserMentionForSession(sess, cb.Sender)
	text := p.formatResultText(userMention, displayQuery, result.Page, result.TotalPages, result.Total, botName)
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

	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(cb.SenderID)
		if !isAdmin {
			_, _ = cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return nil
		}
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

	markup := buildQualitiesKeyboard(searchKey)

	botName := p.getBotName(cb.Client)
	menuText := p.formatQualitiesMenuText(p.getDisplayQuery(sess), botName)
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

	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(cb.SenderID)
		if !isAdmin {
			_, _ = cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return nil
		}
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
	userMention := p.getUserMentionForSession(sess, cb.Sender)
	text := p.formatResultText(userMention, displayQuery, result.Page, result.TotalPages, result.Total, botName)
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

	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(cb.SenderID)
		if !isAdmin {
			_, _ = cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return nil
		}
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
	userMention := p.getUserMentionForSession(sess, cb.Sender)
	hasFilter := sess.Language != "" || sess.Quality != ""
	text := p.formatResultText(userMention, displayQuery, result.Page, result.TotalPages, result.Total, botName)
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

	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(cb.SenderID)
		if !isAdmin {
			_, _ = cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return nil
		}
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
	userMention := p.getUserMentionForSession(sess, cb.Sender)
	text := p.formatResultText(userMention, sess.Query, result.Page, result.TotalPages, result.Total, botName)
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
	// urlRegex detects http/https URLs and www-prefixed domain patterns in message text.
	urlRegex = regexp.MustCompile(`(?i)https?://[^\s]+|(?:^|\s)www\.[^\s]+`)
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
	// 1. Language: prioritize filename over caption
	if match := reLangMatch.FindString(fileName); match != "" {
		lang = canonicalLanguage(match)
	} else if match := reLangMatch.FindString(caption); match != "" {
		lang = canonicalLanguage(match)
	}

	// 2. Quality: prioritize filename over caption
	res := reResolution.FindString(fileName)
	rip := reRip.FindString(fileName)
	if res == "" {
		res = reResolution.FindString(caption)
	}
	if rip == "" {
		rip = reRip.FindString(caption)
	}
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

	// Find the earliest cut point among year, resolution, rip, and language tags
	firstCut := len(clean)
	for _, re := range []*regexp.Regexp{reYearFind, reResolution, reRip, reLangMatch} {
		if loc := re.FindStringIndex(clean); loc != nil && loc[0] < firstCut && loc[0] > 0 {
			firstCut = loc[0]
		}
	}
	if firstCut < len(clean) {
		clean = clean[:firstCut]
	}

	clean = reCleanChars.ReplaceAllString(clean, " ")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		fallback := reFileExtStrip.ReplaceAllString(fileName, "")
		fallback = reCleanChars.ReplaceAllString(fallback, " ")
		clean = strings.TrimSpace(fallback)
	}
	cleanTitle = clean

	return lang, qual, cleanTitle
}

func formatFileButtonLabel(f search.MediaFile) string {
	return formatFileButtonLabelCustom(f, false)
}

func formatFileButtonLabelCustom(f search.MediaFile, hasIcon bool) string {
	size := strings.TrimSpace(f.Size)
	if size == "" && f.FileSize > 0 {
		size = formatBytes(f.FileSize)
	}
	if size == "" {
		size = "N/A"
	}

	lang, qual, cleanTitle := extractFileMetadata(f.FileName, f.Caption)
	if cleanTitle == "" {
		cleanTitle = strings.TrimSpace(f.FileName)
	}

	var prefix string
	if hasIcon {
		prefix = size + " ▫️ "
	} else {
		prefix = "🔸 " + size + " ▫️ "
	}

	var details []string
	if cleanTitle != "" {
		extraLen := len([]rune(prefix))
		if lang != "" {
			extraLen += 1 + len([]rune(lang))
		}
		if qual != "" {
			extraLen += 1 + len([]rune(qual))
		}

		maxTitleLen := 56 - extraLen
		if maxTitleLen < 12 {
			maxTitleLen = 12
		}

		titleRunes := []rune(cleanTitle)
		if len(titleRunes) > maxTitleLen {
			cleanTitle = string(titleRunes[:maxTitleLen-3]) + "..."
		}
		details = append(details, cleanTitle)
	}

	if lang != "" {
		details = append(details, lang)
	}
	if qual != "" {
		details = append(details, qual)
	}

	label := prefix + strings.Join(details, " ")
	labelRunes := []rune(label)
	if len(labelRunes) > 56 {
		label = string(labelRunes[:53]) + "..."
		labelRunes = []rune(label)
	}

	// Pad with invisible Braille whitespace (\u2800) so that Telegram centers
	// a consistent ~50-rune width string, forcing the visible content flush-left
	// across all button rows regardless of title length.
	const targetWidth = 50
	if len(labelRunes) < targetWidth {
		label = label + strings.Repeat("\u2800", targetWidth-len(labelRunes))
	}

	return label
}

func (p *Plugin) buildLanguagesKeyboard(searchKey string) *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()
	var currentRow []telegram.KeyboardButton

	for i, lang := range languages {
		var btn *telegram.KeyboardButtonCallback
		if lang.Code == "multi" && p != nil && p.isPremiumEmojiEnabled() {
			btn = telegram.Button.Data("Multi Lang", fmt.Sprintf("fl#%s#%s", lang.Code, searchKey)).Icon(p.getMultiLangEmojiID())
		} else {
			btn = telegram.Button.Data(lang.Label, fmt.Sprintf("fl#%s#%s", lang.Code, searchKey))
		}
		currentRow = append(currentRow, btn)
		if (i+1)%2 == 0 || i == len(languages)-1 {
			kb.AddRow(currentRow...)
			currentRow = nil
		}
	}

	// Back to results button (Danger / Red)
	kb.AddRow(telegram.Button.Data("↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭", fmt.Sprintf("back#%s", searchKey)).Danger())
	return kb.Build()
}

func buildLanguagesKeyboard(searchKey string) *telegram.ReplyInlineMarkup {
	return (&Plugin{}).buildLanguagesKeyboard(searchKey)
}

func buildQualitiesKeyboard(searchKey string) *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()
	// Row 1: 480P, 720P
	kb.AddRow(
		telegram.Button.Data("480P", fmt.Sprintf("fq#480p#%s", searchKey)),
		telegram.Button.Data("720P", fmt.Sprintf("fq#720p#%s", searchKey)),
	)
	// Row 2: 1080P, 2160P
	kb.AddRow(
		telegram.Button.Data("1080P", fmt.Sprintf("fq#1080p#%s", searchKey)),
		telegram.Button.Data("2160P", fmt.Sprintf("fq#2160p#%s", searchKey)),
	)
	// Row 3: 4K
	kb.AddRow(
		telegram.Button.Data("4K", fmt.Sprintf("fq#4k#%s", searchKey)),
	)
	// Back button (Danger / Red)
	kb.AddRow(telegram.Button.Data("↭ ʙᴀᴄᴋ ᴛᴏ ꜰɪʟᴇs ↭", fmt.Sprintf("back#%s", searchKey)).Danger())
	return kb.Build()
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

func (p *Plugin) isPremiumEmojiEnabled() bool {
	return p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.EnablePremiumEmoji
}

func (p *Plugin) isMaintenanceMode() bool {
	return p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.MaintenanceMode
}

func (p *Plugin) getPrevEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.PrevButtonEmojiID != 0 {
		return p.appCtx.Config.PrevButtonEmojiID
	}
	return 5469982030773120950
}

func (p *Plugin) getNextEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.NextButtonEmojiID != 0 {
		return p.appCtx.Config.NextButtonEmojiID
	}
	return 5469715085670772857
}

func (p *Plugin) getPageButtonEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.PageButtonEmojiID != 0 {
		return p.appCtx.Config.PageButtonEmojiID
	}
	return 5837048052061311256
}

func (p *Plugin) getQualityEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.QualityButtonEmojiID != 0 {
		return p.appCtx.Config.QualityButtonEmojiID
	}
	return 5276251908774858442
}

func (p *Plugin) getLanguageEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.LanguageButtonEmojiID != 0 {
		return p.appCtx.Config.LanguageButtonEmojiID
	}
	return 5276011261757265100
}

func (p *Plugin) getMultiLangEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.MultiLangButtonEmojiID != 0 {
		return p.appCtx.Config.MultiLangButtonEmojiID
	}
	return 5256169350368353876
}

func (p *Plugin) getSelectLanguageEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.SelectLanguageEmojiID != 0 {
		return p.appCtx.Config.SelectLanguageEmojiID
	}
	return 5447410659077661506
}

func (p *Plugin) getSelectQualityEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.SelectQualityEmojiID != 0 {
		return p.appCtx.Config.SelectQualityEmojiID
	}
	return 5334544901428229844
}

func (p *Plugin) getGreetingPrefixEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.GreetingPrefixEmojiID != 0 {
		return p.appCtx.Config.GreetingPrefixEmojiID
	}
	return 5472055112702629499
}

func (p *Plugin) getGreetingSuffixEmojiID() int64 {
	return p.getGreetingEmojiID()
}

func (p *Plugin) getGreetingEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.GreetingEmojiID != 0 {
		return p.appCtx.Config.GreetingEmojiID
	}
	return 5427232022252764138
}

func (p *Plugin) getQueryEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.QueryEmojiID != 0 {
		return p.appCtx.Config.QueryEmojiID
	}
	return 5190859184312167965
}

func (p *Plugin) getPageTextEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.PageTextEmojiID != 0 {
		return p.appCtx.Config.PageTextEmojiID
	}
	return 5836954069586946866
}

func (p *Plugin) getNotePrefixEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.NotePrefixEmojiID != 0 {
		return p.appCtx.Config.NotePrefixEmojiID
	}
	return 4956461073550017373
}

func (p *Plugin) getNoteSuffixEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.NoteSuffixEmojiID != 0 {
		return p.appCtx.Config.NoteSuffixEmojiID
	}
	return 4958724224962265918
}

func (p *Plugin) getNoteExpireEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.NoteExpireEmojiID != 0 {
		return p.appCtx.Config.NoteExpireEmojiID
	}
	return 5440660757194744323
}

func (p *Plugin) getBotNameEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.BotNameEmojiID != 0 {
		return p.appCtx.Config.BotNameEmojiID
	}
	return 6104631352190043951
}

func (p *Plugin) getFileSizePrefixEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.FileSizePrefixEmojiID != 0 {
		return p.appCtx.Config.FileSizePrefixEmojiID
	}
	return 5992513524440502236
}

func (p *Plugin) getFileSizeSuffixEmojiID() int64 {
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.FileSizeSuffixEmojiID != 0 {
		return p.appCtx.Config.FileSizeSuffixEmojiID
	}
	return 5992519984071315160
}

func (p *Plugin) buildPrevButton(searchKey string, prevOffset int) *telegram.KeyboardButtonCallback {
	data := fmt.Sprintf("next|%s|%d", searchKey, prevOffset)
	if p.isPremiumEmojiEnabled() {
		return telegram.Button.Data("Previous", data).Icon(p.getPrevEmojiID()).Success()
	}
	return telegram.Button.Data("◀ Previous", data).Success()
}

func (p *Plugin) buildNextButton(searchKey string, nextOffset int) *telegram.KeyboardButtonCallback {
	data := fmt.Sprintf("next|%s|%d", searchKey, nextOffset)
	if p.isPremiumEmojiEnabled() {
		return telegram.Button.Data("Next", data).Icon(p.getNextEmojiID()).Success()
	}
	return telegram.Button.Data("Next ▶", data).Success()
}

func (p *Plugin) buildResultKeyboard(files []search.MediaFile, searchKey string, currentPage, totalPages int, hasFilter bool) *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()

	// 1. Languages and Quality header row (Blue color)
	if p.isPremiumEmojiEnabled() {
		kb.AddRow(
			telegram.Button.Data("Language", fmt.Sprintf("languages#%s", searchKey)).Icon(p.getLanguageEmojiID()).Primary(),
			telegram.Button.Data("Quality", fmt.Sprintf("qualities#%s", searchKey)).Icon(p.getQualityEmojiID()).Primary(),
		)
	} else {
		kb.AddRow(
			telegram.Button.Data("🌐 Languages", fmt.Sprintf("languages#%s", searchKey)).Primary(),
			telegram.Button.Data("🎞 Quality", fmt.Sprintf("qualities#%s", searchKey)).Primary(),
		)
	}

	// 2. File list rows (1 button per file)
	for _, f := range files {
		if p.isPremiumEmojiEnabled() {
			fileLabel := formatFileButtonLabelCustom(f, true)
			kb.AddRow(telegram.Button.Data(fileLabel, fmt.Sprintf("file#%s", f.FileID)).Icon(p.getFileSizePrefixEmojiID()))
		} else {
			fileLabel := formatFileButtonLabel(f)
			kb.AddRow(telegram.Button.Data(fileLabel, fmt.Sprintf("file#%s", f.FileID)))
		}
	}

	// 3. Navigation row
	if totalPages < 1 {
		totalPages = 1
	}
	if currentPage < 1 {
		currentPage = 1
	}

	var navButtons []telegram.KeyboardButton

	// Previous button: only when currentPage > 1 (Green color)
	if currentPage > 1 {
		prevOffset := (currentPage - 2) * maxFilesPerPage
		if prevOffset < 0 {
			prevOffset = 0
		}
		navButtons = append(navButtons, p.buildPrevButton(searchKey, prevOffset))
	}

	// Page indicator button: always present
	// Green on page 1, Red on subsequent pages (currentPage > 1)
	var pageBtn *telegram.KeyboardButtonCallback
	if p.isPremiumEmojiEnabled() {
		pageLabel := fmt.Sprintf("page %d/%d", currentPage, totalPages)
		pageBtn = telegram.Button.Data(pageLabel, "pages").Icon(p.getPageButtonEmojiID())
	} else {
		pageLabel := fmt.Sprintf("📄 Page %d / %d", currentPage, totalPages)
		pageBtn = telegram.Button.Data(pageLabel, "pages")
	}
	if currentPage > 1 {
		pageBtn = pageBtn.Danger()
	} else {
		pageBtn = pageBtn.Success()
	}
	navButtons = append(navButtons, pageBtn)

	// Next button: only when currentPage < totalPages (Green color)
	if currentPage < totalPages {
		nextOffset := currentPage * maxFilesPerPage
		navButtons = append(navButtons, p.buildNextButton(searchKey, nextOffset))
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

func (p *Plugin) getFilterDeleteMinutes() int {
	if p != nil && p.appCtx != nil {
		if p.appCtx.AutoDelete != nil {
			dur := p.appCtx.AutoDelete.GetSettings().FilterDeleteDuration
			if dur > 0 {
				mins := int(dur.Minutes())
				if mins > 0 {
					return mins
				}
			}
		}
		if p.appCtx.Config != nil && p.appCtx.Config.AutoDeleteFilterTime != "" {
			if dur, err := autodelete.ParseDuration(p.appCtx.Config.AutoDeleteFilterTime); err == nil && dur > 0 {
				mins := int(dur.Minutes())
				if mins > 0 {
					return mins
				}
			}
		}
	}
	return 15
}

func (p *Plugin) getUserMentionForSession(sess *searchSession, sender *telegram.UserObj) string {
	if sess != nil && sess.UserMention != "" {
		return sess.UserMention
	}
	if sender != nil {
		return formatUserMention(sender)
	}
	if sess != nil && sess.UserID != 0 {
		return fmt.Sprintf(`<a href="tg://user?id=%d">User</a>`, sess.UserID)
	}
	return "User"
}

func (p *Plugin) formatResultText(userMention, query string, page, totalPages int, totalResults int64, botName string) string {
	deleteMins := p.getFilterDeleteMinutes()
	if p.isPremiumEmojiEnabled() {
		return formatResultTextPremium(
			userMention, query, page, totalPages, totalResults, botName, deleteMins,
			p.getGreetingPrefixEmojiID(),
			p.getGreetingSuffixEmojiID(),
			p.getQueryEmojiID(),
			p.getPageTextEmojiID(),
			p.getNotePrefixEmojiID(),
			p.getNoteSuffixEmojiID(),
			p.getNoteExpireEmojiID(),
			p.getBotNameEmojiID(),
		)
	}
	return formatResultText(userMention, query, page, totalPages, totalResults, botName, deleteMins)
}

func formatResultText(userMention, query string, page, totalPages int, totalResults int64, botName string, deleteMins int) string {
	if userMention == "" {
		userMention = "User"
	}
	if page < 1 {
		page = 1
	}
	if deleteMins <= 0 {
		deleteMins = 15
	}
	if botName == "" {
		botName = "Bot"
	}

	return fmt.Sprintf(
		"𝗛𝗘𝗬 👋 %s 🫶\n"+
			"    \n"+
			"📁 Query : %s\n\n"+
			"🗓️ Page No › %d\n\n"+
			"<blockquote>✍️ NOTE : ⚠️ This Message Will Be Auto Deleted Within %d Mins ❗️</blockquote>\n"+
			"©%s 🗡️",
		userMention,
		html.EscapeString(query),
		page,
		deleteMins,
		html.EscapeString(botName),
	)
}

func formatResultTextPremium(
	userMention, query string, page, totalPages int, totalResults int64, botName string, deleteMins int,
	greetPrefixID, greetSuffixID, queryID, pageID, notePrefixID, noteSuffixID, noteExpireID, botNameEmojiID int64,
) string {
	if userMention == "" {
		userMention = "User"
	}
	if page < 1 {
		page = 1
	}
	if deleteMins <= 0 {
		deleteMins = 15
	}
	if botName == "" {
		botName = "Bot"
	}

	return fmt.Sprintf(
		"𝗛𝗘𝗬 <emoji id=\"%d\">👋</emoji> %s <emoji id=\"%d\">🫶</emoji>\n"+
			"    \n"+
			"<emoji id=\"%d\">📁</emoji> Query : %s\n\n"+
			"<emoji id=\"%d\">🗓️</emoji> Page No › %d\n\n"+
			"<blockquote><emoji id=\"%d\">✍️</emoji> NOTE : <emoji id=\"%d\">⚠️</emoji> This Message Will Be Auto Deleted Within %d Mins <emoji id=\"%d\">❗️</emoji></blockquote>\n"+
			"©%s <emoji id=\"%d\">🗡️</emoji>",
		greetPrefixID,
		userMention,
		greetSuffixID,
		queryID,
		html.EscapeString(query),
		pageID,
		page,
		notePrefixID,
		noteSuffixID,
		deleteMins,
		noteExpireID,
		html.EscapeString(botName),
		botNameEmojiID,
	)
}

func (p *Plugin) formatLanguagesMenuText(query string, botName string) string {
	deleteMins := p.getFilterDeleteMinutes()
	if p.isPremiumEmojiEnabled() {
		return fmt.Sprintf(
			"<emoji id=\"%d\">🌐</emoji> <b>Select Desired Language</b>\n\n"+
				"<emoji id=\"%d\">📁</emoji> Query : %s\n\n"+
				"<blockquote><emoji id=\"%d\">✍️</emoji> NOTE : <emoji id=\"%d\">⚠️</emoji> This Message Will Be Auto Deleted Within %d Mins <emoji id=\"%d\">❗️</emoji></blockquote>\n"+
				"©%s <emoji id=\"%d\">🗡️</emoji>",
			p.getSelectLanguageEmojiID(),
			p.getQueryEmojiID(),
			html.EscapeString(query),
			p.getNotePrefixEmojiID(),
			p.getNoteSuffixEmojiID(),
			deleteMins,
			p.getNoteExpireEmojiID(),
			html.EscapeString(botName),
			p.getBotNameEmojiID(),
		)
	}
	return formatLanguagesMenuText(query, botName, deleteMins)
}

func formatLanguagesMenuText(query string, botName string, deleteMins int) string {
	if deleteMins <= 0 {
		deleteMins = 15
	}
	return fmt.Sprintf(
		"🌐 <b>Select Desired Language</b>\n\n"+
			"📁 Query : %s\n\n"+
			"<blockquote>✍️ NOTE : ⚠️ This Message Will Be Auto Deleted Within %d Mins ❗️</blockquote>\n"+
			"©%s 🗡️",
		html.EscapeString(query),
		deleteMins,
		html.EscapeString(botName),
	)
}

func (p *Plugin) formatQualitiesMenuText(query string, botName string) string {
	deleteMins := p.getFilterDeleteMinutes()
	if p.isPremiumEmojiEnabled() {
		return fmt.Sprintf(
			"<emoji id=\"%d\">🎞</emoji> <b>Select Desired Quality</b>\n\n"+
				"<emoji id=\"%d\">📁</emoji> Query : %s\n\n"+
				"<blockquote><emoji id=\"%d\">✍️</emoji> NOTE : <emoji id=\"%d\">⚠️</emoji> This Message Will Be Auto Deleted Within %d Mins <emoji id=\"%d\">❗️</emoji></blockquote>\n"+
				"©%s <emoji id=\"%d\">🗡️</emoji>",
			p.getSelectQualityEmojiID(),
			p.getQueryEmojiID(),
			html.EscapeString(query),
			p.getNotePrefixEmojiID(),
			p.getNoteSuffixEmojiID(),
			deleteMins,
			p.getNoteExpireEmojiID(),
			html.EscapeString(botName),
			p.getBotNameEmojiID(),
		)
	}
	return formatQualitiesMenuText(query, botName, deleteMins)
}

func formatQualitiesMenuText(query string, botName string, deleteMins int) string {
	if deleteMins <= 0 {
		deleteMins = 15
	}
	return fmt.Sprintf(
		"🎞 <b>Select Desired Quality</b>\n\n"+
			"📁 Query : %s\n\n"+
			"<blockquote>✍️ NOTE : ⚠️ This Message Will Be Auto Deleted Within %d Mins ❗️</blockquote>\n"+
			"©%s 🗡️",
		html.EscapeString(query),
		deleteMins,
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
	var recoveredUserMention string
	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if strings.HasPrefix(trimmed, "𝗛𝗘𝗬 👋") && strings.HasSuffix(trimmed, "🫶") {
			mention := strings.TrimPrefix(trimmed, "𝗛𝗘𝗬 👋")
			mention = strings.TrimSuffix(mention, "🫶")
			recoveredUserMention = strings.TrimSpace(mention)
			break
		}
	}
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
					Query:       queryPart,
					Language:    lang,
					Quality:     qual,
					UserID:      userID,
					UserMention: recoveredUserMention,
					ChatID:      chatID,
					UpdatedAt:   time.Now(),
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
