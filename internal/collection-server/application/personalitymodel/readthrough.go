package personalitymodel

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
)

func (s *QueryService) readThroughDetail(
	key string,
	get func() (*PersonalityModelResponse, bool),
	set func(*PersonalityModelResponse),
	load func() (*PersonalityModelResponse, error),
) (*PersonalityModelResponse, error) {
	var setFn func(*PersonalityModelResponse)
	if s.cache != nil {
		setFn = set
	}
	return catalogl1.ReadThrough(key, get, setFn, load, clonePersonalityModelResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}

func (s *QueryService) readThroughList(
	key string,
	get func() (*ListPersonalityModelsResponse, bool),
	set func(*ListPersonalityModelsResponse),
	load func() (*ListPersonalityModelsResponse, error),
) (*ListPersonalityModelsResponse, error) {
	var setFn func(*ListPersonalityModelsResponse)
	if s.cache != nil {
		setFn = set
	}
	return catalogl1.ReadThrough(key, get, setFn, load, cloneListPersonalityModelsResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}

func (s *QueryService) readThroughCategories(
	key string,
	get func() (*PersonalityModelCategoriesResponse, bool),
	set func(*PersonalityModelCategoriesResponse),
	load func() (*PersonalityModelCategoriesResponse, error),
) (*PersonalityModelCategoriesResponse, error) {
	var setFn func(*PersonalityModelCategoriesResponse)
	if s.cache != nil {
		setFn = set
	}
	return catalogl1.ReadThrough(key, get, setFn, load, clonePersonalityModelCategoriesResponse, s.coalescer, s.cache != nil && s.useSingleflight)
}
