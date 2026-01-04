package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

// QueryService 量表查询服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 调用 apiserver 的 gRPC 服务
// 2. 转换 gRPC 响应到 REST DTO
// 3. 可选：缓存热点数据
type QueryService struct {
	scaleClient *grpcclient.ScaleClient
}

// NewQueryService 创建量表查询服务
func NewQueryService(
	scaleClient *grpcclient.ScaleClient,
) *QueryService {
	return &QueryService{
		scaleClient: scaleClient,
	}
}

// Get 获取量表详情
func (s *QueryService) Get(ctx context.Context, code string) (*ScaleResponse, error) {
	log.Infof("Getting scale: code=%s", code)

	result, err := s.scaleClient.GetScale(ctx, code)
	if err != nil {
		log.Errorf("Failed to get scale via gRPC: %v", err)
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	return s.convertScale(result), nil
}

// List 获取量表列表（返回摘要，不含因子详情）
func (s *QueryService) List(ctx context.Context, req *ListScalesRequest) (*ListScalesResponse, error) {
	log.Infof("Listing scales: page=%d, pageSize=%d, category=%s, status=%s", req.Page, req.PageSize, req.Category, req.Status)

	// 默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	// 最大分页限制
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	result, err := s.scaleClient.ListScales(ctx, req.Page, req.PageSize, req.Status, req.Title, req.Category, req.Stages, req.ApplicableAges, req.Reporters, req.Tags)
	if err != nil {
		log.Errorf("Failed to list scales via gRPC: %v", err)
		return nil, err
	}

	// 转换摘要列表（仅保留有效分类）
	scales := make([]ScaleSummaryResponse, 0, len(result.Scales))
	for _, scale := range result.Scales {
		if !domainScale.NewCategory(scale.Category).IsOpen() {
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

// GetCategories 获取量表分类列表
func (s *QueryService) GetCategories(ctx context.Context) (*ScaleCategoriesResponse, error) {
	log.Info("Getting scale categories")

	result, err := s.scaleClient.GetScaleCategories(ctx)
	if err != nil {
		log.Errorf("Failed to get scale categories via gRPC: %v", err)
		return nil, err
	}

	// 转换分类列表
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

// convertScale 转换量表
func (s *QueryService) convertScale(scale *grpcclient.ScaleOutput) *ScaleResponse {
	// 转换因子列表
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
	// 转换解读规则
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
