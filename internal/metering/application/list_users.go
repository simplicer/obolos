package application

import (
	"context"
	"fmt"
)

// ListUsersUseCase lists all users (admin).
type ListUsersUseCase struct {
	users UserRepository
}

// NewListUsersUseCase creates a new list users use case.
func NewListUsersUseCase(users UserRepository) *ListUsersUseCase {
	return &ListUsersUseCase{users: users}
}

// UserSummary is a condensed view of a user for list responses.
type UserSummary struct {
	UserID    string `json:"user_id"`
	PlanType  string `json:"plan_type"`
	Balance   int64  `json:"balance"`
	CreatedAt string `json:"created_at"`
}

// ListUsersResult is the outcome of a list users query.
type ListUsersResult struct {
	Users []UserSummary `json:"users"`
	Total int           `json:"total"`
}

// Execute returns a list of all users.
func (uc *ListUsersUseCase) Execute(ctx context.Context) (*ListUsersResult, error) {
	users, err := uc.users.List(ctx, 500)
	if err != nil {
		return nil, fmt.Errorf("list users failed: %w", err)
	}

	summaries := make([]UserSummary, len(users))
	for i, u := range users {
		summaries[i] = UserSummary{
			UserID:    u.ID,
			PlanType:  u.Plan.String(),
			Balance:   u.Balance.Value(),
			CreatedAt: u.CreatedAt,
		}
	}

	return &ListUsersResult{
		Users: summaries,
		Total: len(summaries),
	}, nil
}
