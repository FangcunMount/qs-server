package typology_test

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestResolveTypologyReportRouting_RejectsMissingDefinitionV2Runtime(t *testing.T) {
	t.Parallel()

	if _, err := typology.ResolveTypologyReportRouting(nil); !errors.Is(err, typology.ErrRuntimeSpecInvalid) {
		t.Fatalf("err = %v, want ErrRuntimeSpecInvalid", err)
	}
}

func TestResolveTypologyReportRouting_ExplicitValid(t *testing.T) {
	t.Parallel()

	payload := &typology.Payload{
		Runtime: &typology.RuntimeSpec{
			FactorGraph: typology.FactorGraphSpec{
				Factors: map[string]typology.FactorSpec{
					"EI": {ID: "EI", Code: "EI", Name: "EI", Kind: typology.FactorSpecKindLeaf, Contributions: []typology.FactorContributionSpec{{QuestionCode: "q1", ScoringMode: typology.QuestionScoringModeQuestionScore, Sign: 1, Weight: 1}}},
				},
				Roots:      []string{"EI"},
				Dimensions: map[string]typology.Dimension{"EI": {Code: "EI", Name: "EI", LeftPole: "I", RightPole: "E"}},
			},
			Decision:       typology.PersonalityDecisionSpec{Kind: binding.DecisionKindPoleComposition},
			OutcomeMapping: typology.OutcomeMappingSpec{DetailKind: typology.OutcomeDetailPersonalityType},
			Report: typology.ReportSpec{
				Kind:       typology.ReportKindTemplate,
				AdapterKey: typology.ReportAdapterPersonalityType,
				TemplateID: "mbti",
			},
		},
	}

	routing, err := typology.ResolveTypologyReportRouting(payload)
	if err != nil {
		t.Fatalf("ResolveTypologyReportRouting() err = %v", err)
	}
	if routing.Source != typology.ReportRoutingDefinitionV2 {
		t.Fatalf("Source = %s, want definition_v2", routing.Source)
	}
	if routing.TemplateID != "mbti" {
		t.Fatalf("TemplateID = %q, want mbti", routing.TemplateID)
	}
	if routing.AdapterKey != typology.ReportAdapterPersonalityType {
		t.Fatalf("AdapterKey = %s, want personality_type", routing.AdapterKey)
	}
	if routing.DecisionKind != binding.DecisionKindPoleComposition {
		t.Fatalf("DecisionKind = %s, want pole_composition", routing.DecisionKind)
	}
}

func TestResolveTypologyReportRouting_ExplicitMalformed(t *testing.T) {
	t.Parallel()

	payload := &typology.Payload{
		Runtime: &typology.RuntimeSpec{
			// Missing required decision/factor graph → ToRuntimeSpec fails.
			Report: typology.ReportSpec{Kind: typology.ReportKindTemplate},
		},
	}

	routing, err := typology.ResolveTypologyReportRouting(payload)
	if !errors.Is(err, typology.ErrRuntimeSpecInvalid) {
		t.Fatalf("err = %v, want ErrRuntimeSpecInvalid", err)
	}
	if routing.Source != "" {
		t.Fatalf("malformed explicit runtime must not return a routing source, got %s", routing.Source)
	}
}

func TestIsRegisteredReportTemplateID(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"mbti", "sbti", "bigfive"} {
		if !typology.IsRegisteredReportTemplateID(id) {
			t.Fatalf("IsRegisteredReportTemplateID(%q) = false", id)
		}
	}
	if typology.IsRegisteredReportTemplateID("") {
		t.Fatal("empty TemplateID must not be registered")
	}
	if typology.IsRegisteredReportTemplateID("unknown-template") {
		t.Fatal("unknown TemplateID must not be registered")
	}
}
