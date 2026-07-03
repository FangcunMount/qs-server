package personalitymodel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

type stubPersonalityModelClient struct {
	getCalls int32
	getFn    func(ctx context.Context, code string) (*grpcclient.PersonalityModelOutput, error)
}

func (s *stubPersonalityModelClient) GetPersonalityModel(ctx context.Context, code string) (*grpcclient.PersonalityModelOutput, error) {
	atomic.AddInt32(&s.getCalls, 1)
	if s.getFn != nil {
		return s.getFn(ctx, code)
	}
	return &grpcclient.PersonalityModelOutput{Summary: grpcclient.PersonalityModelSummaryOutput{Code: code, Title: "sample"}}, nil
}

func (s *stubPersonalityModelClient) ListPersonalityModels(context.Context, int32, int32, string) (*grpcclient.ListPersonalityModelsOutput, error) {
	return &grpcclient.ListPersonalityModelsOutput{}, nil
}

func (s *stubPersonalityModelClient) GetPersonalityModelCategories(context.Context) (*grpcclient.PersonalityModelCategoriesOutput, error) {
	return &grpcclient.PersonalityModelCategoriesOutput{}, nil
}

func TestQueryServiceGetUsesCacheOnSecondCall(t *testing.T) {
	client := &stubPersonalityModelClient{}
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
	cache.SetDetail("mbti", &PersonalityModelResponse{Code: "mbti"})
	cache.SetListByRequest(&ListPersonalityModelsRequest{Page: 1, PageSize: 20}, &ListPersonalityModelsResponse{Total: 1})
	cache.SetCategories(&PersonalityModelCategoriesResponse{})

	cache.EvictOnSignal("mbti")
	if _, ok := cache.GetDetail("mbti"); ok {
		t.Fatal("expected detail evicted")
	}
	if _, ok := cache.GetListByRequest(&ListPersonalityModelsRequest{Page: 1, PageSize: 20}); ok {
		t.Fatal("expected list evicted")
	}
	if _, ok := cache.GetCategories(); ok {
		t.Fatal("expected categories evicted")
	}
}
