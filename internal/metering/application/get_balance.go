package application

import (
	"context"
	"fmt"
	"time"
)

// GetBalanceUseCase retrieves a user's balance and monthly usage.
type GetBalanceUseCase struct {
	users  UserRepository
	events EventRepository
}

// NewGetBalanceUseCase creates a new GetBalance use case.
func NewGetBalanceUseCase(users UserRepository, events EventRepository) *GetBalanceUseCase {
	return &GetBalanceUseCase{users: users, events: events}
}

// Execute returns the balance for the given user.
func (uc *GetBalanceUseCase) Execute(ctx context.Context, userID string) (*BalanceResult, error) {
	user, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	// Calculate total usage this month using SQL aggregation (constant memory)
	n := now()
	startOfMonth := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
	totalUsage, err := uc.events.SumCostByUserAndDateRange(ctx, userID, startOfMonth, n)
	if err != nil {
		return nil, fmt.Errorf("usage lookup failed: %w", err)
	}

	return &BalanceResult{
		UserID:              user.ID,
		Balance:             user.Balance.Value(),
		PlanType:            user.Plan.String(),
		TotalUsageThisMonth: totalUsage,
	}, nil
}
