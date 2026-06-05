package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"codeberg.org/simplicer/metering/internal/metering/domain"
)

// --- Mock repositories ---

type mockUserRepo struct {
	mu    sync.Mutex
	users map[string]*domain.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*domain.User)}
}

func (m *mockUserRepo) FindByID(_ context.Context, userID string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[userID]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockUserRepo) List(_ context.Context, limit int) ([]domain.User, error) {
	var users []domain.User
	for _, u := range m.users {
		users = append(users, *u)
		if limit > 0 && len(users) >= limit {
			break
		}
	}
	return users, nil
}

func (m *mockUserRepo) Upsert(_ context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.ID] = user
	return nil
}

func (m *mockUserRepo) CreateIfAbsent(_ context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[user.ID]; ok {
		return nil
	}
	u := *user
	m.users[user.ID] = &u
	return nil
}

func (m *mockUserRepo) AddCredits(_ context.Context, userID string, amount int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[userID]
	if !ok {
		return 0, ErrUserNotFound
	}
	u.Balance = u.Balance.Credit(amount)
	return u.Balance.Value(), nil
}

func (m *mockUserRepo) SetPlan(_ context.Context, userID string, plan domain.Plan, credits int64, ent domain.Entitlements) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[userID]
	if !ok {
		return ErrUserNotFound
	}
	u.Plan = plan
	u.Balance = domain.NewBalance(credits)
	u.Entitlements = ent
	return nil
}

type mockEventRepo struct {
	events []domain.MeteringEvent
}

func newMockEventRepo() *mockEventRepo {
	return &mockEventRepo{}
}

func (m *mockEventRepo) Save(_ context.Context, event domain.MeteringEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventRepo) FindByUserAndDateRange(_ context.Context, userID string, from, to time.Time, limit int) ([]domain.MeteringEvent, error) {
	var result []domain.MeteringEvent
	for _, e := range m.events {
		if e.UserID == userID && !e.CreatedAt.Before(from) && !e.CreatedAt.After(to) {
			result = append(result, e)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockEventRepo) SumCostByUserAndDateRange(_ context.Context, userID string, from, to time.Time) (int64, error) {
	var sum int64
	for _, e := range m.events {
		if e.UserID == userID && !e.CreatedAt.Before(from) && !e.CreatedAt.After(to) {
			sum += e.FinalCost
		}
	}
	return sum, nil
}

// mockMeterer simulates atomic metering for unit tests.
type mockMeterer struct {
	mu     sync.Mutex
	users  *mockUserRepo
	events *mockEventRepo
}

func newMockMeterer(users *mockUserRepo, events *mockEventRepo) *mockMeterer {
	return &mockMeterer{users: users, events: events}
}

func (m *mockMeterer) MeterAtomic(ctx context.Context, userID, api, endpoint string, cost int64) (*MeterResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, err := m.users.FindByID(ctx, userID)
	if err != nil || user == nil {
		return nil, ErrUserNotFound
	}

	balanceBefore := user.Balance.Value()
	plan := user.Plan

	if !plan.CanDeduct(cost, user.Balance) {
		return &MeterResult{Cost: cost, Balance: balanceBefore, PlanType: plan.String()}, ErrInsufficientCredits
	}

	newBalance := user.Balance.Debit(cost)
	overage := plan.IsOverage(newBalance)
	user.Balance = newBalance
	m.users.Upsert(ctx, user)

	event := domain.NewMeteringEvent(
		generateID(), userID, api, endpoint,
		cost, 1.0, cost,
		balanceBefore, newBalance.Value(),
		plan.String(), overage,
	)
	m.events.Save(ctx, event)

	return &MeterResult{Cost: cost, Balance: newBalance.Value(), PlanType: plan.String()}, nil
}

// --- Meter use case tests ---

func TestMeterUseCase_PrepaidSufficient(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()

	users.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(100),
	})

	uc := NewMeterUseCase(users, events, newMockMeterer(users, events))
	result, err := uc.Execute(context.Background(), MeterInput{
		UserID: "user-1", API: "vatguard", Endpoint: "/v1/vies/{number}", Cost: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cost != 5 {
		t.Errorf("expected cost 5, got %d", result.Cost)
	}
	if result.Balance != 95 {
		t.Errorf("expected balance 95, got %d", result.Balance)
	}
	if result.PlanType != "prepaid" {
		t.Errorf("expected prepaid, got %s", result.PlanType)
	}
}

func TestMeterUseCase_PrepaidInsufficient(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()

	users.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(3),
	})

	uc := NewMeterUseCase(users, events, newMockMeterer(users, events))
	_, err := uc.Execute(context.Background(), MeterInput{
		UserID: "user-1", API: "vatguard", Endpoint: "/v1/vies/{number}", Cost: 5,
	})
	if err != ErrInsufficientCredits {
		t.Errorf("expected ErrInsufficientCredits, got %v", err)
	}
}

