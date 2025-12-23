package report

import (
	"context"

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
type DefaultReportBuilder struct {
	// suggestionGenerator 建议生成器（可选）
	// 如果不提供，则只使用评估结果中的建议
	suggestionGenerator SuggestionGenerator
}

// NewDefaultReportBuilder 创建默认报告构建器
func NewDefaultReportBuilder(suggestionGenerator SuggestionGenerator) *DefaultReportBuilder {
	return &DefaultReportBuilder{
		suggestionGenerator: suggestionGenerator,
	}
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

	// 4. 生成建议
	// 优先使用评估结果中的建议，然后使用 SuggestionGenerator 增强
	suggestions := b.buildSuggestions(context.Background(), result, reportID, medicalScale, dimensions)

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

		// 从量表中获取因子的 maxScore
		var maxScore *float64
		for _, f := range factors {
			if string(f.GetCode()) == string(fs.FactorCode) {
				maxScore = f.GetMaxScore()
				break
			}
		}

		dim := NewDimensionInterpret(
			FactorCode(fs.FactorCode),
			factorName,
			fs.RawScore,
			maxScore,
			RiskLevel(fs.RiskLevel),
			description,
			buildDimensionSuggestions(fs),
		)
		dimensions = append(dimensions, dim)
	}

	return dimensions
}

// buildDimensionSuggestions 将因子解读建议聚合到维度
func buildDimensionSuggestions(fs assessment.FactorScoreResult) []Suggestion {
	if fs.Suggestion == "" {
		return nil
	}
	factorCode := fs.FactorCode
	return []Suggestion{
		{
			Category:   SuggestionCategoryDimension,
			Content:    fs.Suggestion,
			FactorCode: &factorCode,
		},
	}
}

// buildSuggestions 构建建议
// 策略：
// 1. 首先使用 FactorInterpretationSuggestionStrategy 收集因子解读配置中的建议
// 2. 如果配置了 SuggestionGenerator，使用策略生成器生成额外建议
// 3. 合并去重
func (b *DefaultReportBuilder) buildSuggestions(
	ctx context.Context,
	result *assessment.EvaluationResult,
	reportID ID,
	medicalScale *scale.MedicalScale,
	dimensions []DimensionInterpret,
) []Suggestion {
	var allSuggestions []Suggestion

	// 1. 首先收集因子解读配置中的建议（通过 FactorInterpretationSuggestionStrategy）
	factorStrategy := NewFactorInterpretationSuggestionStrategy(result)
	if factorStrategy.CanHandle(nil) {
		factorSuggestions, err := factorStrategy.GenerateSuggestions(ctx, nil)
		if err == nil {
			allSuggestions = append(allSuggestions, factorSuggestions...)
		}
	}

	// 2. 如果配置了 SuggestionGenerator，使用策略生成器生成额外建议
	if b.suggestionGenerator != nil {
		// 构建临时报告用于生成建议
		tempReport := NewInterpretReport(
			reportID,
			medicalScale.GetTitle(),
			medicalScale.GetCode().String(),
			result.TotalScore,
			RiskLevel(result.RiskLevel),
			b.buildConclusion(medicalScale, result),
			dimensions,
			nil, // 建议稍后填充
		)

		generatedSuggestions, err := b.suggestionGenerator.Generate(ctx, tempReport)
		if err == nil {
			allSuggestions = append(allSuggestions, generatedSuggestions...)
		}
		// 失败不影响报告生成，只记录错误
	}

	// 3. 去重
	return uniqueSuggestions(allSuggestions)
}
