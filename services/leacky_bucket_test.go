package services

import (
	"sync"
	"testing"
	"time"
)

// TestNewLeakyBucket tests bucket creation
func TestNewLeakyBucket(t *testing.T) {
	capacity := int64(10)
	leakRate := 100 * time.Millisecond

	bucket := NewLeakyBucket(capacity, leakRate)
	if bucket == nil {
		t.Fatal("NewLeakyBucket returned nil")
	}

	// Initial status
	queueLen, cap, _ := bucket.GetStatus()
	if queueLen != 0 {
		t.Errorf("Expected initial queue length 0, got %d", queueLen)
	}
	if cap != capacity {
		t.Errorf("Expected capacity %d, got %d", capacity, cap)
	}
}

// TestLeakyBucket_TryAdd_Success tests successful request enqueue
func TestLeakyBucket_TryAdd_Success(t *testing.T) {
	bucket := NewLeakyBucket(5, 200*time.Millisecond)

	success := bucket.TryAdd(3)
	if !success {
		t.Error("Expected TryAdd to succeed when within capacity")
	}

	queueLen, _, _ := bucket.GetStatus()
	if queueLen != 3 {
		t.Errorf("Expected queue length 3, got %d", queueLen)
	}
}

// TestLeakyBucket_TryAdd_Failure tests rejecting requests beyond capacity
func TestLeakyBucket_TryAdd_Failure(t *testing.T) {
	bucket := NewLeakyBucket(5, 200*time.Millisecond)

	// Fill bucket
	bucket.TryAdd(5)

	// Try to exceed capacity
	success := bucket.TryAdd(1)
	if success {
		t.Error("Expected TryAdd to fail when over capacity")
	}

	queueLen, _, _ := bucket.GetStatus()
	if queueLen != 5 {
		t.Errorf("Expected queue length to remain 5, got %d", queueLen)
	}
}

// TestLeakyBucket_Leak tests that requests leak over time
func TestLeakyBucket_Leak(t *testing.T) {
	leakRate := 50 * time.Millisecond
	bucket := NewLeakyBucket(5, leakRate)

	bucket.TryAdd(3)

	// Sleep for enough time to leak 2 requests
	time.Sleep(2*leakRate + 10*time.Millisecond)

	queueLen, _, _ := bucket.GetStatus()
	if queueLen != 1 {
		t.Errorf("Expected queue length 1 after leaking, got %d", queueLen)
	}
}

// TestLeakyBucket_ExactCapacity tests adding exactly to capacity
func TestLeakyBucket_ExactCapacity(t *testing.T) {
	bucket := NewLeakyBucket(5, 100*time.Millisecond)

	success := bucket.TryAdd(5)
	if !success {
		t.Error("Expected TryAdd to succeed with exact capacity")
	}

	success = bucket.TryAdd(1)
	if success {
		t.Error("Expected TryAdd to fail when exceeding capacity")
	}
}

// TestLeakyBucket_NegativeRequests tests invalid negative input
func TestLeakyBucket_NegativeRequests(t *testing.T) {
	bucket := NewLeakyBucket(5, 100*time.Millisecond)

	success := bucket.TryAdd(-3)
	if success {
		t.Error("Expected TryAdd with negative requests to fail")
	}
}

// TestLeakyBucket_Concurrency ensures thread safety
func TestLeakyBucket_Concurrency(t *testing.T) {
	bucket := NewLeakyBucket(100, time.Millisecond)

	var wg sync.WaitGroup
	numGoroutines := 20
	requestsPerG := int64(5)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bucket.TryAdd(requestsPerG)
		}()
	}
	wg.Wait()

	queueLen, _, _ := bucket.GetStatus()
	if queueLen > 100 {
		t.Errorf("Expected queue length <= 100, got %d", queueLen)
	}
}
