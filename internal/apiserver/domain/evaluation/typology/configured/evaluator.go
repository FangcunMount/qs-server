package configured

import (
	"fmt"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/specialrule"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/trait"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// Evaluator 计算类型学载荷 通过 配置化运行时 pipeline。
type Evaluator struct {
	rules   specialrule.Engine
	details DetailAssemblerRegistry
}

// NewEvaluator 返回配置化人格评估器 使用 内置 明细组装器。
func NewEvaluator() Evaluator {
	return NewEvaluatorWithDetails(DefaultDetailAssemblerRegistry())
}

// NewEvaluatorWithDetails 返回配置化 evaluator that resolves 明细组装 通过 注册表。
func NewEvaluatorWithDetails(details DetailAssemblerRegistry) Evaluator {
	return Evaluator{
		rules:   specialrule.Engine{},
		details: details,
	}
}

// Score 评估类型学载荷 和 returns 计分结果。
func (e Evaluator) Score(payload *modeltypology.Payload, sheet *evaluationinput.AnswerSheet) (evaluationtypology.ScoringResult, error) {
	if payload == nil {
		return evaluationtypology.ScoringResult{}, fmt.Errorf("typology payload is required")
	}
	if sheet == nil {
		return evaluationtypology.ScoringResult{}, fmt.Errorf("answer sheet is required")
	}
	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		return evaluationtypology.ScoringResult{}, err
	}
	adapterKey := spec.OutcomeMapping.ResolvedDetailAdapterKey(spec.Decision.Kind)

	if match, ok := e.rules.ApplyBeforeScore(spec.SpecialRules, payload, sheet.Answers); ok {
		return e.assembleResult(payload, spec, trait.ProfileVector{}, trait.DecisionSpec{}, trait.OutcomeCandidate{}, SelectedOutcome{
			Code:       match.OutcomeCode,
			Similarity: 1,
			Trigger:    match.Trigger,
		}, &evaluationtypology.ScoringSpecialMatch{
			OutcomeCode: match.OutcomeCode,
			Trigger:     match.Trigger,
			SkipScoring: match.SkipScoring,
		}, adapterKey)
	}

	graph, decision, err := buildGraphAndDecision(payload, spec)
	if err != nil {
		return evaluationtypology.ScoringResult{}, err
	}
	vector, err := trait.ScoreGraph(graph, sheet)
	if err != nil {
		return evaluationtypology.ScoringResult{}, err
	}
	candidate, err := trait.SelectOutcome(vector, decision)
	if err != nil {
		return evaluationtypology.ScoringResult{}, err
	}

	selected := SelectedOutcome{
		Code:       candidate.Code,
		Similarity: candidate.MatchScore,
	}
	var specialMatch *evaluationtypology.ScoringSpecialMatch
	if match, ok := e.rules.ApplyAfterDecision(spec.SpecialRules, spec.Decision, payload, candidate.MatchScore); ok {
		selected.Code = match.OutcomeCode
		selected.Trigger = match.Trigger
		specialMatch = &evaluationtypology.ScoringSpecialMatch{
			OutcomeCode: match.OutcomeCode,
			Trigger:     match.Trigger,
		}
	}

	return e.assembleResult(payload, spec, vector, decision, candidate, selected, specialMatch, adapterKey)
}

func (e Evaluator) assembleResult(
	payload *modeltypology.Payload,
	spec *modeltypology.RuntimeSpec,
	vector trait.ProfileVector,
	decision trait.DecisionSpec,
	candidate trait.OutcomeCandidate,
	selected SelectedOutcome,
	specialMatch *evaluationtypology.ScoringSpecialMatch,
	adapterKey modeltypology.DetailAdapterKey,
) (evaluationtypology.ScoringResult, error) {
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
		return evaluationtypology.ScoringResult{}, err
	}
	return evaluationtypology.ScoringResult{
		Runtime:         spec,
		Vector:          vector,
		Candidate:       candidate,
		SelectedOutcome: evaluationtypology.SelectedOutcome{Code: selected.Code, Similarity: selected.Similarity, Trigger: selected.Trigger},
		SpecialMatch:    specialMatch,
		Detail:          detail,
	}, nil
}
