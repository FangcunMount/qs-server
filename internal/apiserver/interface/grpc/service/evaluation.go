package service

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/evaluation"
)

// EvaluationService 测评 gRPC 服务 - C端接口
// 提供测评结果查询、报告查看等功能
type EvaluationService struct {
	pb.UnimplementedEvaluationServiceServer
	submissionService  assessmentApp.AssessmentSubmissionService
	reportQueryService assessmentApp.ReportQueryService
	scoreQueryService  assessmentApp.ScoreQueryService
	testeeRepo         testee.Repository
	assessmentRepo     assessment.Repository
}

// NewEvaluationService 创建测评 gRPC 服务
func NewEvaluationService(
	submissionService assessmentApp.AssessmentSubmissionService,
	reportQueryService assessmentApp.ReportQueryService,
	scoreQueryService assessmentApp.ScoreQueryService,
	testeeRepo testee.Repository,
	assessmentRepo assessment.Repository,
) *EvaluationService {
	return &EvaluationService{
		submissionService:  submissionService,
		reportQueryService: reportQueryService,
		scoreQueryService:  scoreQueryService,
		testeeRepo:         testeeRepo,
		assessmentRepo:     assessmentRepo,
	}
}

// RegisterService 注册 gRPC 服务
func (s *EvaluationService) RegisterService(server *grpc.Server) {
	pb.RegisterEvaluationServiceServer(server, s)
}

// ==================== 测评查询接口 ====================

// GetMyAssessment 获取我的测评详情
func (s *EvaluationService) GetMyAssessment(ctx context.Context, req *pb.GetMyAssessmentRequest) (*pb.GetMyAssessmentResponse, error) {
	if req.TesteeId == 0 || req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 assessment_id 不能为空")
	}

	result, err := s.submissionService.GetMyAssessment(ctx, req.TesteeId, req.AssessmentId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if result == nil {
		return nil, status.Error(codes.NotFound, "测评不存在或无权访问")
	}

	return &pb.GetMyAssessmentResponse{
		Assessment: toProtoAssessmentDetail(result),
	}, nil
}

// GetMyAssessmentByAnswerSheetID 通过答卷ID获取测评详情
func (s *EvaluationService) GetMyAssessmentByAnswerSheetID(ctx context.Context, req *pb.GetMyAssessmentByAnswerSheetIDRequest) (*pb.GetMyAssessmentByAnswerSheetIDResponse, error) {
	if req.AnswerSheetId == 0 {
		return nil, status.Error(codes.InvalidArgument, "answer_sheet_id 不能为空")
	}

	result, err := s.submissionService.GetMyAssessmentByAnswerSheetID(ctx, req.AnswerSheetId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if result == nil {
		return nil, status.Error(codes.NotFound, "测评不存在")
	}

	return &pb.GetMyAssessmentByAnswerSheetIDResponse{
		Assessment: toProtoAssessmentDetail(result),
	}, nil
}

// ListMyAssessments 获取我的测评列表
func (s *EvaluationService) ListMyAssessments(ctx context.Context, req *pb.ListMyAssessmentsRequest) (*pb.ListMyAssessmentsResponse, error) {
	if req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}

	// 设置默认值
	page := int(req.Page)
	pageSize := int(req.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	dto := assessmentApp.ListMyAssessmentsDTO{
		TesteeID: req.TesteeId,
		Page:     page,
		PageSize: pageSize,
		Status:   req.Status,
	}

	result, err := s.submissionService.ListMyAssessments(ctx, dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	items := make([]*pb.AssessmentSummary, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toProtoAssessmentSummary(item))
	}

	return &pb.ListMyAssessmentsResponse{
		Items:      items,
		Total:      int32(result.Total),
		Page:       int32(result.Page),
		PageSize:   int32(result.PageSize),
		TotalPages: int32(result.TotalPages),
	}, nil
}

// ==================== 得分查询接口 ====================

// GetAssessmentScores 获取测评得分详情
func (s *EvaluationService) GetAssessmentScores(ctx context.Context, req *pb.GetAssessmentScoresRequest) (*pb.GetAssessmentScoresResponse, error) {
	if req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "assessment_id 不能为空")
	}

	// 验证 testee_id 权限：检查该测评是否属于该受试者
	if req.TesteeId > 0 {
		if err := s.validateTesteeAssessmentAccess(ctx, req.TesteeId, req.AssessmentId); err != nil {
			return nil, err
		}
	}

	result, err := s.scoreQueryService.GetByAssessmentID(ctx, req.AssessmentId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if result == nil {
		return nil, status.Error(codes.NotFound, "得分记录不存在")
	}

	factorScores := make([]*pb.FactorScore, 0, len(result.FactorScores))
	for _, fs := range result.FactorScores {
		factorScores = append(factorScores, &pb.FactorScore{
			FactorCode:   fs.FactorCode,
			FactorName:   fs.FactorName,
			RawScore:     fs.RawScore,
			RiskLevel:    fs.RiskLevel,
			Conclusion:   fs.Conclusion,
			Suggestion:   fs.Suggestion,
			IsTotalScore: fs.IsTotalScore,
		})
	}

	return &pb.GetAssessmentScoresResponse{
		AssessmentId: result.AssessmentID,
		TotalScore:   result.TotalScore,
		RiskLevel:    result.RiskLevel,
		FactorScores: factorScores,
	}, nil
}

