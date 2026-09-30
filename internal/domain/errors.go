package domain

import "errors"

var (
	// ErrRateLimitExceeded is returned when a client exceeds their allocated token budget.
	ErrRateLimitExceeded = errors.New("rate limit exceeded: token bucket exhausted")

	// ErrProviderUnavailable is returned when a specific AI provider fails or its circuit is open.
	ErrProviderUnavailable = errors.New("ai provider is currently unavailable")

	// ErrAllProvidersFailed is returned when all configured fallback providers have failed.
	ErrAllProvidersFailed = errors.New("all configured ai providers failed to process the request")

	// ErrInvalidRequest is returned when the incoming completion request violates schema or validation rules.
	ErrInvalidRequest = errors.New("invalid request: missing required fields or parameters")

	// ErrProviderNotFound is returned when an unsupported provider type is requested.
	ErrProviderNotFound = errors.New("requested provider is not registered in the gateway")

	// ErrCircuitBreakerOpen is returned when the circuit breaker for a provider is in Open state.
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open for provider")
)
