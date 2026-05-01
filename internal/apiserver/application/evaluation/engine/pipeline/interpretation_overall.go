package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

// generateOverallInterpretation 生成整体解读
func (g *InterpretationGenerator) generateOverallInterpretation(ctx context.Context, evalCtx *Context) {
	l := logger.L(ctx)
	l.Infow("Generating overall interpretation",
		"total_score", evalCtx.TotalScore,
		"risk_level", evalCtx.RiskLevel)

	// 卫语句：没有量表配置，使用默认解读
	if evalCtx.MedicalScale == nil {
		l.Debugw("No medical scale, using default template for overall interpretation")
		g.generateOverallInterpretationDefault(evalCtx)
		l.Infow("Overall interpretation generated",
			"conclusion", evalCtx.Conclusion)
		return
	}

	// 尝试从总分因子的解读规则获取整体解读
	totalScoreFactor := g.findTotalScoreFactor(evalCtx)
	if totalScoreFactor == nil {
		l.Debugw("No total score factor found, using default template")
		g.generateOverallInterpretationDefault(evalCtx)
		l.Infow("Overall interpretation generated",
			"conclusion", evalCtx.Conclusion)
		return
	}

	l.Debugw("Found total score factor",
		"factor_code", string(totalScoreFactor.FactorCode),
		"score", totalScoreFactor.RawScore)

	// 尝试使用总分因子的解读规则
	if g.tryInterpretWithTotalScoreRule(ctx, evalCtx, totalScoreFactor) {
		l.Infow("Overall interpretation from total score rule",
			"conclusion", evalCtx.Conclusion)
		return
	}

	// 降级使用默认模板
	l.Debugw("No matching rule for total score, using default template")
	g.generateOverallInterpretationDefault(evalCtx)
	l.Infow("Overall interpretation generated",
		"conclusion", evalCtx.Conclusion)
}

// findTotalScoreFactor 查找总分因子
func (g *InterpretationGenerator) findTotalScoreFactor(evalCtx *Context) *assessment.FactorScoreResult {
	for _, fs := range evalCtx.FactorScores {
		if fs.IsTotalScore {
			return &fs
		}
	}
	return nil
}

// tryInterpretWithTotalScoreRule 尝试使用总分因子的规则进行解读
// 返回 true 表示成功解读，false 表示失败需要降级
func (g *InterpretationGenerator) tryInterpretWithTotalScoreRule(
	ctx context.Context,
	evalCtx *Context,
	totalScoreFactor *assessment.FactorScoreResult,
) bool {
	l := logger.L(ctx)

	// 查找因子配置
	scaleFactorCode := scale.NewFactorCode(string(totalScoreFactor.FactorCode))
	factor, found := evalCtx.MedicalScale.FindFactorByCode(scaleFactorCode)
	if !found {
		l.Warnw("Total score factor not found in scale",
			"factor_code", string(totalScoreFactor.FactorCode))
		return false
	}

	// 查找匹配的解读规则
	rule := factor.FindInterpretRule(totalScoreFactor.RawScore)
	if rule == nil {
		l.Debugw("No interpret rule matched for total score",
			"factor_code", string(totalScoreFactor.FactorCode),
			"score", totalScoreFactor.RawScore)
		return false
	}

	// 检查规则是否有内容
	if rule.GetConclusion() == "" {
		l.Debugw("Interpret rule has empty conclusion",
			"factor_code", string(totalScoreFactor.FactorCode))
		return false
	}

	// 应用规则解读
	evalCtx.Conclusion = rule.GetConclusion()
	evalCtx.Suggestion = rule.GetSuggestion()
	return true
}

// generateOverallInterpretationDefault 使用默认模板生成整体解读
func (g *InterpretationGenerator) generateOverallInterpretationDefault(evalCtx *Context) {
	if g.defaultProvider == nil {
		return
	}
	result := g.defaultProvider.ProvideOverall(evalCtx.TotalScore, string(evalCtx.RiskLevel))
	evalCtx.Conclusion = result.Description
	evalCtx.Suggestion = result.Suggestion
}

// buildEvaluationResult 构建评估结果
func (g *InterpretationGenerator) buildEvaluationResult(evalCtx *Context) *assessment.EvaluationResult {
	return assessment.NewEvaluationResult(
		evalCtx.TotalScore,
		evalCtx.RiskLevel,
		evalCtx.Conclusion,
		evalCtx.Suggestion,
		evalCtx.FactorScores,
	)
}
