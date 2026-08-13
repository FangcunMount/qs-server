package evaluation

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

// QueryService 测评查询服务
// 作为 BFF 层的薄服务，主要职责：
// 1. 调用 apiserver 的 gRPC 服务
// 2. 转换 gRPC 响应到 REST DTO
type QueryService struct {
	evaluationClient BFFReader
	accessReader     AssessmentAccessReader
	accessCache      AssessmentAccessCache
	accessCoalescer  loadguard.Coalescer
	detailCache      AssessmentDetailCache
	detailCoalescer  loadguard.Coalescer
}

type QueryOption func(*QueryService)

func WithAssessmentAccessReader(reader AssessmentAccessReader) QueryOption {
	return func(service *QueryService) { service.accessReader = reader }
}

func WithAssessmentAccessCache(cache AssessmentAccessCache, singleflight bool) QueryOption {
	return func(service *QueryService) {
		service.accessCache = cache
		service.accessCoalescer = loadguard.NewCoalescer(singleflight)
	}
}

func WithAssessmentDetailCache(cache AssessmentDetailCache, singleflight bool) QueryOption {
	return func(service *QueryService) {
		service.detailCache = cache
		service.detailCoalescer = loadguard.NewCoalescer(singleflight)
	}
}

// NewQueryService 创建测评查询服务
func NewQueryService(
	evaluationClient BFFReader,
	options ...QueryOption,
) *QueryService {
	service := &QueryService{
		evaluationClient: evaluationClient,
		accessCoalescer:  loadguard.NewCoalescer(false),
		detailCoalescer:  loadguard.NewCoalescer(false),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// AuthorizeAssessment checks only testee/assessment ownership.
func (s *QueryService) AuthorizeAssessment(ctx context.Context, testeeID, assessmentID uint64) error {
	if s == nil || s.accessReader == nil {
		return fmt.Errorf("assessment access reader is not configured")
	}
	if s.accessCache != nil && s.accessCache.Has(testeeID, assessmentID) {
		return nil
	}
	key := assessmentDetailCacheKey(testeeID, assessmentID)
	_, err := s.accessCoalescer.Do(ctx, "evaluation.assessment_access:"+key, func() (any, error) {
		if s.accessCache != nil && s.accessCache.Has(testeeID, assessmentID) {
			return true, nil
		}
		if err := s.accessReader.AuthorizeAssessment(ctx, testeeID, assessmentID); err != nil {
			return nil, err
		}
		if s.accessCache != nil {
			s.accessCache.Set(testeeID, assessmentID)
		}
		return true, nil
	})
	return err
}

// GetMyAssessment 获取测评详情（outcome 投影）。
func (s *QueryService) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentDetailResponse, error) {
	if s == nil {
		return nil, nil
	}
	if s.detailCache != nil {
		if cached, ok := s.detailCache.Get(testeeID, assessmentID); ok {
			return cached, nil
		}
	}
	key := assessmentDetailCacheKey(testeeID, assessmentID)
	loaded, err := s.detailCoalescer.Do(ctx, "evaluation.assessment_detail:"+key, func() (any, error) {
		if s.detailCache != nil {
			if cached, ok := s.detailCache.Get(testeeID, assessmentID); ok {
				return cached, nil
			}
		}
		result, loadErr := queryDetail(ctx, "get_my_assessment", func() (*AssessmentDetailResponse, error) {
			if s.evaluationClient == nil {
				return nil, nil
			}
			return s.evaluationClient.GetMyAssessment(ctx, testeeID, assessmentID)
		}, "testee_id", testeeID, "assessment_id", assessmentID)
		if loadErr == nil && isCacheableAssessmentDetail(result) && s.detailCache != nil {
			s.detailCache.Set(testeeID, assessmentID, result)
		}
		return result, loadErr
	})
	if err != nil || loaded == nil {
		return nil, err
	}
	result, _ := loaded.(*AssessmentDetailResponse)
	return cloneAssessmentDetailResponse(result), nil
}

// ListMyAssessmentsByModelKinds supports product projections that aggregate
// several model kinds while sharing this service's detail cache.
func (s *QueryService) ListMyAssessmentsByModelKinds(ctx context.Context, testeeID uint64, status string, modelKinds []string, page, pageSize int32) (*ListAssessmentsResponse, error) {
	reader, ok := s.evaluationClient.(interface {
		ListMyAssessmentsByModelKinds(context.Context, uint64, string, []string, int32, int32) (*ListAssessmentsResponse, error)
	})
	if !ok {
		return nil, fmt.Errorf("assessment model-kinds reader is not configured")
	}
	return reader.ListMyAssessmentsByModelKinds(ctx, testeeID, status, modelKinds, page, pageSize)
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

// ListAssessmentsByModelKind is the shared product-projection seam used by
// typology routes without exposing the infra reader directly.
func (s *QueryService) ListAssessmentsByModelKind(ctx context.Context, testeeID uint64, status, modelKind string, page, pageSize int32) (*ListAssessmentsResponse, error) {
	if s == nil || s.evaluationClient == nil {
		return nil, nil
	}
	return s.evaluationClient.ListMyAssessments(ctx, testeeID, status, "", "", "", "", modelKind, page, pageSize)
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
