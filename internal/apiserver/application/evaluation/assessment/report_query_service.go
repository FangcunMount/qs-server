package assessment

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// reportQueryService 报告查询服务实现
// 行为者：报告查询者（答题者或管理员）
type reportQueryService struct {
	reportRepo report.ReportRepository
	reader     evaluationreadmodel.ReportReader
}

// NewReportQueryService 创建报告查询服务
func NewReportQueryService(reportRepo report.ReportRepository) ReportQueryService {
	return &reportQueryService{
		reportRepo: reportRepo,
	}
}

func NewReportQueryServiceWithReadModel(reportRepo report.ReportRepository, reader evaluationreadmodel.ReportReader) ReportQueryService {
	return &reportQueryService{
		reportRepo: reportRepo,
		reader:     reader,
	}
}

// GetByAssessmentID 根据测评ID获取报告
func (s *reportQueryService) GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportResult, error) {
	if s.reader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "report read model is not configured")
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}
	return reportRowToResult(*row), nil
}

// ListByTesteeID 获取受试者的报告列表
func (s *reportQueryService) ListByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportListResult, error) {
	page, pageSize := normalizePagination(dto.Page, dto.PageSize)
	if s.reader == nil {
		return nil, errors.WithCode(errorCode.ErrModuleInitializationFailed, "report read model is not configured")
	}
	rows, total, err := s.listReportRows(ctx, dto, page, pageSize)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询报告列表失败")
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
		return nil, 0, errors.WithCode(errorCode.ErrInvalidArgument, "受试者ID不能为空")
	}
	return s.reader.ListReports(ctx, filter, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
}

// ExportPDF 导出PDF报告
func (s *reportQueryService) ExportPDF(_ context.Context, _ uint64) ([]byte, error) {
	return nil, errors.WithCode(errorCode.ErrUnsupportedOperation, "PDF导出当前不支持")
}
