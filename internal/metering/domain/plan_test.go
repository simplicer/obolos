package domain

import "testing"

func TestParsePlanType(t *testing.T) {
	tests := []struct {
		input string
		want  PlanType
		err   bool
	}{
		{"prepaid", PlanTypePrepaid, false},
		{"unlimited", PlanTypeUnlimited, false},
		{"hybrid", PlanTypeHybrid, false},
		{"unknown", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePlanType(tt.input)
			if tt.err && err == nil {
				t.Error("expected error")
			}
			if !tt.err && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlanShouldDeduct(t *testing.T) {
	tests := []struct {
		planType PlanType
		want     bool
	}{
		{PlanTypePrepaid, true},
		{PlanTypeUnlimited, false},
		{PlanTypeHybrid, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.planType), func(t *testing.T) {
			p := Plan{Type: tt.planType}
			if got := p.ShouldDeduct(); got != tt.want {
				t.Errorf("ShouldDeduct() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlanCanDeduct(t *testing.T) {
	tests := []struct {
		name    string
		plan    Plan
		balance int64
		cost    int64
		want    bool
	}{
		{"prepaid sufficient", Plan{Type: PlanTypePrepaid}, 100, 50, true},
		{"prepaid insufficient", Plan{Type: PlanTypePrepaid}, 40, 50, false},
		{"prepaid exact", Plan{Type: PlanTypePrepaid}, 50, 50, true},
		{"prepaid zero balance", Plan{Type: PlanTypePrepaid}, 0, 1, false},
		{"unlimited always passes", Plan{Type: PlanTypeUnlimited}, 0, 999, true},
		{"hybrid sufficient", Plan{Type: PlanTypeHybrid}, 100, 50, true},
		{"hybrid allows negative", Plan{Type: PlanTypeHybrid}, 0, 50, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBalance(tt.balance)
			if got := tt.plan.CanDeduct(tt.cost, b); got != tt.want {
				t.Errorf("CanDeduct() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlanIsOverage(t *testing.T) {
	tests := []struct {
		name    string
		plan    Plan
		balance int64
		want    bool
	}{
		{"hybrid negative is overage", Plan{Type: PlanTypeHybrid}, -10, true},
		{"hybrid zero not overage", Plan{Type: PlanTypeHybrid}, 0, false},
		{"hybrid positive not overage", Plan{Type: PlanTypeHybrid}, 100, false},
		{"prepaid negative impossible", Plan{Type: PlanTypePrepaid}, -10, false},
		{"unlimited negative impossible", Plan{Type: PlanTypeUnlimited}, -10, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBalance(tt.balance)
			if got := tt.plan.IsOverage(b); got != tt.want {
				t.Errorf("IsOverage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewPlans(t *testing.T) {
	p := NewPrepaidPlan()
	if p.Type != PlanTypePrepaid {
		t.Errorf("expected prepaid, got %v", p.Type)
	}

	u := NewUnlimitedPlan()
	if u.Type != PlanTypeUnlimited {
		t.Errorf("expected unlimited, got %v", u.Type)
	}

	h := NewHybridPlan(0.05)
	if h.Type != PlanTypeHybrid || h.OverageRate != 0.05 {
		t.Errorf("expected hybrid with rate 0.05, got %v rate %v", h.Type, h.OverageRate)
	}
}
