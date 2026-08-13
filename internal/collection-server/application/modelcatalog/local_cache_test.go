package modelcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"
)

func newPublishedModelTestCache() *LocalPublishedModelCache {
	opts := localcache.Options{TTL: time.Minute, MaxEntries: 64}
	return NewLocalPublishedModelCache(opts, opts, opts)
}

func TestPublishedModelListCacheKeyCoversEveryEffectiveField(t *testing.T) {
	t.Parallel()

	base := ListRequest{
		Kind: "scale", Algorithm: "score", Category: "medical", Keyword: "attention",
		QuestionnaireCode: "q-1", QuestionnaireVersion: "v2", Page: 2, PageSize: 30,
	}
	baseKey, err := publishedModelListCacheKey(&base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*ListRequest){
		func(request *ListRequest) { request.Kind = "typology" },
		func(request *ListRequest) { request.Kind = ""; request.Kinds = "scale,typology" },
		func(request *ListRequest) { request.Algorithm = "brief2" },
		func(request *ListRequest) { request.Category = "development" },
		func(request *ListRequest) { request.Keyword = "memory" },
		func(request *ListRequest) { request.QuestionnaireCode = "q-2" },
		func(request *ListRequest) { request.QuestionnaireVersion = "v3" },
		func(request *ListRequest) { request.Page = 3 },
		func(request *ListRequest) { request.PageSize = 40 },
	}
	for index, mutate := range mutations {
		request := base
		mutate(&request)
		key, keyErr := publishedModelListCacheKey(&request)
		if keyErr != nil {
			t.Fatalf("mutation %d: %v", index, keyErr)
		}
		if key == baseKey {
			t.Fatalf("mutation %d did not change cache key", index)
		}
	}
}

