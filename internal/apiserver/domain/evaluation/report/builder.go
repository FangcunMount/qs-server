package report

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

// ==================== ReportBuilder 领域服务 ====================

// ReportBuilder 报告构建器接口
// 职责：根据评估结果构建解读报告
// 实现方式：调用 scale 域的解读服务获取解读信息
type ReportBuilder interface {
	// Build 构建解读报告
	// 参数：
	//   - assess: 测评实体
	//   - medicalScale: 量表实体
	//   - evaluationResult: 评估结果
	// 返回：
	//   - *InterpretReport: 解读报告
	//   - error: 构建失败时返回错误
	Build(
		assess *assessment.Assessment,
		medicalScale *scale.MedicalScale,
		evaluationResult *assessment.EvaluationResult,
	) (*InterpretReport, error)
}

// ==================== 默认实现 ====================

// DefaultReportBuilder 默认报告构建器实现
type DefaultReportBuilder struct{}

// NewDefaultReportBuilder 创建默认报告构建器
func NewDefaultReportBuilder() *DefaultReportBuilder {
	return &DefaultReportBuilder{}
}

// Build 构建解读报告
func (b *DefaultReportBuilder) Build(
	assess *assessment.Assessment,
	medicalScale *scale.MedicalScale,
	result *assessment.EvaluationResult,
) (*InterpretReport, error) {
	if assess == nil {
		return nil, ErrInvalidArgument
	}
	if medicalScale == nil {
		return nil, ErrInvalidArgument
	}
	if result == nil {
		return nil, ErrInvalidArgument
	}

	// 1. 转换 assessmentID 为 report.ID
	reportID := ID(assess.ID())

	// 2. 获取总体结论
	conclusion := b.buildConclusion(medicalScale, result)

	// 3. 构建维度解读
	dimensions := b.buildDimensions(medicalScale, result)

	// 4. 生成初始建议（可选，也可由 SuggestionGenerator 后续补充）
	suggestions := b.buildInitialSuggestions(result)

	// 5. 创建报告
	report := NewInterpretReport(
		reportID,
		medicalScale.GetTitle(),
		medicalScale.GetCode().String(),
		result.TotalScore,
		RiskLevel(result.RiskLevel),
		conclusion,
		dimensions,
		suggestions,
	)

	return report, nil
}

// buildConclusion 构建总体结论
func (b *DefaultReportBuilder) buildConclusion(medicalScale *scale.MedicalScale, result *assessment.EvaluationResult) string {
	// TODO: 调用 interpretService 获取结论
	// 这里先返回默认结论
	if assessment.IsHighRisk(result.RiskLevel) {
		return "测评结果显示存在较高风险，建议进一步关注和干预。"
	}
	return "测评结果显示状态良好，请继续保持。"
}

// buildDimensions 构建维度解读
func (b *DefaultReportBuilder) buildDimensions(medicalScale *scale.MedicalScale, result *assessment.EvaluationResult) []DimensionInterpret {
	if result == nil || len(result.FactorScores) == 0 {
		return nil
	}

	// 构建因子名称映射
	factors := medicalScale.GetFactors()
	factorNameMap := make(map[string]string)
	for _, f := range factors {
		factorNameMap[string(f.GetCode())] = f.GetTitle()
	}

	dimensions := make([]DimensionInterpret, 0, len(result.FactorScores))
	for _, fs := range result.FactorScores {
		factorName := factorNameMap[string(fs.FactorCode)]
		if factorName == "" {
			factorName = string(fs.FactorCode)
		}

		// TODO: 调用 interpretService 获取描述
		description := b.getFactorDescription(fs)

		dim := NewDimensionInterpret(
			FactorCode(fs.FactorCode),
			factorName,
			fs.RawScore,
			RiskLevel(fs.RiskLevel),
			description,
		)
		dimensions = append(dimensions, dim)
	}

	return dimensions
}

// getFactorDescription 获取因子描述
func (b *DefaultReportBuilder) getFactorDescription(fs assessment.FactorScoreResult) string {
	// TODO: 调用 interpretService 获取描述
	if assessment.IsHighRisk(fs.RiskLevel) {
		return "该维度得分偏高，需要关注。"
	}
	return "该维度得分正常。"
}

// buildInitialSuggestions 构建初始建议
func (b *DefaultReportBuilder) buildInitialSuggestions(result *assessment.EvaluationResult) []string {
	// 基础建议，具体建议由 SuggestionGenerator 生成
	if assessment.IsHighRisk(result.RiskLevel) {
		return []string{
			"建议与专业人员进行沟通",
			"建议制定个性化干预计划",
		}
	}
	return nil
}
