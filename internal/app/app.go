package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/itsafxal/MYTGBOTZ/internal/autodelete"
	"github.com/itsafxal/MYTGBOTZ/internal/caption"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/fsub"
	"github.com/itsafxal/MYTGBOTZ/internal/oldbot"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
	"github.com/itsafxal/MYTGBOTZ/internal/search"
	"github.com/itsafxal/MYTGBOTZ/internal/stream"
	"github.com/itsafxal/MYTGBOTZ/internal/telegram"
)

// App coordinates the lifecycle of configuration, plugins, logging, database, and Telegram services.
type App struct {
	Config            *config.Config
	Logger            *slog.Logger
	Registry          *plugins.Registry
	AppContext        *plugins.AppContext
	BotService        *telegram.BotService
	OldBotService     *oldbot.Service
	DBManager         *database.DatabaseManager
	DeliveryService   delivery.MediaDeliveryService
	SearchService     search.SearchService
	FSubService       *fsub.Service
	CaptionService    caption.CaptionService
	AutoDeleteService autodelete.AutoDeleteService
	StreamService     stream.StreamService
	StartTime         time.Time
}

// New creates and configures a new App instance.
func New(cfg *config.Config, logger *slog.Logger) (*App, error) {
	if cfg == nil {
		return nil, errors.New("configuration cannot be nil")
	}
	if logger == nil {
		return nil, errors.New("logger cannot be nil")
	}

	startTime := time.Now()
	dbManager := database.NewDatabaseManager(cfg, logger)
	deliveryService := delivery.NewService(dbManager, logger)
	deliveryService.SetStreamConfig(cfg.StreamURL, cfg.EnableFastDownloadButton)
	deliveryService.SetPremiumEmojiConfig(cfg.EnablePremiumEmoji, cfg.FastDownloadButtonEmojiID)

	searchService := search.NewService(dbManager, logger)
	fsubRepo := fsub.NewRepositoryWithProvider(dbManager.MongoPrimaryDB)
	fsubService := fsub.NewService(fsubRepo, cfg, logger)

	sqlProv := func() *sql.DB {
		if dbManager.SQLManager() != nil {
			return dbManager.SQLManager().DB()
		}
		return nil
	}
	captionService := caption.NewService(cfg.CustomCaption, dbManager.MongoPrimaryDB, sqlProv, logger)
	deliveryService.SetCaptionFormatter(captionService)

	autoDeleteService := autodelete.NewService(cfg, sqlProv, dbManager.MongoPrimaryDB, logger)

	streamConfig := stream.Config{
		BaseURL:     cfg.StreamURL,
		Port:        cfg.StreamPort,
		BindAddr:    cfg.StreamBindAddr,
		BotUsername: "",
		Enabled:     cfg.StreamServerEnabled,
	}
	streamService := stream.NewServer(streamConfig, deliveryService, nil, logger)

	appCtx := &plugins.AppContext{
		Config:     cfg,
		Logger:     logger,
		StartTime:  startTime,
		DB:         dbManager,
		Delivery:   deliveryService,
		Search:     searchService,
		FSub:       fsubService,
		Caption:    captionService,
		AutoDelete: autoDeleteService,
		Stream:     streamService,
	}

	reg := plugins.NewRegistry(appCtx)
	botService := telegram.NewBotService(cfg, logger, reg, appCtx)

	oldBotService := oldbot.NewService(oldbot.Config{
		Tokens:          cfg.OldBotTokens,
		AppID:           cfg.AppID,
		AppHash:         cfg.AppHash,
		MainBotUsername: "",
	}, searchService, logger)
	botService.SetOldBotService(oldBotService)

	return &App{
		Config:            cfg,
		Logger:            logger,
		Registry:          reg,
		AppContext:        appCtx,
		BotService:        botService,
		OldBotService:     oldBotService,
		DBManager:         dbManager,
		DeliveryService:   deliveryService,
		SearchService:     searchService,
		FSubService:       fsubService,
		CaptionService:    captionService,
		AutoDeleteService: autoDeleteService,
		StreamService:     streamService,
		StartTime:         startTime,
	}, nil
}


