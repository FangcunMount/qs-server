package service

import (
	"context"
	"time"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	interpretationParticipant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// EvaluationService 测评 gRPC 服务 - C端接口
// 提供测评结果查询、报告查看等功能
type EvaluationService struct {
	pb.UnimplementedEvaluationServiceServer
	intakeService      assessmentApp.AnswerSheetAssessmentIntakeService
	testeeQueryService assessmentApp.TesteeAssessmentQueryService
	participantReports interpretationParticipant.Service
	scoreQueryService  assessmentApp.ScoreQueryService
	assessmentReader   evaluationreadmodel.AssessmentReader
}

// NewEvaluationService 创建测评 gRPC 服务
func NewEvaluationService(
	intakeService assessmentApp.AnswerSheetAssessmentIntakeService,
	testeeQueryService assessmentApp.TesteeAssessmentQueryService,
	participantReports interpretationParticipant.Service,
	scoreQueryService assessmentApp.ScoreQueryService,
	assessmentReader evaluationreadmodel.AssessmentReader,
) *EvaluationService {
	return &EvaluationService{
		intakeService:      intakeService,
		testeeQueryService: testeeQueryService,
		participantReports: participantReports,
		scoreQueryService:  scoreQueryService,
		assessmentReader:   assessmentReader,
	}
}

// RegisterService 注册 gRPC 服务
func (s *EvaluationService) RegisterService(server *grpc.Server) {
	pb.RegisterEvaluationServiceServer(server, s)
}

// ==================== 测评查询接口 ====================

// GetMyAssessment 获取我的测评详情（含 outcome 投影）
func (s *EvaluationService) GetMyAssessment(ctx context.Context, req *pb.GetMyAssessmentRequest) (*pb.GetMyAssessmentResponse, error) {
	if req.TesteeId == 0 || req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 assessment_id 不能为空")
	}
	if _, err := s.testeeQueryService.GetMine(ctx, req.TesteeId, req.AssessmentId); err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	result, err := s.loadAssessmentOutcomeRow(ctx, req.AssessmentId)
	if err != nil {
		return nil, err
	}
	return &pb.GetMyAssessmentResponse{Assessment: toProtoAssessmentDetailFromOutcome(result)}, nil
}

// ResolveAssessmentByAnswerSheetID 通过答卷 ID 解析归属键
func (s *EvaluationService) ResolveAssessmentByAnswerSheetID(ctx context.Context, req *pb.ResolveAssessmentByAnswerSheetIDRequest) (*pb.ResolveAssessmentByAnswerSheetIDResponse, error) {
	if req.AnswerSheetId == 0 {
		return nil, status.Error(codes.InvalidArgument, "answer_sheet_id 不能为空")
	}
	result, err := s.intakeService.FindByAnswerSheetID(ctx, req.AnswerSheetId)
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	if result == nil {
		return nil, status.Error(codes.NotFound, "测评不存在")
	}
	return &pb.ResolveAssessmentByAnswerSheetIDResponse{
		TesteeId:     result.TesteeID,
		AssessmentId: result.ID,
	}, nil
}

// ListMyAssessments 获取我的测评列表（含 outcome 投影）
func (s *EvaluationService) ListMyAssessments(ctx context.Context, req *pb.ListMyAssessmentsRequest) (*pb.ListMyAssessmentsResponse, error) {
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

	if s.assessmentReader == nil {
		dto := assessmentApp.ListMyAssessmentsDTO{
			TesteeID:  req.TesteeId,
			Page:      page,
			PageSize:  pageSize,
			Status:    req.Status,
			ScaleCode: req.ScaleCode,
			RiskLevel: req.RiskLevel,
			ModelKind: req.ModelKind,
		}
		dateFrom, err := parseAssessmentListDate(req.DateFrom, false)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "date_from 格式不正确")
		}
		dateTo, err := parseAssessmentListDate(req.DateTo, true)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "date_to 格式不正确")
		}
		dto.DateFrom = dateFrom
		dto.DateTo = dateTo

		listResult, err := s.testeeQueryService.ListMine(ctx, dto)
		if err != nil {
			return nil, toAssessmentQueryGRPCError(err)
		}
		items := make([]*pb.AssessmentSummary, 0, len(listResult.Items))
		for _, item := range listResult.Items {
			items = append(items, toProtoAssessmentSummaryFromOutcome(legacyAssessmentOutcomeResult(item)))
		}
		total, err := protoInt32FromInt("total", listResult.Total)
		if err != nil {
			return nil, err
		}
		pageOut, err := protoInt32FromInt("page", listResult.Page)
		if err != nil {
			return nil, err
		}
		pageSizeOut, err := protoInt32FromInt("page_size", listResult.PageSize)
		if err != nil {
			return nil, err
		}
		totalPages, err := protoInt32FromInt("total_pages", listResult.TotalPages)
		if err != nil {
			return nil, err
		}
		return &pb.ListMyAssessmentsResponse{
			Items: items, Total: total, Page: pageOut, PageSize: pageSizeOut, TotalPages: totalPages,
		}, nil
	}

	testeeID := req.TesteeId
	filter := evaluationreadmodel.AssessmentFilter{
		TesteeID:  &testeeID,
		Statuses:  normalizeGRPCAssessmentStatuses(req.Status),
		ScaleCode: req.ScaleCode,
		RiskLevel: req.RiskLevel,
		ModelKind: req.ModelKind,
		ModelCode: req.ModelCode,
	}
	if req.DateFrom != "" {
		if parsed, err := time.Parse(time.RFC3339, req.DateFrom); err == nil {
			filter.DateFrom = &parsed
		}
	}
	if req.DateTo != "" {
		if parsed, err := time.Parse(time.RFC3339, req.DateTo); err == nil {
			filter.DateTo = &parsed
		}
	}
	rows, total, err := s.assessmentReader.ListAssessments(ctx, filter, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	outcomeItems, err := assessmentApp.RowsToOutcomeResults(rows)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	items := make([]*pb.AssessmentSummary, 0, len(outcomeItems))
	for _, item := range outcomeItems {
		items = append(items, toProtoAssessmentSummaryFromOutcome(item))
	}
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 && total > 0 {
		totalPages = 1
	}
	return &pb.ListMyAssessmentsResponse{
		Items: items, Total: int32(total), Page: int32(page), PageSize: int32(pageSize), TotalPages: totalPages,
	}, nil
}

