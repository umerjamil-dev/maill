package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// tokenBucketScript atomically allows a request if the bucket has capacity.
// KEYS[1] = bucket key; ARGV = rate(per sec), burst, now(ms), requested.
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])      -- tokens refilled per second
local burst = tonumber(ARGV[2])     -- max tokens
local now = tonumber(ARGV[3])       -- ms timestamp
local requested = tonumber(ARGV[4])

local state = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(state[1])
local ts = tonumber(state[2])
if tokens == nil then tokens = burst ts = now end

local elapsed = math.max(0, now - ts) / 1000.0
tokens = math.min(burst, tokens + elapsed * rate)

if tokens < requested then
  redis.call('HMSET', key, 'tokens', tokens, 'ts', now)
  redis.call('PEXPIRE', key, math.ceil(requested / rate * 1000) + 1000)
  return 0
end

tokens = tokens - requested
redis.call('HMSET', key, 'tokens', tokens, 'ts', now)
redis.call('PEXPIRE', key, 3600000)
return 1
`)

// Limiter is a distributed token-bucket limiter backed by Redis,
// keyed per SMTP account so every worker instance shares one budget.
type Limiter struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Limiter {
	return &Limiter{rdb: rdb}
}

// Allow reports whether one token is available for key right now.
// ratePerMin: sustained emails/minute; burst: instant peak allowance.
func (l *Limiter) Allow(ctx context.Context, key string, ratePerMin, burst int) (bool, error) {
	if burst < 1 {
		burst = 1
	}
	res, err := tokenBucketScript.Run(ctx, l.rdb,
		[]string{"ratelimit:" + key},
		float64(ratePerMin)/60.0, burst,
		time.Now().UnixMilli(), 1,
	).Int()
	if err != nil {
		return false, fmt.Errorf("ratelimit check: %w", err)
	}
	return res == 1, nil
}
