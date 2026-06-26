package specialrule

import (
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// MatchResult is returned when a special rule fires.
type MatchResult struct {
	OutcomeCode    string
	Trigger        string
	SkipScoring    bool
	ReplaceOutcome bool
}

// Engine evaluates configurable special rules against answers and scoring outcomes.
type Engine struct{}

// ApplyBeforeScore checks answer_match rules and returns a match when answers trigger a special outcome.
func (Engine) ApplyBeforeScore(
	rules []modeltypology.SpecialRuleSpec,
	payload *modeltypology.Payload,
	answers []evaluationinput.Answer,
) (MatchResult, bool) {
	if payload == nil {
		return MatchResult{}, false
	}
	for _, rule := range rules {
		if rule.ResolvedKind() != modeltypology.SpecialRuleKindAnswerMatch {
			continue
		}
		if !matchesAnswerTrigger(rule, answers) {
			continue
		}
		outcome, ok := payload.FindOutcome(firstNonEmpty(rule.OutcomeCode, rule.Code))
		if !ok {
			continue
		}
		return MatchResult{
			OutcomeCode: outcome.Code,
			Trigger:     outcome.Trigger,
			SkipScoring: true,
		}, true
	}
	return MatchResult{}, false
}

// ApplyAfterDecision checks fallback_threshold rules when similarity falls below the configured threshold.
func (Engine) ApplyAfterDecision(
	rules []modeltypology.SpecialRuleSpec,
	decision modeltypology.PersonalityDecisionSpec,
	payload *modeltypology.Payload,
	similarity float64,
) (MatchResult, bool) {
	if payload == nil || decision.FallbackSimilarityThreshold <= 0 || similarity >= decision.FallbackSimilarityThreshold {
		return MatchResult{}, false
	}
	fallbackCode := decision.FallbackCode
	if fallbackCode == "" {
		return MatchResult{}, false
	}
	for _, rule := range rules {
		if rule.ResolvedKind() != modeltypology.SpecialRuleKindFallbackThreshold {
			continue
		}
		code := firstNonEmpty(rule.OutcomeCode, rule.Code)
		if code != fallbackCode && rule.OutcomeCode != "" {
			continue
		}
		outcome, ok := payload.FindOutcome(fallbackCode)
		if !ok {
			continue
		}
		return MatchResult{
			OutcomeCode:    outcome.Code,
			Trigger:        outcome.Trigger,
			ReplaceOutcome: true,
		}, true
	}
	outcome, ok := payload.FindOutcome(fallbackCode)
	if !ok {
		return MatchResult{}, false
	}
	return MatchResult{
		OutcomeCode:    outcome.Code,
		Trigger:        outcome.Trigger,
		ReplaceOutcome: true,
	}, true
}

func matchesAnswerTrigger(rule modeltypology.SpecialRuleSpec, answers []evaluationinput.Answer) bool {
	questionCodes := rule.ResolvedQuestionCodes()
	optionValues := rule.ResolvedOptionValues()
	if len(questionCodes) == 0 || len(optionValues) == 0 {
		return false
	}
	questions := evaluationinput.StringSet(questionCodes)
	values := evaluationinput.StringSet(optionValues)
	for _, answer := range answers {
		if !questions[answer.QuestionCode] {
			continue
		}
		if values[evaluationinput.AnswerValueKey(answer.Value)] {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
