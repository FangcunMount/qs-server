package typology_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

func TestValidateRuntimeSpecForPublishRequiresExplicitFactorGraph(t *testing.T) {
	spec := &typology.RuntimeSpec{
		FactorGraph: typology.FactorGraphSpec{
			DimensionOrder: []string{"EI"},
			Dimensions: map[string]typology.Dimension{
				"EI": {Code: "EI", Name: "EI"},
			},
		},
		Decision:       typology.PersonalityDecisionSpec{Kind: assessmentmodel.DecisionKindPoleComposition},
		OutcomeMapping: typology.OutcomeMappingSpec{DetailKind: typology.OutcomeDetailPersonalityType},
		Report:         typology.ReportSpec{Kind: typology.ReportKindTemplate, AdapterKey: "mbti_default"},
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
		Decision:       typology.PersonalityDecisionSpec{Kind: assessmentmodel.DecisionKindPoleComposition},
		OutcomeMapping: typology.OutcomeMappingSpec{DetailKind: typology.OutcomeDetailPersonalityType},
		Report:         typology.ReportSpec{Kind: typology.ReportKindTemplate, AdapterKey: "mbti_default"},
	}
	questionnaire := typology.QuestionnaireSnapshot{
		Questions: []typology.QuestionSnapshot{{Code: "q1", OptionCodes: []string{"A"}}},
	}

	issues := typology.ValidateRuntimeSpecForPublish(spec, questionnaire)
	if !hasIssueCode(issues, "question_mapping.option_not_found") {
		t.Fatalf("issues = %#v, want question_mapping.option_not_found", issues)
	}
}

func hasIssueCode(issues []assessmentmodel.DomainValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
