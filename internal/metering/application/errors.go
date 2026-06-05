package application

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for use cases.
var (
	ErrInsufficientCredits = errors.New("insufficient_credits")
	ErrUserNotFound        = errors.New("user_not_found")
	ErrInvalidPlanType     = errors.New("invalid_plan_type")
	ErrInvalidAmount       = errors.New("invalid_amount")
)

// now returns the current UTC time. Extracted for testability.
var now = func() time.Time {
	return time.Now().UTC()
}

// generateID returns a unique ID using crypto/rand.
var generateID = func() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
