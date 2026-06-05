package infrastructure

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"codeberg.org/simplicer/metering/internal/metering/domain"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestPricingEngine_CostFor_KnownEndpoint(t *testing.T) {
	config := PricingFile{
		APIs: map[string]APIPricing{
			"vatguard": {
				Endpoints: map[string]domain.PricingRule{
					"/v1/vat/{number}":  {BaseCost: 1},
					"/v1/vies/{number}": {BaseCost: 5},
				},
			},
		},
		DefaultCost: 1,
	}

	engine := NewPricingEngine(config, testLogger())
	baseCost, finalCost, mult, err := engine.CostFor(context.Background(), "vatguard", "/v1/vat/{number}", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if baseCost != 1 || finalCost != 1 || mult != 1.0 {
		t.Errorf("got base=%d final=%d mult=%f, want 1/1/1.0", baseCost, finalCost, mult)
	}
}

func TestPricingEngine_CostFor_UnknownAPI(t *testing.T) {
	config := PricingFile{
		DefaultCost: 2,
	}

	engine := NewPricingEngine(config, testLogger())
	_, finalCost, _, err := engine.CostFor(context.Background(), "unknown-api", "/v1/test", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalCost != 2 {
		t.Errorf("expected default cost 2, got %d", finalCost)
	}
}

func TestPricingEngine_CostFor_UnknownEndpoint(t *testing.T) {
	cost := int64(3)
	config := PricingFile{
		APIs: map[string]APIPricing{
			"api-2": {
				Endpoints:   map[string]domain.PricingRule{},
				DefaultCost: &cost,
			},
		},
		DefaultCost: 1,
	}

	engine := NewPricingEngine(config, testLogger())
	_, finalCost, _, err := engine.CostFor(context.Background(), "api-2", "/v1/unknown", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finalCost != 3 {
		t.Errorf("expected API default cost 3, got %d", finalCost)
	}
}

func TestPricingEngine_Reload(t *testing.T) {
	config := PricingFile{DefaultCost: 1}
	engine := NewPricingEngine(config, testLogger())

	// Before reload
	_, cost, _, _ := engine.CostFor(context.Background(), "api", "/ep", time.Now())
	if cost != 1 {
		t.Errorf("expected default 1, got %d", cost)
	}

	// Reload with new config
	newConfig := PricingFile{DefaultCost: 5}
	engine.Reload(newConfig)

	_, cost, _, _ = engine.CostFor(context.Background(), "api", "/ep", time.Now())
	if cost != 5 {
		t.Errorf("expected default 5 after reload, got %d", cost)
	}
}

func TestLoadPricingConfig(t *testing.T) {
	// Create a temp file
	tmpFile, err := os.CreateTemp("", "pricing-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	content := `{
		"apis": {
			"test": {
				"endpoints": {
					"/v1/test": {"base_cost": 3}
				}
			}
		},
		"default_cost": 1
	}`
	tmpFile.WriteString(content)
	tmpFile.Close()

	config, err := loadPricingConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.DefaultCost != 1 {
		t.Errorf("expected default_cost 1, got %d", config.DefaultCost)
	}
	if _, ok := config.APIs["test"]; !ok {
		t.Error("expected 'test' API in config")
	}
}

func TestLoadPricingConfig_InvalidPath(t *testing.T) {
	_, err := loadPricingConfig("/nonexistent/pricing.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
