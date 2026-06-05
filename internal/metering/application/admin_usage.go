package application

import (
	"context"
	"fmt"
	"time"
)

// GetAdminUsageUseCase retrieves usage history for any user (admin).
type GetAdminUsageUseCase struct {
	events EventRepository
}

// NewGetAdminUsageUseCase creates a new admin usage use case.
func NewGetAdminUsageUseCase(events EventRepository) *GetAdminUsageUseCase {
	return &GetAdminUsageUseCase{events: events}
}

// Execute returns usage events for the specified user within the date range.
func (uc *GetAdminUsageUseCase) Execute(ctx context.Context, userID string, from, to time.Time) (*AdminUsageResult, error) {
	events, err := uc.events.FindByUserAndDateRange(ctx, userID, from, to, defaultUsageLimit)
	if err != nil {
		return nil, fmt.Errorf("usage lookup failed: %w", err)
	}

	var totalCost, overageCost int64
	for _, e := range events {
		totalCost += e.FinalCost
		// Only the portion of this charge that crossed below zero is overage,
		// not the whole final cost: depth(after) - depth(before).
		if portion := negDepth(e.BalanceAfter) - negDepth(e.BalanceBefore); portion > 0 {
			overageCost += portion
		}
	}

	return &AdminUsageResult{
		Events:      events,
		TotalCost:   totalCost,
		OverageCost: overageCost,
	}, nil
}

// negDepth is how far below zero a balance is (0 when non-negative).
func negDepth(balance int64) int64 {
	if balance < 0 {
		return -balance
	}
	return 0
}
