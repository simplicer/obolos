package infrastructure

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codeberg.org/simplicer/metering/internal/metering/domain"
)

// PricingEngine implements application.PricingConfig by loading rules from JSON.
type PricingEngine struct {
	mu     sync.RWMutex
	config PricingFile
	logger *slog.Logger
}

// PricingFile represents the top-level pricing JSON structure.
type PricingFile struct {
	APIs        map[string]APIPricing `json:"apis"`
	DefaultCost int64                 `json:"default_cost"`
}

// APIPricing represents pricing for a single API.
type APIPricing struct {
	Endpoints   map[string]domain.PricingRule `json:"endpoints"`
	DefaultCost *int64                        `json:"default_cost,omitempty"`
}

// NewPricingEngine creates a new pricing engine with the given initial config.
func NewPricingEngine(config PricingFile, logger *slog.Logger) *PricingEngine {
	return &PricingEngine{
		config: config,
		logger: logger,
	}
}

// CostFor looks up the cost for an API+endpoint at the given time.
func (e *PricingEngine) CostFor(_ context.Context, api, endpoint string, t time.Time) (baseCost, finalCost int64, multiplier float64, err error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Find the API
	apiConfig, ok := e.config.APIs[api]
	if !ok {
		// Use global default
		baseCost = e.config.DefaultCost
		finalCost = baseCost
		multiplier = 1.0
		e.logger.Debug("api not found in pricing, using default",
			slog.String("api", api),
			slog.Int64("default_cost", baseCost),
		)
		return baseCost, finalCost, multiplier, nil
	}

	// Find the endpoint
	rule, ok := apiConfig.Endpoints[endpoint]
	if !ok {
		// Use API default or global default
		if apiConfig.DefaultCost != nil {
			baseCost = *apiConfig.DefaultCost
		} else {
			baseCost = e.config.DefaultCost
		}
		finalCost = baseCost
		multiplier = 1.0
		e.logger.Debug("endpoint not found in pricing, using default",
			slog.String("api", api),
			slog.String("endpoint", endpoint),
			slog.Int64("default_cost", baseCost),
		)
		return baseCost, finalCost, multiplier, nil
	}

	// Calculate cost with time modifiers
	finalCost, multiplier = rule.CostFor(t)
	baseCost = rule.BaseCost

	e.logger.Debug("pricing resolved",
		slog.String("api", api),
		slog.String("endpoint", endpoint),
		slog.Int64("base_cost", baseCost),
		slog.Float64("multiplier", multiplier),
		slog.Int64("final_cost", finalCost),
	)

	return baseCost, finalCost, multiplier, nil
}

// Reload replaces the pricing configuration atomically.
func (e *PricingEngine) Reload(config PricingFile) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config = config
	e.logger.Info("pricing config reloaded",
		slog.Int("api_count", len(config.APIs)),
	)
}

// LoadPricingFromFile reads a pricing JSON file and returns the config.
func LoadPricingFromFile(path string) (PricingFile, error) {
	return loadPricingConfig(path)
}

// LoadPricingFromURL fetches pricing config from a remote URL.
func LoadPricingFromURL(url string) (PricingFile, error) {
	if url == "" {
		return PricingFile{}, fmt.Errorf("empty URL")
	}
	return fetchPricingConfig(url)
}
