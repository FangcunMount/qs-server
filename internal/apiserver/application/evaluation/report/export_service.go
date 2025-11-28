package report

import (
	"context"
	"io"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// reportExportService 报告导出服务实现
type reportExportService struct {
	reportRepo domainReport.ReportRepository
	exporter   domainReport.ReportExporter
}

// NewReportExportService 创建报告导出服务
func NewReportExportService(
	reportRepo domainReport.ReportRepository,
	exporter domainReport.ReportExporter,
) ReportExportService {
	return &reportExportService{
		reportRepo: reportRepo,
		exporter:   exporter,
	}
}

// ExportPDF 导出PDF格式
func (s *reportExportService) ExportPDF(ctx context.Context, reportID uint64, options ExportOptionsDTO) (io.Reader, error) {
	id := meta.FromUint64(reportID)
	report, err := s.reportRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}

	exportOptions := toExportOptions(options)
	reader, err := s.exporter.Export(ctx, report, domainReport.ExportFormatPDF, exportOptions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportGenerationFailed, "导出PDF失败")
	}

	return reader, nil
}

// ExportHTML 导出HTML格式
func (s *reportExportService) ExportHTML(ctx context.Context, reportID uint64, options ExportOptionsDTO) (io.Reader, error) {
	id := meta.FromUint64(reportID)
	report, err := s.reportRepo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportNotFound, "报告不存在")
	}

	exportOptions := toExportOptions(options)
	reader, err := s.exporter.Export(ctx, report, domainReport.ExportFormatHTML, exportOptions)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInterpretReportGenerationFailed, "导出HTML失败")
	}

	return reader, nil
}

// GetSupportedFormats 获取支持的导出格式
func (s *reportExportService) GetSupportedFormats() []string {
	formats := s.exporter.SupportedFormats()
	result := make([]string, len(formats))
	for i, f := range formats {
		result[i] = f.String()
	}
	return result
}

// toExportOptions 转换导出选项
func toExportOptions(dto ExportOptionsDTO) domainReport.ExportOptions {
	opts := domainReport.DefaultExportOptions()
	opts.TemplateID = dto.TemplateID
	opts.IncludeSuggestions = dto.IncludeSuggestions
	opts.IncludeDimensions = dto.IncludeDimensions
	opts.IncludeCharts = dto.IncludeCharts

	if dto.HeaderTitle != "" || dto.SchoolName != "" {
		opts.HeaderInfo = &domainReport.HeaderInfo{
			Title:      dto.HeaderTitle,
			SchoolName: dto.SchoolName,
		}
	}

	return opts
}
