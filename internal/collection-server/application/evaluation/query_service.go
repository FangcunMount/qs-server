package evaluation

import (
	"context"
	"reflect"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
)

// QueryService 测评查询服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 调用 apiserver 的 gRPC 服务
// 2. 转换 gRPC 响应到 REST DTO
type QueryService struct {
	evaluationClient BFFReader
	reportFilter     *ReportDimensionFilter
}

// NewQueryService 创建测评查询服务
func NewQueryService(
	evaluationClient BFFReader,
	scaleClient scale.CatalogReader,
) *QueryService {
	return &QueryService{
		evaluationClient: evaluationClient,
		reportFilter:     NewReportDimensionFilter(scaleClient),
	}
}

// GetMyAssessment 获取测评详情（outcome 投影）。
func (s *QueryService) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailResponse, error) {
	return queryDetail(ctx, "get_my_assessment", func() (*AssessmentDetailResponse, error) {
		return s.evaluationClient.GetMyAssessment(ctx, testeeID, assessmentID)
	}, "testee_id", testeeID, "assessment_id", assessmentID)
}

// GetLegacyMyAssessment 获取测评详情（deprecated REST v1 量表投影）。
func (s *QueryService) GetLegacyMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*LegacyAssessmentDetailResponse, error) {
	return queryDetail(ctx, "get_legacy_my_assessment", func() (*LegacyAssessmentDetailResponse, error) {
		detail, err := s.evaluationClient.GetMyAssessment(ctx, testeeID, assessmentID)
		if err != nil {
			return nil, err
		}
		return DetailToLegacy(detail), nil
	}, "testee_id", testeeID, "assessment_id", assessmentID)
}

// GetLegacyMyAssessmentByAnswerSheetID 通过答卷 ID 获取测评详情（deprecated REST v1 投影）。
func (s *QueryService) GetLegacyMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*LegacyAssessmentDetailResponse, error) {
	return queryDetail(ctx, "get_legacy_assessment_by_answersheet", func() (*LegacyAssessmentDetailResponse, error) {
		testeeID, assessmentID, err := s.evaluationClient.ResolveAssessmentByAnswerSheetID(ctx, answerSheetID)
		if err != nil {
			return nil, err
		}
		if assessmentID == 0 {
			return nil, nil
		}
		detail, err := s.evaluationClient.GetMyAssessment(ctx, testeeID, assessmentID)
		if err != nil {
			return nil, err
		}
		return DetailToLegacy(detail), nil
	}, "answer_sheet_id", answerSheetID)
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
		"",
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

// ListLegacyMyAssessments 获取测评列表（deprecated REST v1 量表投影）。
func (s *QueryService) ListLegacyMyAssessments(ctx context.Context, testeeID uint64, req *ListAssessmentsRequest) (*LegacyListAssessmentsResponse, error) {
	l := logger.L(ctx)
	startTime := time.Now()
	NormalizeAssessmentListRequest(req, AssessmentListPageLegacy)

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
		"",
		req.Page,
		req.PageSize,
	)
	if err != nil {
		log.Errorf("Failed to list legacy assessments via gRPC: %v", err)
		l.Errorw("查询测评列表失败", "action", "list_legacy_my_assessments", "testee_id", testeeID, "error", err.Error())
		return nil, err
	}

	l.Debugw("查询我的测评列表成功",
		"action", "list_legacy_my_assessments",
		"testee_id", testeeID,
		"total_count", result.Total,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return ListToLegacy(result), nil
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

// GetAssessmentReport 获取测评报告（outcome 投影，不做量表因子过滤）。
func (s *QueryService) GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentReportResponse, error) {
	return queryDetail(ctx, "get_assessment_report", func() (*AssessmentReportResponse, error) {
		return s.evaluationClient.GetAssessmentReport(ctx, testeeID, assessmentID)
	}, "testee_id", testeeID, "assessment_id", assessmentID)
}

// GetLegacyAssessmentReport 获取测评报告（deprecated REST v1 量表投影 + 可见因子过滤）。
func (s *QueryService) GetLegacyAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*LegacyAssessmentReportResponse, error) {
	result, err := s.evaluationClient.GetAssessmentReport(ctx, testeeID, assessmentID)
	if err != nil {
		log.Errorf("Failed to get assessment report via gRPC: %v", err)
		return nil, err
	}
	return s.reportFilter.Apply(ctx, ReportToLegacy(result))
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
