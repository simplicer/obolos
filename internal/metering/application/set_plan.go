package application

import (
	"context"
	"fmt"

	"codeberg.org/simplicer/obolos/internal/metering/domain"
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
	UserID       string
	PlanType     string
	Credits      int64
	OverageRate  float64
	Entitlements domain.Entitlements
}

// Execute sets the user's plan and credits.
func (uc *SetPlanUseCase) Execute(ctx context.Context, input SetPlanInput) (*SetPlanResult, error) {
	pt, err := domain.ParsePlanType(input.PlanType)
	if err != nil {
		return nil, ErrInvalidPlanType
	}

	if err := input.Entitlements.Validate(); err != nil {
		return nil, ErrInvalidEntitlements
	}

	plan := domain.Plan{Type: pt, OverageRate: input.OverageRate}
	if err := uc.users.SetPlan(ctx, input.UserID, plan, input.Credits, input.Entitlements); err != nil {
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
