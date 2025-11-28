package report

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ==================== ID 类型定义 ====================

// ID 报告ID类型（与 AssessmentID 一致，使用 meta.ID）
type ID = meta.ID

// AssessmentID 测评ID类型（用于关联 assessment 域）
type AssessmentID = assessment.ID

// NewID 创建报告ID
func NewID(id uint64) ID {
	return meta.FromUint64(id)
}

// ParseID 解析报告ID
func ParseID(s string) (ID, error) {
	return meta.ParseID(s)
}

// ==================== 风险等级（复用 scale 子域定义）====================

// RiskLevel 风险等级（复用 scale 子域的定义，保持一致性）
type RiskLevel = scale.RiskLevel

// 风险等级常量（复用 scale 子域定义）
const (
	RiskLevelNone   = scale.RiskLevelNone
	RiskLevelLow    = scale.RiskLevelLow
	RiskLevelMedium = scale.RiskLevelMedium
	RiskLevelHigh   = scale.RiskLevelHigh
	RiskLevelSevere = scale.RiskLevelSevere
)

// IsHighRisk 是否高风险（包含 high 和 severe）
func IsHighRisk(r RiskLevel) bool {
	return r == RiskLevelHigh || r == RiskLevelSevere
}

// ==================== 因子编码（复用 scale 子域定义） ====================

// FactorCode 因子编码（复用 scale 子域的定义）
type FactorCode = scale.FactorCode

// NewFactorCode 创建因子编码
func NewFactorCode(code string) FactorCode {
	return scale.NewFactorCode(code)
}

// ==================== 导出格式枚举 ====================

// ExportFormat 报告导出格式
type ExportFormat string

const (
	// ExportFormatPDF PDF格式
	ExportFormatPDF ExportFormat = "pdf"

	// ExportFormatHTML HTML格式
	ExportFormatHTML ExportFormat = "html"

	// ExportFormatJSON JSON格式
	ExportFormatJSON ExportFormat = "json"
)

// String 返回格式的字符串表示
func (f ExportFormat) String() string {
	return string(f)
}

// IsValid 检查格式是否有效
func (f ExportFormat) IsValid() bool {
	switch f {
	case ExportFormatPDF, ExportFormatHTML, ExportFormatJSON:
		return true
	default:
		return false
	}
}

// ==================== 报告生成输入 ====================

// GenerateReportInput 生成报告的输入参数
// 由应用层从评估结果组装后传入
type GenerateReportInput struct {
	// 测评ID（也作为报告ID）
	AssessmentID ID

	// 量表信息
	ScaleName string
	ScaleCode string

	// 评估结果
	TotalScore float64
	RiskLevel  RiskLevel
	Conclusion string

	// 因子得分列表
	FactorScores []FactorScoreInput
}

// FactorScoreInput 因子得分输入
type FactorScoreInput struct {
	FactorCode   FactorCode
	FactorName   string
	RawScore     float64
	RiskLevel    RiskLevel
	Description  string
	IsTotalScore bool
}

// ==================== 建议生成输入 ====================

// SuggestionInput 建议生成输入
type SuggestionInput struct {
	// 总体风险等级
	RiskLevel RiskLevel

	// 高风险因子列表
	HighRiskFactors []FactorScoreInput

	// 原始建议（来自解读规则）
	OriginalSuggestion string
}
