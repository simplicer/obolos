package infrastructure

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/simplicer/metering/internal/metering/application"
	"codeberg.org/simplicer/metering/internal/metering/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var cryptoRandRead = rand.Read

// PostgresStore implements UserRepository, EventRepository, and Meterer using PostgreSQL.
type PostgresStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewPostgresStore creates a new PostgreSQL store.
func NewPostgresStore(ctx context.Context, databaseURL string, logger *slog.Logger) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	logger.Info("connected to PostgreSQL")
	return &PostgresStore{pool: pool, logger: logger}, nil
}

// Close closes the connection pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

// --- UserRepository ---

// List returns all users up to the given limit.
func (s *PostgresStore) List(ctx context.Context, limit int) ([]domain.User, error) {
	query := `SELECT id, plan_type, balance, overage_rate, plan_config, created_at, updated_at FROM users ORDER BY created_at DESC`
	args := []interface{}{}
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		var planType string
		var overageRate float64
		var planConfig []byte
		var createdAt, updatedAt time.Time
		err := rows.Scan(&u.ID, &planType, &u.Balance, &overageRate, &planConfig, &createdAt, &updatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		pt, err := domain.ParsePlanType(planType)
		if err != nil {
			return nil, fmt.Errorf("invalid plan type: %w", err)
		}
		u.Plan = domain.Plan{Type: pt, OverageRate: overageRate}
		if err := unmarshalEntitlements(planConfig, &u.Entitlements); err != nil {
			return nil, fmt.Errorf("decode plan_config: %w", err)
		}
		u.CreatedAt = createdAt.Format(time.RFC3339)
		u.UpdatedAt = updatedAt.Format(time.RFC3339)
		users = append(users, u)
	}

	return users, rows.Err()
}

// FindByID finds a user by their ID.
func (s *PostgresStore) FindByID(ctx context.Context, userID string) (*domain.User, error) {
	var u domain.User
	var planType string
	var overageRate float64
	var planConfig []byte
	var createdAt, updatedAt time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT id, plan_type, balance, overage_rate, plan_config, created_at, updated_at FROM users WHERE id = $1`,
		userID,
	).Scan(&u.ID, &planType, &u.Balance, &overageRate, &planConfig, &createdAt, &updatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}

	pt, err := domain.ParsePlanType(planType)
	if err != nil {
		return nil, fmt.Errorf("invalid plan type in DB: %w", err)
	}
	u.Plan = domain.Plan{Type: pt, OverageRate: overageRate}
	if err := unmarshalEntitlements(planConfig, &u.Entitlements); err != nil {
		return nil, fmt.Errorf("decode plan_config: %w", err)
	}
	u.CreatedAt = createdAt.Format(time.RFC3339)
	u.UpdatedAt = updatedAt.Format(time.RFC3339)

	return &u, nil
}

// Upsert creates or updates a user.
func (s *PostgresStore) Upsert(ctx context.Context, user *domain.User) error {
	planConfig, err := marshalEntitlements(user.Entitlements)
	if err != nil {
		return fmt.Errorf("encode plan_config: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO users (id, plan_type, balance, overage_rate, plan_config, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (id) DO UPDATE SET
		   plan_type = EXCLUDED.plan_type,
		   balance = EXCLUDED.balance,
		   overage_rate = EXCLUDED.overage_rate,
		   plan_config = EXCLUDED.plan_config,
		   updated_at = NOW()`,
		user.ID, user.Plan.Type, user.Balance.Value(), user.Plan.OverageRate, planConfig,
	)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}

