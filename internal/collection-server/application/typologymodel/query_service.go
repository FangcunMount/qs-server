package typologymodel

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

type personalityModelClient = CatalogReader

// QueryService is the BFF layer for personality model catalog reads.
type QueryService struct {
	client          personalityModelClient
	cache           CatalogCache
	coalescer       loadguard.Coalescer
	useSingleflight bool
}

func NewQueryService(client personalityModelClient, cache CatalogCache, useSingleflight bool) *QueryService {
	svc := &QueryService{
		client:          client,
		cache:           cache,
		useSingleflight: useSingleflight,
	}
	if useSingleflight {
		svc.coalescer = loadguard.NewCoalescer(true)
	}
	return svc
}

// HasCachedDetail 进程内 L1 是否已有人格模型详情。
func (s *QueryService) HasCachedDetail(code string) bool {
	if s == nil || s.cache == nil || code == "" {
		return false
	}
	_, ok := s.cache.GetDetail(code)
	return ok
}

// HasCachedList 进程内 L1 是否已有人格模型列表。
func (s *QueryService) HasCachedList(req *ListPersonalityModelsRequest) bool {
	if s == nil || s.cache == nil {
		return false
	}
	if req == nil {
		req = &ListPersonalityModelsRequest{}
	}
	s.normalizeListRequest(req)
	_, ok := s.cache.GetListByRequest(req)
	return ok
}

// HasCachedCategories 进程内 L1 是否已有人格模型分类。
func (s *QueryService) HasCachedCategories() bool {
	if s == nil || s.cache == nil {
		return false
	}
	_, ok := s.cache.GetCategories()
	return ok
}

func (s *QueryService) Get(ctx context.Context, code string) (*PersonalityModelResponse, error) {
	return s.readThroughDetail(
		detailCacheKey(code),
		func() (*PersonalityModelResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetDetail(code)
		},
		func(resp *PersonalityModelResponse) { s.cache.SetDetail(code, resp) },
		func() (*PersonalityModelResponse, error) {
			log.Infof("Getting personality model: code=%s", code)
			result, err := s.client.GetPersonalityModel(ctx, code)
			if err != nil {
				logPersonalityGRPCError("Failed to get personality model via gRPC", err)
				return nil, err
			}
			return result, nil
		},
	)
}

func (s *QueryService) List(ctx context.Context, req *ListPersonalityModelsRequest) (*ListPersonalityModelsResponse, error) {
	if req == nil {
		req = &ListPersonalityModelsRequest{}
	}
	s.normalizeListRequest(req)

	return s.readThroughList(
		listCacheKey(req),
		func() (*ListPersonalityModelsResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetListByRequest(req)
		},
		func(resp *ListPersonalityModelsResponse) { s.cache.SetListByRequest(req, resp) },
		func() (*ListPersonalityModelsResponse, error) {
			result, err := s.client.ListPersonalityModels(ctx, req.Page, req.PageSize, req.Algorithm)
			if err != nil {
				logPersonalityGRPCError("Failed to list personality models via gRPC", err)
				return nil, err
			}
			return result, nil
		},
	)
}

func (s *QueryService) GetCategories(ctx context.Context) (*PersonalityModelCategoriesResponse, error) {
	return s.readThroughCategories(
		cacheKeyCategories,
		func() (*PersonalityModelCategoriesResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetCategories()
		},
		func(resp *PersonalityModelCategoriesResponse) { s.cache.SetCategories(resp) },
		func() (*PersonalityModelCategoriesResponse, error) {
			result, err := s.client.GetPersonalityModelCategories(ctx)
			if err != nil {
				logPersonalityGRPCError("Failed to get personality model categories via gRPC", err)
				return nil, err
			}
			return result, nil
		},
	)
}

func (s *QueryService) normalizeListRequest(req *ListPersonalityModelsRequest) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
}

func logPersonalityGRPCError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}
