// Package participant contains Interpretation use cases initiated by a
// participant reading their own reports.
package participant

import (
	"context"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/internal/reportprojection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type Actor struct{ TesteeID uint64 }
type GetQuery struct{ AssessmentID uint64 }
type ListQuery struct{ Page, PageSize int }

type Report = reportprojection.Report
type ListResult = reportprojection.ListResult

type Access interface {
	AuthorizeOwnAssessment(ctx context.Context, testeeID, assessmentID uint64) error
}

type Service interface {
	GetMyReport(ctx context.Context, actor Actor, query GetQuery) (*Report, error)
	ListMyReports(ctx context.Context, actor Actor, query ListQuery) (*ListResult, error)
}

type service struct {
	reader interpretationreadmodel.ReportReader
	access Access
}

func NewService(reader interpretationreadmodel.ReportReader, access Access) Service {
	return &service{reader: reader, access: access}
}

func (s *service) GetMyReport(ctx context.Context, actor Actor, query GetQuery) (*Report, error) {
	if actor.TesteeID == 0 || query.AssessmentID == 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "testee ID and assessment ID are required")
	}
	if s.access == nil || s.reader == nil {
		return nil, cberrors.WithCode(code.ErrModuleInitializationFailed, "participant report service is not configured")
	}
	if err := s.access.AuthorizeOwnAssessment(ctx, actor.TesteeID, query.AssessmentID); err != nil {
		return nil, err
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, query.AssessmentID)
	if err != nil {
		return nil, cberrors.WrapC(err, code.ErrInterpretReportNotFound, "报告不存在")
	}
	return reportprojection.FromRow(*row, policy.AudienceParticipant)
}

func (s *service) ListMyReports(ctx context.Context, actor Actor, query ListQuery) (*ListResult, error) {
	if actor.TesteeID == 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "testee ID is required")
	}
	if s.reader == nil {
		return nil, cberrors.WithCode(code.ErrModuleInitializationFailed, "participant report service is not configured")
	}
	page, pageSize := normalizePagination(query.Page, query.PageSize)
	testeeID := actor.TesteeID
	rows, total, err := s.reader.ListReports(ctx, interpretationreadmodel.ReportFilter{TesteeID: &testeeID}, interpretationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, cberrors.WrapC(err, code.ErrDatabase, "查询报告列表失败")
	}
	items := make([]*Report, 0, len(rows))
	for _, row := range rows {
		item, mapErr := reportprojection.FromRow(row, policy.AudienceParticipant)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	totalInt := int(total)
	return &ListResult{Items: items, Total: totalInt, Page: page, PageSize: pageSize, TotalPages: (totalInt + pageSize - 1) / pageSize}, nil
}

func normalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
