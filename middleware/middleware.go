package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"github.com/Appy29/rate-limiter/utils"
)

// ContextMiddleware adds context with request ID, logger, and timing to each request
func ContextMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Generate request ID
		requestID := generateRequestID()

		// Create context with values
		ctx := context.WithValue(r.Context(), utils.RequestIDKey, requestID)
		ctx = context.WithValue(ctx, utils.LoggerKey, utils.NewContextLogger(requestID))
		ctx = context.WithValue(ctx, utils.StartTimeKey, time.Now())

		// Create new request with context
		r = r.WithContext(ctx)

		// Get logger from context
		logger := utils.GetLoggerFromContext(ctx)

		// Log request start
		logger.Info("Request started",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
		)

		// Create a custom response writer to capture status code
		wrappedWriter := &responseWriter{ResponseWriter: w, statusCode: 200}

		// Call next handler
		next(wrappedWriter, r)

		// Log request completion
		duration := time.Since(utils.GetStartTimeFromContext(ctx))
		logger.Info("Request completed",
			"status_code", wrappedWriter.statusCode,
			"duration_ms", duration.Milliseconds(),
		)
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// generateRequestID generates a random request ID
func generateRequestID() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
}