// GetFactorTrend 获取因子得分趋势
func (s *EvaluationService) GetFactorTrend(ctx context.Context, req *pb.GetFactorTrendRequest) (*pb.GetFactorTrendResponse, error) {
	if req.TesteeId == 0 || req.FactorCode == "" {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 factor_code 不能为空")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}

	dto := assessmentApp.GetFactorTrendDTO{
		TesteeID:   req.TesteeId,
		FactorCode: req.FactorCode,
		Limit:      limit,
	}

	result, err := s.scoreQueryService.GetFactorTrend(ctx, dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	dataPoints := make([]*pb.TrendPoint, 0, len(result.DataPoints))
	for _, dp := range result.DataPoints {
		dataPoints = append(dataPoints, &pb.TrendPoint{
			AssessmentId: dp.AssessmentID,
			Score:        dp.RawScore,
			RiskLevel:    dp.RiskLevel,
		})
	}

	return &pb.GetFactorTrendResponse{
		TesteeId:   result.TesteeID,
		FactorCode: result.FactorCode,
		FactorName: result.FactorName,
		DataPoints: dataPoints,
	}, nil
}

// GetHighRiskFactors 获取高风险因子
func (s *EvaluationService) GetHighRiskFactors(ctx context.Context, req *pb.GetHighRiskFactorsRequest) (*pb.GetHighRiskFactorsResponse, error) {
	if req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "assessment_id 不能为空")
	}

	// 验证 testee_id 权限：检查该测评是否属于该受试者
	if req.TesteeId > 0 {
		if err := s.validateTesteeAssessmentAccess(ctx, req.TesteeId, req.AssessmentId); err != nil {
			return nil, err
		}
	}

	result, err := s.scoreQueryService.GetHighRiskFactors(ctx, req.AssessmentId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	highRiskFactors := make([]*pb.FactorScore, 0, len(result.HighRiskFactors))
	for _, fs := range result.HighRiskFactors {
		highRiskFactors = append(highRiskFactors, &pb.FactorScore{
			FactorCode:   fs.FactorCode,
			FactorName:   fs.FactorName,
			RawScore:     fs.RawScore,
			RiskLevel:    fs.RiskLevel,
			Conclusion:   fs.Conclusion,
			Suggestion:   fs.Suggestion,
			IsTotalScore: fs.IsTotalScore,
		})
	}

	return &pb.GetHighRiskFactorsResponse{
		AssessmentId:    result.AssessmentID,
		HasHighRisk:     result.HasHighRisk,
		HighRiskFactors: highRiskFactors,
		NeedsUrgentCare: result.NeedsUrgentCare,
	}, nil
}

// ==================== 报告查询接口 ====================

// GetAssessmentReport 获取测评报告
func (s *EvaluationService) GetAssessmentReport(ctx context.Context, req *pb.GetAssessmentReportRequest) (*pb.GetAssessmentReportResponse, error) {
	if req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "assessment_id 不能为空")
	}

	result, err := s.reportQueryService.GetByAssessmentID(ctx, req.AssessmentId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if result == nil {
		return nil, status.Error(codes.NotFound, "报告不存在")
	}

	return &pb.GetAssessmentReportResponse{
		Report: toProtoReport(result),
	}, nil
}

// ListMyReports 获取我的报告列表
func (s *EvaluationService) ListMyReports(ctx context.Context, req *pb.ListMyReportsRequest) (*pb.ListMyReportsResponse, error) {
	if req.TesteeId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 不能为空")
	}

	page := int(req.Page)
	pageSize := int(req.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	dto := assessmentApp.ListReportsDTO{
		TesteeID: req.TesteeId,
		Page:     page,
		PageSize: pageSize,
	}

	result, err := s.reportQueryService.ListByTesteeID(ctx, dto)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	items := make([]*pb.AssessmentReport, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toProtoReport(item))
	}

	return &pb.ListMyReportsResponse{
		Items:      items,
		Total:      int32(result.Total),
		Page:       int32(result.Page),
		PageSize:   int32(result.PageSize),
		TotalPages: int32(result.TotalPages),
	}, nil
}

