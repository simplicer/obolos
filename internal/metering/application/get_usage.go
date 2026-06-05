package application

import (
	"context"
	"fmt"
	"time"
)

const defaultUsageLimit = 1000

// GetUsageUseCase retrieves a user's usage history.
type GetUsageUseCase struct {
	events EventRepository
}

// NewGetUsageUseCase creates a new GetUsage use case.
func NewGetUsageUseCase(events EventRepository) *GetUsageUseCase {
	return &GetUsageUseCase{events: events}
}

// Execute returns usage events for the user within the given date range.
func (uc *GetUsageUseCase) Execute(ctx context.Context, userID string, from, to time.Time) (*UsageResult, error) {
	events, err := uc.events.FindByUserAndDateRange(ctx, userID, from, to, defaultUsageLimit)
	if err != nil {
		return nil, fmt.Errorf("usage lookup failed: %w", err)
	}

	var totalCost int64
	for _, e := range events {
		totalCost += e.FinalCost
	}

	return &UsageResult{
		Events:        events,
		TotalCost:     totalCost,
		TotalRequests: len(events),
	}, nil
}
