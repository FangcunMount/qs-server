package modelcatalog

import localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"

func (s *QueryService) readThroughDetail(code string, load func() (*ModelResponse, error)) (*ModelResponse, error) {
	var get func() (*ModelResponse, bool)
	var set func(*ModelResponse)
	if s != nil && s.cache != nil {
		get = func() (*ModelResponse, bool) { return s.cache.GetDetail(code) }
		set = func(value *ModelResponse) { s.cache.SetDetail(code, value) }
	}
	return localcache.ReadThrough(publishedModelDetailCacheKey(code), get, set, load, cloneModelResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}

func (s *QueryService) readThroughList(request *ListRequest, load func() (*ListResponse, error)) (*ListResponse, error) {
	key, err := publishedModelListCacheKey(request)
	if err != nil {
		return nil, err
	}
	var get func() (*ListResponse, bool)
	var set func(*ListResponse)
	if s != nil && s.cache != nil {
		get = func() (*ListResponse, bool) { return s.cache.GetListByRequest(request) }
		set = func(value *ListResponse) { s.cache.SetListByRequest(request, value) }
	}
	return localcache.ReadThrough(key, get, set, load, cloneListResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}

func (s *QueryService) readThroughOptions(kind string, load func() (*OptionsResponse, error)) (*OptionsResponse, error) {
	var get func() (*OptionsResponse, bool)
	var set func(*OptionsResponse)
	if s != nil && s.cache != nil {
		get = func() (*OptionsResponse, bool) { return s.cache.GetOptions(kind) }
		set = func(value *OptionsResponse) { s.cache.SetOptions(kind, value) }
	}
	return localcache.ReadThrough(publishedModelOptionsCacheKey(kind), get, set, load, cloneOptionsResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}
