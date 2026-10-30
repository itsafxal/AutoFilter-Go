package oldbot

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
	"github.com/itsafxal/AutoFilter-Go/internal/search"
)

// Config holds configuration for the old bots user migration service.
type Config struct {
	Tokens          []string
	AppID           int32
	AppHash         string
	MainBotUsername string
}

// Service manages the lifecycle of old Telegram bot clients running in background goroutines.
// Its sole purpose is funneling / migrating users from old bots into the main bot,
// without causing any lag, hang, or performance degradation on the main bot.
type Service struct {
	cfg           Config
	searchService search.SearchService
	logger        *slog.Logger
	clients       []*telegram.Client
	mu            sync.Mutex
	cancel        context.CancelFunc
}

// NewService constructs a new old bot user migration service.
func NewService(cfg Config, searchService search.SearchService, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		cfg:           cfg,
		searchService: searchService,
		logger:        logger,
	}
}

// Start launches background MTProto clients for each configured old bot token.
// It is fully non-blocking and safe; any connection or authentication failure
// on an old bot will simply log a warning without affecting the main bot.
func (s *Service) Start(ctx context.Context, mainBotUsername string) {
	s.mu.Lock()
	if mainBotUsername != "" {
		s.cfg.MainBotUsername = strings.TrimPrefix(mainBotUsername, "@")
	}
	tokens := make([]string, 0, len(s.cfg.Tokens))
	for _, t := range s.cfg.Tokens {
		t = strings.TrimSpace(t)
		if t != "" {
			tokens = append(tokens, t)
		}
	}
	s.cfg.Tokens = tokens

	if len(tokens) == 0 {
		s.mu.Unlock()
		s.logger.Debug("no old bot tokens configured, user migration service idle")
		return
	}

	botCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	s.logger.Info("starting old bots user migration service...", "count", len(tokens), "main_bot", s.cfg.MainBotUsername)

	for i, token := range tokens {
		go s.runOldBot(botCtx, token, i+1)
	}
}

// Stop gracefully shuts down all old bot clients.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	for _, client := range s.clients {
		if client != nil {
			_ = client.Disconnect()
		}
	}
	s.clients = nil
	s.logger.Info("old bots migration service stopped")
}

// runOldBot handles the connection, authentication, and update loop for an individual old bot.
func (s *Service) runOldBot(ctx context.Context, token string, botIndex int) {
	clientConfig := telegram.ClientConfig{
		AppID:         s.cfg.AppID,
		AppHash:       s.cfg.AppHash,
		MemorySession: true,
		LogLevel:      telegram.LogWarn,
		ParseMode:     "html",
	}

	client, err := telegram.NewClient(clientConfig)
	if err != nil {
		s.logger.Warn("failed to initialize client for old bot", "bot_index", botIndex, "error", err)
		return
	}

	s.mu.Lock()
	s.clients = append(s.clients, client)
	s.mu.Unlock()

	if err := client.Connect(); err != nil {
		s.logger.Warn("failed to connect old bot to MTProto", "bot_index", botIndex, "error", err)
		return
	}

	loginDone := make(chan error, 1)
	go func() {
		loginDone <- client.LoginBot(token)
	}()

	select {
	case err := <-loginDone:
		if err != nil {
			s.logger.Warn("failed to authenticate old bot token", "bot_index", botIndex, "error", err)
			return
		}
	case <-time.After(45 * time.Second):
		s.logger.Warn("authentication timed out for old bot", "bot_index", botIndex)
		return
	case <-ctx.Done():
		return
	}

	me, err := client.GetMe()
	botUsername := fmt.Sprintf("old_bot_%d", botIndex)
	if err == nil && me != nil {
		if me.Username != "" {
			botUsername = "@" + me.Username
		} else if me.FirstName != "" {
			botUsername = me.FirstName
		}
	}

	s.logger.Info("old bot online for user migration", "bot_index", botIndex, "username", botUsername)

	// Register /start command handler
	client.AddCommandHandler("start", func(m *telegram.NewMessage) error {
		go s.handleStart(m)
		return nil
	})

	// Register message handler for user movie queries and other messages
	client.AddMessageHandler(string(telegram.OnNewMessage), func(m *telegram.NewMessage) error {
		go s.handleMessage(m)
		return nil
	}, telegram.Any(telegram.IsIncoming, telegram.IsOutgoing))

	go func() {
		<-ctx.Done()
		_ = client.Disconnect()
	}()

	client.Idle()
}

