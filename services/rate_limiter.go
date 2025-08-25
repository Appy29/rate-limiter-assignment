package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/Appy29/rate-limiter/config"
	"github.com/Appy29/rate-limiter/models"
)

// RedisRateLimiterService manages rate limiting using separate algorithm files
type RedisRateLimiterService struct {
	redisManager *RedisManager
	config       *config.Config
	metrics      MetricsInterface

	// In-memory fallback - only when Redis is completely unavailable
	tokenBuckets map[string]*tokenBucket
	leakyBuckets map[string]*leakyBucket
	mutex        sync.RWMutex
}

// NewRedisRateLimiterService creates a new Redis-backed rate limiter
func NewRedisRateLimiterService(cfg *config.Config) *RedisRateLimiterService {
	redisManager := NewRedisManager(cfg.Redis.Instances, cfg.Redis.Password, cfg.Redis.DB)

	return &RedisRateLimiterService{
		redisManager: redisManager,
		config:       cfg,
		metrics:      NewMetricsCollector(),
		tokenBuckets: make(map[string]*tokenBucket),
		leakyBuckets: make(map[string]*leakyBucket),
	}
}

// Acquire attempts to acquire tokens using specified algorithm
func (rrs *RedisRateLimiterService) Acquire(key string, tokens int64, algorithm string) bool {
	startTime := time.Now()

	var result bool
	var rateLimited bool

	fmt.Printf("DEBUG: Acquiring for key='%s', algorithm='%s'\n", key, algorithm)

	// Get Redis client based on key by hasing
	client := rrs.redisManager.GetClient(key)

	if client == nil {
		fmt.Printf("DEBUG: Redis unavailable - using in-memory fallback\n")
		result = rrs.acquireInMemoryFallback(key, tokens, algorithm)
		rateLimited = !result
	} else {
		switch algorithm {
		case "leaky_bucket":
			leakyBucketRedis := NewLeakyBucketRedis(client, key, rrs.config.RateLimit.DefaultCapacity, rrs.config.RateLimit.DefaultRefill)
			result = leakyBucketRedis.TryAdd(tokens)
		case "token_bucket":
			fallthrough
		default:
			tokenBucketRedis := NewTokenBucketRedis(client, key, rrs.config.RateLimit.DefaultCapacity, rrs.config.RateLimit.DefaultRefill)
			result = tokenBucketRedis.TryConsume(tokens)
		}
		rateLimited = !result
	}

	rrs.metrics.RecordRequest(result, rateLimited, time.Since(startTime))
	return result
}

// acquireInMemoryFallback - only used when Redis is completely unavailable
func (rrs *RedisRateLimiterService) acquireInMemoryFallback(key string, tokens int64, algorithm string) bool {
	fmt.Printf("DEBUG: Using in-memory fallback for %s\n", algorithm)
	switch algorithm {
	case "leaky_bucket":
		bucket := rrs.getOrCreateLeakyBucket(key)
		return bucket.TryAdd(tokens)
	case "token_bucket":
		fallthrough
	default:
		bucket := rrs.getOrCreateTokenBucket(key)
		return bucket.TryConsume(tokens)
	}
}

// GetStatus returns comprehensive status for all algorithms
func (rrs *RedisRateLimiterService) GetStatus(key string) models.StatusResponse {
	fmt.Printf("DEBUG STATUS: Getting status for key='%s'\n", key)

	// Get status for both algorithms using their separate files
	tokenBucketStatus := rrs.getTokenBucketStatus(key)
	leakyBucketStatus := rrs.getLeakyBucketStatus(key)

	// Determine primary algorithm based on which has been used
	var primaryStatus models.AlgorithmStatus
	var primaryAlgorithm string

	if tokenBucketStatus.HasState && leakyBucketStatus.HasState {
		// Both exist - choose the one with activity (not at full capacity)
		if tokenBucketStatus.TokensLeft < tokenBucketStatus.Capacity {
			primaryAlgorithm = "token_bucket"
			primaryStatus = tokenBucketStatus
		} else if leakyBucketStatus.TokensLeft < leakyBucketStatus.Capacity {
			primaryAlgorithm = "leaky_bucket"
			primaryStatus = leakyBucketStatus
		} else {
			// Both at full capacity, default to token bucket
			primaryAlgorithm = "token_bucket"
			primaryStatus = tokenBucketStatus
		}
	} else if tokenBucketStatus.HasState {
		primaryAlgorithm = "token_bucket"
		primaryStatus = tokenBucketStatus
	} else if leakyBucketStatus.HasState {
		primaryAlgorithm = "leaky_bucket"
		primaryStatus = leakyBucketStatus
	} else {
		// No state found, return default token bucket
		primaryAlgorithm = "token_bucket"
		primaryStatus = tokenBucketStatus
	}

	fmt.Printf("DEBUG STATUS: Primary algorithm: %s\n", primaryAlgorithm)

	// Build comprehensive response
	response := models.StatusResponse{
		Key:            key,
		Algorithm:      primaryAlgorithm,
		TokensLeft:     primaryStatus.TokensLeft,
		Capacity:       primaryStatus.Capacity,
		RefillRate:     primaryStatus.RefillRate,
		NextRefillTime: primaryStatus.NextRefillTime,
		IsBlocked:      primaryStatus.IsBlocked,
	}

	// Add detailed status for algorithms that have state
	if tokenBucketStatus.HasState {
		response.TokenBucketStatus = &tokenBucketStatus
	}
	if leakyBucketStatus.HasState {
		response.LeakyBucketStatus = &leakyBucketStatus
	}

	return response
}

