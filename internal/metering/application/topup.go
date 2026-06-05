package application

import (
	"context"
	"fmt"

	"codeberg.org/simplicer/obolos/internal/metering/domain"
)

// TopUpUseCase adds credits to a user's balance.
type TopUpUseCase struct {
	users UserRepository
}

// NewTopUpUseCase creates a new TopUp use case.
func NewTopUpUseCase(users UserRepository) *TopUpUseCase {
	return &TopUpUseCase{users: users}
}

// TopUpInput contains the parameters for a top-up request.
type TopUpInput struct {
	UserID string
	Amount int64
}

// Execute adds amount credits to the user's balance atomically.
func (uc *TopUpUseCase) Execute(ctx context.Context, input TopUpInput) (*TopUpResult, error) {
	if input.Amount <= 0 || input.Amount > domain.MaxCredits {
		return nil, ErrInvalidAmount
	}

	newBalance, err := uc.users.AddCredits(ctx, input.UserID, input.Amount)
	if err != nil {
		if err == ErrUserNotFound {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("balance update failed: %w", err)
	}

	return &TopUpResult{
		UserID:  input.UserID,
		Balance: newBalance,
	}, nil
}
