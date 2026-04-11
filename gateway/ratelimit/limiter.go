package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "rl:"

// tokenBucketScript is an atomic Lua script that:
// 1. Reads or initializes the bucket (tokens + last timestamp)
// 2. Refills tokens based on elapsed time
// 3. Consumes one token if available
// Returns: [allowed (0/1), value]
//   - If allowed: value = remaining tokens * 1000 (int)
//   - If rejected: value = milliseconds until 1 token is available
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local data = redis.call("HMGET", key, "tokens", "ts")
local tokens = tonumber(data[1])
local last_ts = tonumber(data[2])

if tokens == nil then
    tokens = capacity
    last_ts = now
end

local elapsed = math.max(0, now - last_ts)
tokens = math.min(capacity, tokens + elapsed * refill_rate)

if tokens < 1 then
    local wait = (1 - tokens) / refill_rate
    return {0, math.ceil(wait * 1000)}
end

tokens = tokens - 1
redis.call("HMSET", key, "tokens", tostring(tokens), "ts", tostring(now))
redis.call("EXPIRE", key, ttl)
return {1, math.floor(tokens * 1000)}
`)

// Limiter provides per-user token bucket rate limiting backed by Redis.
type Limiter struct {
	rdb        *redis.Client
	capacity   int
	refillRate float64
	ttl        int // seconds
}

// NewLimiter creates a rate limiter connected to the given Redis address.
// capacity: max burst size. refillRate: tokens added per second.
func NewLimiter(addr string, capacity int, refillRate float64) (*Limiter, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis-ratelimit connect: %w", err)
	}
	return &Limiter{
		rdb:        rdb,
		capacity:   capacity,
		refillRate: refillRate,
		ttl:        1800, // 30 minutes
	}, nil
}

// Close shuts down the Redis connection.
func (l *Limiter) Close() error {
	return l.rdb.Close()
}

// Allow checks whether a user can send a message. Returns (allowed, retryAfterMs, err).
func (l *Limiter) Allow(ctx context.Context, userID string) (bool, int, error) {
	now := float64(time.Now().UnixMilli()) / 1000.0
	key := keyPrefix + userID

	result, err := tokenBucketScript.Run(ctx, l.rdb, []string{key},
		l.capacity,
		strconv.FormatFloat(l.refillRate, 'f', -1, 64),
		strconv.FormatFloat(now, 'f', 3, 64),
		l.ttl,
	).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit script: %w", err)
	}

	allowed := result[0] == 1
	value := int(result[1])
	return allowed, value, nil
}

// Delete removes the rate limit bucket for a user (called on disconnect).
func (l *Limiter) Delete(ctx context.Context, userID string) error {
	return l.rdb.Del(ctx, keyPrefix+userID).Err()
}
