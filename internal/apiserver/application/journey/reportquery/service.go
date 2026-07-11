// Package reportquery composes protected Assessment access with Interpretation report reads.
package reportquery

import (
	"context"
	"time"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	interpretationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
)

// AssessmentProjection is the Journey-owned legacy view composed from an
// Evaluation Assessment and, when present, an Interpretation report.
type AssessmentProjection struct {
	Assessment    *assessmentApp.AssessmentResult
	Status        string
	InterpretedAt *time.Time
}

type AssessmentListProjection struct {
	Items      []*AssessmentProjection
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

type Scope struct {
	OrgID          int64
	OperatorUserID int64
}

type AssessmentAccess interface {
	LoadAccessibleAssessment(ctx context.Context, orgID int64, operatorUserID int64, assessmentID uint64) (*assessmentApp.AccessibleAssessmentContext, error)
	ScopeTesteeList(ctx context.Context, orgID int64, operatorUserID int64, testeeID uint64) (assessmentApp.TesteeListAccessScope, error)
}

// Service owns cross-module report authorization and legacy journey projection.
type Service interface {
	ProjectAssessment(ctx context.Context, result *assessmentApp.AssessmentResult) (*AssessmentProjection, error)
	ProjectAssessmentList(ctx context.Context, result *assessmentApp.AssessmentListResult) (*AssessmentListProjection, error)
	GetReport(ctx context.Context, scope Scope, assessmentID uint64) (*interpretationApp.ReportResult, error)
	GetReportOutcome(ctx context.Context, scope Scope, assessmentID uint64) (*interpretationApp.ReportOutcomeResult, error)
	ListReports(ctx context.Context, scope Scope, dto interpretationApp.ListReportsDTO) (*interpretationApp.ReportListResult, error)
	ListReportsOutcome(ctx context.Context, scope Scope, dto interpretationApp.ListReportsDTO) (*interpretationApp.ReportOutcomeListResult, error)
}

type service struct {
	access  AssessmentAccess
	reports interpretationApp.ReportQueryService
}

func NewService(access AssessmentAccess, reports interpretationApp.ReportQueryService) Service {
	return &service{access: access, reports: reports}
}

func (s *service) ProjectAssessment(ctx context.Context, result *assessmentApp.AssessmentResult) (*AssessmentProjection, error) {
	projected := &AssessmentProjection{Assessment: result}
	if result != nil {
		projected.Status = result.Status
	}
	if result == nil || result.Status != "evaluated" || s.reports == nil {
		return projected, nil
	}
	report, err := s.reports.GetByAssessmentID(ctx, result.ID)
	if err != nil {
		if interpretationApp.IsReportNotFound(err) {
			return projected, nil
		}
		return nil, err
	}
	if report == nil {
		return projected, nil
	}
	projected.Status = "interpreted"
	interpretedAt := report.CreatedAt
	projected.InterpretedAt = &interpretedAt
	return projected, nil
}

func (s *service) ProjectAssessmentList(ctx context.Context, result *assessmentApp.AssessmentListResult) (*AssessmentListProjection, error) {
	if result == nil {
		return nil, nil
	}
	projected := &AssessmentListProjection{
		Items:      make([]*AssessmentProjection, 0, len(result.Items)),
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}
	for _, item := range result.Items {
		value, err := s.ProjectAssessment(ctx, item)
		if err != nil {
			return nil, err
		}
		projected.Items = append(projected.Items, value)
	}
	return projected, nil
}

func (s *service) GetReport(ctx context.Context, scope Scope, assessmentID uint64) (*interpretationApp.ReportResult, error) {
	if _, err := s.access.LoadAccessibleAssessment(ctx, scope.OrgID, scope.OperatorUserID, assessmentID); err != nil {
		return nil, err
	}
	return s.reports.GetByAssessmentID(ctx, assessmentID)
}

func (s *service) GetReportOutcome(ctx context.Context, scope Scope, assessmentID uint64) (*interpretationApp.ReportOutcomeResult, error) {
	if _, err := s.access.LoadAccessibleAssessment(ctx, scope.OrgID, scope.OperatorUserID, assessmentID); err != nil {
		return nil, err
	}
	return s.reports.GetOutcomeByAssessmentID(ctx, assessmentID)
}

func (s *service) ListReports(ctx context.Context, scope Scope, dto interpretationApp.ListReportsDTO) (*interpretationApp.ReportListResult, error) {
	scoped, err := s.scopeList(ctx, scope, dto)
	if err != nil {
		return nil, err
	}
	return s.reports.ListByTesteeID(ctx, scoped)
}

func (s *service) ListReportsOutcome(ctx context.Context, scope Scope, dto interpretationApp.ListReportsDTO) (*interpretationApp.ReportOutcomeListResult, error) {
	scoped, err := s.scopeList(ctx, scope, dto)
	if err != nil {
		return nil, err
	}
	return s.reports.ListOutcomeByTesteeID(ctx, scoped)
}

func (s *service) scopeList(ctx context.Context, scope Scope, dto interpretationApp.ListReportsDTO) (interpretationApp.ListReportsDTO, error) {
	access, err := s.access.ScopeTesteeList(ctx, scope.OrgID, scope.OperatorUserID, dto.TesteeID)
	if err != nil {
		return dto, err
	}
	dto.TesteeID = access.TesteeID
	dto.AccessibleTesteeIDs = append([]uint64(nil), access.AccessibleTesteeIDs...)
	dto.RestrictToAccessScope = access.RestrictToAccessScope
	return dto, nil
}

var _ Service = (*service)(nil)
