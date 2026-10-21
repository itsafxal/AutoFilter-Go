package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/dustin/go-humanize"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	dbsql "github.com/itsafxal/MYTGBOTZ/internal/database/sql"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// OldBotService defines the contract for managing old bots running concurrently for user migration.
type OldBotService interface {
	Start(ctx context.Context, mainBotUsername string)
	Stop()
}

// BotService manages the lifecycle of the unified Gogram MTProto Telegram client.
type BotService struct {
	cfg           *config.Config
	logger        *slog.Logger
	registry      *plugins.Registry
	appCtx        *plugins.AppContext
	client        *telegram.Client
	oldBotService OldBotService
	running       bool
	mu            sync.Mutex
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

// SetOldBotService configures the background old bots migration service.
func (s *BotService) SetOldBotService(obs OldBotService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.oldBotService = obs
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

	s.logger.Info("authenticating bot identity with Telegram (migrating DC if needed)...")
	loginDone := make(chan error, 1)
	go func() {
		loginDone <- s.client.LoginBot(s.cfg.BotToken)
	}()

	select {
	case err := <-loginDone:
		if err != nil {
			return fmt.Errorf("failed to authenticate Telegram bot token: %w", err)
		}
	case <-time.After(60 * time.Second):
		return errors.New("authentication timed out after 60s while connecting to Telegram DC")
	case <-ctx.Done():
		return ctx.Err()
	}

	botUser, err := s.client.GetMe()
	if err != nil {
		return fmt.Errorf("failed to get Telegram bot user info: %w", err)
	}

	if s.appCtx != nil {
		s.appCtx.BotUser = botUser
		if s.appCtx.FSub != nil {
			if err := s.appCtx.FSub.Init(ctx, s.client, botUser.ID); err != nil {
				s.logger.Warn("failed to initialize FSub service loadout", "error", err)
			}
		}
	}

	s.logger.Info("gogram telegram bot client authenticated successfully",
		"bot_username", botUser.Username,
		"bot_id", botUser.ID,
		"app_id", s.cfg.AppID,
	)

	s.logger.Info("🚀 Bot is now fully online, ready, and listening for updates!")

	// Send restart notification to admin(s) in background
	go s.notifyAdminsOnRestart(botUser)

	// Launch old bot user migration service in background if configured
	if s.oldBotService != nil && s.cfg != nil && s.cfg.HasOldBots() {
		go s.oldBotService.Start(ctx, botUser.Username)
	}

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

// notifyAdminsOnRestart sends a Telegram notification to the owner and configured admins
// only after the SQL migration has completed and the database is verified ready.
func (s *BotService) notifyAdminsOnRestart(botUser *telegram.UserObj) {
	if s.client == nil || botUser == nil || s.cfg == nil {
		return
	}

	// Wait for SQL migration to complete before notifying admins
	var syncRes *dbsql.SyncResult
	var syncErr error
	if s.appCtx != nil && s.appCtx.DB != nil {
		s.logger.Info("waiting for SQL media migration to complete before sending admin notification...")
		waitCtx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
		syncRes, syncErr = s.appCtx.DB.WaitForSync(waitCtx)
		cancel()
		if syncErr != nil {
			s.logger.Warn("SQL migration finished with error or wait timeout before admin notification", "error", syncErr)
		} else {
			s.logger.Info("SQL media migration verified complete, preparing admin notification")
		}
	}

	var totalMedia int64
	if s.appCtx != nil && s.appCtx.DB != nil && s.appCtx.DB.SQLManager() != nil {
		db := s.appCtx.DB.SQLManager().DB()
		if db != nil {
			_ = db.QueryRow("SELECT COUNT(*) FROM media;").Scan(&totalMedia)
		}
	}

	timeStr := time.Now().Format("2006-01-02 15:04:05 MST")
	mediaStr := humanize.Comma(totalMedia)

	var msgText string
	if syncErr != nil {
		msgText = fmt.Sprintf(
			"⚠️ <b>Bot Online (SQL Notice)</b>\n\n"+
				"🤖 <b>Bot:</b> @%s (<code>%d</code>)\n"+
				"⚠️ <b>SQL Status:</b> Migration issue: <code>%s</code>\n"+
				"📊 <b>Indexed Media:</b> <code>%s</code> records\n"+
				"⏰ <b>Time:</b> <code>%s</code>\n"+
				"🚀 <b>Status:</b> Online &amp; Listening for queries!\n\n"+
				"<i>Bot is operational, but SQL synchronization reported an issue.</i>",
			botUser.Username,
			botUser.ID,
			syncErr.Error(),
			mediaStr,
			timeStr,
		)
	} else {
		sqlStatus := "Synchronized &amp; Ready"
		if syncRes != nil && !syncRes.Skipped && syncRes.Duration > 0 {
			sqlStatus = fmt.Sprintf("Synchronized (%s)", syncRes.Duration.Round(time.Second).String())
		} else if syncRes != nil && syncRes.Skipped {
			sqlStatus = "Verified &amp; Ready"
		}

		msgText = fmt.Sprintf(
			"⚡ <b>Bot Restarted Successfully!</b>\n\n"+
				"🤖 <b>Bot:</b> @%s (<code>%d</code>)\n"+
				"📊 <b>Indexed Media:</b> <code>%s</code> records\n"+
				"💾 <b>SQL Database:</b> %s\n"+
				"⏰ <b>Restarted At:</b> <code>%s</code>\n"+
				"🚀 <b>Status:</b> Online &amp; Listening for queries!\n\n"+
				"<i>SQL migration completed successfully and all services are active.</i>",
			botUser.Username,
			botUser.ID,
			mediaStr,
			sqlStatus,
			timeStr,
		)
	}

	// Collect unique recipient IDs
	recipients := make(map[int64]bool)
	if s.cfg.OwnerID > 0 {
		recipients[s.cfg.OwnerID] = true
	}
	for _, adminID := range s.cfg.Admins {
		if adminID > 0 {
			recipients[adminID] = true
		}
	}

	for adminID := range recipients {
		_, err := s.client.SendMessage(adminID, msgText, &telegram.SendOptions{
			ParseMode: "html",
		})
		if err != nil {
			s.logger.Warn("could not send restart notification to admin (admin might need to /start the bot first)",
				"admin_id", adminID,
				"error", err,
			)
		} else {
			s.logger.Info("startup notification sent to admin (post-SQL migration)",
				"admin_id", adminID,
			)
		}
	}
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
	if s.oldBotService != nil {
		s.oldBotService.Stop()
	}
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
