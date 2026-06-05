package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"codeberg.org/simplicer/obolos/internal/metering/application"
	"codeberg.org/simplicer/obolos/internal/metering/domain"
)

// ProblemJSON follows RFC 7807.
type ProblemJSON struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// Response wraps data in the standard envelope.
type Response struct {
	Data  interface{}  `json:"data,omitempty"`
	Error *ProblemJSON `json:"error,omitempty"`
}

// userIDPattern validates user IDs: alphanumeric, dashes, underscores, dots, max 255 chars.
var userIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,255}$`)

// Pinger verifies a backing dependency is reachable (for the readiness probe).
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler holds all HTTP handlers and their dependencies.
type Handler struct {
	meter      *application.MeterUseCase
	getBalance *application.GetBalanceUseCase
	getUsage   *application.GetUsageUseCase
	topUp      *application.TopUpUseCase
	setPlan    *application.SetPlanUseCase
	adminUsage *application.GetAdminUsageUseCase
	listUsers  *application.ListUsersUseCase
	users      application.UserRepository
	ready      Pinger
	logger     *slog.Logger
	config     *ConfigLoader
}

// NewHandler creates a new Handler with all dependencies.
func NewHandler(
	meter *application.MeterUseCase,
	getBalance *application.GetBalanceUseCase,
	getUsage *application.GetUsageUseCase,
	topUp *application.TopUpUseCase,
	setPlan *application.SetPlanUseCase,
	adminUsage *application.GetAdminUsageUseCase,
	listUsers *application.ListUsersUseCase,
	users application.UserRepository,
	ready Pinger,
	logger *slog.Logger,
	config *ConfigLoader,
) *Handler {
	return &Handler{
		meter:      meter,
		getBalance: getBalance,
		getUsage:   getUsage,
		topUp:      topUp,
		setPlan:    setPlan,
		adminUsage: adminUsage,
		listUsers:  listUsers,
		users:      users,
		ready:      ready,
		logger:     logger,
		config:     config,
	}
}

func (h *Handler) defaultPlan() (domain.PlanType, int64) {
	cfg := h.config.Get()
	pt, err := domain.ParsePlanType(cfg.DefaultPlanType)
	if err != nil {
		pt = domain.PlanTypePrepaid
	}
	return pt, cfg.DefaultPlanCredits
}

// --- Charges ---

// HandleCreateCharge handles POST /v1/charges
func (h *Handler) HandleCreateCharge(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	api := r.Header.Get("X-Api")           // optional metadata
	endpoint := r.Header.Get("X-Endpoint") // optional metadata
	costHeader := r.Header.Get("X-Cost")

	if userID == "" {
		writeProblem(w, http.StatusBadRequest, "missing_user_id", "X-User-Id header is required")
		return
	}
	if !userIDPattern.MatchString(userID) {
		writeProblem(w, http.StatusBadRequest, "invalid_user_id", "X-User-Id format is invalid")
		return
	}
	if costHeader == "" {
		writeProblem(w, http.StatusBadRequest, "missing_cost", "X-Cost header is required (credits charged by the caller)")
		return
	}
	cost, err := strconv.ParseInt(costHeader, 10, 64)
	if err != nil || cost < 0 {
		writeProblem(w, http.StatusBadRequest, "invalid_cost", "X-Cost must be a non-negative integer")
		return
	}

	if err := h.ensureUser(r.Context(), userID); err != nil {
		h.logger.Error("auto-provision failed", slog.String("error", err.Error()))
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Failed to provision user")
		return
	}

	result, err := h.meter.Execute(r.Context(), application.MeterInput{
		UserID:    userID,
		API:       api,
		Endpoint:  endpoint,
		Cost:      cost,
		RequestID: r.Header.Get("X-Request-Id"),
	})

	if err != nil {
		if errors.Is(err, application.ErrBlockedByWindow) {
			h.logger.Info("charge blocked by time window",
				slog.String("owner_id", userID), slog.String("api", api), slog.String("endpoint", endpoint))
			writeProblem(w, http.StatusForbidden, "blocked_by_window", "Requests are not allowed in the current time window")
			return
		}
		if errors.Is(err, application.ErrRateLimited) {
			writeProblem(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many requests for this user")
			return
		}
		if errors.Is(err, application.ErrInsufficientCredits) {
			h.logger.Info("insufficient credits",
				slog.String("owner_id", userID),
				slog.String("api", api),
				slog.String("endpoint", endpoint),
				slog.Int64("cost", result.Cost),
				slog.Int64("balance_after", result.Balance),
			)
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusPaymentRequired)
			json.NewEncoder(w).Encode(Response{
				Data:  result,
				Error: &ProblemJSON{Title: "insufficient_credits", Status: http.StatusPaymentRequired},
			})
			return
		}
		h.logger.Error("meter error", slog.String("error", err.Error()))
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Charge creation failed")
		return
	}

	h.logger.Info("charge created",
		slog.String("owner_id", userID),
		slog.String("api", api),
		slog.String("endpoint", endpoint),
		slog.Int64("cost", result.Cost),
		slog.Int64("balance_after", result.Balance),
	)

	writeJSON(w, http.StatusCreated, Response{Data: result})
}

// --- Users ---

// HandleListUsers handles GET /v1/users
func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	result, err := h.listUsers.Execute(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "User list failed")
		return
	}

	writeJSON(w, http.StatusOK, Response{Data: result})
}

// HandleGetUser handles GET /v1/users/{user_id}
// Owner sees their own info, admin sees any user. A GET never creates state:
// an unknown user returns 404 (users are provisioned only by a charge).
func (h *Handler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("user_id")
	if !userIDPattern.MatchString(userID) {
		writeProblem(w, http.StatusBadRequest, "invalid_user_id", "User ID format is invalid")
		return
	}

	result, err := h.getBalance.Execute(r.Context(), userID)
	if err != nil {
		if errors.Is(err, application.ErrUserNotFound) {
			writeProblem(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "User lookup failed")
		return
	}

	writeJSON(w, http.StatusOK, Response{Data: result})
}

// HandleGetUserCharges handles GET /v1/users/{user_id}/charges
// Owner sees their own charges, admin sees any user's.
func (h *Handler) HandleGetUserCharges(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("user_id")
	if !userIDPattern.MatchString(userID) {
		writeProblem(w, http.StatusBadRequest, "invalid_user_id", "User ID format is invalid")
		return
	}

	from, to := parseDateRange(r)
	result, err := h.adminUsage.Execute(r.Context(), userID, from, to)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Charges lookup failed")
		return
	}

	writeJSON(w, http.StatusOK, Response{Data: result})
}

// HandleAddCredits handles POST /v1/users/{user_id}/credits (admin only)
func (h *Handler) HandleAddCredits(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("user_id")
	if !userIDPattern.MatchString(userID) {
		writeProblem(w, http.StatusBadRequest, "invalid_user_id", "User ID format is invalid")
		return
	}

	var body struct {
		Amount int64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	result, err := h.topUp.Execute(r.Context(), application.TopUpInput{
		UserID: userID,
		Amount: body.Amount,
	})
	if err != nil {
		if errors.Is(err, application.ErrUserNotFound) {
			writeProblem(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		if errors.Is(err, application.ErrInvalidAmount) {
			writeProblem(w, http.StatusBadRequest, "invalid_amount", "Amount must be positive")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Credit addition failed")
		return
	}

	writeJSON(w, http.StatusCreated, Response{Data: result})
}

// HandlePutPlan handles PUT /v1/users/{user_id}/plan (admin only)
func (h *Handler) HandlePutPlan(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("user_id")
	if !userIDPattern.MatchString(userID) {
		writeProblem(w, http.StatusBadRequest, "invalid_user_id", "User ID format is invalid")
		return
	}

	var body struct {
		PlanType     string              `json:"plan_type"`
		Credits      int64               `json:"credits"`
		OverageRate  float64             `json:"overage_rate"`
		Entitlements domain.Entitlements `json:"entitlements"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	result, err := h.setPlan.Execute(r.Context(), application.SetPlanInput{
		UserID:       userID,
		PlanType:     body.PlanType,
		Credits:      body.Credits,
		OverageRate:  body.OverageRate,
		Entitlements: body.Entitlements,
	})
	if err != nil {
		if errors.Is(err, application.ErrUserNotFound) {
			writeProblem(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		if errors.Is(err, application.ErrInvalidPlanType) {
			writeProblem(w, http.StatusBadRequest, "invalid_plan_type", "Valid plan types: prepaid, unlimited, hybrid")
			return
		}
		if errors.Is(err, application.ErrInvalidEntitlements) {
			writeProblem(w, http.StatusBadRequest, "invalid_entitlements", "Entitlement rules are malformed")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Plan update failed")
		return
	}

	writeJSON(w, http.StatusOK, Response{Data: result})
}

// --- Well-known ---

func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Response{Data: map[string]string{"status": "ok"}})
}

