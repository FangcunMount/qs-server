// Package reportquery preserves the cross-module Assessment status projection
// while delegating all authorized report reads to the administration actor.
package reportquery

import (
	"context"
	"errors"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	interpretationAdmin "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"time"
)

type AssessmentProjection struct {
	Assessment    *assessmentApp.AssessmentResult
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

type Service interface {
	ProjectAssessment(context.Context, *assessmentApp.AssessmentResult) (*AssessmentProjection, error)
	ProjectAssessmentList(context.Context, *assessmentApp.AssessmentListResult) (*AssessmentListProjection, error)
	GetReport(context.Context, Scope, uint64) (*Report, error)
	GetReportOutcome(context.Context, Scope, uint64) (*Report, error)
	ListReports(context.Context, Scope, ListQuery) (*ReportList, error)
	ListReportsOutcome(context.Context, Scope, ListQuery) (*ReportList, error)
}
type service struct {
	reader interpretationreadmodel.ReportReader
	admin  interpretationAdmin.Service
}

func NewAdministrationService(reader interpretationreadmodel.ReportReader, admin interpretationAdmin.Service) Service {
	return &service{reader: reader, admin: admin}
}
func (s *service) ProjectAssessment(ctx context.Context, result *assessmentApp.AssessmentResult) (*AssessmentProjection, error) {
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
func (s *service) ProjectAssessmentList(ctx context.Context, result *assessmentApp.AssessmentListResult) (*AssessmentListProjection, error) {
	if result == nil {
		return nil, nil
	}
	out := &AssessmentListProjection{Items: make([]*AssessmentProjection, 0, len(result.Items)), Total: result.Total, Page: result.Page, PageSize: result.PageSize, TotalPages: result.TotalPages}
	for _, item := range result.Items {
		p, err := s.ProjectAssessment(ctx, item)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, p)
	}
	return out, nil
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
