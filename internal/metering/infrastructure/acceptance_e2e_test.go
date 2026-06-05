package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/simplicer/metering/internal/metering/domain"
)

// This file holds the acceptance / end-to-end suite: it drives the full HTTP
// stack (router + middleware + handlers + use cases) over a real loopback
// server with a real http.Client, exercising complete user journeys rather
// than a single handler. The store is in-memory so the suite has no external
// dependency and runs as part of `go test ./...`.

const e2eAdminKey = "e2e-admin-key"

// e2eServer starts a loopback HTTP server wired to an in-memory store and a
// fixed per-request cost, with admin authentication enabled.
func e2eServer(t *testing.T, cost int64) (*httptest.Server, *inMemoryStore) {
	t.Helper()

	// adminAPIKey is a package-level var normally populated from the
	// ADMIN_API_KEY env by NewRouter; set it directly and restore it so the
	// admin routes are reachable and the suite stays hermetic.
	prev := adminAPIKey
	adminAPIKey = e2eAdminKey
	t.Cleanup(func() { adminAPIKey = prev })

	store := newInMemoryStore()
	srv := httptest.NewServer(setupRouter(store, staticPricing{cost: cost}))
	t.Cleanup(srv.Close)
	return srv, store
}

type e2eClient struct {
	t    *testing.T
	base string
	http *http.Client
}

func (c *e2eClient) do(method, path string, headers map[string]string, body string) (int, map[string]any) {
	c.t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = bytes.NewBufferString(body)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		c.t.Fatalf("build request %s %s: %v", method, path, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &env)
	}
	out := env.Data
	if out == nil {
		out = map[string]any{}
	}
	if env.Error != nil {
		out["_error"] = env.Error
	}
	return resp.StatusCode, out
}

func TestE2E_FullMeteringJourney(t *testing.T) {
	srv, _ := e2eServer(t, 5)
	c := &e2eClient{t: t, base: srv.URL, http: srv.Client()}

	owner := map[string]string{"X-User-Id": "alice"}
	admin := map[string]string{"X-Admin-Key": e2eAdminKey}
	proxy := map[string]string{"X-User-Id": "alice", "X-Api": "vatguard", "X-Endpoint": "/v1/vies/{number}"}

	// 1. Well-known probes are public.
	if code, _ := c.do(http.MethodGet, "/health", nil, ""); code != http.StatusOK {
		t.Fatalf("health: got %d, want 200", code)
	}
	if code, data := c.do(http.MethodGet, "/version", nil, ""); code != http.StatusOK || data["version"] == "" {
		t.Fatalf("version: got %d / %v", code, data)
	}

	// 2. First charge auto-provisions alice with the default plan (prepaid,
	//    1000 credits) and deducts the cost.
	code, data := c.do(http.MethodPost, "/v1/charges", proxy, "")
	if code != http.StatusCreated {
		t.Fatalf("first charge: got %d, want 201 (%v)", code, data)
	}
	if data["balance"].(float64) != 995 {
		t.Errorf("balance after first charge = %v, want 995", data["balance"])
	}

	// 3. Owner reads their own balance.
	code, data = c.do(http.MethodGet, "/v1/users/alice", owner, "")
	if code != http.StatusOK || data["balance"].(float64) != 995 {
		t.Fatalf("get own balance: got %d / %v", code, data)
	}

	// 4. Admin adds 100 credits.
	code, data = c.do(http.MethodPost, "/v1/users/alice/credits", admin, `{"amount":100}`)
	if code != http.StatusCreated || data["balance"].(float64) != 1095 {
		t.Fatalf("add credits: got %d / %v", code, data)
	}

	// 5. Admin switches alice to a hybrid plan.
	code, data = c.do(http.MethodPut, "/v1/users/alice/plan", admin, `{"plan_type":"hybrid","credits":1095,"overage_rate":0.01}`)
	if code != http.StatusOK || data["plan_type"] != "hybrid" {
		t.Fatalf("set plan: got %d / %v", code, data)
	}

	// 6. Owner reads their charges; at least the first charge is present.
	code, data = c.do(http.MethodGet, "/v1/users/alice/charges", owner, "")
	if code != http.StatusOK {
		t.Fatalf("get charges: got %d / %v", code, data)
	}
	if _, ok := data["total_cost"]; !ok {
		t.Errorf("charges response missing total_cost: %v", data)
	}

	// 7. Admin lists users.
	code, data = c.do(http.MethodGet, "/v1/users", admin, "")
	if code != http.StatusOK || data["total"].(float64) < 1 {
		t.Fatalf("list users: got %d / %v", code, data)
	}
}

func TestE2E_AuthorizationBoundaries(t *testing.T) {
	srv, store := e2eServer(t, 5)
	c := &e2eClient{t: t, base: srv.URL, http: srv.Client()}

	store.Upsert(context.Background(), &domain.User{
		ID: "alice", Plan: domain.NewPrepaidPlan(), Balance: domain.NewBalance(500),
	})

	// Another user may not read alice.
	if code, _ := c.do(http.MethodGet, "/v1/users/alice", map[string]string{"X-User-Id": "mallory"}, ""); code != http.StatusForbidden {
		t.Errorf("cross-user read: got %d, want 403", code)
	}
	// Listing users without the admin key is unauthorized.
	if code, _ := c.do(http.MethodGet, "/v1/users", nil, ""); code != http.StatusUnauthorized {
		t.Errorf("list without admin key: got %d, want 401", code)
	}
	// Adding credits without the admin key is unauthorized.
	if code, _ := c.do(http.MethodPost, "/v1/users/alice/credits", map[string]string{"X-User-Id": "alice"}, `{"amount":10}`); code != http.StatusUnauthorized {
		t.Errorf("add credits without admin key: got %d, want 401", code)
	}
}

func TestE2E_PrepaidExhaustionReturns402(t *testing.T) {
	srv, store := e2eServer(t, 5)
	c := &e2eClient{t: t, base: srv.URL, http: srv.Client()}

	// Prepaid user with exactly one charge worth of credits.
	store.Upsert(context.Background(), &domain.User{
		ID: "bob", Plan: domain.NewPrepaidPlan(), Balance: domain.NewBalance(5),
	})

	proxy := map[string]string{"X-User-Id": "bob", "X-Api": "vatguard", "X-Endpoint": "/v1/vies/{number}"}

	// First charge succeeds and drains the balance to zero.
	if code, data := c.do(http.MethodPost, "/v1/charges", proxy, ""); code != http.StatusCreated || data["balance"].(float64) != 0 {
		t.Fatalf("draining charge: got %d / %v", code, data)
	}
	// Second charge is rejected with 402 and an insufficient_credits problem.
	code, data := c.do(http.MethodPost, "/v1/charges", proxy, "")
	if code != http.StatusPaymentRequired {
		t.Fatalf("exhausted charge: got %d, want 402 (%v)", code, data)
	}
	if errObj, ok := data["_error"].(map[string]any); !ok || errObj["title"] != "insufficient_credits" {
		t.Errorf("expected insufficient_credits problem, got %v", data)
	}
}