// getTokenBucketStatus gets status using token_bucket.go
func (rrs *RedisRateLimiterService) getTokenBucketStatus(key string) models.AlgorithmStatus {
	client := rrs.redisManager.GetClient(key)

	if client == nil {
		// Redis unavailable - check in-memory fallback
		return rrs.getInMemoryTokenBucketStatus(key)
	}

	// Use the TokenBucketRedis from token_bucket.go
	tokenBucketRedis := NewTokenBucketRedis(client, key, rrs.config.RateLimit.DefaultCapacity, rrs.config.RateLimit.DefaultRefill)

	if !tokenBucketRedis.HasState() {
		// No state in Redis
		return models.AlgorithmStatus{
			Algorithm:      "token_bucket",
			TokensLeft:     rrs.config.RateLimit.DefaultCapacity,
			Capacity:       rrs.config.RateLimit.DefaultCapacity,
			RefillRate:     rrs.config.RateLimit.DefaultRefill,
			NextRefillTime: time.Now().Add(rrs.config.RateLimit.DefaultRefill),
			IsBlocked:      false,
			HasState:       false,
		}
	}

	// Get status from Redis via token_bucket.go
	tokensLeft, capacity, nextRefill := tokenBucketRedis.GetStatus()

	return models.AlgorithmStatus{
		Algorithm:      "token_bucket",
		TokensLeft:     tokensLeft,
		Capacity:       capacity,
		RefillRate:     rrs.config.RateLimit.DefaultRefill,
		NextRefillTime: nextRefill,
		IsBlocked:      tokensLeft == 0,
		HasState:       true,
	}
}

// getLeakyBucketStatus gets status using leaky_bucket.go
func (rrs *RedisRateLimiterService) getLeakyBucketStatus(key string) models.AlgorithmStatus {
	client := rrs.redisManager.GetClient(key)

	if client == nil {
		// Redis unavailable - check in-memory fallback
		return rrs.getInMemoryLeakyBucketStatus(key)
	}

	// Use the LeakyBucketRedis from leaky_bucket.go
	leakyBucketRedis := NewLeakyBucketRedis(client, key, rrs.config.RateLimit.DefaultCapacity, rrs.config.RateLimit.DefaultRefill)

	if !leakyBucketRedis.HasState() {
		// No state in Redis
		return models.AlgorithmStatus{
			Algorithm:      "leaky_bucket",
			TokensLeft:     rrs.config.RateLimit.DefaultCapacity,
			Capacity:       rrs.config.RateLimit.DefaultCapacity,
			RefillRate:     rrs.config.RateLimit.DefaultRefill,
			NextRefillTime: time.Now().Add(rrs.config.RateLimit.DefaultRefill),
			IsBlocked:      false,
			HasState:       false,
		}
	}

	// Get status from Redis via leaky_bucket.go
	queueLength, capacity, nextLeak := leakyBucketRedis.GetStatus()
	availableSpace := capacity - queueLength

	return models.AlgorithmStatus{
		Algorithm:      "leaky_bucket",
		TokensLeft:     availableSpace,
		Capacity:       capacity,
		RefillRate:     rrs.config.RateLimit.DefaultRefill,
		NextRefillTime: nextLeak,
		IsBlocked:      queueLength >= capacity,
		HasState:       true,
	}
}

// ===== IN-MEMORY FALLBACK METHODS (only when Redis is unavailable) =====

func (rrs *RedisRateLimiterService) getInMemoryTokenBucketStatus(key string) models.AlgorithmStatus {
	rrs.mutex.RLock()
	bucket, exists := rrs.tokenBuckets[key]
	rrs.mutex.RUnlock()

	if exists {
		tokensLeft, capacity, nextRefill := bucket.GetStatus()
		return models.AlgorithmStatus{
			Algorithm:      "token_bucket",
			TokensLeft:     tokensLeft,
			Capacity:       capacity,
			RefillRate:     rrs.config.RateLimit.DefaultRefill,
			NextRefillTime: nextRefill,
			IsBlocked:      tokensLeft == 0,
			HasState:       true,
		}
	}

	return models.AlgorithmStatus{
		Algorithm:      "token_bucket",
		TokensLeft:     rrs.config.RateLimit.DefaultCapacity,
		Capacity:       rrs.config.RateLimit.DefaultCapacity,
		RefillRate:     rrs.config.RateLimit.DefaultRefill,
		NextRefillTime: time.Now().Add(rrs.config.RateLimit.DefaultRefill),
		IsBlocked:      false,
		HasState:       false,
	}
}

