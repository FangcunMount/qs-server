package scale

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

type stubScaleClient struct {
	getCalls int32
	getFn    func(ctx context.Context, code string) (*grpcclient.ScaleOutput, error)
}

func (s *stubScaleClient) GetScale(ctx context.Context, code string) (*grpcclient.ScaleOutput, error) {
	atomic.AddInt32(&s.getCalls, 1)
	if s.getFn != nil {
		return s.getFn(ctx, code)
	}
	return &grpcclient.ScaleOutput{Code: code, Title: "sample"}, nil
}

func (s *stubScaleClient) ListScales(context.Context, int32, int32, string, string, string, []string, []string, []string, []string) (*grpcclient.ListScalesOutput, error) {
	return &grpcclient.ListScalesOutput{}, nil
}

func (s *stubScaleClient) ListHotScales(context.Context, int32, int32) (*grpcclient.ListHotScalesOutput, error) {
	return &grpcclient.ListHotScalesOutput{}, nil
}

func (s *stubScaleClient) GetScaleCategories(context.Context) (*grpcclient.ScaleCategoriesOutput, error) {
	return &grpcclient.ScaleCategoriesOutput{}, nil
}

func TestQueryServiceGetUsesCacheOnSecondCall(t *testing.T) {
	client := &stubScaleClient{}
	cache := NewLocalCatalogCache(LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 8})
	service := NewQueryService(client, cache, true)

	first, err := service.Get(context.Background(), "s1")
	if err != nil || first == nil {
		t.Fatalf("first get failed: resp=%+v err=%v", first, err)
	}
	second, err := service.Get(context.Background(), "s1")
	if err != nil || second == nil {
		t.Fatalf("second get failed: resp=%+v err=%v", second, err)
	}
	if got := atomic.LoadInt32(&client.getCalls); got != 1 {
		t.Fatalf("get calls = %d, want 1", got)
	}
}

func TestEvictCatalogOnSignalClearsDetailAndList(t *testing.T) {
	cache := NewLocalCatalogCache(LocalCatalogCacheOptions{TTL: time.Minute, MaxEntries: 16})
	cache.SetDetail("s1", &ScaleResponse{Code: "s1"})
	cache.SetListByRequest(&ListScalesRequest{Page: 1, PageSize: 20}, &ListScalesResponse{Total: 1})
	cache.SetCategories(&ScaleCategoriesResponse{})

	cache.EvictOnSignal("s1")
	if _, ok := cache.GetDetail("s1"); ok {
		t.Fatal("expected detail evicted")
	}
	if _, ok := cache.GetListByRequest(&ListScalesRequest{Page: 1, PageSize: 20}); ok {
		t.Fatal("expected list evicted")
	}
	if _, ok := cache.GetCategories(); ok {
		t.Fatal("expected categories evicted")
	}
}
