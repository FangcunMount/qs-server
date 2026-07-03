package scale

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
)

func (s *QueryService) readThroughDetail(
	key string,
	get func() (*ScaleResponse, bool),
	set func(*ScaleResponse),
	load func() (*ScaleResponse, error),
) (*ScaleResponse, error) {
	var setFn func(*ScaleResponse)
	if s.cache != nil {
		setFn = set
	}
	return catalogl1.ReadThrough(key, get, setFn, load, cloneScaleResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}

func (s *QueryService) readThroughList(
	key string,
	get func() (*ListScalesResponse, bool),
	set func(*ListScalesResponse),
	load func() (*ListScalesResponse, error),
) (*ListScalesResponse, error) {
	var setFn func(*ListScalesResponse)
	if s.cache != nil {
		setFn = set
	}
	return catalogl1.ReadThrough(key, get, setFn, load, cloneListScalesResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}

func (s *QueryService) readThroughHot(
	key string,
	get func() (*ListHotScalesResponse, bool),
	set func(*ListHotScalesResponse),
	load func() (*ListHotScalesResponse, error),
) (*ListHotScalesResponse, error) {
	var setFn func(*ListHotScalesResponse)
	if s.cache != nil {
		setFn = set
	}
	return catalogl1.ReadThrough(key, get, setFn, load, cloneListHotScalesResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}

func (s *QueryService) readThroughCategories(
	key string,
	get func() (*ScaleCategoriesResponse, bool),
	set func(*ScaleCategoriesResponse),
	load func() (*ScaleCategoriesResponse, error),
) (*ScaleCategoriesResponse, error) {
	var setFn func(*ScaleCategoriesResponse)
	if s.cache != nil {
		setFn = set
	}
	return catalogl1.ReadThrough(key, get, setFn, load, cloneScaleCategoriesResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}