func TestMeterUseCase_UnlimitedPlan(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()

	users.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewUnlimitedPlan(),
		Balance: domain.NewBalance(0),
	})

	uc := NewMeterUseCase(users, events, newMockMeterer(users, events))
	result, err := uc.Execute(context.Background(), MeterInput{
		UserID: "user-1", API: "vatguard", Endpoint: "/v1/vies/{number}", Cost: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cost != 0 {
		t.Errorf("unlimited plan should log cost as 0, got %d", result.Cost)
	}
	if result.Balance != 0 {
		t.Errorf("unlimited plan should not change balance, got %d", result.Balance)
	}
}

func TestMeterUseCase_HybridPlan(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()

	users.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewHybridPlan(0.01),
		Balance: domain.NewBalance(3),
	})

	uc := NewMeterUseCase(users, events, newMockMeterer(users, events))
	result, err := uc.Execute(context.Background(), MeterInput{
		UserID: "user-1", API: "vatguard", Endpoint: "/v1/vies/{number}", Cost: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cost != 5 {
		t.Errorf("expected cost 5, got %d", result.Cost)
	}
	if result.Balance != -2 {
		t.Errorf("hybrid should allow negative balance, got %d", result.Balance)
	}

	// Check overage flag on event
	if len(events.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events.events))
	}
	if !events.events[0].Overage {
		t.Error("expected overage flag to be true")
	}
}

