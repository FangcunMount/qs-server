package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisVersionTokenStoreCurrentAndBump(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), DB: 6})
	t.Cleanup(func() {
		_ = client.Close()
	})

	store := NewRedisVersionTokenStore(client)
	ctx := context.Background()

	version, err := store.Current(ctx, "cache:meta:query:version:assessment:list:42")
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if version != 0 {
		t.Fatalf("Current() = %d, want 0 for missing key", version)
	}

	version, err = store.Bump(ctx, "cache:meta:query:version:assessment:list:42")
	if err != nil {
		t.Fatalf("Bump() error = %v", err)
	}
	if version != 1 {
		t.Fatalf("Bump() = %d, want 1", version)
	}

	version, err = store.Current(ctx, "cache:meta:query:version:assessment:list:42")
	if err != nil {
		t.Fatalf("Current() after bump error = %v", err)
	}
	if version != 1 {
		t.Fatalf("Current() after bump = %d, want 1", version)
	}
}
