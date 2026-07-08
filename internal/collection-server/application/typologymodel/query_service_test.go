package typologymodel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type stubTypologyModelClient struct {
	getCalls int32
	getFn    func(ctx context.Context, code string) (*TypologyModelResponse, error)
}

func (s *stubTypologyModelClient) GetTypologyModel(ctx context.Context, code string) (*TypologyModelResponse, error) {
	atomic.AddInt32(&s.getCalls, 1)
	if s.getFn != nil {
		return s.getFn(ctx, code)
	}
	return &TypologyModelResponse{Code: code, Title: "sample"}, nil
}

func (s *stubTypologyModelClient) ListTypologyModels(context.Context, int32, int32) (*ListTypologyModelsResponse, error) {
	return &ListTypologyModelsResponse{}, nil
}

func (s *stubTypologyModelClient) GetTypologyModelCategories(context.Context) (*TypologyModelCategoriesResponse, error) {
	return &TypologyModelCategoriesResponse{}, nil
}

func TestQueryServiceGetUsesCacheOnSecondCall(t *testing.T) {
	client := &stubTypologyModelClient{}
	cache := NewLocalCatalogCache(LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 8})
	service := NewQueryService(client, cache, true)

	first, err := service.Get(context.Background(), "mbti")
	if err != nil || first == nil {
		t.Fatalf("first get failed: resp=%+v err=%v", first, err)
	}
	second, err := service.Get(context.Background(), "mbti")
	if err != nil || second == nil {
		t.Fatalf("second get failed: resp=%+v err=%v", second, err)
	}
	if got := atomic.LoadInt32(&client.getCalls); got != 1 {
		t.Fatalf("get calls = %d, want 1", got)
	}
}

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
