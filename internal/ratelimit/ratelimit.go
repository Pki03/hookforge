package ratelimit

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/prateekkhurmi/hookforge/internal/redis"
)

type Limiter struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Limiter {
	return &Limiter{rdb: rdb}
}

var allowScript = goredis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local data = redis.call("HMGET", key, "tokens", "last_refill")
local tokens = burst
local last_refill = now

if data[1] then
	tokens = tonumber(data[1])
end
if data[2] then
	last_refill = tonumber(data[2])
end

local elapsed = now - last_refill
local refill = math.floor(elapsed * rate / 1000000000)
if refill > 0 then
	tokens = tokens + refill
	if tokens > burst then
		tokens = burst
	end
	last_refill = now
end

if tokens < 1 then
	redis.call("HMSET", key, "tokens", tokens, "last_refill", last_refill)
	redis.call("EXPIRE", key, 60)
	return {0, tokens}
end

tokens = tokens - 1
redis.call("HMSET", key, "tokens", tokens, "last_refill", last_refill)
redis.call("EXPIRE", key, 60)
return {1, tokens}
`)

func (l *Limiter) Allow(ctx context.Context, endpointID string, rate, burst int) (bool, error) {
	key := fmt.Sprintf("ratelimit:%s", endpointID)
	now := time.Now().UnixNano()

	result, err := allowScript.Run(ctx, l.rdb.Raw(), []string{key}, rate, burst, now).Result()
	if err != nil {
		return false, err
	}

	vals, ok := result.([]interface{})
	if !ok || len(vals) < 1 {
		return true, nil
	}

	allowed, ok := vals[0].(int64)
	if !ok {
		return true, nil
	}

	return allowed == 1, nil
}
