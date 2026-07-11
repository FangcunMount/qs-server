package assessment

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	runquery "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// protectedQueryService 受保护的查询服务
type protectedQueryService struct {
	operatorQueryService AssessmentOperatorQueryService
	scoreQueryService    ScoreQueryService
	accessQueryService   AssessmentAccessQueryService
	assessmentReader     evaluationreadmodel.AssessmentReader
	runQueryService      runquery.Service
}

// NewProtectedQueryService 创建受保护的查询服务实例
func NewProtectedQueryService(
	operatorQueryService AssessmentOperatorQueryService,
	scoreQueryService ScoreQueryService,
	accessQueryService AssessmentAccessQueryService,
	assessmentReader evaluationreadmodel.AssessmentReader,
	runQueryService runquery.Service,
) AssessmentProtectedQueryService {
	return &protectedQueryService{
		operatorQueryService: operatorQueryService,
		scoreQueryService:    scoreQueryService,
		accessQueryService:   accessQueryService,
		assessmentReader:     assessmentReader,
		runQueryService:      runQueryService,
	}
}

// GetAssessment 获取测评
func (s *protectedQueryService) GetAssessment(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*AssessmentResult, error) {
	assessmentCtx, err := s.loadAccessibleAssessment(ctx, scope, assessmentID)
	if err != nil {
		return nil, err
	}
	return assessmentCtx.Assessment, nil
}

// ListAssessments 查询测评列表
func (s *protectedQueryService) ListAssessments(ctx context.Context, scope ProtectedQueryScope, dto ListAssessmentsDTO) (*AssessmentListResult, error) {
	if s.operatorQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment operator query service is not configured")
	}
	dto = normalizeAssessmentListQuery(dto)
	orgScope, err := safeconv.Int64ToUint64(scope.OrgID)
	if err != nil {
		return nil, evalerrors.InvalidArgument("org scope exceeds uint64")
	}
	dto.OrgID = orgScope
	accessService, err := s.requireAccessService()
	if err != nil {
		return nil, err
	}
	scopedDTO, err := accessService.ScopeListAssessments(ctx, scope.OrgID, scope.OperatorUserID, dto)
	if err != nil {
		return nil, err
	}
	return s.operatorQueryService.List(ctx, scopedDTO)
}

// GetAssessmentOutcome 获取 结果 测评投影。
func (s *protectedQueryService) GetAssessmentOutcome(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*AssessmentOutcomeResult, error) {
	if _, err := s.loadAccessibleAssessment(ctx, scope, assessmentID); err != nil {
		return nil, err
	}
	if s.assessmentReader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	row, err := s.assessmentReader.GetAssessment(ctx, assessmentID)
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	return assessmentRowToOutcomeResult(*row)
}

// ListAssessmentsOutcome 查询 结果 测评列表。
func (s *protectedQueryService) ListAssessmentsOutcome(ctx context.Context, scope ProtectedQueryScope, dto ListAssessmentsDTO) (*AssessmentOutcomeListResult, error) {
	if s.assessmentReader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	dto = normalizeAssessmentListQuery(dto)
	orgScope, err := safeconv.Int64ToUint64(scope.OrgID)
	if err != nil {
		return nil, evalerrors.InvalidArgument("org scope exceeds uint64")
	}
	dto.OrgID = orgScope
	accessService, err := s.requireAccessService()
	if err != nil {
		return nil, err
	}
	scopedDTO, err := accessService.ScopeListAssessments(ctx, scope.OrgID, scope.OperatorUserID, dto)
	if err != nil {
		return nil, err
	}
	orgID, err := safeconv.Uint64ToInt64(scopedDTO.OrgID)
	if err != nil {
		return nil, evalerrors.InvalidArgument("机构ID超出 int64 范围")
	}
	page, pageSize := normalizePagination(scopedDTO.Page, scopedDTO.PageSize)
	listFilter, err := parseAssessmentListFilter(scopedDTO)
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "无效的受试者ID")
	}
	items, total, err := assessmentAdminQuery{reader: s.assessmentReader}.ListOutcome(ctx, scopedDTO, orgID, page, pageSize, listFilter)
	if err != nil {
		return nil, err
	}
	totalPages, err := safeconv.Int64ToInt((total + int64(pageSize) - 1) / int64(pageSize))
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总页数超出安全范围")
	}
	if totalPages == 0 && total > 0 {
		totalPages = 1
	}
	totalInt, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	return &AssessmentOutcomeListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetScores 获取测评得分
func (s *protectedQueryService) GetScores(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*ScoreResult, error) {
	if s.scoreQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("score query service is not configured")
	}
	assessmentCtx, err := s.loadAccessibleAssessment(ctx, scope, assessmentID)
	if err != nil {
		return nil, err
	}
	return s.scoreQueryService.GetByAssessmentID(ctx, assessmentCtx.AssessmentID)
}

