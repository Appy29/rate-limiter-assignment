package services

import (
	"sync"
	"testing"
	"time"

	"github.com/Appy29/rate-limiter/config"
)

// mockMetrics for testing
type mockMetrics struct {
	requests     int
	successful   int
	rateLimited  int
	totalTime    time.Duration
	redisLatency time.Duration
	redisHealth  map[string]bool
	mu           sync.Mutex
}

func (m *mockMetrics) RecordRequest(allowed, rateLimited bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests++
	m.totalTime += duration
	if allowed {
		m.successful++
	}
	if rateLimited {
		m.rateLimited++
	}
}

func (m *mockMetrics) RecordRedisLatency(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.redisLatency = duration
}

func (m *mockMetrics) UpdateRedisHealth(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.redisHealth == nil {
		m.redisHealth = make(map[string]bool)
	}

	// Store overall health status - you can expand this as needed
	m.redisHealth["overall"] = healthy
}

func (m *mockMetrics) GetMetrics() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	return map[string]interface{}{
		"requests": map[string]interface{}{
			"total":        m.requests,
			"successful":   m.successful,
			"rate_limited": m.rateLimited,
		},
		"performance": map[string]interface{}{
			"avg_response_time_ms": float64(m.totalTime.Nanoseconds()) / float64(time.Millisecond.Nanoseconds()),
			"redis_latency_ms":     float64(m.redisLatency.Nanoseconds()) / float64(time.Millisecond.Nanoseconds()),
		},
		"redis": map[string]interface{}{
			"health": m.redisHealth,
		},
	}
}

func (m *mockMetrics) GetPrometheusMetrics() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return "# Mock Prometheus metrics\nrate_limiter_requests_total " +
		"100\nrate_limiter_redis_latency_seconds 0.001\n"
}

// Override constructors during testing
var (
// Remove mock instances - we'll use real constructors
)

// createTestServiceWithMocks creates a service with mocked dependencies
func createTestServiceWithMocks(redisAvailable bool) *RedisRateLimiterService {
	cfg := createTestConfig()

	// Create service normally
	service := NewRedisRateLimiterService(cfg)

	// Replace metrics with mock
	service.metrics = &mockMetrics{}

	// For Redis unavailable testing, we can simulate by using invalid Redis addresses
	// or by creating a service that will naturally fail Redis connections
	if !redisAvailable {
		// Create service with invalid Redis config to simulate Redis failure
		invalidCfg := createTestConfig()
		invalidCfg.Redis.Instances = []string{"invalid:9999"} // Non-existent Redis

		// Create new service with invalid config
		serviceWithFailingRedis := NewRedisRateLimiterService(invalidCfg)
		serviceWithFailingRedis.metrics = &mockMetrics{}
		return serviceWithFailingRedis
	}

	return service
}

// Helper to create test config
func createTestConfig() *config.Config {
	return &config.Config{
		RateLimit: struct {
			DefaultCapacity int64         `json:"default_capacity"`
			DefaultRefill   time.Duration `json:"default_refill"`
			Algorithm       string        `json:"algorithm"`
		}{
			DefaultCapacity: 100,
			DefaultRefill:   time.Second,
			Algorithm:       "token_bucket",
		},
		Redis: struct {
			Instances []string `json:"instances"`
			Password  string   `json:"password"`
			DB        int      `json:"db"`
		}{
			Instances: []string{"localhost:6379", "localhost:6380"},
			Password:  "",
			DB:        0,
		},
	}
}

// ============= CORE TESTS =============

func TestNewRedisRateLimiterService(t *testing.T) {
	cfg := createTestConfig()
	service := NewRedisRateLimiterService(cfg)

	if service == nil {
		t.Fatal("Expected service to be initialized, got nil")
	}

	if service.config != cfg {
		t.Error("Expected config to be set correctly")
	}

	if service.tokenBuckets == nil || service.leakyBuckets == nil {
		t.Error("Expected fallback buckets to be initialized")
	}

	if service.metrics == nil {
		t.Error("Expected metrics to be initialized")
	}

	if service.redisManager == nil {
		t.Error("Expected redis manager to be initialized")
	}
}

func TestGetStatus_RedisAvailable(t *testing.T) {
	service := createTestServiceWithMocks(true) // Redis available

	// Test getting status when no prior state exists
	status := service.GetStatus("new_user")

	if status.Key != "new_user" {
		t.Errorf("Expected key 'new_user', got '%s'", status.Key)
	}

	// Should have some algorithm set
	if status.Algorithm == "" {
		t.Error("Expected algorithm to be set in status response")
	}

	// Should have capacity set from config
	if status.Capacity != 100 {
		t.Errorf("Expected capacity 100, got %d", status.Capacity)
	}

	// Should have refill rate set
	if status.RefillRate != time.Second {
		t.Errorf("Expected refill rate 1s, got %v", status.RefillRate)
	}
}

func TestGetMetrics(t *testing.T) {
	service := createTestServiceWithMocks(true)

	// Create some state by making requests
	service.Acquire("user1", 5, "token_bucket")
	service.Acquire("user2", 3, "leaky_bucket")

	// Force some fallback state when Redis is unavailable
	fallbackService := createTestServiceWithMocks(false)
	fallbackService.Acquire("fallback_user", 1, "token_bucket")

	metrics := service.GetMetrics()

	if metrics == nil {
		t.Fatal("Expected metrics to be returned")
	}

	// Check that rate limiter metrics exist
	rateLimiterMetrics, exists := metrics["rate_limiter"]
	if !exists {
		t.Fatal("Expected rate_limiter metrics to exist")
	}

	rlm := rateLimiterMetrics.(map[string]interface{})

	// Check basic fields exist
	requiredFields := []string{
		"using_redis", "algorithm", "default_capacity",
		"redis_health", "fallback_token_buckets", "fallback_leaky_buckets",
	}

	for _, field := range requiredFields {
		if _, exists := rlm[field]; !exists {
			t.Errorf("Expected field '%s' to exist in rate limiter metrics", field)
		}
	}

	// Verify some expected values
	if rlm["algorithm"] != "unified_redis" {
		t.Errorf("Expected algorithm 'unified_redis', got %v", rlm["algorithm"])
	}

	if rlm["default_capacity"] != int64(100) {
		t.Errorf("Expected default_capacity 100, got %v", rlm["default_capacity"])
	}
}
