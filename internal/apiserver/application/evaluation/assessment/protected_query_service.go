package assessment

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type protectedQueryService struct {
	managementService  AssessmentManagementService
	reportQueryService ReportQueryService
	scoreQueryService  ScoreQueryService
	waitService        AssessmentWaitService
	accessQueryService AssessmentAccessQueryService
}

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

func (s *protectedQueryService) GetAssessment(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*AssessmentResult, error) {
	assessmentCtx, err := s.loadAccessibleAssessment(ctx, scope, assessmentID)
	if err != nil {
		return nil, err
	}
	return assessmentCtx.Assessment, nil
}

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

func (s *protectedQueryService) WaitReport(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (evaluationwaiter.StatusSummary, error) {
	if _, err := s.loadAccessibleAssessment(ctx, scope, assessmentID); err != nil {
		return evaluationwaiter.StatusSummary{}, err
	}
	if s.waitService == nil {
		return pendingAssessmentStatusSummary(), nil
	}
	return s.waitService.WaitReport(ctx, assessmentID), nil
}

func (s *protectedQueryService) loadAccessibleAssessment(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*AccessibleAssessmentContext, error) {
	accessService, err := s.requireAccessService()
	if err != nil {
		return nil, err
	}
	return accessService.LoadAccessibleAssessment(ctx, scope.OrgID, scope.OperatorUserID, assessmentID)
}

func (s *protectedQueryService) requireAccessService() (AssessmentAccessQueryService, error) {
	if s.accessQueryService == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment access query service is not configured")
	}
	return s.accessQueryService, nil
}

func normalizeAssessmentListQuery(dto ListAssessmentsDTO) ListAssessmentsDTO {
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 10
	}
	return dto
}

func normalizeReportListQuery(dto ListReportsDTO) ListReportsDTO {
	if dto.Page <= 0 {
		dto.Page = 1
	}
	if dto.PageSize <= 0 {
		dto.PageSize = 10
	}
	return dto
}
