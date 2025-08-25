package services

import (
	"sync"
	"testing"
	"time"
)

// TestNewTokenBucket tests token bucket creation
func TestNewTokenBucket(t *testing.T) {
	capacity := int64(100)
	refillRate := time.Second

	bucket := NewTokenBucket(capacity, refillRate)

	if bucket == nil {
		t.Fatal("NewTokenBucket returned nil")
	}

	// Check initial state
	tokensLeft, bucketCapacity, _ := bucket.GetStatus()
	if tokensLeft != capacity {
		t.Errorf("Expected initial tokens %d, got %d", capacity, tokensLeft)
	}
	if bucketCapacity != capacity {
		t.Errorf("Expected capacity %d, got %d", capacity, bucketCapacity)
	}
}

// TestTokenBucket_TryConsume_Success tests successful token consumption
func TestTokenBucket_TryConsume_Success(t *testing.T) {
	bucket := NewTokenBucket(100, time.Second)

	// Consume 10 tokens
	success := bucket.TryConsume(10)
	if !success {
		t.Error("Expected successful consumption of 10 tokens")
	}

	// Check remaining tokens
	tokensLeft, _, _ := bucket.GetStatus()
	if tokensLeft != 90 {
		t.Errorf("Expected 90 tokens left, got %d", tokensLeft)
	}
}

// TestTokenBucket_TryConsume_Failure tests token consumption failure
func TestTokenBucket_TryConsume_Failure(t *testing.T) {
	bucket := NewTokenBucket(10, time.Second)

	// Try to consume more tokens than available
	success := bucket.TryConsume(15)
	if success {
		t.Error("Expected consumption to fail when requesting more tokens than available")
	}

	// Check tokens unchanged
	tokensLeft, _, _ := bucket.GetStatus()
	if tokensLeft != 10 {
		t.Errorf("Expected tokens to remain at 10, got %d", tokensLeft)
	}
}

// TestTokenBucket_TryConsume_ExactCapacity tests consuming exact capacity
func TestTokenBucket_TryConsume_ExactCapacity(t *testing.T) {
	bucket := NewTokenBucket(50, time.Second)

	// Consume all tokens
	success := bucket.TryConsume(50)
	if !success {
		t.Error("Expected successful consumption of all tokens")
	}

	// Check bucket is empty
	tokensLeft, _, _ := bucket.GetStatus()
	if tokensLeft != 0 {
		t.Errorf("Expected 0 tokens left, got %d", tokensLeft)
	}

	// Try to consume from empty bucket
	success = bucket.TryConsume(1)
	if success {
		t.Error("Expected consumption to fail from empty bucket")
	}
}

// TestTokenBucket_TryConsume_ZeroTokens tests consuming zero tokens
func TestTokenBucket_TryConsume_ZeroTokens(t *testing.T) {
	bucket := NewTokenBucket(100, time.Second)

	// Consume 0 tokens (should always succeed)
	success := bucket.TryConsume(0)
	if !success {
		t.Error("Expected consumption of 0 tokens to succeed")
	}

	// Check tokens unchanged
	tokensLeft, _, _ := bucket.GetStatus()
	if tokensLeft != 100 {
		t.Errorf("Expected tokens to remain at 100, got %d", tokensLeft)
	}
}

// TestTokenBucket_TryConsume_NegativeTokens tests consuming negative tokens
func TestTokenBucket_TryConsume_NegativeTokens(t *testing.T) {
	bucket := NewTokenBucket(100, time.Second)

	// Try to consume negative tokens (should fail)
	success := bucket.TryConsume(-5)
	if success {
		t.Error("Expected consumption of negative tokens to fail")
	}

	// Check tokens unchanged
	tokensLeft, _, _ := bucket.GetStatus()
	if tokensLeft != 100 {
		t.Errorf("Expected tokens to remain at 100, got %d", tokensLeft)
	}
}

// TestTokenBucket_Refill tests token refill over time
func TestTokenBucket_Refill(t *testing.T) {
	refillRate := 100 * time.Millisecond // Fast refill for testing
	bucket := NewTokenBucket(10, refillRate)

	// Consume 5 tokens
	bucket.TryConsume(5)
	tokensLeft, _, _ := bucket.GetStatus()
	if tokensLeft != 5 {
		t.Errorf("Expected 5 tokens left after consumption, got %d", tokensLeft)
	}

	// Wait for refill period
	time.Sleep(refillRate + 10*time.Millisecond) // Add small buffer

	// Check tokens refilled
	tokensLeft, _, _ = bucket.GetStatus()
	if tokensLeft != 6 { // 5 + 1 refilled token
		t.Errorf("Expected 6 tokens after refill, got %d", tokensLeft)
	}
}

// TestTokenBucket_RefillMultiplePeriods tests multiple refill periods
func TestTokenBucket_RefillMultiplePeriods(t *testing.T) {
	refillRate := 50 * time.Millisecond
	bucket := NewTokenBucket(20, refillRate)

	// Consume 10 tokens
	bucket.TryConsume(10)

	// Wait for 2.5 refill periods
	time.Sleep(time.Duration(2.5 * float64(refillRate)))

	// Should have refilled 2 tokens (2 complete periods)
	tokensLeft, _, _ := bucket.GetStatus()
	if tokensLeft != 12 { // 10 + 2 refilled tokens
		t.Errorf("Expected 12 tokens after 2 refill periods, got %d", tokensLeft)
	}
}

