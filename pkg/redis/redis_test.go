package rediskit

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestNormalizeNamespace(t *testing.T) {
	if got := NormalizeNamespace("::dev:"); got != "dev" {
		t.Fatalf("NormalizeNamespace() = %q, want %q", got, "dev")
	}
	if got := NewKeyspace("dev").Prefix("stats:key"); got != "dev:stats:key" {
		t.Fatalf("Prefix() = %q, want %q", got, "dev:stats:key")
	}
	if got := NewKeyspace("").Prefix("stats:key"); got != "stats:key" {
		t.Fatalf("Prefix() without namespace = %q, want %q", got, "stats:key")
	}
}

func TestJitterTTL(t *testing.T) {
	base := 10 * time.Minute
	if got := JitterTTL(base, 0); got != base {
		t.Fatalf("JitterTTL() without ratio = %s, want %s", got, base)
	}

	for range 50 {
		got := JitterTTL(base, 0.2)
		if got < 8*time.Minute || got > 12*time.Minute {
			t.Fatalf("JitterTTL() = %s, want within [%s, %s]", got, 8*time.Minute, 12*time.Minute)
		}
	}
}

func TestScanKeys(t *testing.T) {
	client, cleanup := newRedisTestClient(t)
	defer cleanup()

	ctx := context.Background()
	for _, key := range []string{"foo:1", "foo:2", "bar:1"} {
		if err := client.Set(ctx, key, "1", 0).Err(); err != nil {
			t.Fatalf("seed key %s failed: %v", key, err)
		}
	}

	keys, err := ScanKeys(ctx, client, "foo:*", 1)
	if err != nil {
		t.Fatalf("ScanKeys() error = %v", err)
	}
	slices.Sort(keys)
	want := []string{"foo:1", "foo:2"}
	if !slices.Equal(keys, want) {
		t.Fatalf("ScanKeys() = %v, want %v", keys, want)
	}
}

func TestDeleteByPatternWithDel(t *testing.T) {
	client, cleanup := newRedisTestClient(t)
	defer cleanup()

	ctx := context.Background()
	seedKeys(t, client, "cache:a", "cache:b", "other:c")

	deleted, err := DeleteByPattern(ctx, client, "cache:*", DeleteByPatternOptions{
		ScanCount: 1,
		BatchSize: 1,
		UseUnlink: false,
	})
	if err != nil {
		t.Fatalf("DeleteByPattern() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("DeleteByPattern() deleted = %d, want 2", deleted)
	}
	assertKeyExists(t, client, "cache:a", false)
	assertKeyExists(t, client, "cache:b", false)
	assertKeyExists(t, client, "other:c", true)
}

func TestDeleteByPatternWithDefaultUnlink(t *testing.T) {
	client, cleanup := newRedisTestClient(t)
	defer cleanup()

	ctx := context.Background()
	seedKeys(t, client, "tmp:a", "tmp:b", "keep:c")

	deleted, err := DeleteByPattern(ctx, client, "tmp:*", DeleteByPatternOptions{})
	if err != nil {
		t.Fatalf("DeleteByPattern() with defaults error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("DeleteByPattern() deleted = %d, want 2", deleted)
	}
	assertKeyExists(t, client, "tmp:a", false)
	assertKeyExists(t, client, "tmp:b", false)
	assertKeyExists(t, client, "keep:c", true)
}

func TestAcquireAndReleaseLease(t *testing.T) {
	client, cleanup := newRedisTestClient(t)
	defer cleanup()

	ctx := context.Background()
	token, ok, err := AcquireLease(ctx, client, "lock:test", 5*time.Second)
	if err != nil {
		t.Fatalf("AcquireLease() error = %v", err)
	}
	if !ok || token == "" {
		t.Fatalf("AcquireLease() = (%q, %v), want token and success", token, ok)
	}

	_, ok, err = AcquireLease(ctx, client, "lock:test", 5*time.Second)
	if err != nil {
		t.Fatalf("AcquireLease() second error = %v", err)
	}
	if ok {
		t.Fatalf("AcquireLease() should fail while lease is held")
	}

	if err := ReleaseLease(ctx, client, "lock:test", "wrong-token"); err != nil {
		t.Fatalf("ReleaseLease() with wrong token error = %v", err)
	}
	assertKeyExists(t, client, "lock:test", true)

	if err := ReleaseLease(ctx, client, "lock:test", token); err != nil {
		t.Fatalf("ReleaseLease() error = %v", err)
	}
	assertKeyExists(t, client, "lock:test", false)
}

func TestAcquireLeaseAfterExpiration(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx := context.Background()
	_, ok, err := AcquireLease(ctx, client, "lock:ttl", 5*time.Second)
	if err != nil {
		t.Fatalf("AcquireLease() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected initial lease acquisition to succeed")
	}

	mr.FastForward(6 * time.Second)

	token, ok, err := AcquireLease(ctx, client, "lock:ttl", 5*time.Second)
	if err != nil {
		t.Fatalf("AcquireLease() after expiration error = %v", err)
	}
	if !ok || token == "" {
		t.Fatalf("expected lease acquisition after expiration to succeed")
	}
}

func TestConsumeIfExists(t *testing.T) {
	client, cleanup := newRedisTestClient(t)
	defer cleanup()

	ctx := context.Background()
	if err := client.Set(ctx, "otp:test", "1", 0).Err(); err != nil {
		t.Fatalf("seed key failed: %v", err)
	}

	consumed, err := ConsumeIfExists(ctx, client, "otp:test")
	if err != nil {
		t.Fatalf("ConsumeIfExists() error = %v", err)
	}
	if !consumed {
		t.Fatalf("expected first consume to succeed")
	}
	assertKeyExists(t, client, "otp:test", false)

	consumed, err = ConsumeIfExists(ctx, client, "otp:test")
	if err != nil {
		t.Fatalf("ConsumeIfExists() second error = %v", err)
	}
	if consumed {
		t.Fatalf("expected second consume to fail")
	}
}

func TestConsumeIfExistsIsAtomic(t *testing.T) {
	client, cleanup := newRedisTestClient(t)
	defer cleanup()

	ctx := context.Background()
	if err := client.Set(ctx, "otp:atomic", "1", 0).Err(); err != nil {
		t.Fatalf("seed key failed: %v", err)
	}

	const workers = 16
	var (
		wg        sync.WaitGroup
		successes int
		mu        sync.Mutex
	)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumed, err := ConsumeIfExists(ctx, client, "otp:atomic")
			if err != nil {
				t.Errorf("ConsumeIfExists() error = %v", err)
				return
			}
			if consumed {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly one successful consume, got %d", successes)
	}
}

func newRedisTestClient(t *testing.T) (*goredis.Client, func()) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return client, func() {
		_ = client.Close()
	}
}

func seedKeys(t *testing.T, client *goredis.Client, keys ...string) {
	t.Helper()

	ctx := context.Background()
	for _, key := range keys {
		if err := client.Set(ctx, key, "1", 0).Err(); err != nil {
			t.Fatalf("seed key %s failed: %v", key, err)
		}
	}
}

func assertKeyExists(t *testing.T, client *goredis.Client, key string, want bool) {
	t.Helper()

	got, err := client.Exists(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("Exists(%s) error = %v", key, err)
	}
	if (got > 0) != want {
		t.Fatalf("Exists(%s) = %v, want %v", key, got > 0, want)
	}
}