// handleStart sends a stylish migration banner pointing to the main bot.
func (s *Service) handleStart(m *telegram.NewMessage) {
	if m == nil || m.IsChannel() || m.IsChannelPost() {
		return
	}

	plugins.EnsurePeerCached(m, 2*time.Second)

	mainUser := s.cfg.MainBotUsername
	mainBotLink := fmt.Sprintf("https://t.me/%s?start=from_oldbot", mainUser)
	if mainUser == "" {
		mainBotLink = "https://t.me"
	}

	text := fmt.Sprintf(
		"👋 <b>Hey there!</b>\n\n" +
			"⚠️ <b>We Have Moved!</b>\n" +
			"This bot has been upgraded and migrated to our new official <b>Main Bot</b>!\n\n" +
			"✨ <b>What's New in Main Bot:</b>\n" +
			"• 🌐 Multi-Language selection (Malayalam, Tamil, English, Hindi, Telugu, Kannada, Multi)\n" +
			"• 🎞 Quality filters (480p, 720p, 1080p, 4K)\n" +
			"• ⚡ Ultra-fast direct download links\n" +
			"• 📦 Auto-deleting links for storage safety\n\n" +
			"👉 <i>Click below to switch to our Main Bot now:</i>",
	)

	replyMarkup := telegram.NewKeyboard().
		AddRow(telegram.Button.URL("🚀 Switch To Main Bot 🚀", mainBotLink)).
		Build()

	_, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	if err != nil && m.Peer != nil && m.Client != nil {
		_, _ = m.Client.SendMessage(m.Peer, text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
	}
}

// handleMessage handles user movie queries in old bots and generates redirect buttons to the main bot.
func (s *Service) handleMessage(m *telegram.NewMessage) {
	if m == nil || m.IsChannel() || m.IsChannelPost() {
		return
	}

	plugins.EnsurePeerCached(m, 2*time.Second)

	// Commands other than /start (e.g., /help, /about) get migration redirect
	if m.IsCommand() || strings.HasPrefix(m.Text(), "/") {
		cmd := strings.ToLower(strings.Fields(m.Text())[0])
		if cmd == "/start" {
			return // already handled by AddCommandHandler
		}
		mainUser := s.cfg.MainBotUsername
		mainBotLink := fmt.Sprintf("https://t.me/%s?start=from_oldbot", mainUser)
		if mainUser == "" {
			mainBotLink = "https://t.me"
		}
		text := fmt.Sprintf(
			"⚠️ <b>This bot has moved!</b>\n\n" +
				"All commands, file searches, and downloads are now handled exclusively on our Main Bot.\n\n" +
				"👉 <i>Click below to open our Main Bot:</i>",
		)
		replyMarkup := telegram.NewKeyboard().
			AddRow(telegram.Button.URL("🚀 Open Main Bot 🚀", mainBotLink)).
			Build()
		_, err := m.Reply(text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
		if err != nil && m.Peer != nil && m.Client != nil {
			_, _ = m.Client.SendMessage(m.Peer, text, &telegram.SendOptions{
				ParseMode:   "html",
				ReplyMarkup: replyMarkup,
			})
		}
		return
	}

	query := search.ExtractQueryTitle(strings.TrimSpace(m.Text()))
	if !search.IsValidSearchText(query) {
		return
	}

	mainUser := s.cfg.MainBotUsername
	queryPayload := BuildSearchDeepLinkPayload(query)
	mainSearchURL := fmt.Sprintf("https://t.me/%s?start=%s", mainUser, queryPayload)
	mainHomeURL := fmt.Sprintf("https://t.me/%s?start=from_oldbot", mainUser)
	if mainUser == "" {
		mainSearchURL = "https://t.me"
		mainHomeURL = "https://t.me"
	}

	// Non-blocking quick search with timeout so old bot never lags
	searchCtx, searchCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer searchCancel()

	var files []search.MediaFile
	if s.searchService != nil {
		res, err := s.searchService.Search(searchCtx, query, 5, 0, search.SearchFilter{})
		if err == nil && res != nil {
			files = res.Files
		}
	}

	if len(files) > 0 {
		text := fmt.Sprintf(
			"✨ <b>Your file is ready!</b>\n\n"+
				"🎬 <b>Query:</b> <code>%s</code>\n"+
				"📦 <b>Files Found:</b> %d\n\n"+
				"👉 <i>Click below to view full filters or get your file in our Main Bot:</i>",
			html.EscapeString(query),
			len(files),
		)

		kb := telegram.NewKeyboard()
		// Top Button: Opens full interactive filter menu in Main Bot
		kb.AddRow(telegram.Button.URL("🎬 View All Results & Filters ⚡", mainSearchURL))

		// File buttons: Direct file delivery in Main Bot
		for _, f := range files {
			fileLabel := fmt.Sprintf("📁 %s (%s)", truncateText(f.FileName, 36), f.Size)
			fileURL := fmt.Sprintf("https://t.me/%s?start=file_%s", mainUser, f.FileID)
			if mainUser == "" {
				fileURL = "https://t.me"
			}
			kb.AddRow(telegram.Button.URL(fileLabel, fileURL))
		}

		// Bottom Button: Direct Main Bot link
		kb.AddRow(telegram.Button.URL("🚀 Switch To Main Bot 🚀", mainHomeURL))

		_, err := m.Reply(text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: kb.Build(),
		})
		if err != nil && m.Peer != nil && m.Client != nil {
			_, _ = m.Client.SendMessage(m.Peer, text, &telegram.SendOptions{
				ParseMode:   "html",
				ReplyMarkup: kb.Build(),
			})
		}
		return
	}

	// No direct match found
	text := fmt.Sprintf(
		"🔍 <b>Searching for:</b> <code>%s</code>\n\n"+
			"⚠️ <i>No direct files found in old bot index.</i>\n"+
			"💡 <i>Click below to search in our Main Bot with complete movie catalog and spell-check!</i>",
		html.EscapeString(query),
	)

	replyMarkup := telegram.NewKeyboard().
		AddRow(telegram.Button.URL("🔍 Search in Main Bot ⚡", mainSearchURL)).
		AddRow(telegram.Button.URL("🚀 Switch To Main Bot 🚀", mainHomeURL)).
		Build()

	_, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	if err != nil && m.Peer != nil && m.Client != nil {
		_, _ = m.Client.SendMessage(m.Peer, text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
	}
}

// BuildSearchDeepLinkPayload encodes a query string into a safe Telegram start deep-link payload.
// Uses base64 RawURLEncoding with "q_" prefix (max length within Telegram's 64-character limit).
func BuildSearchDeepLinkPayload(query string) string {
	runes := []rune(strings.TrimSpace(query))
	if len(runes) > 40 {
		runes = runes[:40]
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(string(runes)))
	return "q_" + encoded
}

// DecodeSearchDeepLinkPayload decodes a deep-link payload produced by BuildSearchDeepLinkPayload
// or standard search_<query> prefix.
func DecodeSearchDeepLinkPayload(payload string) (string, bool) {
	if strings.HasPrefix(payload, "q_") {
		b64 := strings.TrimPrefix(payload, "q_")
		if raw, err := base64.RawURLEncoding.DecodeString(b64); err == nil && len(raw) > 0 {
			return string(raw), true
		}
		if raw, err := base64.URLEncoding.DecodeString(b64); err == nil && len(raw) > 0 {
			return string(raw), true
		}
		if raw, err := base64.StdEncoding.DecodeString(b64); err == nil && len(raw) > 0 {
			return string(raw), true
		}
	}
	if strings.HasPrefix(payload, "search_") {
		raw := strings.TrimPrefix(payload, "search_")
		query := strings.ReplaceAll(raw, "_", " ")
		if strings.TrimSpace(query) != "" {
			return strings.TrimSpace(query), true
		}
	}
	return "", false
}

func truncateText(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes-3]) + "..."
}