// CreateIfAbsent inserts the user only if its id is not already present.
// An existing user (and its balance) is left untouched.
func (s *PostgresStore) CreateIfAbsent(ctx context.Context, user *domain.User) error {
	planConfig, err := marshalEntitlements(user.Entitlements)
	if err != nil {
		return fmt.Errorf("encode plan_config: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO users (id, plan_type, balance, overage_rate, plan_config, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (id) DO NOTHING`,
		user.ID, user.Plan.Type, user.Balance.Value(), user.Plan.OverageRate, planConfig,
	)
	if err != nil {
		return fmt.Errorf("create user if absent: %w", err)
	}
	return nil
}

// AddCredits atomically adds amount to the user's balance and returns the new
// balance. The single UPDATE avoids the lost-update race with concurrent charges.
func (s *PostgresStore) AddCredits(ctx context.Context, userID string, amount int64) (int64, error) {
	var newBalance int64
	err := s.pool.QueryRow(ctx,
		`UPDATE users SET balance = balance + $2, updated_at = NOW()
		 WHERE id = $1
		 RETURNING balance`,
		userID, amount,
	).Scan(&newBalance)
	if err == pgx.ErrNoRows {
		return 0, application.ErrUserNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("add credits: %w", err)
	}
	return newBalance, nil
}

// SetPlan atomically replaces the user's plan, balance and entitlement rules.
func (s *PostgresStore) SetPlan(ctx context.Context, userID string, plan domain.Plan, credits int64, ent domain.Entitlements) error {
	planConfig, err := marshalEntitlements(ent)
	if err != nil {
		return fmt.Errorf("encode plan_config: %w", err)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET plan_type = $2, balance = $3, overage_rate = $4, plan_config = $5, updated_at = NOW()
		 WHERE id = $1`,
		userID, plan.Type, credits, plan.OverageRate, planConfig,
	)
	if err != nil {
		return fmt.Errorf("set plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrUserNotFound
	}
	return nil
}

// ApplyDueGrant applies the user's periodic grant if one is due. It locks the
// row, re-checks under the lock (so concurrent requests grant at most once per
// period) and updates balance + the grant anchor atomically.
func (s *PostgresStore) ApplyDueGrant(ctx context.Context, userID string, now time.Time) (int64, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin grant tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var balance int64
	var planConfig []byte
	err = tx.QueryRow(ctx,
		`SELECT balance, plan_config FROM users WHERE id = $1 FOR UPDATE`,
		userID,
	).Scan(&balance, &planConfig)
	if err == pgx.ErrNoRows {
		return 0, false, application.ErrUserNotFound
	}
	if err != nil {
		return 0, false, fmt.Errorf("lock user for grant: %w", err)
	}

	var ent domain.Entitlements
	if err := unmarshalEntitlements(planConfig, &ent); err != nil {
		return 0, false, fmt.Errorf("decode plan_config: %w", err)
	}
	if !ent.Grant.IsDue(now) {
		return balance, false, nil
	}

	newBalance, newAnchor := ent.Grant.Applied(balance, now)
	ent.Grant.Anchor = newAnchor
	cfg, err := marshalEntitlements(ent)
	if err != nil {
		return 0, false, fmt.Errorf("encode plan_config: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET balance = $2, plan_config = $3, updated_at = NOW() WHERE id = $1`,
		userID, newBalance, cfg,
	); err != nil {
		return 0, false, fmt.Errorf("apply grant: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit grant: %w", err)
	}
	return newBalance, true, nil
}

// marshalEntitlements encodes entitlements to a JSON document for the
// plan_config JSONB column.
func marshalEntitlements(e domain.Entitlements) ([]byte, error) {
	return json.Marshal(e)
}

// unmarshalEntitlements decodes the plan_config JSONB document; an empty or
// nil document yields the zero value (no rules).
func unmarshalEntitlements(data []byte, e *domain.Entitlements) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, e)
}

// --- EventRepository ---