func TestPublishedModelListCacheKeyCanonicalizesKindsAndPagination(t *testing.T) {
	t.Parallel()

	first, err := publishedModelListCacheKey(&ListRequest{Kinds: " typology,scale,typology ", Page: 0, PageSize: 0})
	if err != nil {
		t.Fatal(err)
	}
	second, err := publishedModelListCacheKey(&ListRequest{Kinds: "scale,typology", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equivalent list requests use different keys: %q != %q", first, second)
	}
}

func TestPublishedModelLocalCacheDeepClonesEveryDTOBucket(t *testing.T) {
	t.Parallel()

	cache := newPublishedModelTestCache()
	detail := &ModelResponse{ModelSummaryResponse: ModelSummaryResponse{
		Code: "scale-a", Stages: []string{"child"}, ApplicableAges: []string{"6-12"},
		Reporters: []string{"parent"}, Tags: []string{"tag"},
	}, Definition: json.RawMessage(`{"value":1}`)}
	list := &ListResponse{Models: []ModelResponse{*detail}, Total: 1, Page: 1, PageSize: 20}
	options := &OptionsResponse{Kinds: []OptionResponse{{Label: "Scale", Value: "scale"}}, Algorithms: []OptionResponse{{Value: "score"}}, Categories: []OptionResponse{{Value: "medical"}}, Stages: []OptionResponse{{Value: "child"}}, ApplicableAges: []OptionResponse{{Value: "6-12"}}, Reporters: []OptionResponse{{Value: "parent"}}}

	cache.SetDetail("SCALE-A", detail)
	cache.SetListByRequest(&ListRequest{}, list)
	cache.SetOptions("", options)
	detail.Stages[0], detail.Definition[0] = "mutated", 'x'
	list.Models[0].Tags[0] = "mutated"
	options.Kinds[0].Value = "mutated"

	cachedDetail, ok := cache.GetDetail(" scale-a ")
	if !ok || cachedDetail.Stages[0] != "child" || string(cachedDetail.Definition) != `{"value":1}` {
		t.Fatalf("cached detail was mutated: %#v", cachedDetail)
	}
	cachedList, ok := cache.GetListByRequest(&ListRequest{Page: 1, PageSize: 20})
	if !ok || cachedList.Models[0].Tags[0] != "tag" {
		t.Fatalf("cached list was mutated: %#v", cachedList)
	}
	cachedOptions, ok := cache.GetOptions("   ")
	if !ok || cachedOptions.Kinds[0].Value != "scale" {
		t.Fatalf("cached options were mutated: %#v", cachedOptions)
	}

	cachedDetail.Stages[0] = "second mutation"
	cachedList.Models[0].Definition[0] = 'x'
	cachedOptions.Reporters[0].Value = "second mutation"
	againDetail, _ := cache.GetDetail("scale-a")
	againList, _ := cache.GetListByRequest(&ListRequest{})
	againOptions, _ := cache.GetOptions("")
	if againDetail.Stages[0] != "child" || string(againList.Models[0].Definition) != `{"value":1}` || againOptions.Reporters[0].Value != "parent" {
		t.Fatal("Get returned aliases into cached DTOs")
	}

	cache.SetOptions("empty", &OptionsResponse{Kinds: []OptionResponse{}})
	emptyOptions, _ := cache.GetOptions("empty")
	if emptyOptions.Kinds == nil || emptyOptions.Algorithms != nil {
		t.Fatalf("options nil/empty semantics changed: %#v", emptyOptions)
	}
}

type catalogReaderStub struct {
	detailCalls  atomic.Int32
	listCalls    atomic.Int32
	hotCalls     atomic.Int32
	optionsCalls atomic.Int32
	delay        time.Duration
	detail       *CatalogModel
	detailErr    error
	list         *CatalogList
	listErr      error
	hot          *HotCatalogList
	hotErr       error
	options      *CatalogOptions
	optionsErr   error
}

func (s *catalogReaderStub) GetPublishedModel(context.Context, string, string) (*CatalogModel, error) {
	s.detailCalls.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.detail, s.detailErr
}

func (s *catalogReaderStub) ListPublishedModels(context.Context, string, []string, string, string, string, string, string, int32, int32) (*CatalogList, error) {
	s.listCalls.Add(1)
	return s.list, s.listErr
}

func (s *catalogReaderStub) ListHotPublishedModels(context.Context, string, int32, int32) (*HotCatalogList, error) {
	s.hotCalls.Add(1)
	return s.hot, s.hotErr
}

func (s *catalogReaderStub) GetCatalogOptions(context.Context, string) (*CatalogOptions, error) {
	s.optionsCalls.Add(1)
	return s.options, s.optionsErr
}

func TestPublishedModelQueryServiceReadThroughAndHotBypass(t *testing.T) {
	t.Parallel()

	reader := &catalogReaderStub{
		detail:  &CatalogModel{Code: "scale-a", Definition: json.RawMessage(`{"value":1}`)},
		list:    &CatalogList{Models: []CatalogModel{{Code: "scale-a", Definition: json.RawMessage(`{"value":1}`)}}, Total: 1, Page: 1, PageSize: 20},
		hot:     &HotCatalogList{},
		options: &CatalogOptions{Kinds: []CatalogOption{{Value: "scale"}}},
	}
	service := NewQueryService(reader, newPublishedModelTestCache(), true)
	for index := 0; index < 2; index++ {
		if _, err := service.Get(context.Background(), " SCALE-A "); err != nil {
			t.Fatal(err)
		}
		if _, err := service.List(context.Background(), &ListRequest{}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Options(context.Background(), " scale "); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ListHot(context.Background(), &HotRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if reader.detailCalls.Load() != 1 || reader.listCalls.Load() != 1 || reader.optionsCalls.Load() != 1 || reader.hotCalls.Load() != 2 {
		t.Fatalf("calls detail=%d list=%d options=%d hot=%d", reader.detailCalls.Load(), reader.listCalls.Load(), reader.optionsCalls.Load(), reader.hotCalls.Load())
	}
	if !service.HasCachedDetail("scale-a") || !service.HasCachedList(&ListRequest{Page: 1, PageSize: 20}) || !service.HasCachedOptions("scale") {
		t.Fatal("peek and read-through cache identities diverged")
	}
}

func TestPublishedModelQueryServiceDoesNotCacheNilOrError(t *testing.T) {
	t.Parallel()

	reader := &catalogReaderStub{}
	service := NewQueryService(reader, newPublishedModelTestCache(), true)
	for index := 0; index < 2; index++ {
		value, err := service.Get(context.Background(), "missing")
		if err != nil || value != nil {
			t.Fatalf("nil load = (%#v, %v)", value, err)
		}
	}
	if reader.detailCalls.Load() != 2 || service.HasCachedDetail("missing") {
		t.Fatal("nil published model was cached")
	}
	for index := 0; index < 2; index++ {
		list, err := service.List(context.Background(), &ListRequest{})
		if err != nil || list != nil {
			t.Fatalf("nil list load = (%#v, %v)", list, err)
		}
		options, err := service.Options(context.Background(), "")
		if err != nil || options != nil {
			t.Fatalf("nil options load = (%#v, %v)", options, err)
		}
	}
	if reader.listCalls.Load() != 2 || reader.optionsCalls.Load() != 2 || service.HasCachedList(&ListRequest{}) || service.HasCachedOptions("") {
		t.Fatal("nil list/options response was cached")
	}

	reader.detailErr = errors.New("backend unavailable")
	reader.listErr = errors.New("list backend unavailable")
	reader.optionsErr = errors.New("options backend unavailable")
	for index := 0; index < 2; index++ {
		if _, err := service.Get(context.Background(), "failed"); err == nil {
			t.Fatal("expected backend error")
		}
		if _, err := service.List(context.Background(), &ListRequest{Page: 2}); err == nil {
			t.Fatal("expected list backend error")
		}
		if _, err := service.Options(context.Background(), "scale"); err == nil {
			t.Fatal("expected options backend error")
		}
	}
	if reader.detailCalls.Load() != 4 || reader.listCalls.Load() != 4 || reader.optionsCalls.Load() != 4 || service.HasCachedDetail("failed") || service.HasCachedList(&ListRequest{Page: 2}) || service.HasCachedOptions("scale") {
		t.Fatal("failed published-model response was cached")
	}
}

func TestPublishedModelQueryServiceSingleflightCoalescesSameKey(t *testing.T) {
	t.Parallel()

	reader := &catalogReaderStub{detail: &CatalogModel{Code: "scale-a"}, delay: 25 * time.Millisecond}
	service := NewQueryService(reader, newPublishedModelTestCache(), true)
	const workers = 12
	var wait sync.WaitGroup
	wait.Add(workers)
	errorsFound := make(chan error, workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer wait.Done()
			_, err := service.Get(context.Background(), "scale-a")
			errorsFound <- err
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	if reader.detailCalls.Load() != 1 {
		t.Fatalf("detail calls = %d, want 1", reader.detailCalls.Load())
	}
}
