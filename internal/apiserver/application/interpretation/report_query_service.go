package interpretation

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

// reportQueryService is the Interpretation-owned read use case for completed
// report projections. Its result shapes remain assessmentApp compatibility
// DTOs until the public transport contracts are migrated in a later batch.
type reportQueryService struct {
	reader evaluationreadmodel.ReportReader
}

// NewReportQueryService creates the Interpretation-owned report query service.
func NewReportQueryService(reader evaluationreadmodel.ReportReader) assessmentApp.ReportQueryService {
	return &reportQueryService{reader: reader}
}

func (s *reportQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*assessmentApp.ReportResult, error) {
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("report read model is not configured")
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, evalerrors.InterpretReportNotFound(err, "报告不存在")
	}
	return assessmentApp.ReportRowToResult(*row), nil
}

func (s *reportQueryService) GetOutcomeByAssessmentID(ctx context.Context, assessmentID uint64) (*assessmentApp.ReportOutcomeResult, error) {
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("report read model is not configured")
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, evalerrors.InterpretReportNotFound(err, "报告不存在")
	}
	return assessmentApp.ReportRowToOutcomeResult(*row), nil
}

func (s *reportQueryService) ListByTesteeID(ctx context.Context, dto assessmentApp.ListReportsDTO) (*assessmentApp.ReportListResult, error) {
	page, pageSize := normalizeReportPagination(dto.Page, dto.PageSize)
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("report read model is not configured")
	}
	rows, total, err := s.listReportRows(ctx, dto, page, pageSize)
	if err != nil {
		return nil, evalerrors.Database(err, "查询报告列表失败")
	}
	items := make([]*assessmentApp.ReportResult, 0, len(rows))
	for _, row := range rows {
		items = append(items, assessmentApp.ReportRowToResult(row))
	}
	totalInt := int(total)
	return &assessmentApp.ReportListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (totalInt + pageSize - 1) / pageSize,
	}, nil
}

func (s *reportQueryService) ListOutcomeByTesteeID(ctx context.Context, dto assessmentApp.ListReportsDTO) (*assessmentApp.ReportOutcomeListResult, error) {
	page, pageSize := normalizeReportPagination(dto.Page, dto.PageSize)
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("report read model is not configured")
	}
	rows, total, err := s.listReportRows(ctx, dto, page, pageSize)
	if err != nil {
		return nil, evalerrors.Database(err, "查询报告列表失败")
	}
	items := make([]*assessmentApp.ReportOutcomeResult, 0, len(rows))
	for _, row := range rows {
		items = append(items, assessmentApp.ReportRowToOutcomeResult(row))
	}
	totalInt := int(total)
	return &assessmentApp.ReportOutcomeListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (totalInt + pageSize - 1) / pageSize,
	}, nil
}

func (s *reportQueryService) listReportRows(
	ctx context.Context,
	dto assessmentApp.ListReportsDTO,
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
		return nil, 0, evalerrors.InvalidArgument("受试者ID不能为空")
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

var _ assessmentApp.ReportQueryService = (*reportQueryService)(nil)
