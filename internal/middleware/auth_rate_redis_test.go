package middleware

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestRedisAuthRateStoreIntegration(t *testing.T) {
	url := os.Getenv("BLINK_TEST_REDIS_URL")
	if url == "" {
		t.Skip("set BLINK_TEST_REDIS_URL to run Redis rate-limit integration tests")
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	prefix := "blink:test:auth-rate:" + uuid.NewString() + ":"
	first, second := NewRedisAuthRateStore(client), NewRedisAuthRateStore(client)
	first.prefix, second.prefix = prefix, prefix
	t.Cleanup(func() { _ = client.Del(context.Background(), prefix+"concurrent", prefix+"expiry").Err() })
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := first
			if i%2 == 0 {
				store = second
			}
			retry, err := store.Take(ctx, "concurrent", 5, 30*time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			if retry == 0 {
				allowed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if allowed.Load() != 5 {
		t.Fatalf("concurrent stores admitted %d requests, want 5", allowed.Load())
	}
	ttl, err := client.PTTL(ctx, prefix+"concurrent").Result()
	if err != nil || ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("counter not expiring: %s, %v", ttl, err)
	}
	if retry, err := first.Take(ctx, "expiry", 1, 100*time.Millisecond); err != nil || retry != 0 {
		t.Fatalf("first request: %s, %v", retry, err)
	}
	if retry, err := second.Take(ctx, "expiry", 1, 100*time.Millisecond); err != nil || retry <= 0 {
		t.Fatalf("replica did not share limit: %s, %v", retry, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		time.Sleep(10 * time.Millisecond)
		retry, err := second.Take(ctx, "expiry", 1, 100*time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		if retry == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("denied requests extended the rate-limit window")
		}
	}
}
