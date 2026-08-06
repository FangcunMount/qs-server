package preview

import (
	"testing"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact/codec"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelpreview"
)

func TestPreviewInterpretationInputUsesExplicitTemplateRoute(t *testing.T) {
	payload := &modeltypology.Payload{Runtime: &modeltypology.RuntimeSpec{
		FactorGraph: modeltypology.FactorGraphSpec{
			Factors: map[string]modeltypology.FactorSpec{
				"EI": {ID: "EI", Code: "EI", Name: "EI", Kind: modeltypology.FactorSpecKindLeaf, Contributions: []modeltypology.FactorContributionSpec{{QuestionCode: "q1", ScoringMode: modeltypology.QuestionScoringModeQuestionScore, Sign: 1, Weight: 1}}},
			},
			Roots:      []string{"EI"},
			Dimensions: map[string]modeltypology.Dimension{"EI": {Code: "EI", Name: "EI", LeftPole: "I", RightPole: "E"}},
		},
		Decision:       modeltypology.PersonalityDecisionSpec{Kind: binding.DecisionKindPoleComposition},
		OutcomeMapping: modeltypology.OutcomeMappingSpec{DetailKind: modeltypology.OutcomeDetailPersonalityType},
		Report: modeltypology.ReportSpec{
			Kind: modeltypology.ReportKindTemplate, AdapterKey: modeltypology.ReportAdapterPersonalityType,
			TemplateID: "mbti", TemplateVersion: "2026-08-v1",
		},
	}}
	req := modelpreview.Request{Input: &evaluationinput.InputSnapshot{
		ModelPayload: evaluationinput.TypologyModelPayload{Payload: payload},
	}}
	outcome := &domainoutcome.Execution{
		ModelRef: domainoutcome.ModelRef{
			ModelKind: modelcatalog.KindTypology, ModelCode: "mbti", ModelVersion: "1",
		},
		Detail: domainoutcome.Detail{Payload: codec.PersonalityTypeDetail{TypeCode: "INTJ"}},
	}

	got, err := previewInterpretationInput(req, outcome)
	if err != nil {
		t.Fatal(err)
	}
	if got.Report.TemplateID != "mbti" || got.Report.TemplateVersion != "2026-08-v1" {
		t.Fatalf("template route = %s@%s", got.Report.TemplateID, got.Report.TemplateVersion)
	}
}
