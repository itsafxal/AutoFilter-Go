package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// BotService manages the lifecycle of the Telegram bot instance, dispatcher, and updater.
type BotService struct {
	cfg        *config.Config
	logger     *slog.Logger
	registry   *plugins.Registry
	appCtx     *plugins.AppContext
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	updater    *ext.Updater
	running    bool
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

// Init creates the gotgbot Bot instance, Dispatcher, and Updater.
func (s *BotService) Init() error {
	if s.cfg == nil || s.cfg.BotToken == "" {
		return errors.New("cannot initialize bot service: configuration or BOT_TOKEN is missing")
	}

	bot, err := gotgbot.NewBot(s.cfg.BotToken, &gotgbot.BotOpts{
		BotClient: &gotgbot.BaseBotClient{
			Client: http.Client{
				Timeout: 30 * time.Second,
			},
			DefaultRequestOpts: &gotgbot.RequestOpts{
				Timeout: 20 * time.Second,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to initialize Telegram bot client: %w", err)
	}
	s.bot = bot

	if s.appCtx != nil {
		s.appCtx.BotUser = &bot.User
	}

	// Create dispatcher
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			s.logger.Error("telegram dispatcher error handling update", "error", err)
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
		Logger:      s.logger,
	})
	s.dispatcher = dispatcher

	// Attach plugins to dispatcher
	if err := s.registry.AttachToDispatcher(dispatcher); err != nil {
		return fmt.Errorf("failed to attach plugin handlers to dispatcher: %w", err)
	}

	// Create updater
	updater := ext.NewUpdater(dispatcher, &ext.UpdaterOpts{
		UnhandledErrFunc: func(err error) {
			s.logger.Error("unhandled telegram updater polling error", "error", err)
		},
		Logger: s.logger,
	})
	s.updater = updater

	return nil
}

// Start begins receiving Telegram updates via long polling until ctx is cancelled.
func (s *BotService) Start(ctx context.Context) error {
	if s.bot == nil || s.updater == nil {
		return errors.New("bot service not initialized; call Init() first")
	}

	s.logger.Info("starting telegram long polling",
		"bot_username", s.bot.User.Username,
		"bot_id", s.bot.User.Id,
	)

	err := s.updater.StartPolling(s.bot, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 10,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: 15 * time.Second,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to start Telegram polling: %w", err)
	}

	s.running = true

	// Watch for context cancellation for graceful shutdown
	go func() {
		<-ctx.Done()
		if s.running {
			s.logger.Info("context cancelled, stopping telegram updater")
			_ = s.Stop()
		}
	}()

	s.updater.Idle()
	return nil
}

// Stop gracefully terminates polling and stops dispatcher routines.
func (s *BotService) Stop() error {
	if !s.running {
		return nil
	}
	s.running = false
	s.logger.Info("shutting down telegram bot service...")

	if s.updater != nil {
		return s.updater.Stop()
	}
	return nil
}

// Bot returns the underlying gotgbot.Bot instance.
func (s *BotService) Bot() *gotgbot.Bot {
	return s.bot
}
