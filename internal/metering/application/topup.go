package application

import (
	"context"
	"fmt"
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

// Execute adds amount credits to the user's balance.
func (uc *TopUpUseCase) Execute(ctx context.Context, input TopUpInput) (*TopUpResult, error) {
	if input.Amount <= 0 {
		return nil, ErrInvalidAmount
	}

	user, err := uc.users.FindByID(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	user.Balance = user.Balance.Credit(input.Amount)
	if err := uc.users.Upsert(ctx, user); err != nil {
		return nil, fmt.Errorf("balance update failed: %w", err)
	}

	return &TopUpResult{
		UserID:  user.ID,
		Balance: user.Balance.Value(),
	}, nil
}
