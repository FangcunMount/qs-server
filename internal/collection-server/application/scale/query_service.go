package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

type scaleClient = CatalogReader

// QueryService 量表查询服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 调用 apiserver 的 gRPC 服务
// 2. 转换 gRPC 响应到 REST DTO
// 3. 可选：缓存热点数据
type QueryService struct {
	scaleClient     scaleClient
	cache           CatalogCache
	coalescer       loadguard.Coalescer
	useSingleflight bool
}

// NewQueryService 创建量表查询服务
func NewQueryService(
	scaleClient scaleClient,
	cache CatalogCache,
	useSingleflight bool,
) *QueryService {
	svc := &QueryService{
		scaleClient:     scaleClient,
		cache:           cache,
		useSingleflight: useSingleflight,
	}
	if useSingleflight {
		svc.coalescer = loadguard.NewCoalescer(true)
	}
	return svc
}

// HasCachedDetail 进程内 L1 是否已有量表详情。
func (s *QueryService) HasCachedDetail(code string) bool {
	if s == nil || s.cache == nil || code == "" {
		return false
	}
	_, ok := s.cache.GetDetail(code)
	return ok
}

// HasCachedList 进程内 L1 是否已有量表列表（req 会按 List 同样规则归一化）。
func (s *QueryService) HasCachedList(req *ListScalesRequest) bool {
	if s == nil || s.cache == nil {
		return false
	}
	if req == nil {
		req = &ListScalesRequest{}
	}
	s.normalizeListRequest(req)
	_, ok := s.cache.GetListByRequest(req)
	return ok
}

// HasCachedHot 进程内 L1 是否已有热门量表列表。
func (s *QueryService) HasCachedHot(req *ListHotScalesRequest) bool {
	if s == nil || s.cache == nil {
		return false
	}
	if req == nil {
		req = &ListHotScalesRequest{}
	}
	_, ok := s.cache.GetHotByRequest(req)
	return ok
}

// HasCachedCategories 进程内 L1 是否已有量表分类。
func (s *QueryService) HasCachedCategories() bool {
	if s == nil || s.cache == nil {
		return false
	}
	_, ok := s.cache.GetCategories()
	return ok
}

// Get 获取量表详情
func (s *QueryService) Get(ctx context.Context, code string) (*ScaleResponse, error) {
	return s.readThroughDetail(
		detailCacheKey(code),
		func() (*ScaleResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetDetail(code)
		},
		func(resp *ScaleResponse) { s.cache.SetDetail(code, resp) },
		func() (*ScaleResponse, error) { return s.fetchScaleFromGRPC(ctx, code) },
	)
}

// List 获取量表列表（返回摘要，不含因子详情）
func (s *QueryService) List(ctx context.Context, req *ListScalesRequest) (*ListScalesResponse, error) {
	if req == nil {
		req = &ListScalesRequest{}
	}
	s.normalizeListRequest(req)

	return s.readThroughList(
		listCacheKey(req),
		func() (*ListScalesResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetListByRequest(req)
		},
		func(resp *ListScalesResponse) { s.cache.SetListByRequest(req, resp) },
		func() (*ListScalesResponse, error) { return s.fetchListFromGRPC(ctx, req) },
	)
}

// ListHot 获取热门量表列表。
func (s *QueryService) ListHot(ctx context.Context, req *ListHotScalesRequest) (*ListHotScalesResponse, error) {
	if req == nil {
		req = &ListHotScalesRequest{}
	}

	return s.readThroughHot(
		hotCacheKey(req),
		func() (*ListHotScalesResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetHotByRequest(req)
		},
		func(resp *ListHotScalesResponse) { s.cache.SetHotByRequest(req, resp) },
		func() (*ListHotScalesResponse, error) { return s.fetchHotFromGRPC(ctx, req) },
	)
}

// GetCategories 获取量表分类列表
func (s *QueryService) GetCategories(ctx context.Context) (*ScaleCategoriesResponse, error) {
	return s.readThroughCategories(
		cacheKeyCategories,
		func() (*ScaleCategoriesResponse, bool) {
			if s.cache == nil {
				return nil, false
			}
			return s.cache.GetCategories()
		},
		func(resp *ScaleCategoriesResponse) { s.cache.SetCategories(resp) },
		func() (*ScaleCategoriesResponse, error) { return s.fetchCategoriesFromGRPC(ctx) },
	)
}

func (s *QueryService) normalizeListRequest(req *ListScalesRequest) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
}

func (s *QueryService) fetchScaleFromGRPC(ctx context.Context, code string) (*ScaleResponse, error) {
	log.Infof("Getting scale: code=%s", code)

	result, err := s.scaleClient.GetScale(ctx, code)
	if err != nil {
		logScaleGRPCError("Failed to get scale via gRPC", err)
		return nil, err
	}
	return result, nil
}

func (s *QueryService) fetchListFromGRPC(ctx context.Context, req *ListScalesRequest) (*ListScalesResponse, error) {
	log.Infof("Listing scales: page=%d, pageSize=%d, category=%s, status=%s", req.Page, req.PageSize, req.Category, req.Status)

	result, err := s.scaleClient.ListScales(ctx, req.Page, req.PageSize, req.Status, req.Title, req.Category, req.Stages, req.ApplicableAges, req.Reporters, req.Tags)
	if err != nil {
		logScaleGRPCError("Failed to list scales via gRPC", err)
		return nil, err
	}
	scales := make([]ScaleSummaryResponse, 0, len(result.Scales))
	for _, scaleItem := range result.Scales {
		if !scaledefinition.NewCategory(scaleItem.Category).IsOpen() {
			continue
		}
		scales = append(scales, scaleItem)
	}
	return &ListScalesResponse{
		Scales:   scales,
		Total:    result.Total,
		Page:     result.Page,
		PageSize: result.PageSize,
	}, nil
}

func (s *QueryService) fetchHotFromGRPC(ctx context.Context, req *ListHotScalesRequest) (*ListHotScalesResponse, error) {
	log.Infof("Listing hot scales: limit=%d, windowDays=%d", req.Limit, req.WindowDays)

	result, err := s.scaleClient.ListHotScales(ctx, req.Limit, req.WindowDays)
	if err != nil {
		log.Errorf("Failed to list hot scales via gRPC: %v", err)
		return nil, err
	}
	scales := make([]HotScaleSummaryResponse, 0, len(result.Scales))
	for _, scaleItem := range result.Scales {
		if !scaledefinition.NewCategory(scaleItem.Category).IsOpen() {
			continue
		}
		scales = append(scales, scaleItem)
	}
	return &ListHotScalesResponse{
		Scales:     scales,
		Total:      int64(len(scales)),
		Limit:      result.Limit,
		WindowDays: result.WindowDays,
	}, nil
}

func (s *QueryService) fetchCategoriesFromGRPC(ctx context.Context) (*ScaleCategoriesResponse, error) {
	log.Info("Getting scale categories")

	result, err := s.scaleClient.GetScaleCategories(ctx)
	if err != nil {
		logScaleGRPCError("Failed to get scale categories via gRPC", err)
		return nil, err
	}
	return result, nil
}

func logScaleGRPCError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}
