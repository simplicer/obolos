package infrastructure

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

// ServiceConfig represents the service configuration.
type ServiceConfig struct {
	Port                      int    `json:"port"`
	LogLevel                  string `json:"log_level"`
	DatabaseURL               string `json:"database_url"`
	DefaultPlanType           string `json:"default_plan_type"`
	DefaultPlanCredits        int64  `json:"default_plan_credits"`
	ConfigRemoteURL           string `json:"config_remote_url"`
	ConfigRefreshIntervalSecs int    `json:"config_refresh_interval_seconds"`
}

// DefaultServiceConfig returns a config with sensible defaults.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		Port:                      8080,
		LogLevel:                  "info",
		DatabaseURL:               "postgres://metering:metering@localhost:5432/metering?sslmode=disable",
		DefaultPlanType:           "prepaid",
		DefaultPlanCredits:        1000,
		ConfigRefreshIntervalSecs: 60,
	}
}

// ConfigLoader manages service configuration from multiple sources.
type ConfigLoader struct {
	mu     sync.RWMutex
	config ServiceConfig
	logger *slog.Logger
}

// NewConfigLoader creates a config loader with the initial config.
func NewConfigLoader(config ServiceConfig, logger *slog.Logger) *ConfigLoader {
	return &ConfigLoader{config: config, logger: logger}
}

// Get returns the current configuration.
func (cl *ConfigLoader) Get() ServiceConfig {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.config
}

// LoadFromFile reads service config from a JSON file.
func (cl *ConfigLoader) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	var config ServiceConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse config JSON: %w", err)
	}
	cl.Reload(config)
	return nil
}

// Reload replaces the configuration atomically.
func (cl *ConfigLoader) Reload(config ServiceConfig) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.config = config
	cl.logger.Info("service config reloaded")
}

// LoadFromURL fetches config from a remote HTTP endpoint.
func (cl *ConfigLoader) LoadFromURL(url string) error {
	if url == "" {
		return nil
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetch config from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("config endpoint returned %d", resp.StatusCode)
	}

	var config ServiceConfig
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return fmt.Errorf("parse remote config: %w", err)
	}

	cl.Reload(config)
	return nil
}

// StartRefreshLoop starts a goroutine that periodically fetches config from the remote URL.
func (cl *ConfigLoader) StartRefreshLoop(ctx interface{ Done() <-chan struct{} }) {
	go func() {
		cfg := cl.Get()
		if cfg.ConfigRemoteURL == "" {
			return
		}
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
				cfg := cl.Get()
				if err := cl.LoadFromURL(cfg.ConfigRemoteURL); err != nil {
					cl.logger.Error("failed to refresh remote config",
						slog.String("error", err.Error()),
					)
				}
			}
		}
	}()
}

// ApplyEnvOverrides applies environment variable overrides to the config.
func (cl *ConfigLoader) ApplyEnvOverrides() {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if v := os.Getenv("PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cl.config.Port)
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cl.config.LogLevel = v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cl.config.DatabaseURL = v
	}
	if v := os.Getenv("CONFIG_REMOTE_URL"); v != "" {
		cl.config.ConfigRemoteURL = v
	}
}
