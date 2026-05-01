package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
)

// logInterpretation 记录因子解读结果
func (g *InterpretationGenerator) logInterpretation(
	ctx context.Context,
	factorCode string,
	factorName string,
	score float64,
	conclusion string,
	suggestion string,
) {
	l := logger.L(ctx)
	l.Infow("Factor interpretation generated",
		"factor_code", factorCode,
		"factor_name", factorName,
		"score", score,
		"conclusion", conclusion,
		"suggestion", suggestion)
}

// logRuleMatch 记录规则匹配
func (g *InterpretationGenerator) logRuleMatch(
	ctx context.Context,
	factorCode string,
	score float64,
	result *interpretengine.Result,
) {
	l := logger.L(ctx)
	l.Infow("Interpretation rule matched",
		"factor_code", factorCode,
		"score", score,
		"risk_level", result.RiskLevel,
		"label", result.Label)
}

// logUseDefault 记录使用默认模板
func (g *InterpretationGenerator) logUseDefault(
	ctx context.Context,
	factorCode string,
	score float64,
) {
	l := logger.L(ctx)
	l.Debugw("Using default interpretation template",
		"factor_code", factorCode,
		"score", score)
}
