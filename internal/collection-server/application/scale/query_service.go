package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
	"golang.org/x/sync/singleflight"
)

type scaleClient interface {
	GetScale(ctx context.Context, code string) (*grpcclient.ScaleOutput, error)
	ListScales(ctx context.Context, page, pageSize int32, status, title, category string, stages, applicableAges, reporters, tags []string) (*grpcclient.ListScalesOutput, error)
	ListHotScales(ctx context.Context, limit, windowDays int32) (*grpcclient.ListHotScalesOutput, error)
	GetScaleCategories(ctx context.Context) (*grpcclient.ScaleCategoriesOutput, error)
}

// QueryService 量表查询服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 调用 apiserver 的 gRPC 服务
// 2. 转换 gRPC 响应到 REST DTO
// 3. 可选：缓存热点数据
type QueryService struct {
	scaleClient       scaleClient
	cache             CatalogCache
	singleflightGroup singleflight.Group
	useSingleflight   bool
}

// NewQueryService 创建量表查询服务
func NewQueryService(
	scaleClient scaleClient,
	cache CatalogCache,
	useSingleflight bool,
) *QueryService {
	return &QueryService{
		scaleClient:     scaleClient,
		cache:           cache,
		useSingleflight: useSingleflight,
	}
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
	if result == nil {
		return nil, nil
	}
	return s.convertScale(result), nil
}

func (s *QueryService) fetchListFromGRPC(ctx context.Context, req *ListScalesRequest) (*ListScalesResponse, error) {
	log.Infof("Listing scales: page=%d, pageSize=%d, category=%s, status=%s", req.Page, req.PageSize, req.Category, req.Status)

	result, err := s.scaleClient.ListScales(ctx, req.Page, req.PageSize, req.Status, req.Title, req.Category, req.Stages, req.ApplicableAges, req.Reporters, req.Tags)
	if err != nil {
		logScaleGRPCError("Failed to list scales via gRPC", err)
		return nil, err
	}

	scales := make([]ScaleSummaryResponse, 0, len(result.Scales))
	for _, scale := range result.Scales {
		if !scaledefinition.NewCategory(scale.Category).IsOpen() {
			continue
		}
		scales = append(scales, ScaleSummaryResponse{
			Code:                 scale.Code,
			Title:                scale.Title,
			Description:          scale.Description,
			Category:             scale.Category,
			Stages:               scale.Stages,
			ApplicableAges:       scale.ApplicableAges,
			Reporters:            scale.Reporters,
			Tags:                 scale.Tags,
			QuestionnaireCode:    scale.QuestionnaireCode,
			QuestionnaireVersion: scale.QuestionnaireVersion,
			Status:               scale.Status,
			QuestionCount:        scale.QuestionCount,
		})
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
	for _, scale := range result.Scales {
		if !scaledefinition.NewCategory(scale.Category).IsOpen() {
			continue
		}
		scales = append(scales, HotScaleSummaryResponse{
			ScaleSummaryResponse: ScaleSummaryResponse{
				Code:                 scale.Code,
				Title:                scale.Title,
				Description:          scale.Description,
				Category:             scale.Category,
				Stages:               scale.Stages,
				ApplicableAges:       scale.ApplicableAges,
				Reporters:            scale.Reporters,
				Tags:                 scale.Tags,
				QuestionnaireCode:    scale.QuestionnaireCode,
				QuestionnaireVersion: scale.QuestionnaireVersion,
				Status:               scale.Status,
				QuestionCount:        scale.QuestionCount,
			},
			Rank:            scale.Rank,
			SubmissionCount: scale.SubmissionCount,
			HeatScore:       scale.HeatScore,
		})
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

	categories := make([]CategoryResponse, len(result.Categories))
	for i, cat := range result.Categories {
		categories[i] = CategoryResponse{
			Value: cat.Value,
			Label: cat.Label,
		}
	}

	stages := make([]StageResponse, len(result.Stages))
	for i, stage := range result.Stages {
		stages[i] = StageResponse{
			Value: stage.Value,
			Label: stage.Label,
		}
	}

	applicableAges := make([]ApplicableAgeResponse, len(result.ApplicableAges))
	for i, age := range result.ApplicableAges {
		applicableAges[i] = ApplicableAgeResponse{
			Value: age.Value,
			Label: age.Label,
		}
	}

	reporters := make([]ReporterResponse, len(result.Reporters))
	for i, rep := range result.Reporters {
		reporters[i] = ReporterResponse{
			Value: rep.Value,
			Label: rep.Label,
		}
	}

	tags := make([]TagResponse, len(result.Tags))
	for i, tag := range result.Tags {
		tags[i] = TagResponse{
			Value:    tag.Value,
			Label:    tag.Label,
			Category: tag.Category,
		}
	}

	return &ScaleCategoriesResponse{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           tags,
	}, nil
}

func logScaleGRPCError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}

// convertScale 转换量表
func (s *QueryService) convertScale(scale *grpcclient.ScaleOutput) *ScaleResponse {
	factors := make([]FactorResponse, len(scale.Factors))
	for i, factor := range scale.Factors {
		factors[i] = s.convertFactor(&factor)
	}

	return &ScaleResponse{
		Code:                 scale.Code,
		Title:                scale.Title,
		Description:          scale.Description,
		Category:             scale.Category,
		Stages:               scale.Stages,
		ApplicableAges:       scale.ApplicableAges,
		Reporters:            scale.Reporters,
		Tags:                 scale.Tags,
		QuestionnaireCode:    scale.QuestionnaireCode,
		QuestionnaireVersion: scale.QuestionnaireVersion,
		Status:               scale.Status,
		Factors:              factors,
		QuestionCount:        scale.QuestionCount,
	}
}

// convertFactor 转换因子
func (s *QueryService) convertFactor(f *grpcclient.FactorOutput) FactorResponse {
	rules := make([]InterpretRuleResponse, len(f.InterpretRules))
	for i, rule := range f.InterpretRules {
		rules[i] = InterpretRuleResponse{
			MinScore:   rule.MinScore,
			MaxScore:   rule.MaxScore,
			RiskLevel:  rule.RiskLevel,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		}
	}

	return FactorResponse{
		Code:            f.Code,
		Title:           f.Title,
		FactorType:      f.FactorType,
		IsTotalScore:    f.IsTotalScore,
		QuestionCodes:   f.QuestionCodes,
		ScoringStrategy: f.ScoringStrategy,
		ScoringParams:   f.ScoringParams,
		MaxScore:        f.MaxScore,
		RiskLevel:       f.RiskLevel,
		InterpretRules:  rules,
	}
}