// GetHighRiskFactors 获取测评高风险因子
func (s *protectedQueryService) GetHighRiskFactors(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*HighRiskFactorsResult, error) {
	if s.scoreQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("score query service is not configured")
	}
	assessmentCtx, err := s.loadAccessibleAssessment(ctx, scope, assessmentID)
	if err != nil {
		return nil, err
	}
	return s.scoreQueryService.GetHighRiskFactors(ctx, assessmentCtx.AssessmentID)
}

// GetFactorTrend 获取测评因子趋势
func (s *protectedQueryService) GetFactorTrend(ctx context.Context, scope ProtectedQueryScope, dto GetFactorTrendDTO) (*FactorTrendResult, error) {
	if s.scoreQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("score query service is not configured")
	}
	accessService, err := s.requireAccessService()
	if err != nil {
		return nil, err
	}
	scopedDTO, err := accessService.ScopeFactorTrend(ctx, scope.OrgID, scope.OperatorUserID, dto)
	if err != nil {
		return nil, err
	}
	return s.scoreQueryService.GetFactorTrend(ctx, scopedDTO)
}

// ListAssessmentRuns 列出评估执行 用于 一个accessible assessment。
func (s *protectedQueryService) ListAssessmentRuns(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64, limit int) (*AssessmentRunListResult, error) {
	if _, err := s.loadAccessibleAssessment(ctx, scope, assessmentID); err != nil {
		return nil, err
	}
	runQuery, err := s.requireRunQueryService()
	if err != nil {
		return nil, err
	}
	result, err := runQuery.ListByAssessmentID(ctx, assessmentID, limit)
	if err != nil {
		return nil, err
	}
	return assessmentRunListFromQuery(result), nil
}

// GetLatestAssessmentRun 返回最新 run 用于 一个accessible assessment。
func (s *protectedQueryService) GetLatestAssessmentRun(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*AssessmentRunResult, error) {
	if _, err := s.loadAccessibleAssessment(ctx, scope, assessmentID); err != nil {
		return nil, err
	}
	runQuery, err := s.requireRunQueryService()
	if err != nil {
		return nil, err
	}
	result, err := runQuery.FindLatestByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return assessmentRunFromQuery(result), nil
}

func (s *protectedQueryService) requireRunQueryService() (runquery.Service, error) {
	if s.runQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run query service is not configured")
	}
	return s.runQueryService, nil
}

func assessmentRunFromQuery(result *runquery.RunResult) *AssessmentRunResult {
	if result == nil {
		return nil
	}
	return &AssessmentRunResult{
		RunID:            result.RunID,
		AssessmentID:     result.AssessmentID,
		AttemptNo:        result.AttemptNo,
		Status:           result.Status,
		Retryable:        result.Retryable,
		ErrorCode:        result.ErrorCode,
		ErrorMessage:     result.ErrorMessage,
		StartedAt:        result.StartedAt,
		FinishedAt:       result.FinishedAt,
		TraceID:          result.TraceID,
		InputSnapshotRef: result.InputSnapshotRef,
	}
}

func assessmentRunListFromQuery(result *runquery.RunListResult) *AssessmentRunListResult {
	if result == nil {
		return &AssessmentRunListResult{}
	}
	items := make([]*AssessmentRunResult, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, assessmentRunFromQuery(item))
	}
	return &AssessmentRunListResult{Items: items}
}

// loadAccessibleAssessment 加载可访问的测评
// 场景：受保护的查询服务加载可访问的测评
// 说明：加载测评数据，并检查是否属于当前机构
func (s *protectedQueryService) loadAccessibleAssessment(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*AccessibleAssessmentContext, error) {
	accessService, err := s.requireAccessService()
	if err != nil {
		return nil, err
	}
	return accessService.LoadAccessibleAssessment(ctx, scope.OrgID, scope.OperatorUserID, assessmentID)
}

// requireAccessService 获取访问查询服务
// 场景：受保护的查询服务获取访问查询服务
// 说明：获取访问查询服务，并检查是否配置
func (s *protectedQueryService) requireAccessService() (AssessmentAccessQueryService, error) {
	if s.accessQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment access query service is not configured")
	}
	return s.accessQueryService, nil
}

// normalizeAssessmentListQuery 规范化测评列表查询
// 场景：受保护的查询服务规范化测评列表查询
// 说明：规范化测评列表查询，确保页码和页大小有效
func normalizeAssessmentListQuery(dto ListAssessmentsDTO) ListAssessmentsDTO {
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 10
	}
	return dto
}
