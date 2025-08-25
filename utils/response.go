package utils

import (
	"encoding/json"
	"net/http"

	"github.com/Appy29/rate-limiter/models"
)

// SendJSON sends a JSON response
func SendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// SendError sends an error response
func SendError(w http.ResponseWriter, status int, message string) {
	SendJSON(w, status, models.ErrorResponse{
		Error:   http.StatusText(status),
		Code:    status,
		Message: message,
	})
}

// SendSuccess sends a success response for acquire
func SendAcquireSuccess(w http.ResponseWriter) {
	SendJSON(w, http.StatusOK, models.AcquireResponse{
		Allowed: true,
		Message: "Request allowed",
	})
}

// SendRateLimited sends a rate limited response
func SendRateLimited(w http.ResponseWriter, retryAfter *int) {
	response := models.AcquireResponse{
		Allowed: false,
		Message: "Rate limit exceeded",
	}

	if retryAfter != nil {
		response.RetryAfter = retryAfter
		w.Header().Set("Retry-After", string(rune(*retryAfter)))
	}

	SendJSON(w, http.StatusTooManyRequests, response)
}
