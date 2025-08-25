package services

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// tokenBucket represents a token bucket for a specific key (private struct)
type tokenBucket struct {
	capacity   int64         // Maximum number of tokens
	tokens     int64         // Current number of tokens
	refillRate time.Duration // How often to add tokens
	lastRefill time.Time     // Last time bucket was refilled
	mutex      sync.RWMutex  // Thread safety
}

// TokenBucketRedis handles Redis-based token bucket operations
type TokenBucketRedis struct {
	client     *redis.Client
	key        string
	capacity   int64
	refillRate time.Duration
}

// NewTokenBucket creates a new in-memory token bucket (fallback only)
func NewTokenBucket(capacity int64, refillRate time.Duration) *tokenBucket {
	return &tokenBucket{
		capacity:   capacity,
		tokens:     capacity, // Start with full bucket
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// NewTokenBucketRedis creates a new Redis-based token bucket
func NewTokenBucketRedis(client *redis.Client, key string, capacity int64, refillRate time.Duration) *TokenBucketRedis {
	return &TokenBucketRedis{
		client:     client,
		key:        "rate_limit:token_bucket:" + key,
		capacity:   capacity,
		refillRate: refillRate,
	}
}

// TryConsume attempts to consume tokens from Redis-based token bucket
func (tbr *TokenBucketRedis) TryConsume(tokens int64) bool {
	if tokens < 0 {
		return false
	}

	ctx := context.Background()

	// Redis Lua script for atomic token bucket operations
	luaScript := `
		local bucket_key = KEYS[1]
		local tokens_needed = tonumber(ARGV[1])
		local capacity = tonumber(ARGV[2])
		local refill_rate_ns = tonumber(ARGV[3])
		local now_ns = tonumber(ARGV[4])
		
		-- Get current bucket data
		local bucket_data = redis.call('GET', bucket_key)
		local current_tokens, last_refill_ns
		
		if bucket_data then
			local data = cjson.decode(bucket_data)
			current_tokens = data.tokens
			last_refill_ns = data.last_refill_ns
		else
			-- New bucket, start with full capacity
			current_tokens = capacity
			last_refill_ns = now_ns
		end
		
		-- Calculate tokens to add based on time elapsed
		local time_passed_ns = now_ns - last_refill_ns
		local tokens_to_add = math.floor(time_passed_ns / refill_rate_ns)
		
		if tokens_to_add > 0 then
			current_tokens = math.min(capacity, current_tokens + tokens_to_add)
			last_refill_ns = last_refill_ns + (tokens_to_add * refill_rate_ns)
		end
		
		-- Check if we can consume the requested tokens
		if current_tokens >= tokens_needed then
			current_tokens = current_tokens - tokens_needed
			
			-- Save updated bucket data
			local updated_data = {
				algorithm = "token_bucket",
				capacity = capacity,
				tokens = current_tokens,
				refill_rate_ns = refill_rate_ns,
				last_refill_ns = last_refill_ns,
				last_updated = now_ns
			}
			
			redis.call('SET', bucket_key, cjson.encode(updated_data))
			redis.call('EXPIRE', bucket_key, 3600) -- Expire in 1 hour if unused
			
			return 1 -- Success
		else
			-- Save current state even if request failed (for accurate timing)
			local updated_data = {
				algorithm = "token_bucket",
				capacity = capacity,
				tokens = current_tokens,
				refill_rate_ns = refill_rate_ns,
				last_refill_ns = last_refill_ns,
				last_updated = now_ns
			}
			
			redis.call('SET', bucket_key, cjson.encode(updated_data))
			redis.call('EXPIRE', bucket_key, 3600)
			
			return 0 -- Failed
		end
	`

	// Execute the Lua script
	refillRate := tbr.refillRate.Nanoseconds()
	now := time.Now().UnixNano()

	result, err := tbr.client.Eval(ctx, luaScript, []string{tbr.key}, tokens, tbr.capacity, refillRate, now).Result()

	if err != nil {
		return false
	}

	return result.(int64) == 1
}

// GetStatus returns current status from Redis
func (tbr *TokenBucketRedis) GetStatus() (tokensLeft int64, capacity int64, nextRefill time.Time) {
	ctx := context.Background()

	bucketData, err := tbr.client.Get(ctx, tbr.key).Result()
	if err != nil {
		// No data in Redis, return default values
		return tbr.capacity, tbr.capacity, time.Now().Add(tbr.refillRate)
	}

	// Parse bucket data
	var data struct {
		Algorithm    string  `json:"algorithm"`
		Capacity     int64   `json:"capacity"`
		Tokens       int64   `json:"tokens"`
		RefillRateNs int64   `json:"refill_rate_ns"`
		LastRefillNs float64 `json:"last_refill_ns"`
		LastUpdated  float64 `json:"last_updated"`
	}

	if err := json.Unmarshal([]byte(bucketData), &data); err != nil {
		return tbr.capacity, tbr.capacity, time.Now().Add(tbr.refillRate)
	}

	// Calculate current tokens (simulate refill)
	now := time.Now()
	lastRefillNs := int64(data.LastRefillNs)
	timePassed := now.UnixNano() - lastRefillNs
	tokensToAdd := timePassed / data.RefillRateNs
	currentTokens := data.Tokens + tokensToAdd

	if currentTokens > data.Capacity {
		currentTokens = data.Capacity
	}

	nextRefillTime := time.Unix(0, lastRefillNs).Add(time.Duration(data.RefillRateNs))

	return currentTokens, data.Capacity, nextRefillTime
}

// HasState checks if this token bucket has state in Redis
func (tbr *TokenBucketRedis) HasState() bool {
	ctx := context.Background()
	_, err := tbr.client.Get(ctx, tbr.key).Result()
	return err == nil
}

// ===== IN-MEMORY TOKEN BUCKET (FALLBACK ONLY) =====

// TryConsume attempts to consume the specified number of tokens (in-memory)
func (tb *tokenBucket) TryConsume(tokens int64) bool {
	// Add this validation at the beginning
	if tokens < 0 {
		return false // Reject negative token requests
	}

	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	// First, refill the bucket based on time elapsed
	tb.refill()

	// Check if we have enough tokens
	if tb.tokens >= tokens {
		tb.tokens -= tokens
		return true
	}

	return false
}

// GetStatus returns current status of the bucket (in-memory)
func (tb *tokenBucket) GetStatus() (tokensLeft int64, capacity int64, nextRefill time.Time) {
	tb.mutex.RLock()
	defer tb.mutex.RUnlock()

	// Refill before returning status
	tb.refill()

	return tb.tokens, tb.capacity, tb.lastRefill.Add(tb.refillRate)
}

// refill adds tokens to the bucket based on elapsed time
// Note: This method assumes the caller already holds the lock
func (tb *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	refillPeriods := elapsed.Nanoseconds() / tb.refillRate.Nanoseconds()

	if refillPeriods > 0 {
		// Add tokens (1 token per refill period)
		tokensToAdd := refillPeriods
		tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)

		actualTimeProcessed := time.Duration(refillPeriods) * tb.refillRate
		tb.lastRefill = tb.lastRefill.Add(actualTimeProcessed)
	}
}

// min helper function
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
