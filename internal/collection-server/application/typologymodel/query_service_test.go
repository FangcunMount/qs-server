package typologymodel

import (
	"testing"
	"time"
)

func TestEvictCatalogOnSignalClearsDetailAndList(t *testing.T) {
	cache := NewLocalCatalogCache(LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 16})
	cache.SetDetail("mbti", &TypologyModelResponse{Code: "mbti"})
	cache.SetListByRequest(&ListTypologyModelsRequest{Page: 1, PageSize: 20}, &ListTypologyModelsResponse{Total: 1})
	cache.SetCategories(&TypologyModelCategoriesResponse{})

	cache.EvictOnSignal("mbti")
	if _, ok := cache.GetDetail("mbti"); ok {
		t.Fatal("expected detail evicted")
	}
	if _, ok := cache.GetListByRequest(&ListTypologyModelsRequest{Page: 1, PageSize: 20}); ok {
		t.Fatal("expected list evicted")
	}
	if _, ok := cache.GetCategories(); ok {
		t.Fatal("expected categories evicted")
	}
}
