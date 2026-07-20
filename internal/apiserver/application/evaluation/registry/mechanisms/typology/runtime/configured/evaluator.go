package configured

import (
	"fmt"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	calcclassification "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	calcspecialrule "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification/specialrule"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// Evaluator 计算类型学载荷 通过 配置化运行时 pipeline。
type Evaluator struct {
	rules   calcspecialrule.Engine
	details DetailAssemblerRegistry
}

// NewEvaluator 返回配置化人格评估器 使用 内置 明细组装器。
func NewEvaluator() Evaluator {
	return NewEvaluatorWithDetails(DefaultDetailAssemblerRegistry())
}

// NewEvaluatorWithDetails 返回配置化 evaluator that resolves 明细组装 通过 注册表。
func NewEvaluatorWithDetails(details DetailAssemblerRegistry) Evaluator {
	return Evaluator{
		rules:   calcspecialrule.Engine{},
		details: details,
	}
}

// Score 评估类型学载荷 和 returns 计分结果。
func (e Evaluator) Score(payload *modeltypology.Payload, sheet *evalinput.AnswerSheet) (outcometypology.ScoringResult, error) {
	return e.ScoreWithDefinition(payload, nil, sheet)
}

// ScoreWithDefinition prefers canonical Definition runtime spec over compat payload (MC-R017 batch 5).
func (e Evaluator) ScoreWithDefinition(
	payload *modeltypology.Payload,
	def *modeldefinition.Definition,
	sheet *evalinput.AnswerSheet,
) (outcometypology.ScoringResult, error) {
	if payload == nil {
		return outcometypology.ScoringResult{}, fmt.Errorf("typology payload is required")
	}
	if sheet == nil {
		return outcometypology.ScoringResult{}, fmt.Errorf("answer sheet is required")
	}
	spec, err := modeltypology.ResolveRuntimeSpec(def, payload)
	if err != nil {
		return outcometypology.ScoringResult{}, err
	}
	adapterKey := spec.OutcomeMapping.ResolvedDetailAdapterKey(spec.Decision.Kind)
	specialRules := specialRulesForCalculation(spec.SpecialRules)
	specialOutcomes := specialRuleOutcomes(payload)

	if match, ok := e.rules.ApplyBeforeScore(specialRules, specialOutcomes, classificationAnswers(sheet)); ok {
		return e.assembleResult(payload, spec, calcclassification.ProfileVector{}, calcclassification.DecisionSpec{}, calcclassification.OutcomeCandidate{}, SelectedOutcome{
			Code:       match.OutcomeCode,
			Similarity: 1,
			Trigger:    match.Trigger,
		}, &outcometypology.ScoringSpecialMatch{
			OutcomeCode: match.OutcomeCode,
			Trigger:     match.Trigger,
			SkipScoring: match.SkipScoring,
		}, adapterKey)
	}

	graph, decision, err := buildGraphAndDecision(payload, spec)
	if err != nil {
		return outcometypology.ScoringResult{}, err
	}
	vector, err := calcclassification.ScoreGraph(graph, classificationAnswerSheet(sheet))
	if err != nil {
		return outcometypology.ScoringResult{}, err
	}
	candidate, err := calcclassification.SelectOutcome(vector, decision)
	if err != nil {
		return outcometypology.ScoringResult{}, err
	}

	selected := SelectedOutcome{
		Code:       candidate.Code,
		Similarity: candidate.MatchScore,
	}
	var specialMatch *outcometypology.ScoringSpecialMatch
	if match, ok := e.rules.ApplyAfterDecision(specialRules, specialRuleDecision(spec.Decision), specialOutcomes, candidate.MatchScore); ok {
		selected.Code = match.OutcomeCode
		selected.Trigger = match.Trigger
		specialMatch = &outcometypology.ScoringSpecialMatch{
			OutcomeCode: match.OutcomeCode,
			Trigger:     match.Trigger,
		}
	}

	return e.assembleResult(payload, spec, vector, decision, candidate, selected, specialMatch, adapterKey)
}

func specialRulesForCalculation(rules []modeltypology.SpecialRuleSpec) []calcspecialrule.Rule {
	if len(rules) == 0 {
		return nil
	}
	converted := make([]calcspecialrule.Rule, 0, len(rules))
	for _, rule := range rules {
		converted = append(converted, calcspecialrule.Rule{
			Code:        rule.Code,
			Kind:        calcspecialrule.RuleKind(rule.Kind),
			Phase:       calcspecialrule.RulePhase(rule.Phase),
			OutcomeCode: rule.OutcomeCode,
			Condition: calcspecialrule.Condition{
				QuestionCodes: append([]string(nil), rule.Condition.QuestionCodes...),
				OptionValues:  append([]string(nil), rule.Condition.OptionValues...),
			},
			QuestionCodes: append([]string(nil), rule.QuestionCodes...),
			OptionValues:  append([]string(nil), rule.OptionValues...),
		})
	}
	return converted
}

func specialRuleDecision(decision modeltypology.PersonalityDecisionSpec) calcspecialrule.Decision {
	return calcspecialrule.Decision{
		FallbackSimilarityThreshold: decision.FallbackSimilarityThreshold,
		FallbackCode:                decision.FallbackCode,
	}
}

func specialRuleOutcomes(payload *modeltypology.Payload) []calcspecialrule.Outcome {
	if payload == nil || len(payload.Outcomes) == 0 {
		return nil
	}
	outcomes := make([]calcspecialrule.Outcome, 0, len(payload.Outcomes))
	for _, outcome := range payload.Outcomes {
		outcomes = append(outcomes, calcspecialrule.Outcome{
			Code:    outcome.Code,
			Trigger: outcome.Trigger,
		})
	}
	return outcomes
}

func (e Evaluator) assembleResult(
	payload *modeltypology.Payload,
	spec *modeltypology.RuntimeSpec,
	vector calcclassification.ProfileVector,
	decision calcclassification.DecisionSpec,
	candidate calcclassification.OutcomeCandidate,
	selected SelectedOutcome,
	specialMatch *outcometypology.ScoringSpecialMatch,
	adapterKey modeltypology.DetailAdapterKey,
) (outcometypology.ScoringResult, error) {
	detail, err := e.details.Assemble(DetailInput{
		Payload:   payload,
		Spec:      spec,
		Vector:    vector,
		Decision:  decision,
		Candidate: candidate,
		Selected:  selected,
		Adapter:   adapterKey,
	})
	if err != nil {
		return outcometypology.ScoringResult{}, err
	}
	return outcometypology.ScoringResult{
		Runtime:         spec,
		Vector:          vector,
		Candidate:       candidate,
		SelectedOutcome: outcometypology.SelectedOutcome{Code: selected.Code, Similarity: selected.Similarity, Trigger: selected.Trigger},
		SpecialMatch:    specialMatch,
		Detail:          detail,
	}, nil
}
