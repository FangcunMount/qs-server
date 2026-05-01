package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
)

// generateFactorInterpretations 生成因子解读
func (g *InterpretationGenerator) generateFactorInterpretations(ctx context.Context, evalCtx *Context) {
	l := logger.L(ctx)
	l.Infow("Generating factor interpretations", "factor_count", len(evalCtx.FactorScores))

	updatedScores := make([]assessment.FactorScoreResult, 0, len(evalCtx.FactorScores))

	for _, fs := range evalCtx.FactorScores {
		// 尝试从量表获取因子的解读规则
		conclusion, suggestion := g.interpretFactorWithRules(ctx, evalCtx, fs)

		updatedScore := assessment.NewFactorScoreResult(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.RiskLevel,
			conclusion,
			suggestion,
			fs.IsTotalScore,
		)
		updatedScores = append(updatedScores, updatedScore)

		g.logInterpretation(ctx, string(fs.FactorCode), fs.FactorName, fs.RawScore, conclusion, suggestion)
	}

	evalCtx.FactorScores = updatedScores
	l.Infow("Factor interpretations generated", "factor_count", len(updatedScores))
}

// interpretFactorWithRules 使用量表规则解读因子
func (g *InterpretationGenerator) interpretFactorWithRules(ctx context.Context, evalCtx *Context, fs assessment.FactorScoreResult) (conclusion, suggestion string) {
	l := logger.L(ctx)

	// 卫语句：没有量表配置，直接使用默认解读
	if evalCtx.MedicalScale == nil {
		g.logUseDefault(ctx, string(fs.FactorCode), fs.RawScore)
		return g.interpretFactorDefault(fs.FactorName, fs.RiskLevel, fs.RawScore)
	}

	// 卫语句：查找因子
	scaleFactorCode := scale.NewFactorCode(string(fs.FactorCode))
	factor, found := evalCtx.MedicalScale.FindFactorByCode(scaleFactorCode)
	if !found {
		l.Warnw("Factor not found in scale",
			"factor_code", string(fs.FactorCode))
		g.logUseDefault(ctx, string(fs.FactorCode), fs.RawScore)
		return g.interpretFactorDefault(fs.FactorName, fs.RiskLevel, fs.RawScore)
	}

	l.Debugw("Found factor in scale",
		"factor_code", string(fs.FactorCode),
		"score", fs.RawScore)

	// 卫语句：构建解读配置
	config := g.buildInterpretConfig(factor)
	if config == nil || len(config.Rules) == 0 {
		l.Debugw("No interpret rules in factor config",
			"factor_code", string(fs.FactorCode))
		g.logUseDefault(ctx, string(fs.FactorCode), fs.RawScore)
		return g.interpretFactorDefault(fs.FactorName, fs.RiskLevel, fs.RawScore)
	}

	// 使用解读服务进行解读（默认使用区间策略）
	if g.interpreter == nil {
		l.Warnw("Interpret engine is not configured",
			"factor_code", string(fs.FactorCode))
		g.logUseDefault(ctx, string(fs.FactorCode), fs.RawScore)
		return g.interpretFactorDefault(fs.FactorName, fs.RiskLevel, fs.RawScore)
	}

	result, err := g.interpreter.InterpretFactor(
		fs.RawScore,
		config,
		interpretengine.StrategyTypeRange,
	)
	if err != nil || result == nil {
		l.Warnw("Failed to interpret factor",
			"factor_code", string(fs.FactorCode),
			"error", err)
		g.logUseDefault(ctx, string(fs.FactorCode), fs.RawScore)
		return g.interpretFactorDefault(fs.FactorName, fs.RiskLevel, fs.RawScore)
	}

	// 成功使用规则解读
	g.logRuleMatch(ctx, string(fs.FactorCode), fs.RawScore, result)
	return result.Description, result.Suggestion
}

// interpretFactorDefault 使用默认模板解读因子
func (g *InterpretationGenerator) interpretFactorDefault(factorName string, riskLevel assessment.RiskLevel, score float64) (conclusion, suggestion string) {
	if g.defaultProvider == nil {
		return "", ""
	}
	result := g.defaultProvider.ProvideFactor(factorName, score, string(riskLevel))
	return result.Description, result.Suggestion
}
