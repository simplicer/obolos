package application

import (
	"context"
	"fmt"

	"codeberg.org/simplicer/metering/internal/metering/domain"
)

// MeterUseCase meters a single API request. The cost (in credits) is supplied
// by the trusted caller (the reverse proxy / middleware that knows the price);
// this service only enforces the client's credit rules.
type MeterUseCase struct {
	users   UserRepository
	events  EventRepository
	meterer Meterer // atomic balance deduction + event recording
}

// NewMeterUseCase creates a new Meter use case.
func NewMeterUseCase(users UserRepository, events EventRepository, meterer Meterer) *MeterUseCase {
	return &MeterUseCase{users: users, events: events, meterer: meterer}
}

// MeterInput contains the parameters for a metering request. Cost is the credit
// cost reported by the caller; API and Endpoint are optional metadata recorded
// on the event for analytics.
type MeterInput struct {
	UserID   string
	API      string
	Endpoint string
	Cost     int64
}

// Execute meters a request: validates the reported cost, checks the plan's
// credit rules, deducts, and records the event.
func (uc *MeterUseCase) Execute(ctx context.Context, input MeterInput) (*MeterResult, error) {
	if input.Cost < 0 {
		return nil, ErrInvalidCost
	}

	user, err := uc.users.FindByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	// Lazily apply a periodic credit grant if one is due (no cron); the store
	// re-checks under a row lock so it grants at most once per period.
	if user.Entitlements.Grant.IsDue(now()) {
		newBalance, applied, err := uc.users.ApplyDueGrant(ctx, input.UserID, now())
		if err != nil {
			return nil, fmt.Errorf("grant refresh failed: %w", err)
		}
		if applied {
			user.Balance = domain.NewBalance(newBalance)
		}
	}

	plan := user.Plan

	// Apply the user's entitlement time windows to the reported cost: a window
	// may block the request, make it free, or scale its cost.
	cost := input.Cost
	if adjusted, blocked, matched := user.Entitlements.ApplyWindowToCost(cost, now()); matched {
		if blocked {
			return &MeterResult{Cost: cost, Balance: user.Balance.Value(), PlanType: plan.String()}, ErrBlockedByWindow
		}
		cost = adjusted
	}
	input.Cost = cost

	// Zero cost, or an unlimited plan: pass through with no deduction, but still
	// record the event for analytics (cost logged as the reported value / 0).
	if input.Cost == 0 || !plan.ShouldDeduct() {
		loggedCost := input.Cost
		if !plan.ShouldDeduct() {
			loggedCost = 0
		}
		event := domain.NewMeteringEvent(
			generateID(), input.UserID, input.API, input.Endpoint,
			loggedCost, 1.0, loggedCost,
			user.Balance.Value(), user.Balance.Value(),
			plan.String(), false,
		)
		if err := uc.events.Save(ctx, event); err != nil {
			return nil, fmt.Errorf("event save failed: %w", err)
		}
		return &MeterResult{Cost: loggedCost, Balance: user.Balance.Value(), PlanType: plan.String()}, nil
	}

	// Fast pre-check before the DB transaction; the authoritative re-check
	// happens under a row lock inside MeterAtomic.
	if !plan.CanDeduct(input.Cost, user.Balance) {
		return &MeterResult{
			Cost:     input.Cost,
			Balance:  user.Balance.Value(),
			PlanType: plan.String(),
		}, ErrInsufficientCredits
	}

	result, err := uc.meterer.MeterAtomic(ctx, input.UserID, input.API, input.Endpoint, input.Cost)
	if err != nil {
		if err == ErrInsufficientCredits {
			return &MeterResult{
				Cost:     input.Cost,
				Balance:  user.Balance.Value(),
				PlanType: plan.String(),
			}, ErrInsufficientCredits
		}
		return nil, fmt.Errorf("atomic metering failed: %w", err)
	}

	return result, nil
}
