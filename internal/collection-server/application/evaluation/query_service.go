package evaluation

import (
	"context"
	"reflect"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
)

// QueryService 测评查询服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 调用 apiserver 的 gRPC 服务
// 2. 转换 gRPC 响应到 REST DTO
type QueryService struct {
	evaluationClient BFFReader
}

// NewQueryService 创建测评查询服务
func NewQueryService(
	evaluationClient BFFReader,
) *QueryService {
	return &QueryService{
		evaluationClient: evaluationClient,
	}
}

// GetMyAssessment 获取测评详情（outcome 投影）。
func (s *QueryService) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailResponse, error) {
	return queryDetail(ctx, "get_my_assessment", func() (*AssessmentDetailResponse, error) {
		return s.evaluationClient.GetMyAssessment(ctx, testeeID, assessmentID)
	}, "testee_id", testeeID, "assessment_id", assessmentID)
}

// ListMyAssessments 获取测评列表（outcome 投影）。
func (s *QueryService) ListMyAssessments(ctx context.Context, testeeID uint64, req *ListAssessmentsRequest) (*ListAssessmentsResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()
	NormalizeAssessmentListRequest(req, AssessmentListPageDefault)

	modelKind, err := NormalizeAssessmentKind(req.AssessmentKind)
	if err != nil {
		return nil, err
	}

	result, err := s.evaluationClient.ListMyAssessments(
		ctx,
		testeeID,
		req.Status,
		req.ScaleCode,
		req.RiskLevel,
		req.DateFrom,
		req.DateTo,
		modelKind,
		req.Page,
		req.PageSize,
	)
	if err != nil {
		log.Errorf("Failed to list assessments via gRPC: %v", err)
		l.Errorw("查询测评列表失败", "action", "list_my_assessments", "testee_id", testeeID, "error", err.Error())
		return nil, err
	}

	l.Debugw("查询我的测评列表成功",
		"action", "list_my_assessments",
		"testee_id", testeeID,
		"total_count", result.Total,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return result, nil
}

// GetAssessmentScores 获取测评得分详情
func (s *QueryService) GetAssessmentScores(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreResponse, error) {
	result, err := s.evaluationClient.GetAssessmentScores(ctx, testeeID, assessmentID)
	if err != nil {
		log.Errorf("Failed to get assessment scores via gRPC: %v", err)
		return nil, err
	}
	return result, nil
}

// GetAssessmentReport 获取测评报告（outcome 投影；维度可见性已在 apiserver 成品投影中冻结）。
func (s *QueryService) GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportResponse, error) {
	return queryDetail(ctx, "get_assessment_report", func() (*AssessmentReportResponse, error) {
		return s.evaluationClient.GetAssessmentReport(ctx, testeeID, assessmentID)
	}, "testee_id", testeeID, "assessment_id", assessmentID)
}

// GetFactorTrend 获取因子得分趋势
func (s *QueryService) GetFactorTrend(ctx context.Context, testeeID uint64, req *GetFactorTrendRequest) ([]TrendPointResponse, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	result, err := s.evaluationClient.GetFactorTrend(ctx, testeeID, req.FactorCode, req.Limit)
	if err != nil {
		log.Errorf("Failed to get factor trend via gRPC: %v", err)
		return nil, err
	}
	return result, nil
}

// GetHighRiskFactors 获取高风险因子
func (s *QueryService) GetHighRiskFactors(ctx context.Context, testeeID, assessmentID uint64) ([]FactorScoreResponse, error) {
	result, err := s.evaluationClient.GetHighRiskFactors(ctx, testeeID, assessmentID)
	if err != nil {
		log.Errorf("Failed to get high risk factors via gRPC: %v", err)
		return nil, err
	}
	return result, nil
}

func queryDetail[T any](ctx context.Context, action string, fetch func() (T, error), fields ...any) (T, error) {
	var zero T
	l := logger.L(ctx)
	startTime := time.Now()
	result, err := fetch()
	if err != nil {
		log.Errorf("Failed %s via gRPC: %v", action, err)
		args := append([]any{"action", action, "result", "failed", "error", err.Error()}, fields...)
		l.Errorw("测评查询失败", args...)
		return zero, err
	}
	if isNilValue(result) {
		l.Warnw("测评查询结果为空", append([]any{"action", action}, fields...)...)
		return zero, nil
	}
	args := append([]any{"action", action, "duration_ms", time.Since(startTime).Milliseconds()}, fields...)
	l.Debugw("测评查询成功", args...)
	return result, nil
}

func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}
