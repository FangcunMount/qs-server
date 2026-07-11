package interpretation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

// reportQueryService is the Interpretation-owned read use case for completed report projections.
type reportQueryService struct {
	reader evaluationreadmodel.ReportReader
}

// NewReportQueryService creates the Interpretation-owned report query service.
func NewReportQueryService(reader evaluationreadmodel.ReportReader) ReportQueryService {
	return &reportQueryService{reader: reader}
}

func (s *reportQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportResult, error) {
	if s.reader == nil {
		return nil, queryModuleNotConfigured("report read model is not configured")
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, queryReportNotFound(err, "报告不存在")
	}
	return reportRowToResult(*row), nil
}

func (s *reportQueryService) GetOutcomeByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportOutcomeResult, error) {
	if s.reader == nil {
		return nil, queryModuleNotConfigured("report read model is not configured")
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, queryReportNotFound(err, "报告不存在")
	}
	return reportRowToOutcomeResult(*row), nil
}

func (s *reportQueryService) ListByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportListResult, error) {
	page, pageSize := normalizeReportPagination(dto.Page, dto.PageSize)
	if s.reader == nil {
		return nil, queryModuleNotConfigured("report read model is not configured")
	}
	rows, total, err := s.listReportRows(ctx, dto, page, pageSize)
	if err != nil {
		return nil, queryDatabase(err, "查询报告列表失败")
	}
	items := make([]*ReportResult, 0, len(rows))
	for _, row := range rows {
		items = append(items, reportRowToResult(row))
	}
	totalInt := int(total)
	return &ReportListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (totalInt + pageSize - 1) / pageSize,
	}, nil
}

func (s *reportQueryService) ListOutcomeByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportOutcomeListResult, error) {
	page, pageSize := normalizeReportPagination(dto.Page, dto.PageSize)
	if s.reader == nil {
		return nil, queryModuleNotConfigured("report read model is not configured")
	}
	rows, total, err := s.listReportRows(ctx, dto, page, pageSize)
	if err != nil {
		return nil, queryDatabase(err, "查询报告列表失败")
	}
	items := make([]*ReportOutcomeResult, 0, len(rows))
	for _, row := range rows {
		items = append(items, reportRowToOutcomeResult(row))
	}
	totalInt := int(total)
	return &ReportOutcomeListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (totalInt + pageSize - 1) / pageSize,
	}, nil
}

func (s *reportQueryService) listReportRows(
	ctx context.Context,
	dto ListReportsDTO,
	page int,
	pageSize int,
) ([]evaluationreadmodel.ReportRow, int64, error) {
	filter := evaluationreadmodel.ReportFilter{}
	switch {
	case dto.TesteeID != 0:
		filter.TesteeID = &dto.TesteeID
	case dto.RestrictToAccessScope:
		if len(dto.AccessibleTesteeIDs) == 0 {
			return []evaluationreadmodel.ReportRow{}, 0, nil
		}
		filter.TesteeIDs = dto.AccessibleTesteeIDs
	default:
		return nil, 0, queryInvalidArgument("受试者ID不能为空")
	}
	return s.reader.ListReports(ctx, filter, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
}

func normalizeReportPagination(page, pageSize int) (int, int) {
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

var _ ReportQueryService = (*reportQueryService)(nil)
