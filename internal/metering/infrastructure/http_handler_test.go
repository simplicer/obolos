package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"codeberg.org/simplicer/obolos/internal/metering/application"
	"codeberg.org/simplicer/obolos/internal/metering/domain"
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

func (s *inMemoryStore) Ping(_ context.Context) error { return nil }

func (s *inMemoryStore) CreateIfAbsent(_ context.Context, u *domain.User) error {
	if _, ok := s.users[u.ID]; ok {
		return nil
	}
	cp := *u
	s.users[u.ID] = &cp
	return nil
}

func (s *inMemoryStore) AddCredits(_ context.Context, userID string, amount int64) (int64, error) {
	u, ok := s.users[userID]
	if !ok {
		return 0, application.ErrUserNotFound
	}
	u.Balance = u.Balance.Credit(amount)
	return u.Balance.Value(), nil
}

func (s *inMemoryStore) SetPlan(_ context.Context, userID string, plan domain.Plan, credits int64, ent domain.Entitlements) error {
	u, ok := s.users[userID]
	if !ok {
		return application.ErrUserNotFound
	}
	u.Plan = plan
	u.Balance = domain.NewBalance(credits)
	u.Entitlements = ent
	return nil
}

func (s *inMemoryStore) ApplyDueGrant(_ context.Context, userID string, now time.Time) (int64, bool, error) {
	u, ok := s.users[userID]
	if !ok {
		return 0, false, application.ErrUserNotFound
	}
	if !u.Entitlements.Grant.IsDue(now) {
		return u.Balance.Value(), false, nil
	}
	nb, na := u.Entitlements.Grant.Applied(u.Balance.Value(), now)
	u.Balance = domain.NewBalance(nb)
	u.Entitlements.Grant.Anchor = na
	return nb, true, nil
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

func (s *inMemoryStore) CountByUserSince(_ context.Context, userID string, since time.Time) (int, error) {
	count := 0
	for _, e := range s.events {
		if e.UserID == userID && !e.CreatedAt.Before(since) {
			count++
		}
	}
	return count, nil
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

func (s *inMemoryStore) MeterAtomic(_ context.Context, userID, api, endpoint, requestID string, cost int64) (*application.MeterResult, error) {
	if requestID != "" {
		for _, e := range s.events {
			if e.UserID == userID && e.RequestID == requestID {
				return &application.MeterResult{Cost: e.FinalCost, Balance: e.BalanceAfter, PlanType: e.PlanType}, nil
			}
		}
	}
	user := s.users[userID]
	if user == nil {
		return nil, application.ErrUserNotFound
	}
	balanceBefore := user.Balance.Value()
	plan := user.Plan

	if !plan.CanDeduct(cost, user.Balance) {
		return &application.MeterResult{Cost: cost, Balance: balanceBefore, PlanType: plan.String()}, application.ErrInsufficientCredits
	}

	newBalance := user.Balance.Debit(cost)
	overage := plan.IsOverage(newBalance)
	user.Balance = newBalance
	s.users[userID] = user

	ev := domain.NewMeteringEvent(
		"test-id", userID, api, endpoint,
		cost, 1.0, cost,
		balanceBefore, newBalance.Value(),
		plan.String(), overage,
	)
	ev.RequestID = requestID
	s.events = append(s.events, ev)

	return &application.MeterResult{Cost: cost, Balance: newBalance.Value(), PlanType: plan.String()}, nil
}

func setupHandler(store *inMemoryStore) *Handler {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	configLoader := NewConfigLoader(DefaultServiceConfig(), logger)

	meterUC := application.NewMeterUseCase(store, store, store)
	getBalanceUC := application.NewGetBalanceUseCase(store, store)
	getUsageUC := application.NewGetUsageUseCase(store)
	topUpUC := application.NewTopUpUseCase(store)
	setPlanUC := application.NewSetPlanUseCase(store)
	adminUsageUC := application.NewGetAdminUsageUseCase(store)
	listUsersUC := application.NewListUsersUseCase(store)

	return NewHandler(meterUC, getBalanceUC, getUsageUC, topUpUC, setPlanUC, adminUsageUC, listUsersUC, store, store, logger, configLoader)
}

func setupRouter(store *inMemoryStore) http.Handler {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	configLoader := NewConfigLoader(DefaultServiceConfig(), logger)

	meterUC := application.NewMeterUseCase(store, store, store)
	getBalanceUC := application.NewGetBalanceUseCase(store, store)
	getUsageUC := application.NewGetUsageUseCase(store)
	topUpUC := application.NewTopUpUseCase(store)
	setPlanUC := application.NewSetPlanUseCase(store)
	adminUsageUC := application.NewGetAdminUsageUseCase(store)
	listUsersUC := application.NewListUsersUseCase(store)
	handler := NewHandler(meterUC, getBalanceUC, getUsageUC, topUpUC, setPlanUC, adminUsageUC, listUsersUC, store, store, logger, configLoader)

	return NewRouter(handler, logger)
}

// --- Handler tests ---

func TestHandleCreateCharge_Success(t *testing.T) {
	store := newInMemoryStore()
	handler := setupHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/v1/charges", nil)
	req.Header.Set("X-User-Id", "user-1")
	req.Header.Set("X-Api", "vatguard")
	req.Header.Set("X-Endpoint", "/v1/vat/{number}")
	req.Header.Set("X-Cost", "1")

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
	handler := setupHandler(store)

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{"no user id", map[string]string{"X-Cost": "1"}},
		{"no cost", map[string]string{"X-User-Id": "u"}},
		{"invalid cost", map[string]string{"X-User-Id": "u", "X-Cost": "abc"}},
		{"negative cost", map[string]string{"X-User-Id": "u", "X-Cost": "-5"}},
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

	handler := setupHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/v1/charges", nil)
	req.Header.Set("X-User-Id", "user-1")
	req.Header.Set("X-Api", "vatguard")
	req.Header.Set("X-Endpoint", "/v1/vies/{number}")
	req.Header.Set("X-Cost", "5")

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

	handler := setupHandler(store)

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

func TestHandleGetUser_UnknownReturns404NoProvision(t *testing.T) {
	store := newInMemoryStore()
	handler := setupHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/users/ghost", nil)
	req.SetPathValue("user_id", "ghost")
	w := httptest.NewRecorder()
	handler.HandleGetUser(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("unknown user GET: got %d, want 404", w.Code)
	}
	if _, exists := store.users["ghost"]; exists {
		t.Error("GET must not provision a user")
	}
}

func TestHandleAddCredits(t *testing.T) {
	store := newInMemoryStore()
	store.Upsert(context.Background(), &domain.User{
		ID:      "user-1",
		Plan:    domain.NewPrepaidPlan(),
		Balance: domain.NewBalance(100),
	})

	handler := setupHandler(store)

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

	handler := setupHandler(store)

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

func TestHandleReady_OK(t *testing.T) {
	router := setupRouter(newInMemoryStore())
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("ready: got %d, want 200", w.Code)
	}
}

type failingPinger struct{}

func (failingPinger) Ping(context.Context) error { return context.DeadlineExceeded }

func TestHandleReady_DBDown(t *testing.T) {
	h := &Handler{ready: failingPinger{}, logger: testLogger()}
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	h.HandleReady(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("ready with DB down: got %d, want 503", w.Code)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- Router integration tests ---

func TestRouter_CreateCharge(t *testing.T) {
	router := setupRouter(newInMemoryStore())

	req := httptest.NewRequest(http.MethodPost, "/v1/charges", nil)
	req.Header.Set("X-User-Id", "user-1")
	req.Header.Set("X-Api", "vatguard")
	req.Header.Set("X-Endpoint", "/v1/vat/{number}")
	req.Header.Set("X-Cost", "1")
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
	router := setupRouter(store)

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
	router := setupRouter(store)

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
	router := setupRouter(newInMemoryStore())

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
	router := setupRouter(store)

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
	router := setupRouter(newInMemoryStore())

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
	router := setupRouter(newInMemoryStore())

	// User tries to read another user's charges
	req := httptest.NewRequest(http.MethodGet, "/v1/users/user-2/charges", nil)
	req.Header.Set("X-User-Id", "user-1")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for other user's charges, got %d", w.Code)
	}
}