func TestMeterUseCase_ZeroCost(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()

	users.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(10),
	})

	uc := NewMeterUseCase(users, events, newMockMeterer(users, events))
	result, err := uc.Execute(context.Background(), MeterInput{
		UserID: "user-1", API: "vatguard", Endpoint: "/health", Cost: 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cost != 0 {
		t.Errorf("expected cost 0, got %d", result.Cost)
	}
	if result.Balance != 10 {
		t.Errorf("balance should not change for zero-cost, got %d", result.Balance)
	}
}

func TestMeterUseCase_UserNotFound(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()

	uc := NewMeterUseCase(users, events, newMockMeterer(users, events))
	_, err := uc.Execute(context.Background(), MeterInput{
		UserID: "unknown", API: "vatguard", Endpoint: "/v1/vat/{number}", Cost: 1,
	})
	if err == nil {
		t.Error("expected error for unknown user")
	}
}

func TestMeterUseCase_WindowFree(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()
	users.Upsert(context.Background(), &domain.User{
		ID: "user-1", Plan: domain.NewPrepaidPlan(), Balance: domain.NewBalance(100),
		Entitlements: domain.Entitlements{Windows: []domain.TimeWindow{{Action: domain.WindowFree}}},
	})

	uc := NewMeterUseCase(users, events, newMockMeterer(users, events))
	res, err := uc.Execute(context.Background(), MeterInput{UserID: "user-1", Cost: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Cost != 0 || res.Balance != 100 {
		t.Errorf("free window: got cost=%d balance=%d, want 0/100", res.Cost, res.Balance)
	}
}

func TestMeterUseCase_WindowBlock(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()
	users.Upsert(context.Background(), &domain.User{
		ID: "user-1", Plan: domain.NewPrepaidPlan(), Balance: domain.NewBalance(100),
		Entitlements: domain.Entitlements{Windows: []domain.TimeWindow{{Action: domain.WindowBlock}}},
	})

	uc := NewMeterUseCase(users, events, newMockMeterer(users, events))
	_, err := uc.Execute(context.Background(), MeterInput{UserID: "user-1", Cost: 5})
	if err != ErrBlockedByWindow {
		t.Errorf("expected ErrBlockedByWindow, got %v", err)
	}
}

func TestMeterUseCase_WindowMultiply(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()
	users.Upsert(context.Background(), &domain.User{
		ID: "user-1", Plan: domain.NewPrepaidPlan(), Balance: domain.NewBalance(100),
		Entitlements: domain.Entitlements{Windows: []domain.TimeWindow{{Action: domain.WindowMultiply, Factor: 2}}},
	})

	uc := NewMeterUseCase(users, events, newMockMeterer(users, events))
	res, err := uc.Execute(context.Background(), MeterInput{UserID: "user-1", Cost: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Cost != 10 || res.Balance != 90 {
		t.Errorf("multiply window: got cost=%d balance=%d, want 10/90", res.Cost, res.Balance)
	}
}

// --- TopUp use case tests ---

func TestTopUpUseCase_Success(t *testing.T) {
	users := newMockUserRepo()
	users.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(100),
	})

	uc := NewTopUpUseCase(users)
	result, err := uc.Execute(context.Background(), TopUpInput{UserID: "user-1", Amount: 500})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Balance != 600 {
		t.Errorf("expected balance 600, got %d", result.Balance)
	}
}

func TestTopUpUseCase_InvalidAmount(t *testing.T) {
	uc := NewTopUpUseCase(newMockUserRepo())
	_, err := uc.Execute(context.Background(), TopUpInput{UserID: "user-1", Amount: 0})
	if err != ErrInvalidAmount {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}

	_, err = uc.Execute(context.Background(), TopUpInput{UserID: "user-1", Amount: -10})
	if err != ErrInvalidAmount {
		t.Errorf("expected ErrInvalidAmount for negative, got %v", err)
	}
}

func TestTopUpUseCase_UserNotFound(t *testing.T) {
	uc := NewTopUpUseCase(newMockUserRepo())
	_, err := uc.Execute(context.Background(), TopUpInput{UserID: "ghost", Amount: 100})
	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// --- SetPlan use case tests ---

func TestSetPlanUseCase_Success(t *testing.T) {
	users := newMockUserRepo()
	users.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(100),
	})

	uc := NewSetPlanUseCase(users)
	result, err := uc.Execute(context.Background(), SetPlanInput{
		UserID: "user-1", PlanType: "hybrid", Credits: 10000, OverageRate: 0.01,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PlanType != "hybrid" {
		t.Errorf("expected hybrid, got %s", result.PlanType)
	}
	if result.Credits != 10000 {
		t.Errorf("expected 10000 credits, got %d", result.Credits)
	}
}

func TestSetPlanUseCase_InvalidPlan(t *testing.T) {
	uc := NewSetPlanUseCase(newMockUserRepo())
	_, err := uc.Execute(context.Background(), SetPlanInput{
		UserID: "user-1", PlanType: "bogus",
	})
	if err != ErrInvalidPlanType {
		t.Errorf("expected ErrInvalidPlanType, got %v", err)
	}
}

// --- GetBalance use case tests ---

func TestGetBalanceUseCase_Success(t *testing.T) {
	users := newMockUserRepo()
	events := newMockEventRepo()

	users.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(500),
	})

	uc := NewGetBalanceUseCase(users, events)
	result, err := uc.Execute(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Balance != 500 {
		t.Errorf("expected balance 500, got %d", result.Balance)
	}
	if result.PlanType != "prepaid" {
		t.Errorf("expected prepaid, got %s", result.PlanType)
	}
}

func TestGetBalanceUseCase_UserNotFound(t *testing.T) {
	uc := NewGetBalanceUseCase(newMockUserRepo(), newMockEventRepo())
	_, err := uc.Execute(context.Background(), "ghost")
	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}
