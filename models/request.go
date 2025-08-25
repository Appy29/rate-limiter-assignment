package models

import "time"

// AcquireRequest represents the request to acquire tokens
type AcquireRequest struct {
	Key       string `json:"key"`       // user ID, API key, or any identifier
	Tokens    int64  `json:"tokens"`    // number of tokens to acquire (default: 1)
	Algorithm string `json:"algorithm"` // "token_bucket" or "leaky_bucket" (optional)
}

// AcquireResponse represents the response from acquire endpoint
type AcquireResponse struct {
	Allowed    bool   `json:"allowed"`
	Message    string `json:"message"`
	RetryAfter *int   `json:"retry_after,omitempty"` // seconds to wait before retry
}

// StatusRequest represents the request to get status (via query params)
type StatusRequest struct {
	Key string `json:"key"`
}

// StatusResponse represents the current status of rate limiter for a key
// Now supports multi-algorithm scenarios
type StatusResponse struct {
	Key            string        `json:"key"`
	Algorithm      string        `json:"algorithm"`
	TokensLeft     int64         `json:"tokens_left"`
	Capacity       int64         `json:"capacity"`
	RefillRate     time.Duration `json:"refill_rate"`
	NextRefillTime time.Time     `json:"next_refill_time"`
	IsBlocked      bool          `json:"is_blocked"`

	// Extended fields for multi-algorithm support (optional)
	// These fields are only populated when user has used multiple algorithms
	TokenBucketStatus *AlgorithmStatus `json:"token_bucket_status,omitempty"`
	LeakyBucketStatus *AlgorithmStatus `json:"leaky_bucket_status,omitempty"`
}

// AlgorithmStatus represents status for a specific algorithm
// Used to show detailed status when user has used multiple algorithms
type AlgorithmStatus struct {
	Algorithm      string        `json:"algorithm"`
	TokensLeft     int64         `json:"tokens_left"`
	Capacity       int64         `json:"capacity"`
	RefillRate     time.Duration `json:"refill_rate"`
	NextRefillTime time.Time     `json:"next_refill_time"`
	IsBlocked      bool          `json:"is_blocked"`
	HasState       bool          `json:"has_state"` // Whether this algorithm has been used
}

// RateLimitConfig represents the configuration for a specific key
type RateLimitConfig struct {
	Key        string        `json:"key"`
	Algorithm  string        `json:"algorithm"`   // "token_bucket" or "leaky_bucket"
	Capacity   int64         `json:"capacity"`    // max tokens/requests
	RefillRate time.Duration `json:"refill_rate"` // how often to refill
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ===== HELPER METHODS =====

// Validate validates and sets defaults for AcquireRequest
func (ar *AcquireRequest) Validate() error {
	// Set defaults
	if ar.Tokens <= 0 {
		ar.Tokens = 1
	}
	if ar.Algorithm == "" {
		ar.Algorithm = "token_bucket"
	}

	// Validate algorithm
	if ar.Algorithm != "token_bucket" && ar.Algorithm != "leaky_bucket" {
		ar.Algorithm = "token_bucket" // Default to token bucket for invalid algorithms
	}

	return nil
}

// IsMultiAlgorithm checks if the status response contains multiple algorithms
func (sr *StatusResponse) IsMultiAlgorithm() bool {
	return sr.TokenBucketStatus != nil && sr.LeakyBucketStatus != nil
}

// GetAlgorithmCount returns the number of algorithms that have state
func (sr *StatusResponse) GetAlgorithmCount() int {
	count := 0
	if sr.TokenBucketStatus != nil && sr.TokenBucketStatus.HasState {
		count++
	}
	if sr.LeakyBucketStatus != nil && sr.LeakyBucketStatus.HasState {
		count++
	}
	return count
}

// GetActiveAlgorithms returns a list of algorithms that have been used
func (sr *StatusResponse) GetActiveAlgorithms() []string {
	var algorithms []string

	if sr.TokenBucketStatus != nil && sr.TokenBucketStatus.HasState {
		algorithms = append(algorithms, "token_bucket")
	}
	if sr.LeakyBucketStatus != nil && sr.LeakyBucketStatus.HasState {
		algorithms = append(algorithms, "leaky_bucket")
	}

	return algorithms
}

// HasTokenBucketState checks if token bucket algorithm has been used
func (sr *StatusResponse) HasTokenBucketState() bool {
	return sr.TokenBucketStatus != nil && sr.TokenBucketStatus.HasState
}

// HasLeakyBucketState checks if leaky bucket algorithm has been used
func (sr *StatusResponse) HasLeakyBucketState() bool {
	return sr.LeakyBucketStatus != nil && sr.LeakyBucketStatus.HasState
}