func parseAssessmentListDate(raw string, endExclusive bool) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}

	layouts := []string{time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err != nil {
			continue
		}
		if layout == "2006-01-02" && endExclusive {
			parsed = parsed.Add(24 * time.Hour)
		}
		return &parsed, nil
	}
	return nil, status.Error(codes.InvalidArgument, "invalid date format")
}

func toAssessmentQueryGRPCError(err error) error {
	if err == nil {
		return nil
	}

	coder := pkgerrors.ParseCoder(err)
	switch coder.Code() {
	case errorCode.ErrAssessmentNotFound, errorCode.ErrInterpretReportNotFound:
		return status.Error(codes.NotFound, err.Error())
	case errorCode.ErrPermissionDenied, errorCode.ErrForbidden:
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
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

// GetAssessmentReport 获取当前受试者自己的测评报告。
func (s *EvaluationService) GetAssessmentReport(ctx context.Context, req *pb.GetAssessmentReportRequest) (*pb.GetAssessmentReportResponse, error) {
	if req.TesteeId == 0 || req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 assessment_id 不能为空")
	}
	if s.participantReports == nil {
		return nil, status.Error(codes.FailedPrecondition, "participant report service is not configured")
	}
	result, err := s.participantReports.GetMyReport(ctx, interpretationParticipant.Actor{TesteeID: req.TesteeId}, interpretationParticipant.GetQuery{AssessmentID: req.AssessmentId})
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	if result == nil {
		return nil, status.Error(codes.NotFound, "报告不存在")
	}
	return &pb.GetAssessmentReportResponse{Report: toProtoParticipantReport(result)}, nil
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

	result, err := s.participantReports.ListMyReports(ctx, interpretationParticipant.Actor{TesteeID: req.TesteeId}, interpretationParticipant.ListQuery{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	items := make([]*pb.AssessmentReport, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toProtoParticipantReport(item))
	}
	total, err := protoInt32FromInt("total", result.Total)
	if err != nil {
		return nil, err
	}
	pageOut, err := protoInt32FromInt("page", result.Page)
	if err != nil {
		return nil, err
	}
	pageSizeOut, err := protoInt32FromInt("page_size", result.PageSize)
	if err != nil {
		return nil, err
	}
	totalPages, err := protoInt32FromInt("total_pages", result.TotalPages)
	if err != nil {
		return nil, err
	}

	return &pb.ListMyReportsResponse{
		Items:      items,
		Total:      total,
		Page:       pageOut,
		PageSize:   pageSizeOut,
		TotalPages: totalPages,
	}, nil
}

// ==================== 权限验证辅助方法 ====================

// validateTesteeAssessmentAccess 验证受试者是否有权访问指定测评
func (s *EvaluationService) validateTesteeAssessmentAccess(ctx context.Context, testeeID uint64, assessmentID uint64) error {
	if s.testeeQueryService == nil {
		return status.Error(codes.FailedPrecondition, "测评服务未初始化")
	}
	result, err := s.testeeQueryService.GetMine(ctx, testeeID, assessmentID)
	if err != nil {
		return toAssessmentQueryGRPCError(err)
	}
	if result == nil {
		return status.Error(codes.NotFound, "测评不存在")
	}
	if result.TesteeID != testeeID {
		return status.Error(codes.PermissionDenied, "无权访问该测评")
	}
	return nil
}

func (s *EvaluationService) loadAssessmentOutcomeRow(ctx context.Context, assessmentID uint64) (*assessmentApp.AssessmentOutcomeResult, error) {
	if s.assessmentReader == nil {
		return nil, status.Error(codes.FailedPrecondition, "assessment read model is not configured")
	}
	row, err := s.assessmentReader.GetAssessment(ctx, assessmentID)
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	return assessmentApp.RowToOutcomeResult(*row)
}

func normalizeGRPCAssessmentStatuses(raw string) []string {
	switch raw {
	case "":
		return nil
	case "pending":
		return []string{"pending", "submitted"}
	case "done":
		return []string{"evaluated", "interpreted"}
	default:
		return []string{raw}
	}
}
