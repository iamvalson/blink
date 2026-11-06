package middleware

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisAuthRateStore implements a fixed window beginning at the first request.
// Atomic Lua execution prevents concurrent API replicas from exceeding the limit.
type RedisAuthRateStore struct {
	client redis.UniversalClient
	prefix string
}

func NewRedisAuthRateStore(client redis.UniversalClient) *RedisAuthRateStore {
	return &RedisAuthRateStore{client: client, prefix: "blink:auth-rate:v1:"}
}

var authRateScript = redis.NewScript(`
local count = tonumber(redis.call('GET', KEYS[1]) or '0')
local ttl = redis.call('PTTL', KEYS[1])
if count >= tonumber(ARGV[1]) and ttl > 0 then
    return ttl
end
if ttl <= 0 then
    redis.call('SET', KEYS[1], 1, 'PX', ARGV[2])
else
    redis.call('INCR', KEYS[1])
end
return 0
`)

func (s *RedisAuthRateStore) Take(ctx context.Context, key string, limit int, window time.Duration) (time.Duration, error) {
	if limit <= 0 || window < time.Millisecond {
		return 0, errors.New("invalid authentication rate policy")
	}
	millis, err := authRateScript.Run(ctx, s.client, []string{s.prefix + key}, limit, window.Milliseconds()).Int64()
	if err != nil {
		return 0, err
	}
	return time.Duration(millis) * time.Millisecond, nil
}
