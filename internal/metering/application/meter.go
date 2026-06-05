package application

import (
	"context"
	"fmt"

	"codeberg.org/simplicer/metering/internal/metering/domain"
)

// MeterUseCase meters a single API request.
type MeterUseCase struct {
	users   UserRepository
	events  EventRepository
	pricing PricingConfig
	meterer Meterer // atomic balance deduction + event recording
}

// NewMeterUseCase creates a new Meter use case.
func NewMeterUseCase(users UserRepository, events EventRepository, pricing PricingConfig, meterer Meterer) *MeterUseCase {
	return &MeterUseCase{users: users, events: events, pricing: pricing, meterer: meterer}
}

// MeterInput contains the parameters for a metering request.
type MeterInput struct {
	UserID   string
	API      string
	Endpoint string
}

// Execute meters a request: calculates cost, checks balance, deducts, records event.
func (uc *MeterUseCase) Execute(ctx context.Context, input MeterInput) (*MeterResult, error) {
	// Calculate cost
	baseCost, finalCost, multiplier, err := uc.pricing.CostFor(ctx, input.API, input.Endpoint, now())
	if err != nil {
		return nil, fmt.Errorf("pricing lookup failed: %w", err)
	}

	// Find user
	user, err := uc.users.FindByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}

	if user == nil {
		return nil, fmt.Errorf("user %s not found", input.UserID)
	}

	plan := user.Plan

	// Cost = 0: pass through, no deduction
	if finalCost == 0 {
		event := domain.NewMeteringEvent(
			generateID(), input.UserID, input.API, input.Endpoint,
			baseCost, multiplier, 0,
			user.Balance.Value(), user.Balance.Value(),
			plan.String(), false,
		)
		if err := uc.events.Save(ctx, event); err != nil {
			return nil, fmt.Errorf("event save failed: %w", err)
		}
		return &MeterResult{Cost: 0, Balance: user.Balance.Value(), PlanType: plan.String()}, nil
	}

	// Unlimited plan: no deduction, cost logged as 0
	if !plan.ShouldDeduct() {
		event := domain.NewMeteringEvent(
			generateID(), input.UserID, input.API, input.Endpoint,
			baseCost, multiplier, 0,
			user.Balance.Value(), user.Balance.Value(),
			plan.String(), false,
		)
		if err := uc.events.Save(ctx, event); err != nil {
			return nil, fmt.Errorf("event save failed: %w", err)
		}
		return &MeterResult{Cost: 0, Balance: user.Balance.Value(), PlanType: plan.String()}, nil
	}

	// Check if deduction is allowed BEFORE hitting the DB transaction
	if !plan.CanDeduct(finalCost, user.Balance) {
		return &MeterResult{
			Cost:     finalCost,
			Balance:  user.Balance.Value(),
			PlanType: plan.String(),
		}, ErrInsufficientCredits
	}

	// Use atomic metering for balance deduction + event recording
	result, err := uc.meterer.MeterAtomic(ctx, input.UserID, input.API, input.Endpoint, baseCost, multiplier, finalCost)
	if err != nil {
		if err == ErrInsufficientCredits {
			return &MeterResult{
				Cost:     finalCost,
				Balance:  user.Balance.Value(),
				PlanType: plan.String(),
			}, ErrInsufficientCredits
		}
		return nil, fmt.Errorf("atomic metering failed: %w", err)
	}

	return result, nil
}
