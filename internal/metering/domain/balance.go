package domain

import "database/sql/driver"

// Balance represents a user's credit balance as a signed integer.
// Positive = prepaid credits remaining.
// Zero = no credits left (behavior depends on plan).
// Negative = overage consumed (only possible with hybrid plans).
type Balance struct {
	value int64
}

// MaxCredits bounds a single credit amount (top-up or grant). It is far below
// math.MaxInt64, so balance arithmetic cannot overflow in practice.
const MaxCredits int64 = 1_000_000_000_000_000 // 1e15

// NewBalance creates a Balance with the given value.
func NewBalance(value int64) Balance {
	return Balance{value: value}
}

// Value returns the raw balance amount.
func (b Balance) Value() int64 {
	return b.value
}

// CanDebit reports whether the balance can cover the given cost.
func (b Balance) CanDebit(cost int64) bool {
	return b.value >= cost
}

// Debit subtracts cost from the balance. Callers should check CanDebit first
// when the plan requires it (prepaid).
func (b Balance) Debit(cost int64) Balance {
	return Balance{value: b.value - cost}
}

// Credit adds amount to the balance (top-up).
func (b Balance) Credit(amount int64) Balance {
	return Balance{value: b.value + amount}
}

// IsNegative reports whether the balance is below zero (overage territory).
func (b Balance) IsNegative() bool {
	return b.value < 0
}

// IsZero reports whether the balance is exactly zero.
func (b Balance) IsZero() bool {
	return b.value == 0
}

// Scan implements sql.Scanner for Balance.
func (b *Balance) Scan(src interface{}) error {
	if src == nil {
		b.value = 0
		return nil
	}
	switch v := src.(type) {
	case int64:
		b.value = v
		return nil
	default:
		// Let pgx handle the conversion
		if valuer, ok := src.(driver.Valuer); ok {
			val, err := valuer.Value()
			if err != nil {
				return err
			}
			if intVal, ok := val.(int64); ok {
				b.value = intVal
				return nil
			}
		}
		b.value = 0
		return nil
	}
}
