package domain

import (
	"time"
)

// MeteringEvent records a single metered API request.
type MeteringEvent struct {
	ID                string
	UserID            string
	API               string
	Endpoint          string
	BaseCost          int64
	AppliedMultiplier float64
	FinalCost         int64
	BalanceBefore     int64
	BalanceAfter      int64
	PlanType          string
	Overage           bool
	CreatedAt         time.Time
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