func (rrs *RedisRateLimiterService) getInMemoryLeakyBucketStatus(key string) models.AlgorithmStatus {
	rrs.mutex.RLock()
	bucket, exists := rrs.leakyBuckets[key]
	rrs.mutex.RUnlock()

	if exists {
		queueLength, capacity, nextLeak := bucket.GetStatus()
		return models.AlgorithmStatus{
			Algorithm:      "leaky_bucket",
			TokensLeft:     capacity - queueLength,
			Capacity:       capacity,
			RefillRate:     rrs.config.RateLimit.DefaultRefill,
			NextRefillTime: nextLeak,
			IsBlocked:      queueLength >= capacity,
			HasState:       true,
		}
	}

	return models.AlgorithmStatus{
		Algorithm:      "leaky_bucket",
		TokensLeft:     rrs.config.RateLimit.DefaultCapacity,
		Capacity:       rrs.config.RateLimit.DefaultCapacity,
		RefillRate:     rrs.config.RateLimit.DefaultRefill,
		NextRefillTime: time.Now().Add(rrs.config.RateLimit.DefaultRefill),
		IsBlocked:      false,
		HasState:       false,
	}
}

// Bucket creation methods (fallback only when Redis is unavailable)
func (rrs *RedisRateLimiterService) getOrCreateTokenBucket(key string) *tokenBucket {
	rrs.mutex.RLock()
	if bucket, exists := rrs.tokenBuckets[key]; exists {
		rrs.mutex.RUnlock()
		return bucket
	}
	rrs.mutex.RUnlock()

	rrs.mutex.Lock()
	defer rrs.mutex.Unlock()

	if bucket, exists := rrs.tokenBuckets[key]; exists {
		return bucket
	}

	bucket := NewTokenBucket(
		rrs.config.RateLimit.DefaultCapacity,
		rrs.config.RateLimit.DefaultRefill,
	)
	rrs.tokenBuckets[key] = bucket
	return bucket
}

func (rrs *RedisRateLimiterService) getOrCreateLeakyBucket(key string) *leakyBucket {
	rrs.mutex.RLock()
	if bucket, exists := rrs.leakyBuckets[key]; exists {
		rrs.mutex.RUnlock()
		return bucket
	}
	rrs.mutex.RUnlock()

	rrs.mutex.Lock()
	defer rrs.mutex.Unlock()

	if bucket, exists := rrs.leakyBuckets[key]; exists {
		return bucket
	}

	bucket := NewLeakyBucket(
		rrs.config.RateLimit.DefaultCapacity,
		rrs.config.RateLimit.DefaultRefill,
	)
	rrs.leakyBuckets[key] = bucket
	return bucket
}

// GetMetrics returns basic metrics about the rate limiter
func (rrs *RedisRateLimiterService) GetMetrics() map[string]interface{} {
	healthStatus := rrs.redisManager.GetHealthStatus()
	healthyCount := 0
	for _, healthy := range healthStatus {
		if healthy {
			healthyCount++
		}
	}

	rrs.mutex.RLock()
	tokenBucketCount := len(rrs.tokenBuckets)
	leakyBucketCount := len(rrs.leakyBuckets)
	rrs.mutex.RUnlock()

	// Get metrics from our metrics collector
	metricsData := rrs.metrics.GetMetrics()

	// Merge with rate limiter specific metrics
	result := make(map[string]interface{})
	for k, v := range metricsData {
		result[k] = v
	}

	// Add rate limiter specific info
	result["rate_limiter"] = map[string]interface{}{
		"using_redis":            true,
		"redis_instances":        len(rrs.redisManager.clients),
		"healthy_instances":      healthyCount,
		"using_fallback":         healthyCount == 0,
		"algorithm":              "unified_redis", // Both algorithms use Redis
		"default_capacity":       rrs.config.RateLimit.DefaultCapacity,
		"default_refill_rate":    rrs.config.RateLimit.DefaultRefill.String(),
		"redis_health":           healthStatus,
		"fallback_token_buckets": tokenBucketCount,
		"fallback_leaky_buckets": leakyBucketCount,
	}

	return result
}

// GetPrometheusMetrics returns metrics in Prometheus format
func (rrs *RedisRateLimiterService) GetPrometheusMetrics() string {
	return rrs.metrics.GetPrometheusMetrics()
}
