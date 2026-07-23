package service

import (
	"context"
	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// TesteeEvaluationService is the thin gRPC adapter for participant scoring queries.
type TesteeEvaluationService struct {
	pb.UnimplementedTesteeEvaluationServiceServer
	testeeService evaluationtestee.Service
}

func NewTesteeEvaluationService(testeeService evaluationtestee.Service) *TesteeEvaluationService {
	return &TesteeEvaluationService{testeeService: testeeService}
}

// RegisterService 注册 gRPC 服务
func (s *TesteeEvaluationService) RegisterService(server *grpc.Server) {
	pb.RegisterTesteeEvaluationServiceServer(server, s)
}

// ==================== 测评查询接口 ====================

// GetMyAssessment 获取我的测评详情（含 outcome 投影）
func (s *TesteeEvaluationService) GetMyAssessment(ctx context.Context, req *pb.GetMyAssessmentRequest) (*pb.GetMyAssessmentResponse, error) {
	if req.TesteeId == 0 || req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 assessment_id 不能为空")
	}
	result, err := s.testeeService.GetAssessment(ctx, evaluationtestee.Actor{TesteeID: req.TesteeId}, req.AssessmentId)
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	return &pb.GetMyAssessmentResponse{Assessment: toProtoAssessmentDetailFromOutcome(result)}, nil
}

// ListMyAssessments 获取我的测评列表（含 outcome 投影）
func (s *TesteeEvaluationService) ListMyAssessments(ctx context.Context, req *pb.ListMyAssessmentsRequest) (*pb.ListMyAssessmentsResponse, error) {
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
	modelKinds, err := normalizeModelKinds(req.GetModelKind(), req.GetModelKinds())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	listResult, err := s.testeeService.ListAssessments(ctx, evaluationtestee.Actor{TesteeID: req.TesteeId}, evaluationtestee.ListQuery{
		Page: page, PageSize: pageSize, Status: req.Status, ScaleCode: req.ScaleCode,
		RiskLevel: req.RiskLevel, ModelKind: req.ModelKind, ModelKinds: modelKinds, ModelCode: req.ModelCode,
		DateFrom: req.DateFrom, DateTo: req.DateTo,
	})
	if err != nil {
		return nil, toAssessmentQueryGRPCError(err)
	}
	items := make([]*pb.AssessmentSummary, 0, len(listResult.Items))
	for _, item := range listResult.Items {
		items = append(items, toProtoAssessmentSummaryFromOutcome(item))
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

func normalizeModelKinds(modelKind string, modelKinds []string) ([]string, error) {
	if len(modelKinds) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(modelKind) != "" {
		return nil, status.Error(codes.InvalidArgument, "model_kind and model_kinds cannot be used together")
	}
	seen := make(map[string]struct{}, len(modelKinds))
	result := make([]string, 0, len(modelKinds))
	for _, raw := range modelKinds {
		kind := strings.TrimSpace(raw)
		if kind == "" {
			return nil, status.Error(codes.InvalidArgument, "model_kinds cannot contain an empty value")
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		result = append(result, kind)
	}
	return result, nil
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
		return status.Error(codes.Internal, "internal error")
	}
}

// ==================== 得分查询接口 ====================

// GetAssessmentScores 获取测评得分详情
func (s *TesteeEvaluationService) GetAssessmentScores(ctx context.Context, req *pb.GetAssessmentScoresRequest) (*pb.GetAssessmentScoresResponse, error) {
	if req.TesteeId == 0 || req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 assessment_id 不能为空")
	}

	result, err := s.testeeService.GetScore(ctx, evaluationtestee.Actor{TesteeID: req.TesteeId}, req.AssessmentId)
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
func (s *TesteeEvaluationService) GetFactorTrend(ctx context.Context, req *pb.GetFactorTrendRequest) (*pb.GetFactorTrendResponse, error) {
	if req.TesteeId == 0 || req.FactorCode == "" {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 factor_code 不能为空")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}

	dto := evaluationtestee.TrendQuery{
		FactorCode: req.FactorCode,
		Limit:      limit,
	}

	result, err := s.testeeService.GetFactorTrend(ctx, evaluationtestee.Actor{TesteeID: req.TesteeId}, dto)
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
func (s *TesteeEvaluationService) GetHighRiskFactors(ctx context.Context, req *pb.GetHighRiskFactorsRequest) (*pb.GetHighRiskFactorsResponse, error) {
	if req.TesteeId == 0 || req.AssessmentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "testee_id 和 assessment_id 不能为空")
	}

	result, err := s.testeeService.GetHighRiskFactors(ctx, evaluationtestee.Actor{TesteeID: req.TesteeId}, req.AssessmentId)
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
