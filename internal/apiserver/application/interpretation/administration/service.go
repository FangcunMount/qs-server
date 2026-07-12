// Package administration contains organization-scoped Interpretation queries
// initiated by administrators and operations staff.
package administration

import (
	"context"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/internal/reportprojection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type Actor struct{ OrgID, OperatorUserID int64 }
type GetQuery struct{ AssessmentID uint64 }
type ListQuery struct {
	TesteeID       uint64
	Page, PageSize int
}
type ListScope struct {
	OrgID               int64
	TesteeID            uint64
	AccessibleTesteeIDs []uint64
	Restricted          bool
}

type Report = reportprojection.Report
type ListResult = reportprojection.ListResult
type ModelIdentity = reportprojection.ModelIdentity
type ScoreValue = reportprojection.ScoreValue
type ResultLevel = reportprojection.ResultLevel
type ModelExtra = reportprojection.ModelExtra
type Dimension = reportprojection.Dimension
type Suggestion = reportprojection.Suggestion

type Access interface {
	AuthorizeAssessment(ctx context.Context, actor Actor, assessmentID uint64) error
	ScopeReports(ctx context.Context, actor Actor, testeeID uint64) (ListScope, error)
}

type Service interface {
	GetReport(ctx context.Context, actor Actor, query GetQuery) (*Report, error)
	ListReports(ctx context.Context, actor Actor, query ListQuery) (*ListResult, error)
}

type service struct {
	reader interpretationreadmodel.ReportReader
	access Access
}

func NewService(reader interpretationreadmodel.ReportReader, access Access) Service {
	return &service{reader: reader, access: access}
}

func (s *service) GetReport(ctx context.Context, actor Actor, query GetQuery) (*Report, error) {
	if actor.OrgID == 0 || actor.OperatorUserID == 0 || query.AssessmentID == 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "administrator identity and assessment ID are required")
	}
	if s.reader == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrModuleInitializationFailed, "administration report service is not configured")
	}
	if err := s.access.AuthorizeAssessment(ctx, actor, query.AssessmentID); err != nil {
		return nil, err
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, query.AssessmentID)
	if err != nil {
		return nil, cberrors.WrapC(err, code.ErrInterpretReportNotFound, "报告不存在")
	}
	return reportprojection.FromRow(*row, policy.AudienceAdmin)
}

func (s *service) ListReports(ctx context.Context, actor Actor, query ListQuery) (*ListResult, error) {
	if actor.OrgID == 0 || actor.OperatorUserID == 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "administrator identity is required")
	}
	if s.reader == nil || s.access == nil {
		return nil, cberrors.WithCode(code.ErrModuleInitializationFailed, "administration report service is not configured")
	}
	scope, err := s.access.ScopeReports(ctx, actor, query.TesteeID)
	if err != nil {
		return nil, err
	}
	filter := interpretationreadmodel.ReportFilter{}
	switch {
	case scope.TesteeID != 0:
		filter.TesteeID = &scope.TesteeID
	case scope.Restricted && len(scope.AccessibleTesteeIDs) == 0:
		return emptyList(query), nil
	case scope.Restricted:
		filter.TesteeIDs = append([]uint64(nil), scope.AccessibleTesteeIDs...)
	default:
		orgID := scope.OrgID
		if orgID == 0 {
			return nil, cberrors.WithCode(code.ErrInvalidArgument, "report organization scope is empty")
		}
		filter.OrgID = &orgID
	}
	page, pageSize := normalize(query.Page, query.PageSize)
	rows, total, err := s.reader.ListReports(ctx, filter, interpretationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, cberrors.WrapC(err, code.ErrDatabase, "查询报告列表失败")
	}
	items := make([]*Report, 0, len(rows))
	for _, row := range rows {
		item, mapErr := reportprojection.FromRow(row, policy.AudienceAdmin)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	totalInt := int(total)
	return &ListResult{Items: items, Total: totalInt, Page: page, PageSize: pageSize, TotalPages: (totalInt + pageSize - 1) / pageSize}, nil
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
func emptyList(q ListQuery) *ListResult {
	p, s := normalize(q.Page, q.PageSize)
	return &ListResult{Items: []*Report{}, Page: p, PageSize: s}
}
