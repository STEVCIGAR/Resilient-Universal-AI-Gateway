package ratelimit_test

import (
	"context"
	"testing"

	"github.com/jsjg1/resilient-universal-ai-gateway/internal/infrastructure/ratelimit"
)

func TestRedisRateLimiter_FailOpenWhenRedisNil(t *testing.T) {
	limiter := ratelimit.NewRedisRateLimiter(nil)

	res, err := limiter.Allow(context.Background(), "test-client", 10, 20)
	if err != nil {
		t.Fatalf("expected no error on nil redis client, got: %v", err)
	}

	if !res.Allowed {
		t.Errorf("expected rate limiter to fail open when redis is not configured")
	}

	if res.Remaining != 20 {
		t.Errorf("expected remaining tokens to match capacity (20), got %d", res.Remaining)
	}
}
