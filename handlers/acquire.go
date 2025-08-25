package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/Appy29/rate-limiter/middleware"
	"github.com/Appy29/rate-limiter/models"
	"github.com/Appy29/rate-limiter/services"
	"github.com/Appy29/rate-limiter/utils"
)

// Handlers struct to hold dependencies
type Handlers struct {
	RateLimiter services.RateLimiterInterface
}

// NewHandlers creates a new handlers instance
func NewHandlers(rateLimiter services.RateLimiterInterface) *Handlers {
	return &Handlers{
		RateLimiter: rateLimiter,
	}
}

// AcquireHandler handles POST /acquire requests
func (h *Handlers) AcquireHandler(w http.ResponseWriter, r *http.Request) {
	// Get logger from context
	logger := utils.GetLoggerFromContext(r.Context())

	if r.Method != http.MethodPost {
		logger.Warn("Invalid method", "method", r.Method)
		utils.SendError(w, http.StatusMethodNotAllowed, "Only POST method allowed")
		return
	}

	// Get user ID from JWT
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		logger.Error("User ID not found in context", nil)
		utils.SendError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	var req models.AcquireRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("Failed to decode JSON", err)
		utils.SendError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	// Override key with user ID from JWT
	req.Key = userID

	// Validate and set defaults
	if req.Tokens <= 0 {
		req.Tokens = 1 // default to 1 token
	}

	if req.Algorithm == "" {
		req.Algorithm = "token_bucket" // default algorithm
	}

	logger.Info("Processing acquire request",
		"user_id", userID,
		"tokens", req.Tokens,
		"algorithm", req.Algorithm,
	)

	// Use the rate limiter service with user ID as key
	allowed := h.RateLimiter.Acquire(req.Key, req.Tokens, req.Algorithm)

	if allowed {
		logger.Info("Request allowed", "user_id", userID)
		utils.SendAcquireSuccess(w)
	} else {
		logger.Warn("Request rate limited", "user_id", userID, "tokens_requested", req.Tokens)
		utils.SendRateLimited(w, nil) // We'll calculate retry-after later
	}
}

// StatusHandler handles GET /status requests
func (h *Handlers) StatusHandler(w http.ResponseWriter, r *http.Request) {
	// Get logger from context
	logger := utils.GetLoggerFromContext(r.Context())

	if r.Method != http.MethodGet {
		logger.Warn("Invalid method", "method", r.Method)
		utils.SendError(w, http.StatusMethodNotAllowed, "Only GET method allowed")
		return
	}

	// Get user ID from JWT context
	userID := middleware.GetUserIDFromContext(r.Context())
	if userID == "" {
		logger.Error("User ID not found in context", nil)
		utils.SendError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	logger.Info("Processing status request", "user_id", userID)

	// Get status from rate limiter service using user ID as key
	response := h.RateLimiter.GetStatus(userID)

	logger.Info("Returning status",
		"user_id", userID,
		"tokens_left", response.TokensLeft,
		"capacity", response.Capacity,
	)

	// Send JSON response
	utils.SendJSON(w, http.StatusOK, response)
}

// GenerateTokenHandler handles POST /generate-token requests (for testing)
func (h *Handlers) GenerateTokenHandler(jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := utils.GetLoggerFromContext(r.Context())

		if r.Method != http.MethodPost {
			logger.Warn("Invalid method", "method", r.Method)
			utils.SendError(w, http.StatusMethodNotAllowed, "Only POST method allowed")
			return
		}

		var req struct {
			UserID string `json:"user_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("Failed to decode JSON", err)
			utils.SendError(w, http.StatusBadRequest, "Invalid JSON body")
			return
		}

		if req.UserID == "" {
			logger.Warn("Missing user_id in request")
			utils.SendError(w, http.StatusBadRequest, "user_id is required")
			return
		}

		// Generate JWT token
		token, err := middleware.GenerateJWT(req.UserID, jwtSecret)
		if err != nil {
			logger.Error("Failed to generate JWT", err)
			utils.SendError(w, http.StatusInternalServerError, "Failed to generate token")
			return
		}

		logger.Info("Generated token", "user_id", req.UserID)

		response := map[string]interface{}{
			"token":   token,
			"user_id": req.UserID,
		}

		utils.SendJSON(w, http.StatusOK, response)
	}
}

// MetricsHandler handles GET /metrics requests
func (h *Handlers) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	// Get logger from context
	logger := utils.GetLoggerFromContext(r.Context())

	if r.Method != http.MethodGet {
		logger.Warn("Invalid method", "method", r.Method)
		utils.SendError(w, http.StatusMethodNotAllowed, "Only GET method allowed")
		return
	}

	logger.Info("Processing metrics request")

	// Check if client wants Prometheus format
	acceptHeader := r.Header.Get("Accept")
	if acceptHeader == "text/plain" || r.URL.Query().Get("format") == "prometheus" {
		// Return Prometheus format
		prometheusMetrics := h.RateLimiter.GetPrometheusMetrics()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(prometheusMetrics))
	} else {
		// Return JSON format
		metrics := h.RateLimiter.GetMetrics()
		utils.SendJSON(w, http.StatusOK, metrics)
	}

	logger.Info("Metrics returned successfully")
}
