package services

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

// MetricsInterface defines the interface for metrics collection
type MetricsInterface interface {
	RecordRequest(success bool, rateLimited bool, responseTime time.Duration)
	RecordRedisLatency(latency time.Duration)
	UpdateRedisHealth(healthy bool)
	GetMetrics() map[string]interface{}
	GetPrometheusMetrics() string
}

// MetricsCollector collects and tracks various metrics
type MetricsCollector struct {
	// Request counters (using atomic for thread safety)
	totalRequests       int64
	successfulRequests  int64
	rateLimitedRequests int64
	errorRequests       int64

	// Performance metrics
	totalResponseTime int64 // in nanoseconds
	redisLatencyTotal int64 // in nanoseconds
	redisRequestCount int64

	// Redis health
	redisHealthy   int32 // using int32 for atomic operations (0=false, 1=true)
	lastRedisCheck int64 // Unix timestamp

	// Service start time
	startTime time.Time
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() MetricsInterface {
	return &MetricsCollector{
		startTime: time.Now(),
	}
}

// RecordRequest records a request with its outcome and timing
func (mc *MetricsCollector) RecordRequest(success bool, rateLimited bool, responseTime time.Duration) {
	atomic.AddInt64(&mc.totalRequests, 1)
	atomic.AddInt64(&mc.totalResponseTime, responseTime.Nanoseconds())

	if success {
		atomic.AddInt64(&mc.successfulRequests, 1)
	} else if rateLimited {
		atomic.AddInt64(&mc.rateLimitedRequests, 1)
	} else {
		atomic.AddInt64(&mc.errorRequests, 1)
	}
}

// RecordRedisLatency records Redis operation latency
func (mc *MetricsCollector) RecordRedisLatency(latency time.Duration) {
	atomic.AddInt64(&mc.redisLatencyTotal, latency.Nanoseconds())
	atomic.AddInt64(&mc.redisRequestCount, 1)
}

// UpdateRedisHealth updates Redis health status
func (mc *MetricsCollector) UpdateRedisHealth(healthy bool) {
	var value int32 = 0
	if healthy {
		value = 1
	}
	atomic.StoreInt32(&mc.redisHealthy, value)
	atomic.StoreInt64(&mc.lastRedisCheck, time.Now().Unix())
}

// GetMetrics returns metrics in a structured format
func (mc *MetricsCollector) GetMetrics() map[string]interface{} {
	// Load all atomic values
	totalRequests := atomic.LoadInt64(&mc.totalRequests)
	successfulRequests := atomic.LoadInt64(&mc.successfulRequests)
	rateLimitedRequests := atomic.LoadInt64(&mc.rateLimitedRequests)
	errorRequests := atomic.LoadInt64(&mc.errorRequests)
	totalResponseTime := atomic.LoadInt64(&mc.totalResponseTime)
	redisLatencyTotal := atomic.LoadInt64(&mc.redisLatencyTotal)
	redisRequestCount := atomic.LoadInt64(&mc.redisRequestCount)
	redisHealthy := atomic.LoadInt32(&mc.redisHealthy) == 1
	lastRedisCheck := atomic.LoadInt64(&mc.lastRedisCheck)

	// Calculate averages
	var avgResponseTime float64
	var avgRedisLatency float64
	var requestRate float64

	if totalRequests > 0 {
		avgResponseTime = float64(totalResponseTime) / float64(totalRequests) / 1e6 // Convert to milliseconds
		uptime := time.Since(mc.startTime).Seconds()
		if uptime > 0 {
			requestRate = float64(totalRequests) / uptime
		}
	}

	if redisRequestCount > 0 {
		avgRedisLatency = float64(redisLatencyTotal) / float64(redisRequestCount) / 1e6 // Convert to milliseconds
	}

	return map[string]interface{}{
		"service": map[string]interface{}{
			"name":    "rate-limiter",
			"version": "1.0.0",
			"uptime":  time.Since(mc.startTime).Seconds(),
		},
		"requests": map[string]interface{}{
			"total":        totalRequests,
			"successful":   successfulRequests,
			"rate_limited": rateLimitedRequests,
			"errors":       errorRequests,
			"rate_per_sec": requestRate,
		},
		"performance": map[string]interface{}{
			"avg_response_time_ms": avgResponseTime,
			"active_goroutines":    runtime.NumGoroutine(),
		},
		"redis": map[string]interface{}{
			"healthy":              redisHealthy,
			"last_health_check":    lastRedisCheck,
			"avg_latency_ms":       avgRedisLatency,
			"total_redis_requests": redisRequestCount,
		},
		"memory": map[string]interface{}{
			"alloc_mb": bToMb(getCurrentMemoryUsage()),
			"sys_mb":   bToMb(getSystemMemoryUsage()),
			"gc_runs":  getGCRuns(),
		},
		"timestamp": time.Now().Unix(),
	}
}

// GetPrometheusMetrics returns metrics in Prometheus format
func (mc *MetricsCollector) GetPrometheusMetrics() string {
	// Load all atomic values
	totalRequests := atomic.LoadInt64(&mc.totalRequests)
	successfulRequests := atomic.LoadInt64(&mc.successfulRequests)
	rateLimitedRequests := atomic.LoadInt64(&mc.rateLimitedRequests)
	errorRequests := atomic.LoadInt64(&mc.errorRequests)
	totalResponseTime := atomic.LoadInt64(&mc.totalResponseTime)
	redisLatencyTotal := atomic.LoadInt64(&mc.redisLatencyTotal)
	redisRequestCount := atomic.LoadInt64(&mc.redisRequestCount)
	redisHealthy := atomic.LoadInt32(&mc.redisHealthy) == 1

	// Calculate averages
	var avgResponseTime float64
	var avgRedisLatency float64
	var requestRate float64

	if totalRequests > 0 {
		avgResponseTime = float64(totalResponseTime) / float64(totalRequests) / 1e6
		uptime := time.Since(mc.startTime).Seconds()
		if uptime > 0 {
			requestRate = float64(totalRequests) / uptime
		}
	}

	if redisRequestCount > 0 {
		avgRedisLatency = float64(redisLatencyTotal) / float64(redisRequestCount) / 1e6
	}

	redisHealthyValue := 0
	if redisHealthy {
		redisHealthyValue = 1
	}

	prometheus := `# HELP rate_limiter_requests_total Total number of requests
# TYPE rate_limiter_requests_total counter
rate_limiter_requests_total{status="success"} %d
rate_limiter_requests_total{status="rate_limited"} %d
rate_limiter_requests_total{status="error"} %d

# HELP rate_limiter_requests_current Current request rate per second
# TYPE rate_limiter_requests_current gauge
rate_limiter_requests_current %.2f

# HELP rate_limiter_response_time_avg Average response time in milliseconds
# TYPE rate_limiter_response_time_avg gauge
rate_limiter_response_time_avg %.2f

# HELP rate_limiter_goroutines Active goroutines
# TYPE rate_limiter_goroutines gauge
rate_limiter_goroutines %d

# HELP rate_limiter_redis_healthy Redis health status (1=healthy, 0=unhealthy)
# TYPE rate_limiter_redis_healthy gauge
rate_limiter_redis_healthy %d

# HELP rate_limiter_redis_latency_avg Average Redis latency in milliseconds
# TYPE rate_limiter_redis_latency_avg gauge
rate_limiter_redis_latency_avg %.2f

# HELP rate_limiter_memory_alloc_mb Allocated memory in MB
# TYPE rate_limiter_memory_alloc_mb gauge
rate_limiter_memory_alloc_mb %.2f

# HELP rate_limiter_uptime_seconds Service uptime in seconds
# TYPE rate_limiter_uptime_seconds gauge
rate_limiter_uptime_seconds %.2f
`

	return fmt.Sprintf(prometheus,
		successfulRequests, rateLimitedRequests, errorRequests,
		requestRate,
		avgResponseTime,
		runtime.NumGoroutine(),
		redisHealthyValue,
		avgRedisLatency,
		bToMb(getCurrentMemoryUsage()),
		time.Since(mc.startTime).Seconds(),
	)
}

// Helper functions for memory metrics
func getCurrentMemoryUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func getSystemMemoryUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys
}

func getGCRuns() uint32 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.NumGC
}

func bToMb(b uint64) float64 {
	return float64(b) / 1024 / 1024
}
