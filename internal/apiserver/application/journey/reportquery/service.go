// Package reportquery preserves the cross-module Assessment status projection
// while delegating all authorized report reads to the administration actor.
package reportquery

import (
	"context"
	"errors"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	interpretationAdmin "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type AssessmentProjection struct {
	Assessment    *evaluationoperator.Assessment
	Status        string
	InterpretedAt *time.Time
}
type AssessmentListProjection struct {
	Items                             []*AssessmentProjection
	Total, Page, PageSize, TotalPages int
}
type Scope struct{ OrgID, OperatorUserID int64 }
type ListQuery = interpretationAdmin.ListQuery
type Report = interpretationAdmin.Report
type ReportList = interpretationAdmin.ListResult

type AssessmentQuery interface {
	GetAssessment(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Assessment, error)
	ListAssessments(context.Context, evaluationoperator.Actor, evaluationoperator.ListQuery) (*evaluationoperator.AssessmentList, error)
}

type Service interface {
	GetAssessmentProjection(context.Context, Scope, uint64) (*AssessmentProjection, error)
	ListAssessmentProjection(context.Context, Scope, evaluationoperator.ListQuery) (*AssessmentListProjection, error)
	ProjectAssessment(context.Context, *evaluationoperator.Assessment) (*AssessmentProjection, error)
	GetReport(context.Context, Scope, uint64) (*Report, error)
	GetReportOutcome(context.Context, Scope, uint64) (*Report, error)
	ListReports(context.Context, Scope, ListQuery) (*ReportList, error)
	ListReportsOutcome(context.Context, Scope, ListQuery) (*ReportList, error)
}
type service struct {
	reader   interpretationreadmodel.ReportReader
	admin    interpretationAdmin.Service
	operator AssessmentQuery
}

func NewAdministrationService(reader interpretationreadmodel.ReportReader, admin interpretationAdmin.Service, operator AssessmentQuery) Service {
	return &service{reader: reader, admin: admin, operator: operator}
}
func (s *service) GetAssessmentProjection(ctx context.Context, scope Scope, id uint64) (*AssessmentProjection, error) {
	result, err := s.operator.GetAssessment(ctx, evaluationoperator.Actor{OrgID: scope.OrgID, OperatorUserID: scope.OperatorUserID}, id)
	if err != nil {
		return nil, err
	}
	return s.ProjectAssessment(ctx, result)
}
func (s *service) ListAssessmentProjection(ctx context.Context, scope Scope, query evaluationoperator.ListQuery) (*AssessmentListProjection, error) {
	result, err := s.operator.ListAssessments(ctx, evaluationoperator.Actor{OrgID: scope.OrgID, OperatorUserID: scope.OperatorUserID}, query)
	if err != nil {
		return nil, err
	}
	out := &AssessmentListProjection{Items: make([]*AssessmentProjection, 0, len(result.Items)), Total: result.Total, Page: result.Page, PageSize: result.PageSize, TotalPages: result.TotalPages}
	for _, item := range result.Items {
		projected, projectErr := s.ProjectAssessment(ctx, item)
		if projectErr != nil {
			return nil, projectErr
		}
		out.Items = append(out.Items, projected)
	}
	return out, nil
}
func (s *service) ProjectAssessment(ctx context.Context, result *evaluationoperator.Assessment) (*AssessmentProjection, error) {
	p := &AssessmentProjection{Assessment: result}
	if result != nil {
		p.Status = result.Status
	}
	if result == nil || result.Status != "evaluated" || s.reader == nil {
		return p, nil
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, result.ID)
	if err != nil {
		if errors.Is(err, interpretationreadmodel.ErrReportNotFound) || cberrors.IsCode(err, code.ErrInterpretReportNotFound) {
			return p, nil
		}
		return nil, err
	}
	if row != nil {
		p.Status = "interpreted"
		at := row.CreatedAt
		p.InterpretedAt = &at
	}
	return p, nil
}
func (s *service) GetReport(ctx context.Context, scope Scope, id uint64) (*Report, error) {
	return s.admin.GetReport(ctx, actor(scope), interpretationAdmin.GetQuery{AssessmentID: id})
}
func (s *service) GetReportOutcome(ctx context.Context, scope Scope, id uint64) (*Report, error) {
	return s.GetReport(ctx, scope, id)
}
func (s *service) ListReports(ctx context.Context, scope Scope, q ListQuery) (*ReportList, error) {
	return s.admin.ListReports(ctx, actor(scope), q)
}
func (s *service) ListReportsOutcome(ctx context.Context, scope Scope, q ListQuery) (*ReportList, error) {
	return s.ListReports(ctx, scope, q)
}
func actor(s Scope) interpretationAdmin.Actor {
	return interpretationAdmin.Actor{OrgID: s.OrgID, OperatorUserID: s.OperatorUserID}
}