// RegisterPlugins registers one or more plugins into the application registry.
func (a *App) RegisterPlugins(plugs ...plugins.Plugin) error {
	return a.Registry.Register(plugs...)
}

// ValidatePlugins validates all registered plugins, command mappings, and aliases.
func (a *App) ValidatePlugins() error {
	a.Logger.Info("validating registered plugins and command mappings...")
	if err := a.Registry.Validate(); err != nil {
		return fmt.Errorf("plugin validation failed: %w", err)
	}

	a.Logger.Info("plugin registry validation successful",
		"plugins_count", a.Registry.PluginCount(),
		"commands_count", a.Registry.CommandCount(),
	)
	return nil
}

// InitDatabase initializes MongoDB and performs runtime media synchronization to SQLite.
// The bot does not report the database as ready until required initialization and sync have succeeded.
func (a *App) InitDatabase(ctx context.Context) error {
	a.Logger.Info("initializing database layer...")
	if err := a.DBManager.Init(ctx); err != nil {
		return fmt.Errorf("database initialization failure: %w", err)
	}
	if a.CaptionService != nil {
		if err := a.CaptionService.Init(ctx); err != nil {
			a.Logger.Warn("failed initializing caption service from database", "error", err)
		}
	}
	a.Logger.Info("database layer initialized and ready")
	return nil
}


// InitBot initializes the Telegram MTProto client and dispatcher.
func (a *App) InitBot() error {
	a.Logger.Info("initializing telegram bot client...")
	if err := a.BotService.Init(); err != nil {
		return err
	}
	if a.DeliveryService != nil {
		a.DeliveryService.SetSender(a.BotService.Client())
		if a.Config != nil {
			a.DeliveryService.SetAdminConfig(a.Config.OwnerID, a.Config.Admins)
		}
	}
	if a.AutoDeleteService != nil {
		_ = a.AutoDeleteService.Init(context.Background(), a.BotService.Client())
		a.AutoDeleteService.Start(context.Background())
	}
	if a.StreamService != nil {
		if srv, ok := a.StreamService.(*stream.Server); ok {
			if a.DeliveryService != nil {
				srv.SetRefresher(a.DeliveryService)
			}
			if a.BotService != nil && a.BotService.Client() != nil {
				// Primary authenticated bot client matches the delivery session and file references
				srv.SetDownloader(a.BotService.Client())
				a.Logger.Info("stream service initialized with primary bot MTProto client")
			}
		}
		if a.Config != nil && a.Config.StreamServerEnabled {
			_ = a.StreamService.Start(context.Background())
		}
	}
	return nil
}

// Run starts the application and begins listening for updates until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	if err := a.ValidatePlugins(); err != nil {
		return err
	}

	if err := a.InitDatabase(ctx); err != nil {
		return err
	}

	if err := a.InitBot(); err != nil {
		return err
	}

	return a.BotService.Start(ctx)
}

// Shutdown gracefully stops all running bot services and cleans up database connections.
func (a *App) Shutdown() error {
	a.Logger.Info("initiating graceful application shutdown...")
	var errs []error

	if a.StreamService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := a.StreamService.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stream server shutdown error: %w", err))
		}
		cancel()
	}

	if a.AutoDeleteService != nil {
		a.AutoDeleteService.Stop()
	}

	if a.BotService != nil {
		if err := a.BotService.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("bot shutdown error: %w", err))
		}
	}

	if a.OldBotService != nil {
		a.OldBotService.Stop()
	}

	if a.DBManager != nil {
		if err := a.DBManager.Close(); err != nil {
			errs = append(errs, fmt.Errorf("database shutdown error: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}
