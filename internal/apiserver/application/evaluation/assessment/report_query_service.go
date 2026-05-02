package assessment

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

// reportQueryService 报告查询服务实现
// 行为者：报告查询者（答题者或管理员）
type reportQueryService struct {
	reader evaluationreadmodel.ReportReader
}

func NewReportQueryService(reader evaluationreadmodel.ReportReader) ReportQueryService {
	return &reportQueryService{
		reader: reader,
	}
}

// GetByAssessmentID 根据测评ID获取报告
func (s *reportQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportResult, error) {
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("report read model is not configured")
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, evalerrors.InterpretReportNotFound(err, "报告不存在")
	}
	return reportRowToResult(*row), nil
}

// ListByTesteeID 获取受试者的报告列表
func (s *reportQueryService) ListByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportListResult, error) {
	page, pageSize := normalizePagination(dto.Page, dto.PageSize)
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("report read model is not configured")
	}
	rows, total, err := s.listReportRows(ctx, dto, page, pageSize)
	if err != nil {
		return nil, evalerrors.Database(err, "查询报告列表失败")
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
		return nil, 0, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	return s.reader.ListReports(ctx, filter, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
}
