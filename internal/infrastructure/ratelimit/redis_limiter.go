package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/jsjg1/resilient-universal-ai-gateway/internal/domain"
	"github.com/redis/go-redis/v9"
)

// tokenBucketLuaScript performs atomic token bucket rate limiting in Redis.
const tokenBucketLuaScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])        -- tokens added per second
local capacity = tonumber(ARGV[2])    -- maximum burst capacity
local now = tonumber(ARGV[3])         -- current timestamp in milliseconds
local requested = tonumber(ARGV[4])   -- tokens requested (typically 1)

local data = redis.call("HMGET", key, "tokens", "last_updated")
local tokens = tonumber(data[1])
local last_updated = tonumber(data[2])

if tokens == nil or last_updated == nil then
    tokens = capacity
    last_updated = now
else
    local delta = math.max(0, now - last_updated) / 1000.0
    tokens = math.min(capacity, tokens + delta * rate)
    last_updated = now
end

local allowed = 0
local remaining = math.floor(tokens)
local reset_ms = 0

if tokens >= requested then
    tokens = tokens - requested
    allowed = 1
    remaining = math.floor(tokens)
else
    local missing = requested - tokens
    reset_ms = math.ceil((missing / rate) * 1000)
end

local ttl = math.ceil((capacity / rate) * 2)
if ttl < 60 then
    ttl = 60
end

redis.call("HMSET", key, "tokens", tokens, "last_updated", last_updated)
redis.call("EXPIRE", key, ttl)

return { allowed, remaining, reset_ms }
`

// RedisRateLimiter implements domain.RateLimiter using Redis and Lua script.
type RedisRateLimiter struct {
	client     *redis.Client
	scriptSHA  string
	scriptCode *redis.Script
}

// NewRedisRateLimiter constructs a new distributed rate limiter.
func NewRedisRateLimiter(client *redis.Client) *RedisRateLimiter {
	script := redis.NewScript(tokenBucketLuaScript)
	return &RedisRateLimiter{
		client:     client,
		scriptCode: script,
	}
}

// Allow evaluates if the action for the given key is within the rate limit.
func (r *RedisRateLimiter) Allow(ctx context.Context, key string, rate float64, capacity int64) (*domain.RateLimitResult, error) {
	if r.client == nil {
		// If Redis is not available, fail-open gracefully for high availability
		return &domain.RateLimitResult{
			Allowed:   true,
			Remaining: capacity,
			ResetMs:   0,
		}, nil
	}

	redisKey := fmt.Sprintf("gw:ratelimit:%s", key)
	nowMs := time.Now().UnixMilli()

	res, err := r.scriptCode.Run(ctx, r.client, []string{redisKey}, rate, capacity, nowMs, 1).Result()
	if err != nil {
		return nil, fmt.Errorf("ratelimit: redis execution error: %w", err)
	}

	vals, ok := res.([]any)
	if !ok || len(vals) < 3 {
		return nil, fmt.Errorf("ratelimit: unexpected redis script return type: %T", res)
	}

	allowedInt, _ := vals[0].(int64)
	remainingInt, _ := vals[1].(int64)
	resetMsInt, _ := vals[2].(int64)

	return &domain.RateLimitResult{
		Allowed:   allowedInt == 1,
		Remaining: remainingInt,
		ResetMs:   resetMsInt,
	}, nil
}
