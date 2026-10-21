package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/itsafxal/MYTGBOTZ/internal/app"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/logging"
	"github.com/itsafxal/MYTGBOTZ/plugins/autodelete"
	"github.com/itsafxal/MYTGBOTZ/plugins/autofilter"
	"github.com/itsafxal/MYTGBOTZ/plugins/caption"
	"github.com/itsafxal/MYTGBOTZ/plugins/cmds"
	"github.com/itsafxal/MYTGBOTZ/plugins/delivery"
	"github.com/itsafxal/MYTGBOTZ/plugins/fsub"
	"github.com/itsafxal/MYTGBOTZ/plugins/id"
	"github.com/itsafxal/MYTGBOTZ/plugins/info"
	"github.com/itsafxal/MYTGBOTZ/plugins/latest"
	"github.com/itsafxal/MYTGBOTZ/plugins/ping"
	"github.com/itsafxal/MYTGBOTZ/plugins/start"
	"github.com/itsafxal/MYTGBOTZ/plugins/channel"
	"github.com/itsafxal/MYTGBOTZ/plugins/index"
	"github.com/itsafxal/MYTGBOTZ/plugins/botstats"
	streamPlugin "github.com/itsafxal/MYTGBOTZ/plugins/stream"
)


func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Initial logger for bootstrapping
	bootstrapLogger := logging.NewLogger(config.LogLevelInfo, os.Stdout)
	bootstrapLogger.Info("starting bot application initialization...")

	// 2. Load and validate environment configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("configuration validation failure: %w", err)
	}

	// 3. Configure application logger with validated log level
	logger := logging.NewLogger(cfg.LogLevel, os.Stdout)
	logger.Info("configuration loaded successfully",
		"owner_id", cfg.OwnerID,
		"admins_count", len(cfg.Admins),
		"log_level", string(cfg.LogLevel),
		"bot_token", cfg.MaskedToken(),
		"app_id", cfg.AppID,
		"app_hash", cfg.MaskedAppHash(),
	)

	// 4. Initialize application container
	application, err := app.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("application initialization failure: %w", err)
	}

	// 5. Explicit compile-time plugin registration
	logger.Info("registering core plugins...")
	err = application.RegisterPlugins(
		start.New(application.AppContext),
		cmds.New(application.AppContext),
		ping.New(application.AppContext),
		id.New(application.AppContext),
		info.New(application.AppContext),
		delivery.New(application.AppContext),
		autofilter.New(application.AppContext),
		fsub.New(application.AppContext, application.FSubService),
		caption.New(application.AppContext),
		latest.New(application.AppContext),
		autodelete.New(application.AppContext, application.AutoDeleteService),
		streamPlugin.New(application.AppContext),
		channel.New(application.AppContext),
		index.New(application.AppContext),
		botstats.New(application.AppContext),
	)
	if err != nil {
		return fmt.Errorf("plugin registration failure: %w", err)
	}


	// 6. Setup signal notification context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// 7. Run application
	logger.Info("application ready, starting services...")
	if err := application.Run(ctx); err != nil {
		return fmt.Errorf("application runtime error: %w", err)
	}

	logger.Info("application stopped cleanly")
	return nil
}
