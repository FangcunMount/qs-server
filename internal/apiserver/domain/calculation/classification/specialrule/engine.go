package specialrule

import (
	"fmt"
	"strings"

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

// RulePhase 选择when special rule 是 evaluated。
type RulePhase string

const (
	RuleBeforeScore    RulePhase = "before_score"
	RuleBeforeDecision RulePhase = "before_decision"
	RuleAfterDecision  RulePhase = "after_decision"
)

// RuleKind 选择评估 strategy 用于 special rule。
type RuleKind string

const (
	RuleKindAnswerMatch       RuleKind = "answer_match"
	RuleKindFallbackThreshold RuleKind = "fallback_threshold"
)

// Rule 描述配置urable special 结果 rule。
type Rule struct {
	Code          string
	Kind          RuleKind
	Phase         RulePhase
	OutcomeCode   string
	Condition     Condition
	QuestionCodes []string
	OptionValues  []string
}

// Condition 携带类型-特定 match parameters。
type Condition struct {
	QuestionCodes []string
	OptionValues  []string
}

// ResolvedKind 返回配置化 类型, deriving 从 旧版 字段 when needed。
func (r Rule) ResolvedKind() RuleKind {
	if r.Kind != "" {
		return r.Kind
	}
	if len(r.ResolvedQuestionCodes()) > 0 {
		return RuleKindAnswerMatch
	}
	if r.Phase == RuleAfterDecision {
		return RuleKindFallbackThreshold
	}
	return ""
}

// ResolvedQuestionCodes 返回question 编码 从 condition 或 旧版 flat 字段。
func (r Rule) ResolvedQuestionCodes() []string {
	if len(r.Condition.QuestionCodes) > 0 {
		return append([]string(nil), r.Condition.QuestionCodes...)
	}
	return append([]string(nil), r.QuestionCodes...)
}

// ResolvedOptionValues 返回选项 values 从 condition 或 旧版 flat 字段。
func (r Rule) ResolvedOptionValues() []string {
	if len(r.Condition.OptionValues) > 0 {
		return append([]string(nil), r.Condition.OptionValues...)
	}
	return append([]string(nil), r.OptionValues...)
}

// Decision contains the small part of typology decision state used by special rules.
type Decision struct {
	FallbackSimilarityThreshold float64
	FallbackCode                string
}

// Outcome contains the small part of typology outcome state used by special rules.
type Outcome struct {
	Code    string
	Trigger string
}

// EvaluationContext 携带rule inputs 用于 一个special-rule phase。
type EvaluationContext struct {
	Outcomes   []Outcome
	Answers    []classification.Answer
	Decision   Decision
	Similarity float64
}

type strategyFunc func(Rule, EvaluationContext) (MatchResult, bool)

// StrategyRegistry 解析special-rule strategies 按 rule 类型。
type StrategyRegistry struct {
	strategies map[RuleKind]strategyFunc
}

// DefaultStrategyRegistry 返回内置 special-rule strategies。
func DefaultStrategyRegistry() StrategyRegistry {
	return StrategyRegistry{
		strategies: map[RuleKind]strategyFunc{
			RuleKindAnswerMatch:       applyAnswerMatchRule,
			RuleKindFallbackThreshold: applyFallbackThresholdRule,
		},
	}
}

// Engine 评估配置urable special rules 针对 answers 和 计分结果。
type Engine struct {
	strategies StrategyRegistry
}

// ApplyBeforeScore 检查answer_match rules 和 returns match when answers trigger special 结果。
func (e Engine) ApplyBeforeScore(
	rules []Rule,
	outcomes []Outcome,
	answers []classification.Answer,
) (MatchResult, bool) {
	return e.applyPhase(RuleBeforeScore, rules, EvaluationContext{
		Outcomes: outcomes,
		Answers:  answers,
	})
}

// ApplyAfterDecision 检查fallback_threshold rules when similarity falls below 配置化 threshold。
func (e Engine) ApplyAfterDecision(
	rules []Rule,
	decision Decision,
	outcomes []Outcome,
	similarity float64,
) (MatchResult, bool) {
	ctx := EvaluationContext{
		Outcomes:   outcomes,
		Decision:   decision,
		Similarity: similarity,
	}
	if match, ok := e.applyPhase(RuleAfterDecision, rules, ctx); ok {
		return match, true
	}
	return applyDefaultFallbackThreshold(ctx)
}

func (e Engine) applyPhase(
	phase RulePhase,
	rules []Rule,
	ctx EvaluationContext,
) (MatchResult, bool) {
	if len(ctx.Outcomes) == 0 {
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

func (r StrategyRegistry) Resolve(kind RuleKind) (strategyFunc, bool) {
	strategy, ok := r.strategies[kind]
	return strategy, ok
}

func (r StrategyRegistry) Len() int {
	return len(r.strategies)
}

func ruleMatchesPhase(rule Rule, phase RulePhase) bool {
	if rule.Phase != "" {
		return rule.Phase == phase
	}
	switch rule.ResolvedKind() {
	case RuleKindAnswerMatch:
		return phase == RuleBeforeScore
	case RuleKindFallbackThreshold:
		return phase == RuleAfterDecision
	default:
		return false
	}
}

func applyAnswerMatchRule(rule Rule, ctx EvaluationContext) (MatchResult, bool) {
	if !matchesAnswerTrigger(rule, ctx.Answers) {
		return MatchResult{}, false
	}
	outcome, ok := findOutcome(ctx.Outcomes, firstNonEmpty(rule.OutcomeCode, rule.Code))
	if !ok {
		return MatchResult{}, false
	}
	return MatchResult{
		OutcomeCode: outcome.Code,
		Trigger:     outcome.Trigger,
		SkipScoring: true,
	}, true
}

func applyFallbackThresholdRule(rule Rule, ctx EvaluationContext) (MatchResult, bool) {
	if !fallbackThresholdExceeded(ctx) {
		return MatchResult{}, false
	}
	fallbackCode := ctx.Decision.FallbackCode
	code := firstNonEmpty(rule.OutcomeCode, rule.Code)
	if code != fallbackCode && rule.OutcomeCode != "" {
		return MatchResult{}, false
	}
	outcome, ok := findOutcome(ctx.Outcomes, fallbackCode)
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
	outcome, ok := findOutcome(ctx.Outcomes, ctx.Decision.FallbackCode)
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
	return len(ctx.Outcomes) > 0 &&
		ctx.Decision.FallbackSimilarityThreshold > 0 &&
		ctx.Similarity < ctx.Decision.FallbackSimilarityThreshold &&
		ctx.Decision.FallbackCode != ""
}

func findOutcome(outcomes []Outcome, code string) (Outcome, bool) {
	for _, outcome := range outcomes {
		if outcome.Code == code {
			return outcome, true
		}
	}
	return Outcome{}, false
}

func matchesAnswerTrigger(rule Rule, answers []classification.Answer) bool {
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
