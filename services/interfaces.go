package services

import (
	"time"

	"github.com/Appy29/rate-limiter/models"
)

// RateLimiterInterface defines the contract for rate limiting operations
type RateLimiterInterface interface {
	Acquire(key string, tokens int64, algorithm string) bool
	GetStatus(key string) models.StatusResponse
	GetMetrics() map[string]interface{}
	GetPrometheusMetrics() string
}

// TokenBucketInterface defines the interface for token bucket operations
type TokenBucketInterface interface {
	TryConsume(tokens int64) bool
	GetStatus() (tokensLeft int64, capacity int64, nextRefill time.Time)
}

// LeakyBucketInterface defines the interface for leaky bucket operations
type LeakyBucketInterface interface {
	TryAdd(requests int64) bool
	GetStatus() (queueLength int64, capacity int64, nextLeak time.Time)
}

// Ensure interfaces are implemented
var (
	_ RateLimiterInterface = (*RedisRateLimiterService)(nil)
	_ TokenBucketInterface = (*tokenBucket)(nil)
	_ LeakyBucketInterface = (*leakyBucket)(nil)
)
