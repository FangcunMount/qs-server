package questionnaire

import (
	"testing"
	"time"
)

func TestEvictPublishedDetailClearsAllVersionsWhenVersionEmpty(t *testing.T) {
	cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
	cache.Set("q1", "", sampleCachedResponse("q1", "latest"))
	cache.Set("q1", "2.0.0", sampleCachedResponse("q1", "v2"))

	EvictPublishedDetail(cache, "q1", "")
	if _, ok := cache.Get("q1", ""); ok {
		t.Fatal("expected default version evicted")
	}
	if _, ok := cache.Get("q1", "2.0.0"); ok {
		t.Fatal("expected versioned entry evicted")
	}
}

func TestEvictPublishedDetailClearsVersionAndDefault(t *testing.T) {
	cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
	cache.Set("q1", "", sampleCachedResponse("q1", "latest"))
	cache.Set("q1", "2.0.0", sampleCachedResponse("q1", "v2"))
	cache.Set("q2", "", sampleCachedResponse("q2", "other"))

	EvictPublishedDetail(cache, "q1", "2.0.0")
	if _, ok := cache.Get("q1", ""); ok {
		t.Fatal("expected default version evicted with versioned signal")
	}
	if _, ok := cache.Get("q1", "2.0.0"); ok {
		t.Fatal("expected signaled version evicted")
	}
	if _, ok := cache.Get("q2", ""); !ok {
		t.Fatal("expected unrelated cache entry to remain")
	}
}

func TestEvictPublishedDetailNoOpOnNilCacheOrEmptyCode(t *testing.T) {
	cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 4})
	cache.Set("q1", "", sampleCachedResponse("q1", "latest"))

	EvictPublishedDetail(nil, "q1", "")
	EvictPublishedDetail(cache, "", "1.0.0")

	if _, ok := cache.Get("q1", ""); !ok {
		t.Fatal("expected cache entry to remain")
	}
}
