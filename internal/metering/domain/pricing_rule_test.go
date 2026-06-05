package domain

import (
	"testing"
	"time"
)

func TestPricingRuleCostFor_NoModifiers(t *testing.T) {
	rule := PricingRule{BaseCost: 5}
	cost, mult := rule.CostFor(time.Now())
	if cost != 5 {
		t.Errorf("expected cost 5, got %d", cost)
	}
	if mult != 1.0 {
		t.Errorf("expected multiplier 1.0, got %f", mult)
	}
}

func TestPricingRuleCostFor_WithTimeModifier(t *testing.T) {
	rule := PricingRule{
		BaseCost: 10,
		TimeModifiers: []TimeModifier{
			{
				Hours:      "09:00-17:00",
				Days:       []string{"mon", "tue", "wed", "thu", "fri"},
				Multiplier: 1.5,
			},
		},
	}

	// Monday 10:00 should match
	mon := time.Date(2025, 6, 2, 10, 0, 0, 0, time.UTC)
	cost, mult := rule.CostFor(mon)
	if cost != 15 {
		t.Errorf("expected cost 15, got %d", cost)
	}
	if mult != 1.5 {
		t.Errorf("expected multiplier 1.5, got %f", mult)
	}

	// Saturday 10:00 should NOT match (day mismatch)
	sat := time.Date(2025, 6, 7, 10, 0, 0, 0, time.UTC)
	cost, mult = rule.CostFor(sat)
	if cost != 10 {
		t.Errorf("expected base cost 10 on Saturday, got %d", cost)
	}
	if mult != 1.0 {
		t.Errorf("expected multiplier 1.0 on Saturday, got %f", mult)
	}

	// Monday 18:00 should NOT match (hour mismatch)
	eve := time.Date(2025, 6, 2, 18, 0, 0, 0, time.UTC)
	cost, mult = rule.CostFor(eve)
	if cost != 10 {
		t.Errorf("expected base cost 10 at 18:00, got %d", cost)
	}
}

func TestPricingRuleCostFor_FirstModifierWins(t *testing.T) {
	rule := PricingRule{
		BaseCost: 10,
		TimeModifiers: []TimeModifier{
			{
				Hours:      "00:00-23:59",
				Multiplier: 2.0,
			},
			{
				Hours:      "00:00-12:00",
				Multiplier: 3.0,
			},
		},
	}

	// First modifier always matches (all-day)
	cost, mult := rule.CostFor(time.Now())
	if cost != 20 {
		t.Errorf("expected cost 20 (first modifier), got %d", cost)
	}
	if mult != 2.0 {
		t.Errorf("expected multiplier 2.0, got %f", mult)
	}
}

func TestPricingRuleCostFor_ModifierNoDays(t *testing.T) {
	rule := PricingRule{
		BaseCost: 10,
		TimeModifiers: []TimeModifier{
			{
				Hours:      "09:00-17:00",
				Multiplier: 1.5,
			},
		},
	}

	// Should match any day within hours
	sun := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC) // Sunday
	cost, mult := rule.CostFor(sun)
	if cost != 15 {
		t.Errorf("expected cost 15 on Sunday within hours, got %d", cost)
	}
	if mult != 1.5 {
		t.Errorf("expected multiplier 1.5, got %f", mult)
	}
}

func TestPricingRuleCostFor_ModifierNoHours(t *testing.T) {
	rule := PricingRule{
		BaseCost: 10,
		TimeModifiers: []TimeModifier{
			{
				Days:       []string{"mon"},
				Multiplier: 2.0,
			},
		},
	}

	// Monday any time
	mon := time.Date(2025, 6, 2, 22, 0, 0, 0, time.UTC)
	cost, mult := rule.CostFor(mon)
	if cost != 20 {
		t.Errorf("expected cost 20 on Monday, got %d", cost)
	}
	if mult != 2.0 {
		t.Errorf("expected multiplier 2.0, got %f", mult)
	}

	// Tuesday should not match
	tue := time.Date(2025, 6, 3, 10, 0, 0, 0, time.UTC)
	cost, mult = rule.CostFor(tue)
	if cost != 10 {
		t.Errorf("expected base cost 10 on Tuesday, got %d", cost)
	}
}

func TestPricingRuleCostFor_BoundaryHours(t *testing.T) {
	rule := PricingRule{
		BaseCost: 10,
		TimeModifiers: []TimeModifier{
			{
				Hours:      "09:00-17:00",
				Multiplier: 1.5,
			},
		},
	}

	// Exactly 09:00 should match
	at := time.Date(2025, 6, 2, 9, 0, 0, 0, time.UTC)
	cost, _ := rule.CostFor(at)
	if cost != 15 {
		t.Errorf("expected cost 15 at 09:00, got %d", cost)
	}

	// Exactly 17:00 should NOT match (exclusive end)
	at = time.Date(2025, 6, 2, 17, 0, 0, 0, time.UTC)
	cost, _ = rule.CostFor(at)
	if cost != 10 {
		t.Errorf("expected base cost 10 at 17:00 (exclusive), got %d", cost)
	}

	// 16:59 should match
	at = time.Date(2025, 6, 2, 16, 59, 0, 0, time.UTC)
	cost, _ = rule.CostFor(at)
	if cost != 15 {
		t.Errorf("expected cost 15 at 16:59, got %d", cost)
	}
}

func TestPricingRuleCostFor_ZeroBaseCost(t *testing.T) {
	rule := PricingRule{
		BaseCost: 0,
		TimeModifiers: []TimeModifier{
			{
				Hours:      "00:00-23:59",
				Multiplier: 2.0,
			},
		},
	}
	cost, _ := rule.CostFor(time.Now())
	if cost != 0 {
		t.Errorf("expected cost 0, got %d", cost)
	}
}

func TestParseHourRange(t *testing.T) {
	tests := []struct {
		input   string
		wantS   int
		wantE   int
		wantErr bool
	}{
		{"09:00-17:00", 540, 1020, false},
		{"00:00-23:59", 0, 1439, false},
		{"invalid", 0, 0, true},
		{"09:00-invalid", 540, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			s, e, err := parseHourRange(tt.input)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr {
				if s != tt.wantS || e != tt.wantE {
					t.Errorf("got (%d, %d), want (%d, %d)", s, e, tt.wantS, tt.wantE)
				}
			}
		})
	}
}
