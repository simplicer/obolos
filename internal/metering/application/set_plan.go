package application

import (
	"context"
	"fmt"

	"codeberg.org/simplicer/metering/internal/metering/domain"
)

// SetPlanUseCase changes a user's plan and optionally resets their credits.
type SetPlanUseCase struct {
	users UserRepository
}

// NewSetPlanUseCase creates a new SetPlan use case.
func NewSetPlanUseCase(users UserRepository) *SetPlanUseCase {
	return &SetPlanUseCase{users: users}
}

// SetPlanInput contains the parameters for a plan change.
type SetPlanInput struct {
	UserID      string
	PlanType    string
	Credits     int64
	OverageRate float64
}

// Execute sets the user's plan and credits.
func (uc *SetPlanUseCase) Execute(ctx context.Context, input SetPlanInput) (*SetPlanResult, error) {
	pt, err := domain.ParsePlanType(input.PlanType)
	if err != nil {
		return nil, ErrInvalidPlanType
	}

	user, err := uc.users.FindByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	user.Plan = domain.Plan{Type: pt, OverageRate: input.OverageRate}
	user.Balance = domain.NewBalance(input.Credits)

	if err := uc.users.Upsert(ctx, user); err != nil {
		return nil, fmt.Errorf("plan update failed: %w", err)
	}

	return &SetPlanResult{
		UserID:   user.ID,
		PlanType: string(pt),
		Credits:  input.Credits,
	}, nil
}