// Save persists a metering event.
func (s *PostgresStore) Save(ctx context.Context, event domain.MeteringEvent) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO metering_events
		 (id, user_id, api, endpoint, base_cost, applied_multiplier, final_cost,
		  balance_before, balance_after, plan_type, overage, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		event.ID, event.UserID, event.API, event.Endpoint,
		event.BaseCost, event.AppliedMultiplier, event.FinalCost,
		event.BalanceBefore, event.BalanceAfter, event.PlanType,
		event.Overage, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

// FindByUserAndDateRange finds events for a user in a date range with pagination.
// A limit of 0 or less means no limit.
func (s *PostgresStore) FindByUserAndDateRange(ctx context.Context, userID string, from, to time.Time, limit int) ([]domain.MeteringEvent, error) {
	query := `SELECT id, user_id, api, endpoint, base_cost, applied_multiplier, final_cost,
			        balance_before, balance_after, plan_type, overage, created_at
			 FROM metering_events
			 WHERE user_id = $1 AND created_at >= $2 AND created_at <= $3
			 ORDER BY created_at DESC`

	args := []interface{}{userID, from, to}

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []domain.MeteringEvent
	for rows.Next() {
		var e domain.MeteringEvent
		err := rows.Scan(
			&e.ID, &e.UserID, &e.API, &e.Endpoint,
			&e.BaseCost, &e.AppliedMultiplier, &e.FinalCost,
			&e.BalanceBefore, &e.BalanceAfter, &e.PlanType,
			&e.Overage, &e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}

	return events, rows.Err()
}

// SumCostByUserAndDateRange returns the sum of final_cost for a user in a date range.
// Uses SQL aggregation — constant memory regardless of event count.
func (s *PostgresStore) SumCostByUserAndDateRange(ctx context.Context, userID string, from, to time.Time) (int64, error) {
	var sum int64
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(final_cost), 0) FROM metering_events
		 WHERE user_id = $1 AND created_at >= $2 AND created_at <= $3`,
		userID, from, to,
	).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("sum events: %w", err)
	}
	return sum, nil
}

// CountByUserSince returns how many events the user recorded at or after the
// given time, using SQL aggregation.
func (s *PostgresStore) CountByUserSince(ctx context.Context, userID string, since time.Time) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM metering_events WHERE user_id = $1 AND created_at >= $2`,
		userID, since,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count events: %w", err)
	}
	return count, nil
}

// --- Meterer (atomic balance deduction + event recording) ---

// MeterAtomic performs balance deduction and event recording in a single DB transaction.
// It uses SELECT FOR UPDATE to prevent race conditions (double-spend). The cost is
// the credit amount reported by the trusted caller.
func (s *PostgresStore) MeterAtomic(ctx context.Context, userID, api, endpoint string, cost int64) (*application.MeterResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the user row for update — prevents concurrent deductions
	var planType string
	var overageRate float64
	var balanceBefore int64
	var createdAt, updatedAt time.Time

	err = tx.QueryRow(ctx,
		`SELECT plan_type, balance, overage_rate, created_at, updated_at FROM users WHERE id = $1 FOR UPDATE`,
		userID,
	).Scan(&planType, &balanceBefore, &overageRate, &createdAt, &updatedAt)

	if err != nil {
		return nil, fmt.Errorf("lock user for update: %w", err)
	}

	pt, err := domain.ParsePlanType(planType)
	if err != nil {
		return nil, fmt.Errorf("invalid plan type: %w", err)
	}
	plan := domain.Plan{Type: pt, OverageRate: overageRate}
	balance := domain.NewBalance(balanceBefore)

	// Re-check inside the lock — another request may have changed the balance
	if !plan.CanDeduct(cost, balance) {
		return &application.MeterResult{
			Cost:     cost,
			Balance:  balanceBefore,
			PlanType: planType,
		}, application.ErrInsufficientCredits
	}

	newBalance := balance.Debit(cost)
	overage := plan.IsOverage(newBalance)
	balanceAfter := newBalance.Value()

	// Update balance
	_, err = tx.Exec(ctx,
		`UPDATE users SET balance = $1, updated_at = NOW() WHERE id = $2`,
		balanceAfter, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("update balance: %w", err)
	}

	// Insert event
	eventID := generateEventID()
	_, err = tx.Exec(ctx,
		`INSERT INTO metering_events
		 (id, user_id, api, endpoint, base_cost, applied_multiplier, final_cost,
		  balance_before, balance_after, plan_type, overage, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())`,
		eventID, userID, api, endpoint,
		cost, 1.0, cost,
		balanceBefore, balanceAfter, planType,
		overage,
	)
	if err != nil {
		return nil, fmt.Errorf("insert event: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	s.logger.Info("atomic metering complete",
		slog.String("user_id", userID),
		slog.Int64("cost", cost),
		slog.Int64("balance_before", balanceBefore),
		slog.Int64("balance_after", balanceAfter),
		slog.Bool("overage", overage),
	)

	return &application.MeterResult{
		Cost:     cost,
		Balance:  balanceAfter,
		PlanType: planType,
	}, nil
}

// generateEventID creates a UUID v4 for events.
func generateEventID() string {
	b := make([]byte, 16)
	_, _ = cryptoRandRead(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
