package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// BotService manages the lifecycle of the unified Gogram MTProto Telegram client.
type BotService struct {
	cfg      *config.Config
	logger   *slog.Logger
	registry *plugins.Registry
	appCtx   *plugins.AppContext
	client   *telegram.Client
	running  bool
	mu       sync.Mutex
}

// NewBotService constructs a new Telegram bot service.
func NewBotService(cfg *config.Config, logger *slog.Logger, reg *plugins.Registry, appCtx *plugins.AppContext) *BotService {
	return &BotService{
		cfg:      cfg,
		logger:   logger,
		registry: reg,
		appCtx:   appCtx,
	}
}

// Init creates and initializes the unified Gogram client and attaches all validated plugin handlers.
func (s *BotService) Init() error {
	if s.cfg == nil || s.cfg.BotToken == "" {
		return errors.New("cannot initialize bot service: configuration or BOT_TOKEN is missing")
	}
	if s.cfg.AppID <= 0 || s.cfg.AppHash == "" {
		return errors.New("cannot initialize bot service: API_ID and API_HASH are required for Gogram MTProto client")
	}

	var gogramLogLevel telegram.LogLevel
	switch s.cfg.LogLevel {
	case config.LogLevelDebug:
		gogramLogLevel = telegram.LogDebug
	case config.LogLevelWarn:
		gogramLogLevel = telegram.LogWarn
	case config.LogLevelError:
		gogramLogLevel = telegram.LogError
	default:
		gogramLogLevel = telegram.LogInfo
	}

	clientConfig := telegram.ClientConfig{
		AppID:         s.cfg.AppID,
		AppHash:       s.cfg.AppHash,
		Session:       s.cfg.SessionPath,
		MemorySession: s.cfg.MemorySession,
		LogLevel:      gogramLogLevel,
		ParseMode:     "html",
	}

	client, err := telegram.NewClient(clientConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize Gogram client: %w", err)
	}
	s.client = client

	if s.appCtx != nil {
		s.appCtx.Client = client
	}

	// Attach plugins to Gogram client
	if s.registry != nil {
		if err := s.registry.AttachToClient(client); err != nil {
			return fmt.Errorf("failed to attach plugin handlers to Gogram client: %w", err)
		}
	}

	return nil
}

// Start begins receiving Telegram updates via MTProto until ctx is cancelled.
func (s *BotService) Start(ctx context.Context) error {
	if s.client == nil {
		return errors.New("bot service not initialized; call Init() first")
	}

	s.logger.Info("connecting to Telegram MTProto servers via Gogram...")
	if err := s.client.Connect(); err != nil {
		return fmt.Errorf("failed to connect to Telegram MTProto servers: %w", err)
	}

	s.logger.Info("authenticating bot identity with Telegram...")
	if err := s.client.LoginBot(s.cfg.BotToken); err != nil {
		return fmt.Errorf("failed to authenticate Telegram bot token: %w", err)
	}

	botUser, err := s.client.GetMe()
	if err != nil {
		return fmt.Errorf("failed to get Telegram bot user info: %w", err)
	}

	if s.appCtx != nil {
		s.appCtx.BotUser = botUser
	}

	s.logger.Info("gogram telegram bot client authenticated successfully",
		"bot_username", botUser.Username,
		"bot_id", botUser.ID,
		"app_id", s.cfg.AppID,
	)

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	// Watch for context cancellation for graceful shutdown
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		running := s.running
		s.mu.Unlock()
		if running {
			s.logger.Info("context cancelled, stopping Gogram telegram client")
			_ = s.Stop()
		}
	}()

	s.client.Idle()
	return nil
}

// Stop gracefully terminates the MTProto connection and stops background routines.
func (s *BotService) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("shutting down Gogram bot service...")
	if s.client != nil {
		return s.client.Stop()
	}
	return nil
}

// Client returns the underlying *telegram.Client instance.
func (s *BotService) Client() *telegram.Client {
	return s.client
}

// HistoryReader defines clean contracts for future Telegram channel history access.
// Note: Bot accounts can only access message history in channels/supergroups where they have
// been granted administrative read permissions. Arbitrary historical media scraping for public
// or private channels where the bot is not an admin requires a separately authorized user session.
type HistoryReader interface {
	GetChannelHistory(ctx context.Context, channelID any, offsetID int32, limit int32) ([]*telegram.NewMessage, error)
}
