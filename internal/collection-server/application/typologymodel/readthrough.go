package typologymodel

import localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"

func (s *QueryService) readThroughDetail(
	key string,
	get func() (*TypologyModelResponse, bool),
	set func(*TypologyModelResponse),
	load func() (*TypologyModelResponse, error),
) (*TypologyModelResponse, error) {
	var setFn func(*TypologyModelResponse)
	if s.cache != nil {
		setFn = set
	}
	return localcache.ReadThrough(key, get, setFn, load, cloneTypologyModelResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}

func (s *QueryService) readThroughList(
	key string,
	get func() (*ListTypologyModelsResponse, bool),
	set func(*ListTypologyModelsResponse),
	load func() (*ListTypologyModelsResponse, error),
) (*ListTypologyModelsResponse, error) {
	var setFn func(*ListTypologyModelsResponse)
	if s.cache != nil {
		setFn = set
	}
	return localcache.ReadThrough(key, get, setFn, load, cloneListTypologyModelsResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}

func (s *QueryService) readThroughCategories(
	key string,
	get func() (*TypologyModelCategoriesResponse, bool),
	set func(*TypologyModelCategoriesResponse),
	load func() (*TypologyModelCategoriesResponse, error),
) (*TypologyModelCategoriesResponse, error) {
	var setFn func(*TypologyModelCategoriesResponse)
	if s.cache != nil {
		setFn = set
	}
	return localcache.ReadThrough(key, get, setFn, load, cloneTypologyModelCategoriesResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}
