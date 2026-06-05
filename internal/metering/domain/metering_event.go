package domain

import (
	"time"
)

// MeteringEvent records a single metered API request.
type MeteringEvent struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	API               string    `json:"api"`
	Endpoint          string    `json:"endpoint"`
	BaseCost          int64     `json:"base_cost"`
	AppliedMultiplier float64   `json:"applied_multiplier"`
	FinalCost         int64     `json:"final_cost"`
	BalanceBefore     int64     `json:"balance_before"`
	BalanceAfter      int64     `json:"balance_after"`
	PlanType          string    `json:"plan_type"`
	Overage           bool      `json:"overage"`
	RequestID         string    `json:"request_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// NewMeteringEvent creates a new event with the given details.
func NewMeteringEvent(
	id, userID, api, endpoint string,
	baseCost int64,
	appliedMultiplier float64,
	finalCost int64,
	balanceBefore, balanceAfter int64,
	planType string,
	overage bool,
) MeteringEvent {
	return MeteringEvent{
		ID:                id,
		UserID:            userID,
		API:               api,
		Endpoint:          endpoint,
		BaseCost:          baseCost,
		AppliedMultiplier: appliedMultiplier,
		FinalCost:         finalCost,
		BalanceBefore:     balanceBefore,
		BalanceAfter:      balanceAfter,
		PlanType:          planType,
		Overage:           overage,
		CreatedAt:         time.Now().UTC(),
	}
}
