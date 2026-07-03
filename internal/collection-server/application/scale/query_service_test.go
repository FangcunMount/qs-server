package scale

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type stubScaleClient struct {
	getCalls  int32
	listCalls int32
	hotCalls  int32
	getFn     func(ctx context.Context, code string) (*ScaleResponse, error)
	listFn    func(ctx context.Context, page, pageSize int32, status, title, category string, stages, applicableAges, reporters, tags []string) (*ListScalesResponse, error)
	hotFn     func(ctx context.Context, limit, windowDays int32) (*ListHotScalesResponse, error)
}

func (s *stubScaleClient) GetScale(ctx context.Context, code string) (*ScaleResponse, error) {
	atomic.AddInt32(&s.getCalls, 1)
	if s.getFn != nil {
		return s.getFn(ctx, code)
	}
	return &ScaleResponse{Code: code, Title: "sample"}, nil
}

func (s *stubScaleClient) ListScales(ctx context.Context, page, pageSize int32, status, title, category string, stages, applicableAges, reporters, tags []string) (*ListScalesResponse, error) {
	atomic.AddInt32(&s.listCalls, 1)
	if s.listFn != nil {
		return s.listFn(ctx, page, pageSize, status, title, category, stages, applicableAges, reporters, tags)
	}
	return &ListScalesResponse{}, nil
}

func (s *stubScaleClient) ListHotScales(ctx context.Context, limit, windowDays int32) (*ListHotScalesResponse, error) {
	atomic.AddInt32(&s.hotCalls, 1)
	if s.hotFn != nil {
		return s.hotFn(ctx, limit, windowDays)
	}
	return &ListHotScalesResponse{}, nil
}

func (s *stubScaleClient) GetScaleCategories(context.Context) (*ScaleCategoriesResponse, error) {
	return &ScaleCategoriesResponse{}, nil
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

func TestIsOpenScaleCategoryMatchesCurrentPublishedSet(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		categoryADHD:               true,
		categoryTicDisorder:        true,
		categoryASD:                true,
		categoryPressure:           true,
		categorySensoryIntegration: true,
		categoryExecutiveFunction:  true,
		categoryEmotion:            true,
		categorySleep:              true,
		categoryPersonality:        false,
		"":                         false,
		"unknown":                  false,
	}
	for category, want := range cases {
		if got := isOpenScaleCategory(category); got != want {
			t.Fatalf("isOpenScaleCategory(%q) = %v, want %v", category, got, want)
		}
	}
}

func TestQueryServiceListFiltersClosedScaleCategories(t *testing.T) {
	t.Parallel()

	client := &stubScaleClient{
		listFn: func(context.Context, int32, int32, string, string, string, []string, []string, []string, []string) (*ListScalesResponse, error) {
			return &ListScalesResponse{
				Scales: []ScaleSummaryResponse{
					{Code: "adhd", Category: categoryADHD},
					{Code: "personality", Category: categoryPersonality},
					{Code: "unknown", Category: "unknown"},
					{Code: "sleep", Category: categorySleep},
				},
				Total:    4,
				Page:     1,
				PageSize: 50,
			}, nil
		},
	}
	service := NewQueryService(client, nil, false)

	got, err := service.List(context.Background(), &ListScalesRequest{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got.Total != 4 {
		t.Fatalf("Total = %d, want original total 4", got.Total)
	}
	if len(got.Scales) != 2 {
		t.Fatalf("scales len = %d, want 2: %+v", len(got.Scales), got.Scales)
	}
	if got.Scales[0].Code != "adhd" || got.Scales[1].Code != "sleep" {
		t.Fatalf("scales = %+v, want adhd and sleep", got.Scales)
	}
}

func TestQueryServiceListHotFiltersClosedScaleCategories(t *testing.T) {
	t.Parallel()

	client := &stubScaleClient{
		hotFn: func(context.Context, int32, int32) (*ListHotScalesResponse, error) {
			return &ListHotScalesResponse{
				Scales: []HotScaleSummaryResponse{
					{ScaleSummaryResponse: ScaleSummaryResponse{Code: "emotion", Category: categoryEmotion}},
					{ScaleSummaryResponse: ScaleSummaryResponse{Code: "personality", Category: categoryPersonality}},
					{ScaleSummaryResponse: ScaleSummaryResponse{Code: "sleep", Category: categorySleep}},
				},
				Total:      3,
				Limit:      10,
				WindowDays: 30,
			}, nil
		},
	}
	service := NewQueryService(client, nil, false)

	got, err := service.ListHot(context.Background(), &ListHotScalesRequest{Limit: 10, WindowDays: 30})
	if err != nil {
		t.Fatalf("ListHot() error = %v", err)
	}
	if got.Total != 2 {
		t.Fatalf("Total = %d, want filtered total 2", got.Total)
	}
	if len(got.Scales) != 2 {
		t.Fatalf("scales len = %d, want 2: %+v", len(got.Scales), got.Scales)
	}
	if got.Scales[0].Code != "emotion" || got.Scales[1].Code != "sleep" {
		t.Fatalf("scales = %+v, want emotion and sleep", got.Scales)
	}
}
