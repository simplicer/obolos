package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeberg.org/simplicer/metering/internal/metering/application"
	"codeberg.org/simplicer/metering/internal/metering/infrastructure"
)

// Build-time variables set via ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	// Initialize structured logger (JSON, Loki-compatible)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("starting tollgate service",
		slog.String("version", Version),
		slog.String("commit", Commit),
		slog.String("build_time", BuildTime),
	)

	// Set version in infrastructure package
	infrastructure.Version = Version

	// Load configuration
	configLoader := infrastructure.NewConfigLoader(infrastructure.DefaultServiceConfig(), logger)

	// Try loading from local config file
	configPath := "etc/config.json"
	if _, err := os.Stat(configPath); err == nil {
		if err := configLoader.LoadFromFile(configPath); err != nil {
			logger.Error("failed to load config file", slog.String("error", err.Error()))
		}
	}

	// Apply environment variable overrides
	configLoader.ApplyEnvOverrides()

	cfg := configLoader.Get()

	// Set log level from config
	switch cfg.LogLevel {
	case "debug":
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		slog.SetDefault(logger)
	case "warn":
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
		slog.SetDefault(logger)
	case "error":
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
		slog.SetDefault(logger)
	}

	// Load pricing configuration
	pricingConfig := infrastructure.PricingFile{DefaultCost: 1}
	pricingPath := "etc/pricing.json"
	if _, err := os.Stat(pricingPath); err == nil {
		pricingConfig, err = infrastructure.LoadPricingFromFile(pricingPath)
		if err != nil {
			logger.Error("failed to load pricing config", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
	pricingEngine := infrastructure.NewPricingEngine(pricingConfig, logger)

	// Connect to PostgreSQL
	ctx := context.Background()
	store, err := infrastructure.NewPostgresStore(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		logger.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer store.Close()

	// Wire up use cases — store implements Meterer for atomic transactions
	meterUC := application.NewMeterUseCase(store, store, pricingEngine, store)
	getBalanceUC := application.NewGetBalanceUseCase(store, store)
	getUsageUC := application.NewGetUsageUseCase(store)
	topUpUC := application.NewTopUpUseCase(store)
	setPlanUC := application.NewSetPlanUseCase(store)
	adminUsageUC := application.NewGetAdminUsageUseCase(store)
	listUsersUC := application.NewListUsersUseCase(store)

	// Create handler
	handler := infrastructure.NewHandler(
		meterUC, getBalanceUC, getUsageUC, topUpUC, setPlanUC, adminUsageUC, listUsersUC,
		store, logger, configLoader,
	)

	// Create router
	router := infrastructure.NewRouter(handler, logger)

	// Start config refresh loop (also reloads pricing config)
	startConfigRefresh(ctx, configLoader, pricingEngine, pricingPath, logger)

	// Start HTTP server
	addr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		for {
			sig := <-sigCh
			if sig == syscall.SIGHUP {
				logger.Info("SIGHUP received, reloading config")
				reloadAllConfig(configLoader, pricingEngine, pricingPath, logger)
				continue
			}
			logger.Info("received signal, shutting down", slog.String("signal", sig.String()))

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				logger.Error("server shutdown error", slog.String("error", err.Error()))
			}
			return
		}
	}()

	logger.Info("tollgate service listening", slog.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("tollgate service stopped")
}

// startConfigRefresh periodically reloads config and pricing.
func startConfigRefresh(ctx context.Context, cl *infrastructure.ConfigLoader, pe *infrastructure.PricingEngine, pricingPath string, logger *slog.Logger) {
	go func() {
		cfg := cl.Get()
		interval := time.Duration(cfg.ConfigRefreshIntervalSecs) * time.Second
		if interval < 10*time.Second {
			interval = 10 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reloadAllConfig(cl, pe, pricingPath, logger)
			}
		}
	}()
}

// reloadAllConfig reloads both service config and pricing config.
func reloadAllConfig(cl *infrastructure.ConfigLoader, pe *infrastructure.PricingEngine, pricingPath string, logger *slog.Logger) {
	// Reload pricing config
	if pricingPath != "" {
		if newPricing, err := infrastructure.LoadPricingFromFile(pricingPath); err == nil {
			pe.Reload(newPricing)
		} else {
			logger.Error("failed to reload pricing", slog.String("error", err.Error()))
		}
	}

	// Reload remote config if configured
	cfg := cl.Get()
	if cfg.ConfigRemoteURL != "" {
		if err := cl.LoadFromURL(cfg.ConfigRemoteURL); err != nil {
			logger.Error("failed to reload remote config", slog.String("error", err.Error()))
		}
	}
}