// ==================== 权限验证辅助方法 ====================

// validateTesteeAssessmentAccess 验证受试者是否有权访问指定测评
func (s *EvaluationService) validateTesteeAssessmentAccess(ctx context.Context, testeeID uint64, assessmentID uint64) error {
	// 1. 查询测评记录
	assessmentEntity, err := s.assessmentRepo.FindByID(ctx, assessment.ID(assessmentID))
	if err != nil {
		return status.Error(codes.NotFound, "测评不存在")
	}

	// 2. 验证测评是否属于该受试者
	if uint64(assessmentEntity.TesteeID()) != testeeID {
		return status.Error(codes.PermissionDenied, "无权访问该测评")
	}

	return nil
}

// ==================== 转换函数 ====================

// toProtoAssessmentDetail 转换为 proto 测评详情
func toProtoAssessmentDetail(result *assessmentApp.AssessmentResult) *pb.AssessmentDetail {
	if result == nil {
		return nil
	}

	detail := &pb.AssessmentDetail{
		Id:                   result.ID,
		OrgId:                result.OrgID,
		TesteeId:             result.TesteeID,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetId:        result.AnswerSheetID,
		OriginType:           result.OriginType,
		Status:               result.Status,
	}

	if result.MedicalScaleCode != nil {
		detail.ScaleCode = *result.MedicalScaleCode
	}
	if result.MedicalScaleName != nil {
		detail.ScaleName = *result.MedicalScaleName
	}
	if result.OriginID != nil {
		detail.OriginId = *result.OriginID
	}
	if result.TotalScore != nil {
		detail.TotalScore = *result.TotalScore
	}
	if result.RiskLevel != nil {
		detail.RiskLevel = *result.RiskLevel
	}
	if result.SubmittedAt != nil {
		detail.SubmittedAt = result.SubmittedAt.Format("2006-01-02 15:04:05")
	}
	if result.InterpretedAt != nil {
		detail.InterpretedAt = result.InterpretedAt.Format("2006-01-02 15:04:05")
	}
	if result.FailedAt != nil {
		detail.FailedAt = result.FailedAt.Format("2006-01-02 15:04:05")
	}
	if result.FailureReason != nil {
		detail.FailureReason = *result.FailureReason
	}

	return detail
}

// toProtoAssessmentSummary 转换为 proto 测评摘要
func toProtoAssessmentSummary(result *assessmentApp.AssessmentResult) *pb.AssessmentSummary {
	if result == nil {
		return nil
	}

	summary := &pb.AssessmentSummary{
		Id:                   result.ID,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetId:        result.AnswerSheetID,
		OriginType:           result.OriginType,
		Status:               result.Status,
	}

	if result.MedicalScaleCode != nil {
		summary.ScaleCode = *result.MedicalScaleCode
	}
	if result.MedicalScaleName != nil {
		summary.ScaleName = *result.MedicalScaleName
	}
	if result.TotalScore != nil {
		summary.TotalScore = *result.TotalScore
	}
	if result.RiskLevel != nil {
		summary.RiskLevel = *result.RiskLevel
	}
	if result.SubmittedAt != nil {
		summary.SubmittedAt = result.SubmittedAt.Format("2006-01-02 15:04:05")
	}
	if result.InterpretedAt != nil {
		summary.InterpretedAt = result.InterpretedAt.Format("2006-01-02 15:04:05")
	}

	return summary
}

// toProtoReport 转换为 proto 报告
func toProtoReport(result *assessmentApp.ReportResult) *pb.AssessmentReport {
	if result == nil {
		return nil
	}

	dimensions := make([]*pb.DimensionInterpret, 0, len(result.Dimensions))
	for _, d := range result.Dimensions {
		var maxScore float64
		if d.MaxScore != nil {
			maxScore = *d.MaxScore
		}
		dimensions = append(dimensions, &pb.DimensionInterpret{
			FactorCode:  d.FactorCode,
			FactorName:  d.FactorName,
			RawScore:    d.RawScore,
			MaxScore:    maxScore,
			RiskLevel:   d.RiskLevel,
			Description: d.Description,
		})
	}

	return &pb.AssessmentReport{
		AssessmentId: result.AssessmentID,
		ScaleCode:    result.ScaleCode,
		ScaleName:    result.ScaleName,
		TotalScore:   result.TotalScore,
		RiskLevel:    result.RiskLevel,
		Conclusion:   result.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  result.Suggestions,
		CreatedAt:    result.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
