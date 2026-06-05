package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"codeberg.org/simplicer/metering/internal/metering/application"
	"codeberg.org/simplicer/metering/internal/metering/domain"
)

type inMemoryStore struct {
	users  map[string]*domain.User
	events []domain.MeteringEvent
}

func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{users: make(map[string]*domain.User)}
}

func (s *inMemoryStore) FindByID(_ context.Context, id string) (*domain.User, error) {
	return s.users[id], nil
}

func (s *inMemoryStore) List(_ context.Context, limit int) ([]domain.User, error) {
	var users []domain.User
	for _, u := range s.users {
		users = append(users, *u)
		if limit > 0 && len(users) >= limit {
			break
		}
	}
	return users, nil
}

func (s *inMemoryStore) Upsert(_ context.Context, u *domain.User) error {
	s.users[u.ID] = u
	return nil
}

func (s *inMemoryStore) Save(_ context.Context, e domain.MeteringEvent) error {
	s.events = append(s.events, e)
	return nil
}

func (s *inMemoryStore) FindByUserAndDateRange(_ context.Context, userID string, from, to time.Time, limit int) ([]domain.MeteringEvent, error) {
	var result []domain.MeteringEvent
	for _, e := range s.events {
		if e.UserID == userID && !e.CreatedAt.Before(from) && !e.CreatedAt.After(to) {
			result = append(result, e)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *inMemoryStore) SumCostByUserAndDateRange(_ context.Context, userID string, from, to time.Time) (int64, error) {
	var sum int64
	for _, e := range s.events {
		if e.UserID == userID && !e.CreatedAt.Before(from) && !e.CreatedAt.After(to) {
			sum += e.FinalCost
		}
	}
	return sum, nil
}

func (s *inMemoryStore) MeterAtomic(_ context.Context, userID, api, endpoint string, baseCost int64, multiplier float64, finalCost int64) (*application.MeterResult, error) {
	user := s.users[userID]
	if user == nil {
		return nil, application.ErrUserNotFound
	}
	balanceBefore := user.Balance.Value()
	plan := user.Plan

	if !plan.CanDeduct(finalCost, user.Balance) {
		return &application.MeterResult{Cost: finalCost, Balance: balanceBefore, PlanType: plan.String()}, application.ErrInsufficientCredits
	}

	newBalance := user.Balance.Debit(finalCost)
	overage := plan.IsOverage(newBalance)
	user.Balance = newBalance
	s.users[userID] = user

	s.events = append(s.events, domain.NewMeteringEvent(
		"test-id", userID, api, endpoint,
		baseCost, multiplier, finalCost,
		balanceBefore, newBalance.Value(),
		plan.String(), overage,
	))

	return &application.MeterResult{Cost: finalCost, Balance: newBalance.Value(), PlanType: plan.String()}, nil
}

type staticPricing struct {
	cost int64
}

func (p staticPricing) CostFor(_ context.Context, _, _ string, _ time.Time) (int64, int64, float64, error) {
	return p.cost, p.cost, 1.0, nil
}

func setupHandler(store *inMemoryStore, pricing application.PricingConfig) *Handler {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	configLoader := NewConfigLoader(DefaultServiceConfig(), logger)

	meterUC := application.NewMeterUseCase(store, store, pricing, store)
	getBalanceUC := application.NewGetBalanceUseCase(store, store)
	getUsageUC := application.NewGetUsageUseCase(store)
	topUpUC := application.NewTopUpUseCase(store)
	setPlanUC := application.NewSetPlanUseCase(store)
	adminUsageUC := application.NewGetAdminUsageUseCase(store)
	listUsersUC := application.NewListUsersUseCase(store)

	return NewHandler(meterUC, getBalanceUC, getUsageUC, topUpUC, setPlanUC, adminUsageUC, listUsersUC, store, logger, configLoader)
}

func setupRouter(store *inMemoryStore, pricing application.PricingConfig) http.Handler {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	configLoader := NewConfigLoader(DefaultServiceConfig(), logger)

	meterUC := application.NewMeterUseCase(store, store, pricing, store)
	getBalanceUC := application.NewGetBalanceUseCase(store, store)
	getUsageUC := application.NewGetUsageUseCase(store)
	topUpUC := application.NewTopUpUseCase(store)
	setPlanUC := application.NewSetPlanUseCase(store)
	adminUsageUC := application.NewGetAdminUsageUseCase(store)
	listUsersUC := application.NewListUsersUseCase(store)
	handler := NewHandler(meterUC, getBalanceUC, getUsageUC, topUpUC, setPlanUC, adminUsageUC, listUsersUC, store, logger, configLoader)

	return NewRouter(handler, logger)
}

// --- Handler tests ---

func TestHandleCreateCharge_Success(t *testing.T) {
	store := newInMemoryStore()
	handler := setupHandler(store, staticPricing{cost: 1})

	req := httptest.NewRequest(http.MethodPost, "/v1/charges", nil)
	req.Header.Set("X-User-Id", "user-1")
	req.Header.Set("X-Api", "vatguard")
	req.Header.Set("X-Endpoint", "/v1/vat/{number}")

	w := httptest.NewRecorder()
	handler.HandleCreateCharge(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp Response
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	if data["cost"].(float64) != 1 {
		t.Errorf("expected cost 1, got %v", data["cost"])
	}
	if data["balance"].(float64) != 999 {
		t.Errorf("expected balance 999, got %v", data["balance"])
	}
}

func TestHandleCreateCharge_MissingHeaders(t *testing.T) {
	store := newInMemoryStore()
	handler := setupHandler(store, staticPricing{cost: 1})

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{"no user id", map[string]string{"X-Api": "a", "X-Endpoint": "e"}},
		{"no api", map[string]string{"X-User-Id": "u", "X-Endpoint": "e"}},
		{"no endpoint", map[string]string{"X-User-Id": "u", "X-Api": "a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/charges", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			handler.HandleCreateCharge(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestHandleCreateCharge_InsufficientCredits(t *testing.T) {
	store := newInMemoryStore()
	store.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(0),
	})

	handler := setupHandler(store, staticPricing{cost: 5})

	req := httptest.NewRequest(http.MethodPost, "/v1/charges", nil)
	req.Header.Set("X-User-Id", "user-1")
	req.Header.Set("X-Api", "vatguard")
	req.Header.Set("X-Endpoint", "/v1/vies/{number}")

	w := httptest.NewRecorder()
	handler.HandleCreateCharge(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", w.Code)
	}
}

func TestHandleGetUser(t *testing.T) {
	store := newInMemoryStore()
	store.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(500),
	})

	handler := setupHandler(store, staticPricing{cost: 1})

	req := httptest.NewRequest(http.MethodGet, "/v1/users/user-1", nil)
	req.SetPathValue("user_id", "user-1")

	w := httptest.NewRecorder()
	handler.HandleGetUser(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp Response
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	if data["balance"].(float64) != 500 {
		t.Errorf("expected balance 500, got %v", data["balance"])
	}
}

func TestHandleAddCredits(t *testing.T) {
	store := newInMemoryStore()
	store.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(100),
	})

	handler := setupHandler(store, staticPricing{cost: 1})

	body := bytes.NewBufferString(`{"amount": 500}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/users/user-1/credits", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("user_id", "user-1")

	w := httptest.NewRecorder()
	handler.HandleAddCredits(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp Response
	json.NewDecoder(w.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	if data["balance"].(float64) != 600 {
		t.Errorf("expected balance 600, got %v", data["balance"])
	}
}

func TestHandlePutPlan(t *testing.T) {
	store := newInMemoryStore()
	store.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(100),
	})

	handler := setupHandler(store, staticPricing{cost: 1})

	body := bytes.NewBufferString(`{"plan_type": "hybrid", "credits": 10000, "overage_rate": 0.01}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/users/user-1/plan", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("user_id", "user-1")

	w := httptest.NewRecorder()
	handler.HandlePutPlan(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Router integration tests ---

func TestRouter_CreateCharge(t *testing.T) {
	router := setupRouter(newInMemoryStore(), staticPricing{cost: 1})

	req := httptest.NewRequest(http.MethodPost, "/v1/charges", nil)
	req.Header.Set("X-User-Id", "user-1")
	req.Header.Set("X-Api", "vatguard")
	req.Header.Set("X-Endpoint", "/v1/vat/{number}")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRouter_GetOwnUser(t *testing.T) {
	store := newInMemoryStore()
	store.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(500),
	})
	router := setupRouter(store, staticPricing{cost: 1})

	// User reads their own data: X-User-Id matches path {id}
	req := httptest.NewRequest(http.MethodGet, "/v1/users/user-1", nil)
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRouter_GetOtherUser_Forbidden(t *testing.T) {
	store := newInMemoryStore()
	store.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(500),
	})
	router := setupRouter(store, staticPricing{cost: 1})

	// User tries to read another user's data: X-User-Id != path {id}
	req := httptest.NewRequest(http.MethodGet, "/v1/users/user-2", nil)
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRouter_ListUsers_AdminOnly(t *testing.T) {
	router := setupRouter(newInMemoryStore(), staticPricing{cost: 1})

	// Without admin key → 401
	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without admin key, got %d", w.Code)
	}
}

func TestRouter_AddCredits_AdminOnly(t *testing.T) {
	store := newInMemoryStore()
	store.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(100),
	})
	router := setupRouter(store, staticPricing{cost: 1})

	// Without admin key → 401
	body := bytes.NewBufferString(`{"amount": 500}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/users/user-1/credits", body)
	req.Header.Set("X-User-Id", "user-1")
	req.SetPathValue("user_id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without admin key, got %d", w.Code)
	}
}

func TestRouter_UserCharges_OwnerAccess(t *testing.T) {
	router := setupRouter(newInMemoryStore(), staticPricing{cost: 1})

	// Owner reads their own charges
	req := httptest.NewRequest(http.MethodGet, "/v1/users/user-1/charges", nil)
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for owner charges, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRouter_UserCharges_OtherUser_Forbidden(t *testing.T) {
	router := setupRouter(newInMemoryStore(), staticPricing{cost: 1})

	// User tries to read another user's charges
	req := httptest.NewRequest(http.MethodGet, "/v1/users/user-2/charges", nil)
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for other user's charges, got %d", w.Code)
	}
}
