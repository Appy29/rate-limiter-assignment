package handlers

import (
	"net/http"
)

// HandlersInterface defines the interface for rate limiter HTTP handlers
type HandlersInterface interface {
	AcquireHandler(w http.ResponseWriter, r *http.Request)
	StatusHandler(w http.ResponseWriter, r *http.Request)
	GenerateTokenHandler(jwtSecret string) http.HandlerFunc
	MetricsHandler(w http.ResponseWriter, r *http.Request)
}

var _ HandlersInterface = (*Handlers)(nil)
