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
	queryService appScale.ScaleQueryService
}

// NewScaleService 创建量表 gRPC 服务
func NewScaleService(queryService appScale.ScaleQueryService) *ScaleService {
	return &ScaleService{
		queryService: queryService,
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
	if req.Reporter != "" {
		dto.Conditions["reporter"] = req.Reporter
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
	// 构建分类列表
	categories := []*pb.ScaleCategory{
		{Value: "adhd", Label: "ADHD"},
		{Value: "tic_disorder", Label: "抽动障碍"},
		{Value: "sensory_integration", Label: "感统"},
		{Value: "executive_function", Label: "执行功能"},
		{Value: "mental_health", Label: "心理健康"},
		{Value: "neurodevelopmental_screening", Label: "神经发育筛查"},
		{Value: "chronic_disease_management", Label: "慢性病管理"},
		{Value: "quality_of_life", Label: "生活质量"},
	}

	stages := []*pb.ScaleStage{
		{Value: "screening", Label: "筛查"},
		{Value: "deep_assessment", Label: "深评"},
		{Value: "follow_up", Label: "随访"},
		{Value: "outcome", Label: "结局"},
	}

	applicableAges := []*pb.ApplicableAge{
		{Value: "infant", Label: "婴幼儿"},
		{Value: "school_age", Label: "学龄"},
		{Value: "adolescent_adult", Label: "青少年/成人"},
		{Value: "child_adolescent", Label: "儿童/青少年"},
	}

	reporters := []*pb.Reporter{
		{Value: "parent", Label: "家长评"},
		{Value: "teacher", Label: "教师评"},
		{Value: "self", Label: "自评"},
		{Value: "clinical", Label: "临床评定"},
	}

	tags := []*pb.Tag{
		// 阶段标签
		{Value: "screening", Label: "筛查", Category: "stage"},
		{Value: "deep_assessment", Label: "深评", Category: "stage"},
		{Value: "follow_up", Label: "随访", Category: "stage"},
		{Value: "outcome", Label: "功能结局", Category: "stage"},
		// 主题标签
		{Value: "brief_version", Label: "简版", Category: "theme"},
		{Value: "broad_spectrum", Label: "广谱", Category: "theme"},
		{Value: "comorbidity", Label: "共病", Category: "theme"},
		{Value: "function", Label: "功能", Category: "theme"},
		{Value: "family_system", Label: "家庭系统", Category: "theme"},
		{Value: "stress", Label: "压力", Category: "theme"},
		{Value: "infant", Label: "婴幼儿", Category: "theme"},
		{Value: "school_age", Label: "学龄", Category: "theme"},
		{Value: "adolescent", Label: "青少年/成人", Category: "theme"},
		// 状态标签
		{Value: "needs_versioning", Label: "需定版", Category: "status"},
		{Value: "custom", Label: "自定义", Category: "status"},
		// 填报人标签
		{Value: "parent_rating", Label: "家长评", Category: "reporter"},
		{Value: "teacher_rating", Label: "教师评", Category: "reporter"},
		{Value: "self_rating", Label: "自评", Category: "reporter"},
		{Value: "clinical_rating", Label: "临床评定", Category: "reporter"},
	}

	return &pb.GetScaleCategoriesResponse{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           tags,
	}, nil
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

	return &pb.Scale{
		Code:                 result.Code,
		Title:                result.Title,
		Description:          result.Description,
		Category:             result.Category,
		Stage:                result.Stage,
		ApplicableAge:        result.ApplicableAge,
		Reporter:             result.Reporter,
		Tags:                 tags,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Status:               result.Status,
		Factors:              protoFactors,
	}
}

// toProtoScaleSummary 转换为 protobuf 量表摘要（不包含因子详情）
func (s *ScaleService) toProtoScaleSummary(result *appScale.ScaleSummaryResult) *pb.ScaleSummary {
	if result == nil {
		return nil
	}

	// 转换标签列表
	tags := append([]string(nil), result.Tags...)

	return &pb.ScaleSummary{
		Code:                 result.Code,
		Title:                result.Title,
		Description:          result.Description,
		Category:             result.Category,
		Stage:                result.Stage,
		ApplicableAge:        result.ApplicableAge,
		Reporter:             result.Reporter,
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
