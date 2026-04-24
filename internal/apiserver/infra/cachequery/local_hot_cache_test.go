package cachequery

import (
	"testing"
	"time"
)

func TestLocalHotCacheEvictsOldestEntry(t *testing.T) {
	t.Parallel()

	cache := NewLocalHotCache[string](time.Minute, 2)
	cache.Set("a", "1")
	cache.Set("b", "2")
	cache.Set("c", "3")

	if _, ok := cache.Get("a"); ok {
		t.Fatal("expected oldest entry to be evicted")
	}
	if got, ok := cache.Get("b"); !ok || got != "2" {
		t.Fatalf("cache.Get(b) = %q, %v", got, ok)
	}
	if got, ok := cache.Get("c"); !ok || got != "3" {
		t.Fatalf("cache.Get(c) = %q, %v", got, ok)
	}
}

func TestLocalHotCacheExpiresEntries(t *testing.T) {
	t.Parallel()

	cache := NewLocalHotCache[string](20*time.Millisecond, 2)
	cache.Set("a", "1")
	time.Sleep(30 * time.Millisecond)

	if _, ok := cache.Get("a"); ok {
		t.Fatal("expected entry to expire")
	}
}
