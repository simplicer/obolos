package domain

// User represents a metered user with their balance, plan and entitlement rules.
type User struct {
	ID           string
	Plan         Plan
	Balance      Balance
	Entitlements Entitlements
	CreatedAt    string
	UpdatedAt    string
}

// NewUser creates a new user with the given ID, plan, and initial balance.
func NewUser(id string, plan Plan, initialBalance int64) User {
	return User{
		ID:      id,
		Plan:    plan,
		Balance: NewBalance(initialBalance),
	}
}