// HandleReady is the readiness probe: it reports 503 when the database is
// unreachable so the orchestrator can route traffic away from the instance.
func (h *Handler) HandleReady(w http.ResponseWriter, r *http.Request) {
	if h.ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.ready.Ping(ctx); err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "not_ready", "Database is unavailable")
			return
		}
	}
	writeJSON(w, http.StatusOK, Response{Data: map[string]string{"status": "ready"}})
}

func (h *Handler) HandleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Response{Data: map[string]string{"version": Version}})
}

// --- Helpers ---

func (h *Handler) ensureUser(ctx context.Context, userID string) error {
	pt, credits := h.defaultPlan()
	newUser := domain.NewUser(userID, domain.Plan{Type: pt}, credits)
	// Atomic provision: only creates the user when absent, so two concurrent
	// first requests cannot reset an already-charged balance.
	return h.users.CreateIfAbsent(ctx, &newUser)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{
		Error: &ProblemJSON{Title: title, Status: status, Detail: detail},
	})
}

func parseDateRange(r *http.Request) (from, to time.Time) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	now := time.Now().UTC()

	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t.Add(24 * time.Hour)
		}
	}

	if from.IsZero() {
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	if to.IsZero() {
		to = now
	}
	return from, to
}

// extractAndValidateUserID is a fallback for non-path-param routes.
func extractAndValidateUserID(urlPath, prefix, suffix string) string {
	if len(urlPath) <= len(prefix)+len(suffix) {
		return ""
	}
	trimmed := urlPath[len(prefix):]
	if suffix != "" && strings.HasSuffix(trimmed, suffix) {
		trimmed = trimmed[:len(trimmed)-len(suffix)]
	}
	trimmed = strings.TrimSpace(trimmed)
	if !userIDPattern.MatchString(trimmed) {
		return ""
	}
	return trimmed
}
