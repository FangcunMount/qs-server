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
	// 优先使用总分因子的解读作为总体结论
	for _, fs := range result.FactorScores {
		if fs.IsTotalScore && fs.Conclusion != "" {
			return fs.Conclusion
		}
	}

	// 如果没有总分因子解读，使用评估结果的总体结论
	if result.Conclusion != "" {
		return result.Conclusion
	}

	// 如果都没有，返回空字符串
	return ""
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

		// 直接使用评估引擎已生成的解读内容
		// 不使用默认文案兜底，如果没有生成解读则为空
		description := fs.Conclusion

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

// buildInitialSuggestions 构建初始建议
func (b *DefaultReportBuilder) buildInitialSuggestions(result *assessment.EvaluationResult) []string {
	// 优先使用评估结果中的总体建议
	if result.Suggestion != "" {
		return []string{result.Suggestion}
	}

	// 如果没有总体建议，尝试使用总分因子的建议
	for _, fs := range result.FactorScores {
		if fs.IsTotalScore && fs.Suggestion != "" {
			return []string{fs.Suggestion}
		}
	}

	// 如果都没有，返回空列表
	return nil
}
