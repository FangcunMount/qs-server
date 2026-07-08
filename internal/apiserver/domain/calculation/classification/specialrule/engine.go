package specialrule

import (
	"fmt"
	"strings"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
)

// MatchResult 是returned when special rule fires。
type MatchResult struct {
	OutcomeCode    string
	Trigger        string
	SkipScoring    bool
	ReplaceOutcome bool
}

// EvaluationContext 携带rule inputs 用于 一个special-rule phase。
type EvaluationContext struct {
	Payload    *modeltypology.Payload
	Answers    []classification.Answer
	Decision   modeltypology.PersonalityDecisionSpec
	Similarity float64
}

type strategyFunc func(modeltypology.SpecialRuleSpec, EvaluationContext) (MatchResult, bool)

// StrategyRegistry 解析special-rule strategies 按 rule 类型。
type StrategyRegistry struct {
	strategies map[modeltypology.SpecialRuleKind]strategyFunc
}

// DefaultStrategyRegistry 返回内置 special-rule strategies。
func DefaultStrategyRegistry() StrategyRegistry {
	return StrategyRegistry{
		strategies: map[modeltypology.SpecialRuleKind]strategyFunc{
			modeltypology.SpecialRuleKindAnswerMatch:       applyAnswerMatchRule,
			modeltypology.SpecialRuleKindFallbackThreshold: applyFallbackThresholdRule,
		},
	}
}

// Engine 评估配置urable special rules 针对 answers 和 计分结果。
type Engine struct {
	strategies StrategyRegistry
}

// ApplyBeforeScore 检查answer_match rules 和 returns match when answers trigger special 结果。
func (e Engine) ApplyBeforeScore(
	rules []modeltypology.SpecialRuleSpec,
	payload *modeltypology.Payload,
	answers []classification.Answer,
) (MatchResult, bool) {
	return e.applyPhase(modeltypology.SpecialRuleBeforeScore, rules, EvaluationContext{
		Payload: payload,
		Answers: answers,
	})
}

// ApplyAfterDecision 检查fallback_threshold rules when similarity falls below 配置化 threshold。
func (e Engine) ApplyAfterDecision(
	rules []modeltypology.SpecialRuleSpec,
	decision modeltypology.PersonalityDecisionSpec,
	payload *modeltypology.Payload,
	similarity float64,
) (MatchResult, bool) {
	ctx := EvaluationContext{
		Payload:    payload,
		Decision:   decision,
		Similarity: similarity,
	}
	if match, ok := e.applyPhase(modeltypology.SpecialRuleAfterDecision, rules, ctx); ok {
		return match, true
	}
	return applyDefaultFallbackThreshold(ctx)
}

func (e Engine) applyPhase(
	phase modeltypology.SpecialRulePhase,
	rules []modeltypology.SpecialRuleSpec,
	ctx EvaluationContext,
) (MatchResult, bool) {
	if ctx.Payload == nil {
		return MatchResult{}, false
	}
	strategies := e.strategies
	if strategies.Len() == 0 {
		strategies = DefaultStrategyRegistry()
	}
	for _, rule := range rules {
		if !ruleMatchesPhase(rule, phase) {
			continue
		}
		strategy, ok := strategies.Resolve(rule.ResolvedKind())
		if !ok {
			continue
		}
		if match, ok := strategy(rule, ctx); ok {
			return match, true
		}
	}
	return MatchResult{}, false
}

func (r StrategyRegistry) Resolve(kind modeltypology.SpecialRuleKind) (strategyFunc, bool) {
	strategy, ok := r.strategies[kind]
	return strategy, ok
}

func (r StrategyRegistry) Len() int {
	return len(r.strategies)
}

func ruleMatchesPhase(rule modeltypology.SpecialRuleSpec, phase modeltypology.SpecialRulePhase) bool {
	if rule.Phase != "" {
		return rule.Phase == phase
	}
	switch rule.ResolvedKind() {
	case modeltypology.SpecialRuleKindAnswerMatch:
		return phase == modeltypology.SpecialRuleBeforeScore
	case modeltypology.SpecialRuleKindFallbackThreshold:
		return phase == modeltypology.SpecialRuleAfterDecision
	default:
		return false
	}
}

func applyAnswerMatchRule(rule modeltypology.SpecialRuleSpec, ctx EvaluationContext) (MatchResult, bool) {
	if !matchesAnswerTrigger(rule, ctx.Answers) {
		return MatchResult{}, false
	}
	outcome, ok := ctx.Payload.FindOutcome(firstNonEmpty(rule.OutcomeCode, rule.Code))
	if !ok {
		return MatchResult{}, false
	}
	return MatchResult{
		OutcomeCode: outcome.Code,
		Trigger:     outcome.Trigger,
		SkipScoring: true,
	}, true
}

func applyFallbackThresholdRule(rule modeltypology.SpecialRuleSpec, ctx EvaluationContext) (MatchResult, bool) {
	if !fallbackThresholdExceeded(ctx) {
		return MatchResult{}, false
	}
	fallbackCode := ctx.Decision.FallbackCode
	code := firstNonEmpty(rule.OutcomeCode, rule.Code)
	if code != fallbackCode && rule.OutcomeCode != "" {
		return MatchResult{}, false
	}
	outcome, ok := ctx.Payload.FindOutcome(fallbackCode)
	if !ok {
		return MatchResult{}, false
	}
	return MatchResult{
		OutcomeCode:    outcome.Code,
		Trigger:        outcome.Trigger,
		ReplaceOutcome: true,
	}, true
}

func applyDefaultFallbackThreshold(ctx EvaluationContext) (MatchResult, bool) {
	if !fallbackThresholdExceeded(ctx) {
		return MatchResult{}, false
	}
	outcome, ok := ctx.Payload.FindOutcome(ctx.Decision.FallbackCode)
	if !ok {
		return MatchResult{}, false
	}
	return MatchResult{
		OutcomeCode:    outcome.Code,
		Trigger:        outcome.Trigger,
		ReplaceOutcome: true,
	}, true
}

func fallbackThresholdExceeded(ctx EvaluationContext) bool {
	return ctx.Payload != nil &&
		ctx.Decision.FallbackSimilarityThreshold > 0 &&
		ctx.Similarity < ctx.Decision.FallbackSimilarityThreshold &&
		ctx.Decision.FallbackCode != ""
}

func matchesAnswerTrigger(rule modeltypology.SpecialRuleSpec, answers []classification.Answer) bool {
	questionCodes := rule.ResolvedQuestionCodes()
	optionValues := rule.ResolvedOptionValues()
	if len(questionCodes) == 0 || len(optionValues) == 0 {
		return false
	}
	questions := stringSet(questionCodes)
	values := stringSet(optionValues)
	for _, answer := range answers {
		if !questions[answer.QuestionCode] {
			continue
		}
		if values[answerValueKey(answer.Value)] {
			return true
		}
	}
	return false
}

func answerValueKey(raw any) string {
	switch value := raw.(type) {
	case []string:
		if len(value) == 0 {
			return ""
		}
		return answerValueKey(value[0])
	case []any:
		if len(value) == 0 {
			return ""
		}
		return answerValueKey(value[0])
	default:
		if option, ok := answervalue.NormalizeSingleOption(raw); ok {
			return option
		}
		if raw == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(raw))
	}
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values)*2)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		set[trimmed] = true
		set[strings.ToUpper(trimmed)] = true
	}
	return set
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
