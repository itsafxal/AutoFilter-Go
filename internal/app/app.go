package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
	"github.com/itsafxal/MYTGBOTZ/internal/search"
	"github.com/itsafxal/MYTGBOTZ/internal/telegram"
)

// App coordinates the lifecycle of configuration, plugins, logging, database, and Telegram services.
type App struct {
	Config          *config.Config
	Logger          *slog.Logger
	Registry        *plugins.Registry
	AppContext      *plugins.AppContext
	BotService      *telegram.BotService
	DBManager       *database.DatabaseManager
	DeliveryService delivery.MediaDeliveryService
	SearchService   search.SearchService
	StartTime       time.Time
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
	searchService := search.NewService(dbManager, logger)

	appCtx := &plugins.AppContext{
		Config:    cfg,
		Logger:    logger,
		StartTime: startTime,
		DB:        dbManager,
		Delivery:  deliveryService,
		Search:    searchService,
	}

	reg := plugins.NewRegistry(appCtx)
	botService := telegram.NewBotService(cfg, logger, reg, appCtx)

	return &App{
		Config:          cfg,
		Logger:          logger,
		Registry:        reg,
		AppContext:      appCtx,
		BotService:      botService,
		DBManager:       dbManager,
		DeliveryService: deliveryService,
		SearchService:   searchService,
		StartTime:       startTime,
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
	a.Logger.Info("database layer initialized and ready")
	return nil
}

// InitBot initializes the Telegram Bot API client and dispatcher.
func (a *App) InitBot() error {
	a.Logger.Info("initializing telegram bot client...")
	if err := a.BotService.Init(); err != nil {
		return err
	}
	if a.DeliveryService != nil {
		a.DeliveryService.SetSender(a.BotService.Bot())
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

	if a.BotService != nil {
		if err := a.BotService.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("bot shutdown error: %w", err))
		}
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
