package report

import "errors"

// ==================== 领域错误（哨兵错误）====================
// 设计原则：领域层只定义哨兵错误，错误码由 pkg/code 统一管理

var (
	// ErrReportNotFound 报告不存在
	ErrReportNotFound = errors.New("report not found")

	// ErrInvalidArgument 无效参数
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrReportAlreadyExists 报告已存在
	ErrReportAlreadyExists = errors.New("report already exists")

	// ErrExportFailed 导出失败
	ErrExportFailed = errors.New("export failed")

	// ErrUnsupportedFormat 不支持的导出格式
	ErrUnsupportedFormat = errors.New("unsupported export format")
)
