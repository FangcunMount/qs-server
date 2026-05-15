package assessment

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// protectedQueryService 受保护的查询服务
type protectedQueryService struct {
	managementService  AssessmentManagementService
	reportQueryService ReportQueryService
	scoreQueryService  ScoreQueryService
	waitService        AssessmentWaitService
	accessQueryService AssessmentAccessQueryService
}

// NewProtectedQueryService 创建受保护的查询服务实例
func NewProtectedQueryService(
	managementService AssessmentManagementService,
	reportQueryService ReportQueryService,
	scoreQueryService ScoreQueryService,
	waitService AssessmentWaitService,
	accessQueryService AssessmentAccessQueryService,
) AssessmentProtectedQueryService {
	return &protectedQueryService{
		managementService:  managementService,
		reportQueryService: reportQueryService,
		scoreQueryService:  scoreQueryService,
		waitService:        waitService,
		accessQueryService: accessQueryService,
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
	if s.managementService == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment management service is not configured")
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
	return s.managementService.List(ctx, scopedDTO)
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

// GetReport 获取测评报告
func (s *protectedQueryService) GetReport(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*ReportResult, error) {
	if s.reportQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("report query service is not configured")
	}
	assessmentCtx, err := s.loadAccessibleAssessment(ctx, scope, assessmentID)
	if err != nil {
		return nil, err
	}
	return s.reportQueryService.GetByAssessmentID(ctx, assessmentCtx.AssessmentID)
}

// ListReports 查询测评报告列表
func (s *protectedQueryService) ListReports(ctx context.Context, scope ProtectedQueryScope, dto ListReportsDTO) (*ReportListResult, error) {
	if s.reportQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("report query service is not configured")
	}
	dto = normalizeReportListQuery(dto)
	accessService, err := s.requireAccessService()
	if err != nil {
		return nil, err
	}
	scopedDTO, err := accessService.ScopeListReports(ctx, scope.OrgID, scope.OperatorUserID, dto)
	if err != nil {
		return nil, err
	}
	return s.reportQueryService.ListByTesteeID(ctx, scopedDTO)
}

// WaitReport 等待测评报告生成
func (s *protectedQueryService) WaitReport(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (evaluationwaiter.StatusSummary, error) {
	if _, err := s.loadAccessibleAssessment(ctx, scope, assessmentID); err != nil {
		return evaluationwaiter.StatusSummary{}, err
	}
	if s.waitService == nil {
		return pendingAssessmentStatusSummary(), nil
	}
	return s.waitService.WaitReport(ctx, assessmentID), nil
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

// normalizeReportListQuery 规范化测评报告列表查询
// 场景：受保护的查询服务规范化测评报告列表查询
// 说明：规范化测评报告列表查询，确保页码和页大小有效
func normalizeReportListQuery(dto ListReportsDTO) ListReportsDTO {
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 10
	}
	return dto
}
