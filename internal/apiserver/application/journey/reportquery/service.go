// Package reportquery composes protected Assessment access with Interpretation report reads.
package reportquery

import (
	"context"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	interpretationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
)

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
	ProjectAssessment(ctx context.Context, result *assessmentApp.AssessmentResult) (*assessmentApp.AssessmentResult, error)
	ProjectAssessmentList(ctx context.Context, result *assessmentApp.AssessmentListResult) (*assessmentApp.AssessmentListResult, error)
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

func (s *service) ProjectAssessment(ctx context.Context, result *assessmentApp.AssessmentResult) (*assessmentApp.AssessmentResult, error) {
	if result == nil || result.Status != "evaluated" || s.reports == nil {
		return result, nil
	}
	report, err := s.reports.GetByAssessmentID(ctx, result.ID)
	if err != nil {
		if interpretationApp.IsReportNotFound(err) {
			return result, nil
		}
		return nil, err
	}
	if report == nil {
		return result, nil
	}
	projected := *result
	projected.Status = "interpreted"
	interpretedAt := report.CreatedAt
	projected.InterpretedAt = &interpretedAt
	return &projected, nil
}

func (s *service) ProjectAssessmentList(ctx context.Context, result *assessmentApp.AssessmentListResult) (*assessmentApp.AssessmentListResult, error) {
	if result == nil {
		return nil, nil
	}
	projected := *result
	projected.Items = append([]*assessmentApp.AssessmentResult(nil), result.Items...)
	for index, item := range projected.Items {
		value, err := s.ProjectAssessment(ctx, item)
		if err != nil {
			return nil, err
		}
		projected.Items[index] = value
	}
	return &projected, nil
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
