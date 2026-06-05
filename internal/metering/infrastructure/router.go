package infrastructure

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// Version is set at build time via ldflags.
var Version = "dev"

// adminAPIKey is the key required for admin-only routes.
// Set via ADMIN_API_KEY env var.
var adminAPIKey = ""

// NewRouter creates and configures the HTTP router.
func NewRouter(handler *Handler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Well-known
	mux.HandleFunc("GET /health", handler.HandleHealth)
	mux.HandleFunc("GET /version", handler.HandleVersion)

	// Charges: create a charge (called by reverse proxy via forwardAuth)
	mux.HandleFunc("POST /v1/charges", handler.HandleCreateCharge)

	// Users (owner or admin)
	mux.HandleFunc("GET /v1/users", handler.HandleListUsers)
	mux.HandleFunc("GET /v1/users/{user_id}", handler.HandleGetUser)
	mux.HandleFunc("GET /v1/users/{user_id}/charges", handler.HandleGetUserCharges)

	// Users (admin only)
	mux.HandleFunc("POST /v1/users/{user_id}/credits", handler.HandleAddCredits)
	mux.HandleFunc("PUT /v1/users/{user_id}/plan", handler.HandlePutPlan)

	// Resolve admin key
	if v := os.Getenv("ADMIN_API_KEY"); v != "" {
		adminAPIKey = v
	}

	// Middleware chain (outermost first)
	var h http.Handler = mux
	h = authMiddleware(h, logger)
	h = requestSizeLimitMiddleware(h, 1<<20) // 1 MB
	h = loggingMiddleware(h, logger)
	h = recoveryMiddleware(h, logger)

	return h
}

// authMiddleware enforces authorization rules:
//
//   - POST /v1/charges → only needs X-User-Id (proxy-injected)
//   - GET /v1/users → admin only (X-Admin-Key)
//   - GET /v1/users/{id}, GET /v1/users/{id}/charges → owner (X-User-Id == {id}) or admin
//   - POST /v1/users/{id}/credits, PUT /v1/users/{id}/plan → admin only
func authMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Non-user routes pass through (charges, health, version)
		if !strings.HasPrefix(path, "/v1/users") {
			next.ServeHTTP(w, r)
			return
		}

		isAdmin := isAdminRequest(r)

		// GET /v1/users → admin only (list collection)
		if path == "/v1/users" && r.Method == http.MethodGet {
			if !isAdmin {
				writeProblem(w, http.StatusUnauthorized, "unauthorized", "Admin access required")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// POST credits, PUT plan → admin only
		if strings.HasSuffix(path, "/credits") && r.Method == http.MethodPost {
			if !isAdmin {
				writeProblem(w, http.StatusUnauthorized, "unauthorized", "Admin access required")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(path, "/plan") && r.Method == http.MethodPut {
			if !isAdmin {
				writeProblem(w, http.StatusUnauthorized, "unauthorized", "Admin access required")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// GET /v1/users/{id}, GET /v1/users/{id}/charges → owner or admin
		userID := extractUserIDFromPathValue(r)
		if userID == "" {
			next.ServeHTTP(w, r) // let handler return 400
			return
		}

		if isAdmin {
			// Admin can access any user
			next.ServeHTTP(w, r)
			return
		}

		// Non-admin: X-User-Id must match the requested {id}
		headerUserID := r.Header.Get("X-User-Id")
		if headerUserID == "" {
			writeProblem(w, http.StatusBadRequest, "missing_user_id", "X-User-Id header is required")
			return
		}
		if headerUserID != userID {
			writeProblem(w, http.StatusForbidden, "forbidden", "You can only access your own user resource")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isAdminRequest checks if the request carries a valid admin key.
func isAdminRequest(r *http.Request) bool {
	if adminAPIKey == "" {
		return false
	}
	provided := r.Header.Get("X-Admin-Key")
	return provided != "" && secureCompare(provided, adminAPIKey)
}

// extractUserIDFromPathValue gets the user_id from /v1/users/{user_id}/... routes.
// Uses URL path parsing since PathValue is not yet set during middleware.
func extractUserIDFromPathValue(r *http.Request) string {
	path := r.URL.Path
	prefix := "/v1/users/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(path, prefix)
	// Take the first segment (before any /)
	if idx := strings.Index(remainder, "/"); idx >= 0 {
		remainder = remainder[:idx]
	}
	if remainder == "" || !userIDPattern.MatchString(remainder) {
		return ""
	}
	return remainder
}

func requestSizeLimitMiddleware(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		logger.Info("http_request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.statusCode),
			slog.Duration("duration", time.Since(start)),
			slog.String("remote_addr", r.RemoteAddr),
		)
	})
}

func recoveryMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					slog.Any("error", err),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				writeProblem(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
