package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	appScale "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/scale"
)

// ScaleService 量表 gRPC 服务 - C端接口
// 提供量表的查询功能：列表查询、详情查看、分类列表
type ScaleService struct {
	pb.UnimplementedScaleServiceServer
	queryService    appScale.ScaleQueryService
	categoryService appScale.ScaleCategoryService
}

// NewScaleService 创建量表 gRPC 服务
func NewScaleService(queryService appScale.ScaleQueryService, categoryService appScale.ScaleCategoryService) *ScaleService {
	return &ScaleService{
		queryService:    queryService,
		categoryService: categoryService,
	}
}

// RegisterService 注册 gRPC 服务
func (s *ScaleService) RegisterService(server *grpc.Server) {
	pb.RegisterScaleServiceServer(server, s)
}

// GetScale 获取已发布量表的详情（C端）
func (s *ScaleService) GetScale(ctx context.Context, req *pb.GetScaleRequest) (*pb.GetScaleResponse, error) {
	// 调用应用服务
	result, err := s.queryService.GetPublishedByCode(ctx, req.Code)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if result == nil {
		return nil, status.Error(codes.NotFound, "量表不存在或未发布")
	}

	// 转换响应
	return &pb.GetScaleResponse{
		Scale: s.toProtoScale(result),
	}, nil
}

// ListScales 获取已发布的量表列表（C端）
func (s *ScaleService) ListScales(ctx context.Context, req *pb.ListScalesRequest) (*pb.ListScalesResponse, error) {
	// 构建查询条件
	dto := appScale.ListScalesDTO{
		Page:       int(req.Page),
		PageSize:   int(req.PageSize),
		Conditions: make(map[string]string),
	}

	if req.Status != "" {
		dto.Conditions["status"] = req.Status
	}
	if req.Title != "" {
		dto.Conditions["title"] = req.Title
	}
	if req.Category != "" {
		dto.Conditions["category"] = req.Category
	}
	if req.Stage != "" {
		dto.Conditions["stage"] = req.Stage
	}
	if req.ApplicableAge != "" {
		dto.Conditions["applicable_age"] = req.ApplicableAge
	}
	// 注意：reporters 是数组，查询条件暂时不支持多值过滤，后续可以扩展
	if len(req.Reporters) > 0 {
		// 使用第一个填报人作为过滤条件（或可以扩展为支持多个）
		dto.Conditions["reporters"] = req.Reporters[0]
	}

	// 调用应用服务 - 使用已发布列表
	result, err := s.queryService.ListPublished(ctx, dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换响应（使用摘要类型，不包含 factors）
	protoScales := make([]*pb.ScaleSummary, 0, len(result.Items))
	for _, item := range result.Items {
		protoScales = append(protoScales, s.toProtoScaleSummary(item))
	}

	return &pb.ListScalesResponse{
		Scales:   protoScales,
		Total:    result.Total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// GetScaleCategories 获取量表分类列表
func (s *ScaleService) GetScaleCategories(ctx context.Context, req *pb.GetScaleCategoriesRequest) (*pb.GetScaleCategoriesResponse, error) {
	// 调用应用层类别服务
	result, err := s.categoryService.GetCategories(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 转换为 protobuf 格式
	return s.toProtoScaleCategories(result), nil
}

// toProtoScale 转换为 protobuf 量表
func (s *ScaleService) toProtoScale(result *appScale.ScaleResult) *pb.Scale {
	if result == nil {
		return nil
	}

	// 转换因子列表
	protoFactors := make([]*pb.Factor, 0, len(result.Factors))
	for i := range result.Factors {
		protoFactors = append(protoFactors, s.toProtoFactor(&result.Factors[i]))
	}

	// 转换标签列表
	tags := append([]string(nil), result.Tags...)

	// 转换填报人列表
	reporters := append([]string(nil), result.Reporters...)

	return &pb.Scale{
		Code:                 result.Code,
		Title:                result.Title,
		Description:          result.Description,
		Category:             result.Category,
		Stage:                result.Stage,
		ApplicableAge:        result.ApplicableAge,
		Reporters:            reporters,
		Tags:                 tags,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Status:               result.Status,
		Factors:              protoFactors,
	}
}

// toProtoScaleCategories 转换为 protobuf 分类列表
func (s *ScaleService) toProtoScaleCategories(result *appScale.ScaleCategoriesResult) *pb.GetScaleCategoriesResponse {
	categories := make([]*pb.ScaleCategory, len(result.Categories))
	for i, cat := range result.Categories {
		categories[i] = &pb.ScaleCategory{
			Value: cat.Value,
			Label: cat.Label,
		}
	}

	stages := make([]*pb.ScaleStage, len(result.Stages))
	for i, stage := range result.Stages {
		stages[i] = &pb.ScaleStage{
			Value: stage.Value,
			Label: stage.Label,
		}
	}

	applicableAges := make([]*pb.ApplicableAge, len(result.ApplicableAges))
	for i, age := range result.ApplicableAges {
		applicableAges[i] = &pb.ApplicableAge{
			Value: age.Value,
			Label: age.Label,
		}
	}

	reporters := make([]*pb.Reporter, len(result.Reporters))
	for i, rep := range result.Reporters {
		reporters[i] = &pb.Reporter{
			Value: rep.Value,
			Label: rep.Label,
		}
	}

	tags := make([]*pb.Tag, len(result.Tags))
	for i, tag := range result.Tags {
		tags[i] = &pb.Tag{
			Value:    tag.Value,
			Label:    tag.Label,
			Category: tag.Category,
		}
	}

	return &pb.GetScaleCategoriesResponse{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           tags,
	}
}

// toProtoScaleSummary 转换为 protobuf 量表摘要（不包含因子详情）
func (s *ScaleService) toProtoScaleSummary(result *appScale.ScaleSummaryResult) *pb.ScaleSummary {
	if result == nil {
		return nil
	}

	// 转换标签列表
	tags := append([]string(nil), result.Tags...)

	// 转换填报人列表
	reporters := append([]string(nil), result.Reporters...)

	return &pb.ScaleSummary{
		Code:                 result.Code,
		Title:                result.Title,
		Description:          result.Description,
		Category:             result.Category,
		Stage:                result.Stage,
		ApplicableAge:        result.ApplicableAge,
		Reporters:            reporters,
		Tags:                 tags,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: "", // 摘要中不包含版本
		Status:               result.Status,
	}
}

// toProtoFactor 转换为 protobuf 因子
func (s *ScaleService) toProtoFactor(f *appScale.FactorResult) *pb.Factor {
	// 转换计分参数
	scoringParams := make(map[string]string)
	if f.ScoringParams != nil {
		for k, v := range f.ScoringParams {
			if str, ok := v.(string); ok {
				scoringParams[k] = str
			}
		}
	}

	// 转换解读规则
	protoRules := make([]*pb.InterpretRule, 0, len(f.InterpretRules))
	for _, rule := range f.InterpretRules {
		protoRules = append(protoRules, &pb.InterpretRule{
			MinScore:   rule.MinScore,
			MaxScore:   rule.MaxScore,
			RiskLevel:  rule.RiskLevel,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		})
	}

	return &pb.Factor{
		Code:            f.Code,
		Title:           f.Title,
		FactorType:      f.FactorType,
		IsTotalScore:    f.IsTotalScore,
		QuestionCodes:   f.QuestionCodes,
		ScoringStrategy: f.ScoringStrategy,
		ScoringParams:   scoringParams,
		RiskLevel:       f.RiskLevel,
		InterpretRules:  protoRules,
	}
}
