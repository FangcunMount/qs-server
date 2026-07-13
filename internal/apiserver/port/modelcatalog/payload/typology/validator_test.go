package typology_test

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestExplicitQuestionScoreContributionDoesNotRequireOptionScores(t *testing.T) {
	spec := validRuntimeSpec()
	factorSpec := spec.FactorGraph.Factors["EI"]
	factorSpec.Contributions = []typology.FactorContributionSpec{{
		QuestionCode: "q1", ScoringMode: typology.QuestionScoringModeQuestionScore, Sign: -1, Weight: 0.5,
	}}
	spec.FactorGraph.Factors["EI"] = factorSpec
	issues := typology.ValidateRuntimeSpecForPublish(spec, validQuestionnaire())
	if modelcatalog.HasValidationErrors(issues) {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestOptionOverrideRequiresExactOptionCoverage(t *testing.T) {
	spec := validRuntimeSpec()
	factorSpec := spec.FactorGraph.Factors["EI"]
	factorSpec.Contributions[0].OptionScores = map[string]float64{"A": 1, "X": 2}
	spec.FactorGraph.Factors["EI"] = factorSpec
	issues := typology.ValidateRuntimeSpecForPublish(spec, validQuestionnaire())
	for _, code := range []string{"option_scores.missing_option", "option_scores.unknown_option"} {
		if !hasIssueCode(issues, code) {
			t.Fatalf("issues = %#v, want %s", issues, code)
		}
	}
}

func TestLegacyContributionProducesNonBlockingWarning(t *testing.T) {
	spec := validRuntimeSpec()
	factorSpec := spec.FactorGraph.Factors["EI"]
	factorSpec.Contributions[0].ScoringMode = ""
	factorSpec.Contributions[0].Weight = 0
	spec.FactorGraph.Factors["EI"] = factorSpec
	issues := typology.ValidateRuntimeSpecForPublish(spec, validQuestionnaire())
	if !hasIssueCode(issues, "question_contribution.legacy_implicit") || modelcatalog.HasValidationErrors(issues) {
		t.Fatalf("issues = %#v, want non-blocking legacy warning", issues)
	}
}

func TestExplicitContributionJSONAppliesOmittedDefaultsAndPreservesExplicitZero(t *testing.T) {
	var omitted typology.FactorContributionSpec
	if err := json.Unmarshal([]byte(`{"question_code":"q1","scoring_mode":"question_score"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.Sign != 1 || omitted.Weight != 1 {
		t.Fatalf("omitted defaults = sign %v weight %v", omitted.Sign, omitted.Weight)
	}
	var explicitZero typology.FactorContributionSpec
	if err := json.Unmarshal([]byte(`{"question_code":"q1","scoring_mode":"question_score","sign":0,"weight":0}`), &explicitZero); err != nil {
		t.Fatal(err)
	}
	if explicitZero.Sign != 0 || explicitZero.Weight != 0 {
		t.Fatalf("explicit zero must be preserved: %#v", explicitZero)
	}
}

func TestValidateRuntimeSpecForPublishRequiresExplicitFactorGraph(t *testing.T) {
	spec := &typology.RuntimeSpec{
		FactorGraph: typology.FactorGraphSpec{
			DimensionOrder: []string{"EI"},
			Dimensions: map[string]typology.Dimension{
				"EI": {Code: "EI", Name: "EI"},
			},
		},
		Decision:       typology.PersonalityDecisionSpec{Kind: modelcatalog.DecisionKindPoleComposition},
		OutcomeMapping: typology.OutcomeMappingSpec{DetailKind: typology.OutcomeDetailPersonalityType},
		Report:         typology.ReportSpec{Kind: typology.ReportKindPersonalityType, AdapterKey: typology.ReportAdapterPersonalityType},
	}

	issues := typology.ValidateRuntimeSpecForPublish(spec, typology.QuestionnaireSnapshot{})
	if !hasIssueCode(issues, "factor_graph.explicit.required") {
		t.Fatalf("issues = %#v, want factor_graph.explicit.required", issues)
	}
}

func TestValidateRuntimeSpecForPublishValidatesQuestionAndOptionRefs(t *testing.T) {
	spec := &typology.RuntimeSpec{
		FactorGraph: typology.FactorGraphSpec{
			Factors: map[string]typology.FactorSpec{
				"EI": {
					ID:   "EI",
					Code: "EI",
					Kind: typology.FactorSpecKindLeaf,
					Contributions: []typology.FactorContributionSpec{{
						QuestionCode: "q1",
						OptionScores: map[string]float64{"missing": 1},
					}},
				},
			},
			Roots: []string{"EI"},
		},
		Decision:       typology.PersonalityDecisionSpec{Kind: modelcatalog.DecisionKindPoleComposition},
		OutcomeMapping: typology.OutcomeMappingSpec{DetailKind: typology.OutcomeDetailPersonalityType},
		Report:         typology.ReportSpec{Kind: typology.ReportKindPersonalityType, AdapterKey: typology.ReportAdapterPersonalityType},
	}
	questionnaire := typology.QuestionnaireSnapshot{
		Questions: []typology.QuestionSnapshot{{Code: "q1", OptionCodes: []string{"A"}}},
	}

	issues := typology.ValidateRuntimeSpecForPublish(spec, questionnaire)
	if !hasIssueCode(issues, "question_mapping.option_not_found") {
		t.Fatalf("issues = %#v, want question_mapping.option_not_found", issues)
	}
}

func TestValidateRuntimeSpecForPublishValidatesOutcomeDefinitions(t *testing.T) {
	spec := validRuntimeSpec()

	issues := typology.ValidateRuntimeSpecForPublishWithContext(spec, validQuestionnaire(), typology.RuntimeSpecValidationContext{
		Algorithm: modelcatalog.AlgorithmMBTI,
		Outcomes: []typology.Outcome{
			{Code: "INTJ", Name: "建筑师"},
			{Code: "INTJ", Name: "重复建筑师"},
			{Code: "ENTP"},
		},
	})

	if !hasIssueCode(issues, "outcome.code.duplicated") {
		t.Fatalf("issues = %#v, want outcome.code.duplicated", issues)
	}
	if !hasIssueCode(issues, "outcome.title.required") {
		t.Fatalf("issues = %#v, want outcome.title.required", issues)
	}
}

func TestValidateRuntimeSpecForPublishValidatesFallbackAndSpecialOutcomeRefs(t *testing.T) {
	spec := validRuntimeSpec()
	spec.Decision.FallbackCode = "MISSING_FALLBACK"
	spec.SpecialRules = []typology.SpecialRuleSpec{{
		Code:        "SPECIAL_MISSING",
		Kind:        typology.SpecialRuleKindAnswerMatch,
		Phase:       typology.SpecialRuleBeforeScore,
		OutcomeCode: "SPECIAL_MISSING",
		Condition: typology.SpecialRuleCondition{
			QuestionCodes: []string{"q1"},
			OptionValues:  []string{"Z"},
		},
	}}

	issues := typology.ValidateRuntimeSpecForPublishWithContext(spec, validQuestionnaire(), typology.RuntimeSpecValidationContext{
		Algorithm: modelcatalog.AlgorithmMBTI,
		Outcomes:  []typology.Outcome{{Code: "INTJ", Name: "建筑师"}},
	})

	for _, code := range []string{
		"decision.fallback_code.not_found",
		"special_rule.outcome.not_found",
		"question_mapping.option_not_found",
	} {
		if !hasIssueCode(issues, code) {
			t.Fatalf("issues = %#v, want %s", issues, code)
		}
	}
}

func TestValidateRuntimeSpecForPublishValidatesDecisionAndLevelRule(t *testing.T) {
	spec := validRuntimeSpec()
	spec.Decision.Kind = modelcatalog.DecisionKindNearestPattern
	spec.Decision.LevelRule = &typology.LevelRuleSpec{LowMax: 5, HighMin: 3}

	issues := typology.ValidateRuntimeSpecForPublishWithContext(spec, validQuestionnaire(), typology.RuntimeSpecValidationContext{
		Algorithm: modelcatalog.AlgorithmMBTI,
		Outcomes:  []typology.Outcome{{Code: "INTJ", Name: "建筑师"}},
	})

	if !hasIssueCode(issues, "decision.level_rule.invalid") {
		t.Fatalf("issues = %#v, want decision.level_rule.invalid", issues)
	}
}

func TestValidateRuntimeSpecForPublishRequiresDecisionKind(t *testing.T) {
	spec := validRuntimeSpec()
	spec.Decision.Kind = ""

	issues := typology.ValidateRuntimeSpecForPublishWithContext(spec, validQuestionnaire(), typology.RuntimeSpecValidationContext{
		Algorithm: modelcatalog.AlgorithmMBTI,
		Outcomes:  []typology.Outcome{{Code: "INTJ", Name: "建筑师"}},
	})

	if !hasIssueCode(issues, "decision.kind.required") {
		t.Fatalf("issues = %#v, want decision.kind.required", issues)
	}
}

func TestValidateRuntimeSpecForPublishValidatesDominantFactorTopKAndOutcomes(t *testing.T) {
	spec := validRuntimeSpec()
	spec.Decision = typology.PersonalityDecisionSpec{Kind: modelcatalog.DecisionKindDominantFactor, TopK: 2}

	issues := typology.ValidateRuntimeSpecForPublishWithContext(spec, validQuestionnaire(), typology.RuntimeSpecValidationContext{
		Algorithm: modelcatalog.AlgorithmPersonalityTypology,
		Outcomes:  []typology.Outcome{{Code: "OTHER", Name: "Other"}},
	})
	if !hasIssueCode(issues, "decision.top_k.invalid") || !hasIssueCode(issues, "decision.dominant_factor.outcome.required") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateRuntimeSpecForPublishRejectsLegacyAdapterKeys(t *testing.T) {
	spec := validRuntimeSpec()
	spec.OutcomeMapping.DetailAdapterKey = typology.DetailAdapterMBTI
	spec.Report.AdapterKey = typology.ReportAdapterMBTI

	issues := typology.ValidateRuntimeSpecForPublishWithContext(spec, validQuestionnaire(), typology.RuntimeSpecValidationContext{
		Algorithm: modelcatalog.AlgorithmMBTI,
		Outcomes:  []typology.Outcome{{Code: "INTJ", Name: "建筑师"}},
	})

	if !hasIssueCode(issues, "outcome_mapping.detail_adapter.deprecated") {
		t.Fatalf("issues = %#v, want outcome_mapping.detail_adapter.deprecated", issues)
	}
	if !hasIssueCode(issues, "report.adapter.deprecated") {
		t.Fatalf("issues = %#v, want report.adapter.deprecated", issues)
	}
}

func TestValidateRuntimeSpecForPublishValidatesAdapterCompatibility(t *testing.T) {
	spec := validRuntimeSpec()
	spec.OutcomeMapping.DetailAdapterKey = typology.DetailAdapterTraitProfile
	spec.Report.AdapterKey = typology.ReportAdapterTraitProfile

	issues := typology.ValidateRuntimeSpecForPublishWithContext(spec, validQuestionnaire(), typology.RuntimeSpecValidationContext{
		Algorithm: modelcatalog.AlgorithmMBTI,
		Outcomes:  []typology.Outcome{{Code: "INTJ", Name: "建筑师"}},
	})

	if !hasIssueCode(issues, "outcome_mapping.detail_adapter.incompatible") {
		t.Fatalf("issues = %#v, want outcome_mapping.detail_adapter.incompatible", issues)
	}
	if !hasIssueCode(issues, "report.adapter.incompatible") {
		t.Fatalf("issues = %#v, want report.adapter.incompatible", issues)
	}
}

func validRuntimeSpec() *typology.RuntimeSpec {
	return &typology.RuntimeSpec{
		FactorGraph: typology.FactorGraphSpec{
			Dimensions: map[string]typology.Dimension{"EI": {Code: "EI", Name: "EI", LeftPole: "I", RightPole: "E"}},
			Factors: map[string]typology.FactorSpec{
				"EI": {
					ID:   "EI",
					Code: "EI",
					Name: "EI",
					Kind: typology.FactorSpecKindLeaf,
					Contributions: []typology.FactorContributionSpec{{
						QuestionCode: "q1",
						ScoringMode:  typology.QuestionScoringModeOptionOverride,
						Sign:         1,
						Weight:       1,
						OptionScores: map[string]float64{"A": 1, "B": -1},
					}},
				},
			},
			Roots: []string{"EI"},
		},
		Decision: typology.PersonalityDecisionSpec{Kind: modelcatalog.DecisionKindPoleComposition},
		OutcomeMapping: typology.OutcomeMappingSpec{
			DetailKind:       typology.OutcomeDetailPersonalityType,
			DetailAdapterKey: typology.DetailAdapterPersonalityType,
		},
		Report: typology.ReportSpec{
			Kind:       typology.ReportKindPersonalityType,
			AdapterKey: typology.ReportAdapterPersonalityType,
		},
	}
}

func validQuestionnaire() typology.QuestionnaireSnapshot {
	return typology.QuestionnaireSnapshot{
		Questions: []typology.QuestionSnapshot{{Code: "q1", Type: "Radio", OptionCodes: []string{"A", "B"}}},
	}
}

func hasIssueCode(issues []modelcatalog.DomainValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
