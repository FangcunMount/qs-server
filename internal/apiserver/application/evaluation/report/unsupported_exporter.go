package report

import (
	"context"
	"io"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
)

type unsupportedReportExporter struct{}

// NewUnsupportedReportExporter returns an explicit unsupported adapter so report
// export remains a stable entrypoint without pretending the capability exists.
func NewUnsupportedReportExporter() domainReport.ReportExporter {
	return unsupportedReportExporter{}
}

func (unsupportedReportExporter) Export(
	_ context.Context,
	_ *domainReport.InterpretReport,
	_ domainReport.ExportFormat,
	_ domainReport.ExportOptions,
) (io.Reader, error) {
	return nil, evalerrors.UnsupportedOperation("报告导出当前不支持")
}

func (unsupportedReportExporter) SupportedFormats() []domainReport.ExportFormat {
	return nil
}
