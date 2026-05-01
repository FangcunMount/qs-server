package interpretengine

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
)

func TestInterpreterRangeStrategyUsesConfiguredRuleAndFallbacksToLastRule(t *testing.T) {
	interpreter := NewInterpreter()
	config := &interpretengine.Config{
		FactorCode: "anxiety",
		Rules: []interpretengine.RuleSpec{
			{Min: 0, Max: 10, RiskLevel: "none", Label: "正常", Description: "normal", Suggestion: "keep"},
			{Min: 10, Max: 20, RiskLevel: "high", Label: "高风险", Description: "high", Suggestion: "care"},
		},
	}

	result, err := interpreter.InterpretFactor(12, config, interpretengine.StrategyTypeRange)
	if err != nil {
		t.Fatalf("InterpretFactor returned error: %v", err)
	}
	if result.Description != "high" || result.Suggestion != "care" || result.RiskLevel != "high" {
		t.Fatalf("result = %#v, want configured high-risk rule", result)
	}

	result, err = interpreter.InterpretFactor(99, config, interpretengine.StrategyTypeRange)
	if err != nil {
		t.Fatalf("InterpretFactor fallback returned error: %v", err)
	}
	if result.Description != "high" {
		t.Fatalf("fallback result = %#v, want last rule", result)
	}
}

func TestDefaultProviderKeepsLegacyChineseTemplates(t *testing.T) {
	provider := NewDefaultProvider()

	factor := provider.ProvideFactor("焦虑", 18, "high")
	if factor.Description != "焦虑得分18.0分，处于较高风险水平" {
		t.Fatalf("factor description = %q", factor.Description)
	}
	if factor.Suggestion != "建议尽快咨询专业人员，了解更多信息" {
		t.Fatalf("factor suggestion = %q", factor.Suggestion)
	}

	overall := provider.ProvideOverall(88, "severe")
	if overall.Description != "测评结果显示存在严重问题，需要立即关注" {
		t.Fatalf("overall description = %q", overall.Description)
	}
	if overall.Suggestion != "强烈建议尽快寻求专业帮助，进行全面评估和干预" {
		t.Fatalf("overall suggestion = %q", overall.Suggestion)
	}
}