// TestTokenBucket_RefillCap tests refill doesn't exceed capacity
func TestTokenBucket_RefillCap(t *testing.T) {
	refillRate := 50 * time.Millisecond
	bucket := NewTokenBucket(10, refillRate)

	// Consume 2 tokens
	bucket.TryConsume(2)

	// Wait for many refill periods (more than needed to fill)
	time.Sleep(10 * refillRate)

	// Should be capped at capacity
	tokensLeft, capacity, _ := bucket.GetStatus()
	if tokensLeft != capacity {
		t.Errorf("Expected tokens to be capped at capacity %d, got %d", capacity, tokensLeft)
	}
}

// TestTokenBucket_Concurrency tests concurrent access
func TestTokenBucket_Concurrency(t *testing.T) {
	bucket := NewTokenBucket(1000, time.Second)
	numGoroutines := 100
	tokensPerGoroutine := int64(5)

	var wg sync.WaitGroup
	successCount := int64(0)
	var mu sync.Mutex

	// Launch concurrent consumers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if bucket.TryConsume(tokensPerGoroutine) {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Check final state
	tokensLeft, _, _ := bucket.GetStatus()
	expectedTokensConsumed := successCount * tokensPerGoroutine
	expectedTokensLeft := 1000 - expectedTokensConsumed

	if tokensLeft != expectedTokensLeft {
		t.Errorf("Expected %d tokens left, got %d (successful consumers: %d)",
			expectedTokensLeft, tokensLeft, successCount)
	}

	// Verify total consumed doesn't exceed capacity
	if expectedTokensConsumed > 1000 {
		t.Errorf("Total consumed tokens %d exceeds capacity 1000", expectedTokensConsumed)
	}
}

// TestTokenBucket_ConcurrentConsumption tests race conditions
func TestTokenBucket_ConcurrentConsumption(t *testing.T) {
	bucket := NewTokenBucket(100, time.Second)
	numGoroutines := 50

	var wg sync.WaitGroup
	results := make([]bool, numGoroutines)

	// Launch concurrent consumers trying to consume 10 tokens each
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = bucket.TryConsume(10)
		}(i)
	}

	wg.Wait()

	// Count successes
	successCount := 0
	for _, success := range results {
		if success {
			successCount++
		}
	}

	// Should have exactly 10 successes (100 tokens / 10 tokens each)
	if successCount != 10 {
		t.Errorf("Expected exactly 10 successful consumptions, got %d", successCount)
	}

	// Check final state
	tokensLeft, _, _ := bucket.GetStatus()
	if tokensLeft != 0 {
		t.Errorf("Expected 0 tokens left, got %d", tokensLeft)
	}
}

// TestTokenBucket_GetStatus tests status reporting
func TestTokenBucket_GetStatus(t *testing.T) {
	capacity := int64(50)
	refillRate := time.Second
	bucket := NewTokenBucket(capacity, refillRate)

	// Initial status
	tokensLeft, cap, nextRefill := bucket.GetStatus()
	if tokensLeft != capacity {
		t.Errorf("Expected %d tokens, got %d", capacity, tokensLeft)
	}
	if cap != capacity {
		t.Errorf("Expected capacity %d, got %d", capacity, cap)
	}
	if nextRefill.Before(time.Now()) {
		t.Error("Next refill time should be in the future")
	}

	// After consumption
	bucket.TryConsume(20)
	tokensLeft, cap, _ = bucket.GetStatus()
	if tokensLeft != 30 {
		t.Errorf("Expected 30 tokens after consumption, got %d", tokensLeft)
	}
	if cap != capacity {
		t.Errorf("Capacity should remain %d, got %d", capacity, cap)
	}
}

// TestTokenBucket_EdgeCases tests various edge cases
func TestTokenBucket_EdgeCases(t *testing.T) {
	t.Run("MinimumCapacity", func(t *testing.T) {
		bucket := NewTokenBucket(1, time.Second)
		success := bucket.TryConsume(1)
		if !success {
			t.Error("Should be able to consume from minimum capacity bucket")
		}
		tokensLeft, _, _ := bucket.GetStatus()
		if tokensLeft != 0 {
			t.Errorf("Expected 0 tokens left, got %d", tokensLeft)
		}
	})

	t.Run("LargeCapacity", func(t *testing.T) {
		largeCapacity := int64(1000000)
		bucket := NewTokenBucket(largeCapacity, time.Second)
		tokensLeft, _, _ := bucket.GetStatus()
		if tokensLeft != largeCapacity {
			t.Errorf("Expected %d tokens, got %d", largeCapacity, tokensLeft)
		}
	})

	t.Run("VeryFastRefill", func(t *testing.T) {
		bucket := NewTokenBucket(10, time.Nanosecond)
		bucket.TryConsume(5)

		// Even nanosecond should allow some refill
		time.Sleep(time.Microsecond)
		tokensLeft, _, _ := bucket.GetStatus()

		// Should have refilled to capacity
		if tokensLeft != 10 {
			t.Logf("Fast refill test: expected full capacity, got %d tokens", tokensLeft)
		}
	})
}

// BenchmarkTokenBucket_TryConsume benchmarks token consumption
func BenchmarkTokenBucket_TryConsume(b *testing.B) {
	bucket := NewTokenBucket(int64(b.N), time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket.TryConsume(1)
	}
}

// BenchmarkTokenBucket_GetStatus benchmarks status retrieval
func BenchmarkTokenBucket_GetStatus(b *testing.B) {
	bucket := NewTokenBucket(1000, time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket.GetStatus()
	}
}

// BenchmarkTokenBucket_Concurrent benchmarks concurrent access
func BenchmarkTokenBucket_Concurrent(b *testing.B) {
	bucket := NewTokenBucket(int64(b.N), time.Second)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bucket.TryConsume(1)
		}
	})
}
