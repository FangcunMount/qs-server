package report

import (
	"context"
	"io"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
)

// ==================== 报告应用服务接口 ====================

// ReportQueryService 报告查询服务接口
// 行为者：报告查询者（受试者、管理员）
// 职责：提供报告查询能力
type ReportQueryService interface {
	// GetByID 根据报告ID获取报告
	GetByID(ctx context.Context, reportID uint64) (*ReportResult, error)

	// GetByAssessmentID 根据测评ID获取报告
	GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportResult, error)

	// ListByTesteeID 获取受试者的报告列表
	ListByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportListResult, error)

	// ListHighRiskReports 获取高风险报告列表
	ListHighRiskReports(ctx context.Context, dto ListHighRiskReportsDTO) (*ReportListResult, error)
}

// ReportGenerationService 报告生成服务接口
// 行为者：评估引擎 (qs-worker)
// 职责：根据评估结果生成报告
type ReportGenerationService interface {
	// GenerateFromEvaluation 根据评估结果生成报告
	GenerateFromEvaluation(ctx context.Context, dto GenerateReportDTO) (*ReportResult, error)
}

// SuggestionService 建议服务接口
// 行为者：建议生成器
// 职责：为报告生成个性化建议
type SuggestionService interface {
	// GenerateSuggestions 生成建议
	GenerateSuggestions(ctx context.Context, reportID uint64) ([]string, error)

	// UpdateSuggestions 更新报告的建议
	UpdateSuggestions(ctx context.Context, reportID uint64, suggestions []string) error
}

// ReportExportService 报告导出服务接口
// 行为者：导出请求者（受试者、管理员）
// 职责：将报告导出为各种格式
type ReportExportService interface {
	// ExportPDF 导出PDF格式
	ExportPDF(ctx context.Context, reportID uint64, options ExportOptionsDTO) (io.Reader, error)

	// ExportHTML 导出HTML格式
	ExportHTML(ctx context.Context, reportID uint64, options ExportOptionsDTO) (io.Reader, error)

	// GetSupportedFormats 获取支持的导出格式
	GetSupportedFormats() []string
}

// ==================== 输入 DTO ====================

// ListReportsDTO 查询报告列表输入
type ListReportsDTO struct {
	TesteeID uint64
	Page     int
	PageSize int
}

// ListHighRiskReportsDTO 查询高风险报告列表输入
type ListHighRiskReportsDTO struct {
	Page     int
	PageSize int
}

// GenerateReportDTO 生成报告输入
type GenerateReportDTO struct {
	AssessmentID uint64
	ScaleName    string
	ScaleCode    string
	TotalScore   float64
	RiskLevel    string
	Conclusion   string
	Dimensions   []DimensionDTO
	Suggestions  []string
}

// DimensionDTO 维度输入
type DimensionDTO struct {
	FactorCode  string
	FactorName  string
	RawScore    float64
	MaxScore    *float64
	RiskLevel   string
	Description string
}

// ExportOptionsDTO 导出选项
type ExportOptionsDTO struct {
	TemplateID         string
	IncludeSuggestions bool
	IncludeDimensions  bool
	IncludeCharts      bool
	HeaderTitle        string
	SchoolName         string
}

// ==================== 输出 DTO ====================

// ReportResult 报告查询结果
type ReportResult struct {
	ID          uint64            `json:"id"`
	ScaleName   string            `json:"scaleName"`
	ScaleCode   string            `json:"scaleCode"`
	TotalScore  float64           `json:"totalScore"`
	RiskLevel   string            `json:"riskLevel"`
	Conclusion  string            `json:"conclusion"`
	Dimensions  []DimensionResult `json:"dimensions"`
	Suggestions []string          `json:"suggestions"`
	CreatedAt   string            `json:"createdAt"`
}

// DimensionResult 维度查询结果
type DimensionResult struct {
	FactorCode  string   `json:"factorCode"`
	FactorName  string   `json:"factorName"`
	RawScore    float64  `json:"rawScore"`
	MaxScore    *float64 `json:"maxScore,omitempty"`
	RiskLevel   string   `json:"riskLevel"`
	Description string   `json:"description"`
}

// ReportListResult 报告列表查询结果
type ReportListResult struct {
	Items      []*ReportResult `json:"items"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"pageSize"`
	TotalPages int             `json:"totalPages"`
}

// ==================== 转换函数 ====================

// ToReportResult 将领域模型转换为 ReportResult
func ToReportResult(r *domainReport.InterpretReport) *ReportResult {
	if r == nil {
		return nil
	}

	// 转换维度列表
	dimensions := make([]DimensionResult, len(r.Dimensions()))
	for i, d := range r.Dimensions() {
		dimensions[i] = DimensionResult{
			FactorCode:  string(d.FactorCode()),
			FactorName:  d.FactorName(),
			RawScore:    d.RawScore(),
			MaxScore:    d.MaxScore(),
			RiskLevel:   string(d.RiskLevel()),
			Description: d.Description(),
		}
	}

	return &ReportResult{
		ID:          r.ID().Uint64(),
		ScaleName:   r.ScaleName(),
		ScaleCode:   r.ScaleCode(),
		TotalScore:  r.TotalScore(),
		RiskLevel:   string(r.RiskLevel()),
		Conclusion:  r.Conclusion(),
		Dimensions:  dimensions,
		Suggestions: r.Suggestions(),
		CreatedAt:   r.CreatedAt().Format("2006-01-02 15:04:05"),
	}
}
