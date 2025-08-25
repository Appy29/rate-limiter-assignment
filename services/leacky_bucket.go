package services

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// leakyBucket represents a leaky bucket for a specific key (private struct)
type leakyBucket struct {
	capacity int64         // Maximum requests that can be queued
	queue    int64         // Current requests in queue
	leakRate time.Duration // Time between processing requests
	lastLeak time.Time     // Last time a request was processed
	mutex    sync.RWMutex  // Thread safety
}

// LeakyBucketRedis handles Redis-based leaky bucket operations
type LeakyBucketRedis struct {
	client   *redis.Client
	key      string
	capacity int64
	leakRate time.Duration
}

// NewLeakyBucket creates a new in-memory leaky bucket (fallback only)
func NewLeakyBucket(capacity int64, leakRate time.Duration) *leakyBucket {
	return &leakyBucket{
		capacity: capacity,
		queue:    0, // Start with empty bucket
		leakRate: leakRate,
		lastLeak: time.Now(),
	}
}

// NewLeakyBucketRedis creates a new Redis-based leaky bucket
func NewLeakyBucketRedis(client *redis.Client, key string, capacity int64, leakRate time.Duration) *LeakyBucketRedis {
	return &LeakyBucketRedis{
		client:   client,
		key:      "rate_limit:leaky_bucket:" + key,
		capacity: capacity,
		leakRate: leakRate,
	}
}

// TryAdd attempts to add requests to Redis-based leaky bucket
func (lbr *LeakyBucketRedis) TryAdd(requests int64) bool {
	if requests < 0 {
		return false
	}

	ctx := context.Background()

	// Redis Lua script for atomic leaky bucket operations
	luaScript := `
		local bucket_key = KEYS[1]
		local requests_to_add = tonumber(ARGV[1])
		local capacity = tonumber(ARGV[2])
		local leak_rate_ns = tonumber(ARGV[3])
		local now_ns = tonumber(ARGV[4])
		
		-- Get current bucket data
		local bucket_data = redis.call('GET', bucket_key)
		local current_queue, last_leak_ns
		
		if bucket_data then
			local data = cjson.decode(bucket_data)
			current_queue = data.queue_length
			last_leak_ns = data.last_leak_ns
		else
			-- New bucket, start empty
			current_queue = 0
			last_leak_ns = now_ns
		end
		
		-- Calculate how many requests have leaked out
		local time_passed_ns = now_ns - last_leak_ns
		local leak_periods = math.floor(time_passed_ns / leak_rate_ns)
		
		if leak_periods > 0 and current_queue > 0 then
			-- Remove leaked requests (one request per leak period)
			local requests_to_leak = math.min(leak_periods, current_queue)
			current_queue = current_queue - requests_to_leak
			last_leak_ns = last_leak_ns + (requests_to_leak * leak_rate_ns)
		end
		
		-- Check if we can add the new requests
		if current_queue + requests_to_add <= capacity then
			current_queue = current_queue + requests_to_add
			
			-- Save updated bucket data
			local updated_data = {
				algorithm = "leaky_bucket",
				capacity = capacity,
				queue_length = current_queue,
				leak_rate_ns = leak_rate_ns,
				last_leak_ns = last_leak_ns,
				last_updated = now_ns
			}
			
			redis.call('SET', bucket_key, cjson.encode(updated_data))
			redis.call('EXPIRE', bucket_key, 3600) -- Expire in 1 hour if unused
			
			return 1 -- Success
		else
			-- Save current state even if request failed
			local updated_data = {
				algorithm = "leaky_bucket",
				capacity = capacity,
				queue_length = current_queue,
				leak_rate_ns = leak_rate_ns,
				last_leak_ns = last_leak_ns,
				last_updated = now_ns
			}
			
			redis.call('SET', bucket_key, cjson.encode(updated_data))
			redis.call('EXPIRE', bucket_key, 3600)
			
			return 0 -- Failed
		end
	`

	// Execute the Lua script
	leakRateNs := lbr.leakRate.Nanoseconds()
	nowNs := time.Now().UnixNano()

	result, err := lbr.client.Eval(ctx, luaScript, []string{lbr.key}, requests, lbr.capacity, leakRateNs, nowNs).Result()

	if err != nil {
		return false
	}

	return result.(int64) == 1
}

// GetStatus returns current status from Redis
func (lbr *LeakyBucketRedis) GetStatus() (queueLength int64, capacity int64, nextLeak time.Time) {
	ctx := context.Background()

	bucketData, err := lbr.client.Get(ctx, lbr.key).Result()
	if err != nil {
		// No data in Redis, return default values
		return 0, lbr.capacity, time.Now().Add(lbr.leakRate)
	}

	// Parse bucket data
	var data struct {
		Algorithm   string  `json:"algorithm"`
		Capacity    int64   `json:"capacity"`
		QueueLength int64   `json:"queue_length"`
		LeakRateNs  int64   `json:"leak_rate_ns"`
		LastLeakNs  float64 `json:"last_leak_ns"`
		LastUpdated float64 `json:"last_updated"`
	}

	if err := json.Unmarshal([]byte(bucketData), &data); err != nil {
		return 0, lbr.capacity, time.Now().Add(lbr.leakRate)
	}

	// Calculate current state (simulate leaking)
	now := time.Now()
	lastLeakNs := int64(data.LastLeakNs)
	timePassed := now.UnixNano() - lastLeakNs
	leakPeriods := timePassed / data.LeakRateNs
	currentQueue := data.QueueLength - leakPeriods

	if currentQueue < 0 {
		currentQueue = 0
	}

	nextLeakTime := time.Unix(0, lastLeakNs).Add(time.Duration(data.LeakRateNs))

	return currentQueue, data.Capacity, nextLeakTime
}

// HasState checks if this leaky bucket has state in Redis
func (lbr *LeakyBucketRedis) HasState() bool {
	ctx := context.Background()
	_, err := lbr.client.Get(ctx, lbr.key).Result()
	return err == nil
}

// TryAdd attempts to add requests to the bucket (in-memory)
// Returns true if successful, false if bucket overflows
func (lb *leakyBucket) TryAdd(requests int64) bool {
	if requests < 0 {
		return false
	}

	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	// First, process any leaked requests based on time elapsed
	lb.leak()

	// Check if adding these requests would overflow the bucket
	if lb.queue+requests > lb.capacity {
		return false
	}

	// Add requests to the queue
	lb.queue += requests
	return true
}

// GetStatus returns current status of the bucket (in-memory)
func (lb *leakyBucket) GetStatus() (queueLength int64, capacity int64, nextLeak time.Time) {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	// Process leaks before returning status
	lb.leak()

	return lb.queue, lb.capacity, lb.lastLeak.Add(lb.leakRate)
}

// leak processes requests based on elapsed time (in-memory)
func (lb *leakyBucket) leak() {
	now := time.Now()
	elapsed := now.Sub(lb.lastLeak)

	// Calculate how many leak periods have passed
	leakPeriods := int64(elapsed / lb.leakRate)

	if leakPeriods > 0 && lb.queue > 0 {
		// Remove requests from queue (one request per leak period)
		requestsToLeak := min(leakPeriods, lb.queue)
		lb.queue -= requestsToLeak

		// Update last leak time
		lb.lastLeak = lb.lastLeak.Add(time.Duration(requestsToLeak) * lb.leakRate)
	}
}
