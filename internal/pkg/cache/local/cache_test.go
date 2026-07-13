package local

import (
	"testing"
	"time"
)

func TestCacheHitMissAndTTL(t *testing.T) {
	cache := New(Options{TTL: 20 * time.Millisecond, MaxEntries: 4}, func(v string) string { return v })

	cache.Set("k1", "v1")
	if got, ok := cache.Get("k1"); !ok || got != "v1" {
		t.Fatalf("expected hit, got %q ok=%v", got, ok)
	}

	time.Sleep(25 * time.Millisecond)
	if _, ok := cache.Get("k1"); ok {
		t.Fatal("expected expired entry to miss")
	}

	hits, misses := cache.Stats()
	if hits != 1 || misses != 1 {
		t.Fatalf("stats hits=%d misses=%d, want 1/1", hits, misses)
	}
}

func TestCacheCloneOnGet(t *testing.T) {
	type item struct{ Title string }
	cache := New(Options{TTL: time.Minute, MaxEntries: 4}, func(v item) item { return v })

	cache.Set("k1", item{Title: "before"})
	got, ok := cache.Get("k1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	got.Title = "mutated"

	gotAgain, ok := cache.Get("k1")
	if !ok {
		t.Fatal("expected second cache hit")
	}
	if gotAgain.Title != "before" {
		t.Fatalf("cache entry mutated, title=%q", gotAgain.Title)
	}
}

func TestCacheMaxEntriesEviction(t *testing.T) {
	cache := New(Options{TTL: time.Minute, MaxEntries: 2}, func(v string) string { return v })
	cache.Set("k1", "v1")
	cache.Set("k2", "v2")
	cache.Set("k3", "v3")

	if _, ok := cache.Get("k1"); ok {
		t.Fatal("expected oldest entry to be evicted")
	}
	if _, ok := cache.Get("k2"); !ok {
		t.Fatal("expected k2 to remain")
	}
	if _, ok := cache.Get("k3"); !ok {
		t.Fatal("expected k3 to remain")
	}
}

func TestCacheDeletePrefix(t *testing.T) {
	cache := New(Options{TTL: time.Minute, MaxEntries: 8}, func(v string) string { return v })
	cache.Set("ns:a", "1")
	cache.Set("ns:a:extra", "2")
	cache.Set("other", "3")

	cache.DeletePrefix("ns:a")
	if _, ok := cache.Get("ns:a"); ok {
		t.Fatal("expected ns:a deleted")
	}
	if _, ok := cache.Get("ns:a:extra"); ok {
		t.Fatal("expected ns:a:extra deleted")
	}
	if _, ok := cache.Get("other"); !ok {
		t.Fatal("expected unrelated key to remain")
	}
}

func TestCacheTTLJitterSpread(t *testing.T) {
	base := 40 * time.Millisecond
	ratio := 0.5
	// 落在 (base, base*(1+ratio)) 内，严格 TTL 应已过期，部分 jitter 项仍应命中。
	window := base + time.Duration(float64(base)*ratio*0.4)

	strict := New(Options{TTL: base, MaxEntries: 4}, func(v string) string { return v })
	strict.Set("k", "v")

	jittered := New(Options{
		TTL:            base,
		MaxEntries:     32,
		TTLJitterRatio: ratio,
	}, func(v string) string { return v })

	const keyCount = 16
	for i := 0; i < keyCount; i++ {
		jittered.Set(string(rune('a'+i)), "v")
	}

	time.Sleep(window)

	if _, ok := strict.Get("k"); ok {
		t.Fatal("strict TTL cache should expire before jittered max TTL")
	}

	surviving := 0
	for i := 0; i < keyCount; i++ {
		if _, ok := jittered.Get(string(rune('a' + i))); ok {
			surviving++
		}
	}
	if surviving == 0 {
		t.Fatal("expected some jittered entries to outlive base TTL")
	}
	if surviving == keyCount {
		t.Fatalf("expected partial expiry before max jitter TTL, stillHot=%d", surviving)
	}

	time.Sleep(time.Duration(float64(base)*ratio) + 15*time.Millisecond)
	for i := 0; i < keyCount; i++ {
		if _, ok := jittered.Get(string(rune('a' + i))); ok {
			t.Fatalf("key %q still hot after max jitter TTL", string(rune('a'+i)))
		}
	}
}
