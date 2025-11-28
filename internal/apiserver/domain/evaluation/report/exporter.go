package report

import (
	"context"
	"io"
)

// ==================== ReportExporter 领域服务 ====================

// ReportExporter 报告导出器接口
// 职责：将解读报告导出为不同格式（PDF、HTML等）
// 实现方式：基于模板引擎生成文档
type ReportExporter interface {
	// Export 导出报告
	// 参数：
	//   - ctx: 上下文
	//   - report: 解读报告
	//   - format: 导出格式
	//   - options: 导出选项
	// 返回：
	//   - io.Reader: 导出内容的读取器
	//   - error: 导出失败时返回错误
	Export(ctx context.Context, report *InterpretReport, format ExportFormat, options ExportOptions) (io.Reader, error)

	// SupportedFormats 支持的格式列表
	SupportedFormats() []ExportFormat
}

// ExportOptions 导出选项
type ExportOptions struct {
	// 模板ID（可选，使用自定义模板）
	TemplateID string

	// 是否包含建议
	IncludeSuggestions bool

	// 是否包含维度详情
	IncludeDimensions bool

	// 是否包含图表
	IncludeCharts bool

	// 页眉信息
	HeaderInfo *HeaderInfo

	// 页脚信息
	FooterInfo *FooterInfo

	// 自定义元数据
	Metadata map[string]string
}

// HeaderInfo 页眉信息
type HeaderInfo struct {
	Title      string
	Logo       []byte
	SchoolName string
}

// FooterInfo 页脚信息
type FooterInfo struct {
	ContactInfo string
	Disclaimer  string
}

// DefaultExportOptions 默认导出选项
func DefaultExportOptions() ExportOptions {
	return ExportOptions{
		IncludeSuggestions: true,
		IncludeDimensions:  true,
		IncludeCharts:      false,
	}
}

// ==================== 模板渲染接口 ====================

// TemplateRenderer 模板渲染器接口
type TemplateRenderer interface {
	// Render 渲染模板
	Render(ctx context.Context, templateID string, data TemplateData) ([]byte, error)
}

// TemplateData 模板数据
type TemplateData struct {
	Report     *InterpretReport
	Options    ExportOptions
	ExportTime string
}

// ==================== PDF 导出器 ====================

// PDFExporter PDF 导出器接口
type PDFExporter interface {
	ReportExporter

	// ExportWithWatermark 带水印导出
	ExportWithWatermark(ctx context.Context, report *InterpretReport, watermark string) (io.Reader, error)
}

// ==================== 导出事件 ====================

// ExportEvent 导出事件
type ExportEvent struct {
	ReportID ID
	Format   ExportFormat
	Success  bool
	Error    string
}
