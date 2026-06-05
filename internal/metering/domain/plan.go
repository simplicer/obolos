package domain

import "fmt"

// PlanType enumerates the supported billing models.
type PlanType string

const (
	PlanTypePrepaid   PlanType = "prepaid"
	PlanTypeUnlimited PlanType = "unlimited"
	PlanTypeHybrid    PlanType = "hybrid"
)

// ParsePlanType converts a string to a PlanType, returning an error for unknown values.
func ParsePlanType(s string) (PlanType, error) {
	switch PlanType(s) {
	case PlanTypePrepaid:
		return PlanTypePrepaid, nil
	case PlanTypeUnlimited:
		return PlanTypeUnlimited, nil
	case PlanTypeHybrid:
		return PlanTypeHybrid, nil
	default:
		return "", fmt.Errorf("unknown plan type: %q", s)
	}
}

// Plan defines the billing model for a user.
type Plan struct {
	Type        PlanType
	OverageRate float64 // cost per unit when balance is exhausted (hybrid only)
}

// NewPrepaidPlan creates a prepaid plan.
func NewPrepaidPlan() Plan {
	return Plan{Type: PlanTypePrepaid}
}

// NewUnlimitedPlan creates an unlimited plan.
func NewUnlimitedPlan() Plan {
	return Plan{Type: PlanTypeUnlimited}
}

// NewHybridPlan creates a hybrid plan with the given overage rate.
func NewHybridPlan(overageRate float64) Plan {
	return Plan{Type: PlanTypeHybrid, OverageRate: overageRate}
}

// ShouldDeduct reports whether this plan deducts credits on a request.
// Unlimited plans never deduct.
func (p Plan) ShouldDeduct() bool {
	return p.Type != PlanTypeUnlimited
}

// CanDeduct reports whether a request with the given cost is allowed
// for the current balance under this plan.
func (p Plan) CanDeduct(cost int64, balance Balance) bool {
	switch p.Type {
	case PlanTypeUnlimited:
		return true
	case PlanTypePrepaid:
		return balance.CanDebit(cost)
	case PlanTypeHybrid:
		return true // hybrid never blocks; balance can go negative
	default:
		return false
	}
}

// IsOverage reports whether the balance is in overage territory for this plan.
// Only hybrid plans allow negative balances.
func (p Plan) IsOverage(balance Balance) bool {
	return p.Type == PlanTypeHybrid && balance.IsNegative()
}

// String returns the plan type as a string.
func (p Plan) String() string {
	return string(p.Type)
}
