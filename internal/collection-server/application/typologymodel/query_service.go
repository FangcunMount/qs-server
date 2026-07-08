package typologymodel

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

type typologyModelClient = CatalogReader

// QueryService is the BFF layer for typology model catalog reads.
type QueryService struct {
	client          typologyModelClient
	cache           CatalogCache
	coalescer       loadguard.Coalescer
	useSingleflight bool
}

func NewQueryService(client typologyModelClient, cache CatalogCache, useSingleflight bool) *QueryService {
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
func (s *QueryService) HasCachedList(req *ListTypologyModelsRequest) bool {
	if s == nil || s.cache == nil {
		return false
	}
	if req == nil {
		req = &ListTypologyModelsRequest{}
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

func (s *QueryService) Get(ctx context.Context, code string) (*TypologyModelResponse, error) {
	return s.readThroughDetail(
		detailCacheKey(code),
		func() (*TypologyModelResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetDetail(code)
		},
		func(resp *TypologyModelResponse) { s.cache.SetDetail(code, resp) },
		func() (*TypologyModelResponse, error) {
			log.Infof("Getting typology model: code=%s", code)
			result, err := s.client.GetTypologyModel(ctx, code)
			if err != nil {
				logTypologyGRPCError("Failed to get typology model via gRPC", err)
				return nil, err
			}
			return result, nil
		},
	)
}

func (s *QueryService) List(ctx context.Context, req *ListTypologyModelsRequest) (*ListTypologyModelsResponse, error) {
	if req == nil {
		req = &ListTypologyModelsRequest{}
	}
	s.normalizeListRequest(req)

	return s.readThroughList(
		listCacheKey(req),
		func() (*ListTypologyModelsResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetListByRequest(req)
		},
		func(resp *ListTypologyModelsResponse) { s.cache.SetListByRequest(req, resp) },
		func() (*ListTypologyModelsResponse, error) {
			result, err := s.client.ListTypologyModels(ctx, req.Page, req.PageSize, req.Algorithm)
			if err != nil {
				logTypologyGRPCError("Failed to list typology models via gRPC", err)
				return nil, err
			}
			return result, nil
		},
	)
}

func (s *QueryService) GetCategories(ctx context.Context) (*TypologyModelCategoriesResponse, error) {
	return s.readThroughCategories(
		cacheKeyCategories,
		func() (*TypologyModelCategoriesResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetCategories()
		},
		func(resp *TypologyModelCategoriesResponse) { s.cache.SetCategories(resp) },
		func() (*TypologyModelCategoriesResponse, error) {
			result, err := s.client.GetTypologyModelCategories(ctx)
			if err != nil {
				logTypologyGRPCError("Failed to get typology model categories via gRPC", err)
				return nil, err
			}
			return result, nil
		},
	)
}

func (s *QueryService) normalizeListRequest(req *ListTypologyModelsRequest) {
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

func logTypologyGRPCError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}
