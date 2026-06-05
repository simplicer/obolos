package application

import (
	"context"
	"time"

	"codeberg.org/simplicer/metering/internal/metering/domain"
)

// UserRepository persists and retrieves users.
//
// Balance-changing operations are expressed as atomic methods (CreateIfAbsent,
// AddCredits, SetPlan) rather than read-modify-write over Upsert, so they do
// not lose updates when they race with a concurrent charge (which locks the
// row inside Meterer.MeterAtomic).
type UserRepository interface {
	FindByID(ctx context.Context, userID string) (*domain.User, error)
	List(ctx context.Context, limit int) ([]domain.User, error)
	Upsert(ctx context.Context, user *domain.User) error

	// CreateIfAbsent provisions a user only if it does not already exist; an
	// existing user (and its balance) is left untouched.
	CreateIfAbsent(ctx context.Context, user *domain.User) error
	// AddCredits atomically adds amount to the balance and returns the new
	// balance. Returns ErrUserNotFound if the user does not exist.
	AddCredits(ctx context.Context, userID string, amount int64) (int64, error)
	// SetPlan atomically replaces the user's plan and balance. Returns
	// ErrUserNotFound if the user does not exist.
	SetPlan(ctx context.Context, userID string, plan domain.Plan, credits int64) error
}

// EventRepository persists and retrieves metering events.
type EventRepository interface {
	Save(ctx context.Context, event domain.MeteringEvent) error
	FindByUserAndDateRange(ctx context.Context, userID string, from, to time.Time, limit int) ([]domain.MeteringEvent, error)
	SumCostByUserAndDateRange(ctx context.Context, userID string, from, to time.Time) (int64, error)
}

// Meterer performs an atomic balance deduction + event recording in a single transaction.
// This prevents race conditions and ensures financial consistency. The cost (in
// credits) is supplied by the trusted caller; this service does not price requests.
type Meterer interface {
	MeterAtomic(ctx context.Context, userID, api, endpoint string, cost int64) (*MeterResult, error)
}

// MeterResult is the outcome of a metering request.
type MeterResult struct {
	Cost     int64  `json:"cost"`
	Balance  int64  `json:"balance"`
	PlanType string `json:"plan_type"`
}

// BalanceResult is the outcome of a balance query.
type BalanceResult struct {
	UserID              string `json:"user_id"`
	Balance             int64  `json:"balance"`
	PlanType            string `json:"plan_type"`
	TotalUsageThisMonth int64  `json:"total_usage_this_month"`
}

// UsageResult is the outcome of a usage query.
type UsageResult struct {
	Events        []domain.MeteringEvent `json:"events"`
	TotalCost     int64                  `json:"total_cost"`
	TotalRequests int                    `json:"total_requests"`
}

// TopUpResult is the outcome of a top-up operation.
type TopUpResult struct {
	UserID  string `json:"user_id"`
	Balance int64  `json:"balance"`
}

// SetPlanResult is the outcome of setting a user's plan.
type SetPlanResult struct {
	UserID   string `json:"user_id"`
	PlanType string `json:"plan_type"`
	Credits  int64  `json:"credits"`
}

// AdminUsageResult is the outcome of an admin usage query.
type AdminUsageResult struct {
	Events      []domain.MeteringEvent `json:"events"`
	TotalCost   int64                  `json:"total_cost"`
	OverageCost int64                  `json:"overage_cost"`
}
