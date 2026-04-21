package redisplane

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local ttl = tonumber(ARGV[5])

if rate <= 0 or burst <= 0 or requested <= 0 then
  return {0, 1000}
end

local state = redis.call("HMGET", key, "tokens", "ts")
local tokens = tonumber(state[1])
local ts = tonumber(state[2])

if tokens == nil then
  tokens = burst
end
if ts == nil then
  ts = now
end

local delta = math.max(0, now - ts) / 1000.0
tokens = math.min(burst, tokens + (delta * rate))

local allowed = 0
local retry_after_ms = 0
if tokens >= requested then
  tokens = tokens - requested
  allowed = 1
else
  retry_after_ms = math.ceil(((requested - tokens) / rate) * 1000)
end

redis.call("HSET", key, "tokens", tokens, "ts", now)
redis.call("PEXPIRE", key, ttl)
return {allowed, retry_after_ms}
`)

// DistributedLimiter is a shared Redis-backed token bucket limiter.
type DistributedLimiter struct {
	handle *Handle
}

// NewDistributedLimiter creates a limiter backed by the provided runtime handle.
func NewDistributedLimiter(handle *Handle) *DistributedLimiter {
	return &DistributedLimiter{handle: handle}
}

// Allow checks one token-bucket key.
func (l *DistributedLimiter) Allow(ctx context.Context, key string, ratePerSecond float64, burst int) (bool, time.Duration, error) {
	if l == nil || l.handle == nil || l.handle.Client == nil {
		return false, 0, fmt.Errorf("redis limiter is unavailable")
	}
	if key == "" {
		return false, 0, fmt.Errorf("rate limit key is empty")
	}
	if ratePerSecond <= 0 || burst <= 0 {
		return false, 0, fmt.Errorf("rate and burst must be positive")
	}

	ttl := time.Duration(float64(time.Second)*float64(burst)/ratePerSecond) + 5*time.Second
	if ttl < 5*time.Second {
		ttl = 5 * time.Second
	}
	redisKey := key
	if l.handle.Builder != nil {
		redisKey = l.handle.Builder.BuildLockKey(key)
	}
	result, err := tokenBucketScript.Run(ctx, l.handle.Client, []string{redisKey},
		time.Now().UnixMilli(),
		ratePerSecond,
		burst,
		1,
		ttl.Milliseconds(),
	).Result()
	if err != nil {
		return false, 0, err
	}

	values, ok := result.([]interface{})
	if !ok || len(values) < 2 {
		return false, 0, fmt.Errorf("unexpected limiter result type %T", result)
	}

	allowed, err := toInt64(values[0])
	if err != nil {
		return false, 0, err
	}
	retryAfterMS, err := toInt64(values[1])
	if err != nil {
		return false, 0, err
	}
	return allowed == 1, time.Duration(retryAfterMS) * time.Millisecond, nil
}

func toInt64(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		return 0, fmt.Errorf("unexpected string limiter result %q", typed)
	default:
		return 0, fmt.Errorf("unexpected limiter result type %T", value)
	}
}
