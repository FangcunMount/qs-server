// Package clinician contains report queries initiated by a clinician for an
// explicitly authorized participant.
package clinician

import (
	"context"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportprojection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type Actor struct{ OrgID, OperatorUserID int64 }
type GetQuery struct{ TesteeID, AssessmentID uint64 }
type ListQuery struct {
	TesteeID       uint64
	Page, PageSize int
}
type Report = reportprojection.Report
type ListResult = reportprojection.ListResult
type Access interface {
	AuthorizeParticipant(context.Context, Actor, uint64) error
	AuthorizeParticipantAssessment(context.Context, Actor, uint64, uint64) error
}
type Service interface {
	GetParticipantReport(context.Context, Actor, GetQuery) (*Report, error)
	ListParticipantReports(context.Context, Actor, ListQuery) (*ListResult, error)
}
type service struct {
	reader     interpretationreadmodel.ReportReader
	access     Access
	projection reportprojection.Mapper
}

func NewService(reader interpretationreadmodel.ReportReader, access Access, projection ...reportprojection.Mapper) Service {
	mapper := reportprojection.Mapper{}
	if len(projection) > 0 {
		mapper = projection[0]
	}
	return &service{reader: reader, access: access, projection: mapper}
}
func (s *service) GetParticipantReport(ctx context.Context, actor Actor, q GetQuery) (*Report, error) {
	if actor.OrgID == 0 || actor.OperatorUserID == 0 || q.TesteeID == 0 || q.AssessmentID == 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "clinician identity, testee ID and assessment ID are required")
	}
	if s.reader == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrModuleInitializationFailed, "clinician report service is not configured")
	}
	if err := s.access.AuthorizeParticipantAssessment(ctx, actor, q.TesteeID, q.AssessmentID); err != nil {
		return nil, err
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, q.AssessmentID)
	if err != nil {
		return nil, cberrors.WrapC(err, code.ErrInterpretReportNotFound, "报告不存在")
	}
	return s.projection.FromRow(ctx, *row, policy.AudienceClinician)
}
func (s *service) ListParticipantReports(ctx context.Context, actor Actor, q ListQuery) (*ListResult, error) {
	if actor.OrgID == 0 || actor.OperatorUserID == 0 || q.TesteeID == 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "clinician identity and testee ID are required")
	}
	if s.reader == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrModuleInitializationFailed, "clinician report service is not configured")
	}
	if err := s.access.AuthorizeParticipant(ctx, actor, q.TesteeID); err != nil {
		return nil, err
	}
	page, size := normalize(q.Page, q.PageSize)
	id := q.TesteeID
	rows, total, err := s.reader.ListReports(ctx, interpretationreadmodel.ReportFilter{TesteeID: &id}, interpretationreadmodel.PageRequest{Page: page, PageSize: size})
	if err != nil {
		return nil, cberrors.WrapC(err, code.ErrDatabase, "查询报告列表失败")
	}
	items := make([]*Report, 0, len(rows))
	for _, row := range rows {
		item, mapErr := s.projection.FromRow(ctx, row, policy.AudienceClinician)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	n := int(total)
	return &ListResult{Items: items, Total: n, Page: page, PageSize: size, TotalPages: (n + size - 1) / size}, nil
}
func normalize(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
