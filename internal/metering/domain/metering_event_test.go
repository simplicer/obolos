package domain

import "testing"

func TestNewMeteringEvent(t *testing.T) {
	evt := NewMeteringEvent(
		"id-1",
		"user-1",
		"vatguard",
		"/v1/vat/{number}",
		1,
		1.0,
		1,
		1000,
		999,
		"prepaid",
		false,
	)

	if evt.ID != "id-1" {
		t.Errorf("expected id-1, got %s", evt.ID)
	}
	if evt.UserID != "user-1" {
		t.Errorf("expected user-1, got %s", evt.UserID)
	}
	if evt.API != "vatguard" {
		t.Errorf("expected vatguard, got %s", evt.API)
	}
	if evt.Endpoint != "/v1/vat/{number}" {
		t.Errorf("expected /v1/vat/{number}, got %s", evt.Endpoint)
	}
	if evt.BaseCost != 1 {
		t.Errorf("expected base_cost 1, got %d", evt.BaseCost)
	}
	if evt.FinalCost != 1 {
		t.Errorf("expected final_cost 1, got %d", evt.FinalCost)
	}
	if evt.BalanceBefore != 1000 {
		t.Errorf("expected balance_before 1000, got %d", evt.BalanceBefore)
	}
	if evt.BalanceAfter != 999 {
		t.Errorf("expected balance_after 999, got %d", evt.BalanceAfter)
	}
	if evt.PlanType != "prepaid" {
		t.Errorf("expected prepaid, got %s", evt.PlanType)
	}
	if evt.Overage {
		t.Error("expected overage to be false")
	}
	if evt.CreatedAt.IsZero() {
		t.Error("expected created_at to be set")
	}
}
