package configured

import (
	"fmt"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/specialrule"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// Evaluator scores typology payloads through the configured runtime pipeline.
type Evaluator struct {
	rules   specialrule.Engine
	details DetailAssemblerRegistry
}

// NewEvaluator returns a configured personality evaluator.
func NewEvaluator() Evaluator {
	return Evaluator{
		rules:   specialrule.Engine{},
		details: DefaultDetailAssemblerRegistry(),
	}
}

// Score evaluates a typology payload and returns a scoring result.
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
		return e.assembleResult(payload, spec, profile.ProfileVector{}, profile.DecisionSpec{}, profile.OutcomeCandidate{}, SelectedOutcome{
			Code:       match.OutcomeCode,
			Similarity: 1,
			Trigger:    match.Trigger,
		}, &evaluationtypology.ScoringSpecialMatch{
			OutcomeCode: match.OutcomeCode,
			Trigger:     match.Trigger,
			SkipScoring: match.SkipScoring,
		}, modeltypology.DetailAdapterSBTI)
	}

	graph, decision, err := buildGraphAndDecision(payload, spec)
	if err != nil {
		return evaluationtypology.ScoringResult{}, err
	}
	vector, err := profile.ScoreGraph(graph, sheet)
	if err != nil {
		return evaluationtypology.ScoringResult{}, err
	}
	candidate, err := profile.SelectOutcome(vector, decision)
	if err != nil {
		return evaluationtypology.ScoringResult{}, err
	}

	selected := SelectedOutcome{
		Code:       candidate.Code,
		Similarity: candidate.MatchScore,
	}
	var specialMatch *evaluationtypology.ScoringSpecialMatch
	if adapterKey == modeltypology.DetailAdapterSBTI {
		selected.Dimensions = buildSBTIDimensionLevels(spec, vector, decision.LevelRule)
		if match, ok := e.rules.ApplyAfterDecision(spec.SpecialRules, spec.Decision, payload, candidate.MatchScore); ok {
			selected.Code = match.OutcomeCode
			selected.Trigger = match.Trigger
			specialMatch = &evaluationtypology.ScoringSpecialMatch{
				OutcomeCode: match.OutcomeCode,
				Trigger:     match.Trigger,
			}
		}
	}

	return e.assembleResult(payload, spec, vector, decision, candidate, selected, specialMatch, adapterKey)
}

func (e Evaluator) assembleResult(
	payload *modeltypology.Payload,
	spec *modeltypology.RuntimeSpec,
	vector profile.ProfileVector,
	decision profile.DecisionSpec,
	candidate profile.OutcomeCandidate,
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

func buildSBTIDimensionLevels(
	spec *modeltypology.RuntimeSpec,
	vector profile.ProfileVector,
	rule profile.LevelRule,
) []DimensionLevel {
	input := DetailInput{
		Spec:     spec,
		Vector:   vector,
		Decision: profile.DecisionSpec{LevelRule: rule},
	}
	return buildSBTIDimensions(input)
}
