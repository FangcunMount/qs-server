package questionnaire

import (
	"testing"
	"time"
)

func TestLocalCacheHitMissAndTTL(t *testing.T) {
	cache := NewLocalCache(LocalCacheOptions{TTL: 20 * time.Millisecond, MaxEntries: 4})

	cache.Set("ABC", "", sampleCachedResponse("ABC", "v1"))
	if _, ok := cache.Get("abc", ""); !ok {
		t.Fatal("expected cache hit")
	}

	time.Sleep(25 * time.Millisecond)
	if _, ok := cache.Get("abc", ""); ok {
		t.Fatal("expected expired entry to miss")
	}

	hits, misses := cache.Stats()
	if hits != 1 || misses != 1 {
		t.Fatalf("stats hits=%d misses=%d, want 1/1", hits, misses)
	}
}

func TestLocalCacheVersionIsolation(t *testing.T) {
	cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
	cache.Set("q1", "", sampleCachedResponse("q1", "v-default"))
	cache.Set("q1", "2.0.0", sampleCachedResponse("q1", "v2"))

	gotDefault, ok := cache.Get("q1", "")
	if !ok || gotDefault.Version != "v-default" {
		t.Fatalf("default version cache miss or wrong value: %+v", gotDefault)
	}
	gotVersion, ok := cache.Get("q1", "2.0.0")
	if !ok || gotVersion.Version != "v2" {
		t.Fatalf("versioned cache miss or wrong value: %+v", gotVersion)
	}
}

func TestLocalCacheCloneOnGet(t *testing.T) {
	cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 4})
	original := sampleCachedResponse("q1", "v1")
	original.Questions[0].Title = "before"
	cache.Set("q1", "", original)

	got, ok := cache.Get("q1", "")
	if !ok {
		t.Fatal("expected cache hit")
	}
	got.Questions[0].Title = "mutated"

	gotAgain, ok := cache.Get("q1", "")
	if !ok {
		t.Fatal("expected second cache hit")
	}
	if gotAgain.Questions[0].Title != "before" {
		t.Fatalf("cache entry mutated, title=%q", gotAgain.Questions[0].Title)
	}
}

func TestLocalCacheMaxEntriesEviction(t *testing.T) {
	cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 2})
	cache.Set("q1", "", sampleCachedResponse("q1", "v1"))
	cache.Set("q2", "", sampleCachedResponse("q2", "v1"))
	cache.Set("q3", "", sampleCachedResponse("q3", "v1"))

	if _, ok := cache.Get("q1", ""); ok {
		t.Fatal("expected oldest entry to be evicted")
	}
	if _, ok := cache.Get("q2", ""); !ok {
		t.Fatal("expected q2 to remain")
	}
	if _, ok := cache.Get("q3", ""); !ok {
		t.Fatal("expected q3 to remain")
	}
}

func TestLocalCacheDeleteByCode(t *testing.T) {
	cache := NewLocalCache(LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
	cache.Set("q1", "", sampleCachedResponse("q1", "v1"))
	cache.Set("q1", "2.0.0", sampleCachedResponse("q1", "v2"))

	cache.Delete("q1", "")
	if _, ok := cache.Get("q1", ""); ok {
		t.Fatal("expected default version deleted")
	}
	if _, ok := cache.Get("q1", "2.0.0"); ok {
		t.Fatal("expected versioned entry deleted")
	}
}

func sampleCachedResponse(code, version string) *QuestionnaireResponse {
	return &QuestionnaireResponse{
		Code:    code,
		Version: version,
		Questions: []QuestionResponse{
			{Code: "question-1", Title: "title"},
		},
	}
}
