//go:build integration

package infrastructure

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"codeberg.org/simplicer/metering/internal/metering/domain"
)

// These tests exercise PostgresStore against a real PostgreSQL instance.
// They are gated behind the `integration` build tag and require
// TEST_DATABASE_URL to point at an empty, disposable database.
//
// Run them with: make test-integration

func newTestStore(t *testing.T) (*PostgresStore, context.Context) {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration tests")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	store, err := NewPostgresStore(ctx, dbURL, logger)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	t.Cleanup(store.Close)

	applyMigration(t, ctx, store)
	truncate(t, ctx, store)

	return store, ctx
}

func applyMigration(t *testing.T, ctx context.Context, store *PostgresStore) {
	t.Helper()
	// package dir is internal/metering/infrastructure; repo root is three up.
	path := filepath.Join("..", "..", "..", "migrations", "001_initial_schema.sql")
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := store.pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

func truncate(t *testing.T, ctx context.Context, store *PostgresStore) {
	t.Helper()
	if _, err := store.pool.Exec(ctx, `TRUNCATE metering_events, users CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func TestIntegration_UserUpsertAndFind(t *testing.T) {
	store, ctx := newTestStore(t)

	u := domain.NewUser("user-a", domain.Plan{Type: domain.PlanTypePrepaid}, 500)
	if err := store.Upsert(ctx, &u); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.FindByID(ctx, "user-a")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.Balance.Value() != 500 {
		t.Errorf("balance = %d, want 500", got.Balance.Value())
	}
	if got.Plan.Type != domain.PlanTypePrepaid {
		t.Errorf("plan = %v, want prepaid", got.Plan.Type)
	}

	// Upsert again with a new balance/plan — must update in place.
	u2 := domain.NewUser("user-a", domain.Plan{Type: domain.PlanTypeHybrid, OverageRate: 0.01}, 1200)
	if err := store.Upsert(ctx, &u2); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got2, _ := store.FindByID(ctx, "user-a")
	if got2.Balance.Value() != 1200 || got2.Plan.Type != domain.PlanTypeHybrid {
		t.Errorf("after update: balance=%d plan=%v, want 1200/hybrid", got2.Balance.Value(), got2.Plan.Type)
	}

	// Unknown user → (nil, nil).
	missing, err := store.FindByID(ctx, "nobody")
	if err != nil || missing != nil {
		t.Errorf("missing user: got (%v, %v), want (nil, nil)", missing, err)
	}
}

func TestIntegration_ListUsers(t *testing.T) {
	store, ctx := newTestStore(t)

	for _, id := range []string{"u1", "u2", "u3"} {
		u := domain.NewUser(id, domain.Plan{Type: domain.PlanTypePrepaid}, 100)
		if err := store.Upsert(ctx, &u); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}

	users, err := store.List(ctx, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(users) != 3 {
		t.Errorf("len(users) = %d, want 3", len(users))
	}

	limited, err := store.List(ctx, 2)
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("len(limited) = %d, want 2", len(limited))
	}
}

func TestIntegration_EventsSaveAndAggregate(t *testing.T) {
	store, ctx := newTestStore(t)

	u := domain.NewUser("user-b", domain.Plan{Type: domain.PlanTypePrepaid}, 1000)
	if err := store.Upsert(ctx, &u); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		ev := domain.NewMeteringEvent(
			generateEventID(), "user-b", "vatguard", "/v1/vies/{number}",
			5, 1.0, 5, 1000-int64(i*5), 1000-int64((i+1)*5), "prepaid", false,
		)
		if err := store.Save(ctx, ev); err != nil {
			t.Fatalf("save event %d: %v", i, err)
		}
	}

	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)

	events, err := store.FindByUserAndDateRange(ctx, "user-b", from, to, 0)
	if err != nil {
		t.Fatalf("find events: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("len(events) = %d, want 3", len(events))
	}

	sum, err := store.SumCostByUserAndDateRange(ctx, "user-b", from, to)
	if err != nil {
		t.Fatalf("sum: %v", err)
	}
	if sum != 15 {
		t.Errorf("sum = %d, want 15", sum)
	}
}

func TestIntegration_CreateIfAbsentDoesNotReset(t *testing.T) {
	store, ctx := newTestStore(t)

	// Existing user with a non-default balance.
	u := domain.NewUser("dave", domain.Plan{Type: domain.PlanTypePrepaid}, 500)
	if err := store.Upsert(ctx, &u); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Provisioning again with the default credits must NOT touch the balance.
	def := domain.NewUser("dave", domain.Plan{Type: domain.PlanTypePrepaid}, 1000)
	if err := store.CreateIfAbsent(ctx, &def); err != nil {
		t.Fatalf("create if absent: %v", err)
	}
	got, _ := store.FindByID(ctx, "dave")
	if got.Balance.Value() != 500 {
		t.Errorf("balance = %d, want 500 (CreateIfAbsent must not reset)", got.Balance.Value())
	}
}

func TestIntegration_ConcurrentCreditsAreAtomic(t *testing.T) {
	store, ctx := newTestStore(t)

	u := domain.NewUser("erin", domain.Plan{Type: domain.PlanTypePrepaid}, 0)
	if err := store.Upsert(ctx, &u); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// N concurrent +1 top-ups. With a read-modify-write top-up these would
	// lose updates; AddCredits issues a single atomic UPDATE, so the final
	// balance must be exactly N.
	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.AddCredits(ctx, "erin", 1); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent add credits: %v", err)
	}

	got, _ := store.FindByID(ctx, "erin")
	if got.Balance.Value() != n {
		t.Errorf("balance = %d, want %d (lost updates)", got.Balance.Value(), n)
	}
}

func TestIntegration_MeterAtomic(t *testing.T) {
	store, ctx := newTestStore(t)

	// Prepaid user with enough balance.
	u := domain.NewUser("user-c", domain.Plan{Type: domain.PlanTypePrepaid}, 10)
	if err := store.Upsert(ctx, &u); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	res, err := store.MeterAtomic(ctx, "user-c", "vatguard", "/v1/vat/{number}", 4)
	if err != nil {
		t.Fatalf("meter: %v", err)
	}
	if res.Balance != 6 {
		t.Errorf("balance after deduct = %d, want 6", res.Balance)
	}

	// Event must have been recorded atomically.
	events, _ := store.FindByUserAndDateRange(ctx, "user-c", time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 0)
	if len(events) != 1 {
		t.Fatalf("expected 1 event recorded, got %d", len(events))
	}

	// Drain the balance, then a charge that exceeds it must be rejected and
	// must not change the balance.
	if _, err := store.MeterAtomic(ctx, "user-c", "vatguard", "/v1/vat/{number}", 6); err != nil {
		t.Fatalf("second meter: %v", err)
	}
	rejected, err := store.MeterAtomic(ctx, "user-c", "vatguard", "/v1/vat/{number}", 1)
	if err == nil {
		t.Fatal("expected insufficient-credits error, got nil")
	}
	if rejected.Balance != 0 {
		t.Errorf("balance after rejection = %d, want 0", rejected.Balance)
	}
}
