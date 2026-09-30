package domain

import "context"

// RateLimitResult contains metadata about the rate limit decision.
type RateLimitResult struct {
	Allowed   bool  `json:"allowed"`
	Remaining int64 `json:"remaining"`
	ResetMs   int64 `json:"reset_ms"`
}

// RateLimiter defines the contract for distributed rate limiting components.
type RateLimiter interface {
	// Allow checks if the request with the given key is allowed under the current budget.
	// rate: tokens added per second. capacity: maximum burst token bucket size.
	Allow(ctx context.Context, key string, rate float64, capacity int64) (*RateLimitResult, error)
}
