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

	plan := domain.Plan{Type: pt, OverageRate: input.OverageRate}
	if err := uc.users.SetPlan(ctx, input.UserID, plan, input.Credits); err != nil {
		if err == ErrUserNotFound {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("plan update failed: %w", err)
	}

	return &SetPlanResult{
		UserID:   input.UserID,
		PlanType: string(pt),
		Credits:  input.Credits,
	}, nil
}
