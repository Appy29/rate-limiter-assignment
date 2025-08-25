package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Appy29/rate-limiter/handlers"
	"github.com/Appy29/rate-limiter/models"
)

// mockRateLimiter is a simple mock for RateLimiterInterface
type mockRateLimiter struct{}

func (m *mockRateLimiter) Acquire(key string, tokens int64, algorithm string) bool {
	return true // always allow for testing
}

func (m *mockRateLimiter) GetStatus(key string) models.StatusResponse {
	return models.StatusResponse{
		TokensLeft: 10,
		Capacity:   20,
	}
}

func (m *mockRateLimiter) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"total_requests": 100,
		"allowed":        90,
		"rate_limited":   10,
		"current_users":  5,
	}
}

func (m *mockRateLimiter) GetPrometheusMetrics() string {
	return "rate_limiter_total_requests Total requests\nrate_limiter_total_requests 100\n"
}

func TestMetricsHandler_JSON(t *testing.T) {
	h := handlers.NewHandlers(&mockRateLimiter{})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	h.MetricsHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Errorf("failed to decode JSON response: %v", err)
	}
}

func TestMetricsHandler_Prometheus(t *testing.T) {
	h := handlers.NewHandlers(&mockRateLimiter{})

	req := httptest.NewRequest(http.MethodGet, "/metrics?format=prometheus", nil)
	w := httptest.NewRecorder()

	h.MetricsHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	if got := buf.String(); got == "" {
		t.Errorf("expected prometheus output, got empty string")
	}
}
