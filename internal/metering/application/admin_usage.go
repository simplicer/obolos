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
		if e.Overage {
			overageCost += e.FinalCost
		}
	}

	return &AdminUsageResult{
		Events:      events,
		TotalCost:   totalCost,
		OverageCost: overageCost,
	}, nil
}
