package personality

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

func TestDefinitionFromModelNormalizesFullPayloadToEditorShape(t *testing.T) {
	payload := &modeltypology.Payload{
		Algorithm:      domain.AlgorithmMBTI,
		DimensionOrder: []string{"EI"},
		Dimensions: map[string]modeltypology.Dimension{
			"EI": {Code: "EI", Name: "外向/内向"},
		},
		QuestionMappings: []modeltypology.QuestionMapping{
			{
				QuestionCode: "Q1",
				Dimension:    "EI",
				Sign:         1,
				OptionScores: map[string]float64{"1": 1, "2": 2, "3": 3, "4": 4, "5": 5},
			},
		},
		Outcomes: []modeltypology.Outcome{{Code: "INTJ", Name: "建筑师"}},
		Runtime: &modeltypology.RuntimeSpec{
			Decision: modeltypology.PersonalityDecisionSpec{Kind: domain.DecisionKindPoleComposition},
			Report: modeltypology.ReportSpec{
				Kind:       modeltypology.ReportKindPersonalityType,
				AdapterKey: modeltypology.ReportAdapterMBTI,
			},
			FactorGraph: modeltypology.FactorGraphSpec{
				Factors: map[string]modeltypology.FactorSpec{
					"EI": {ID: "EI", Kind: modeltypology.FactorSpecKindLeaf},
				},
				Roots: []string{"EI"},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	model := &domain.AssessmentModel{
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Definition: domain.DefinitionPayload{
			Format: domain.PayloadFormatPersonalityTypologyV1,
			Data:   raw,
		},
	}

	result := definitionFromModel(model)
	if result == nil {
		t.Fatal("definitionFromModel returned nil")
	}

	var editor editorDefinitionPayload
	if err := json.Unmarshal(result.Payload, &editor); err != nil {
		t.Fatalf("unmarshal editor payload: %v", err)
	}
	if editor.FactorGraph.Factors["EI"].ID != "EI" {
		t.Fatalf("factor EI = %#v", editor.FactorGraph.Factors["EI"])
	}
	if len(editor.FactorGraph.QuestionMappings) != 1 {
		t.Fatalf("question_mappings = %#v", editor.FactorGraph.QuestionMappings)
	}
	mapping := editor.FactorGraph.QuestionMappings[0]
	if mapping.FactorCode != "EI" {
		t.Fatalf("factor_code = %s, want EI", mapping.FactorCode)
	}
	if mapping.OptionScores["5"] != 5 {
		t.Fatalf("option_scores = %#v", mapping.OptionScores)
	}
	if len(editor.OutcomeMapping.Outcomes) != 1 || editor.OutcomeMapping.Outcomes[0].Code != "INTJ" {
		t.Fatalf("outcomes = %#v", editor.OutcomeMapping.Outcomes)
	}
	if editor.OutcomeMapping.Outcomes[0].Name != "建筑师" {
		t.Fatalf("outcome name = %q, want 建筑师", editor.OutcomeMapping.Outcomes[0].Name)
	}

	var rawPayload map[string]json.RawMessage
	if err := json.Unmarshal(result.Payload, &rawPayload); err != nil {
		t.Fatalf("unmarshal raw payload: %v", err)
	}
	var outcomeMapping struct {
		Outcomes []struct {
			Code  string `json:"code"`
			Title string `json:"title"`
			Name  string `json:"name"`
		} `json:"outcomes"`
	}
	if err := json.Unmarshal(rawPayload["outcome_mapping"], &outcomeMapping); err != nil {
		t.Fatalf("unmarshal outcome_mapping: %v", err)
	}
	if len(outcomeMapping.Outcomes) != 1 || outcomeMapping.Outcomes[0].Name != "建筑师" {
		t.Fatalf("serialized outcomes = %#v", outcomeMapping.Outcomes)
	}
	if outcomeMapping.Outcomes[0].Title != "" {
		t.Fatalf("serialized outcome should use name, got title=%q", outcomeMapping.Outcomes[0].Title)
	}
}

func TestNormalizeDefinitionPayloadForStorageRoundTripsEditorPayload(t *testing.T) {
	editor := editorDefinitionPayload{
		FactorGraph: editorFactorGraphSpec{
			QuestionMappings: []editorQuestionMapping{{
				QuestionCode: "Q1",
				FactorCode:   "EI",
				Sign:         1,
				OptionScores: defaultLikertOptionScores(),
			}},
			Factors: map[string]modeltypology.FactorSpec{
				"EI": {ID: "EI", Kind: modeltypology.FactorSpecKindLeaf},
			},
			Roots: []string{"EI"},
		},
		Decision: modeltypology.PersonalityDecisionSpec{Kind: domain.DecisionKindPoleComposition},
		OutcomeMapping: editorOutcomeMappingSpec{
			DetailKind:       modeltypology.OutcomeDetailPersonalityType,
			DetailAdapterKey: modeltypology.DetailAdapterMBTI,
			Outcomes:         []modeltypology.Outcome{{Code: "INTJ", Name: "建筑师"}},
		},
		Report: modeltypology.ReportSpec{
			Kind:       modeltypology.ReportKindPersonalityType,
			AdapterKey: modeltypology.ReportAdapterMBTI,
		},
	}
	raw, err := json.Marshal(editor)
	if err != nil {
		t.Fatalf("marshal editor: %v", err)
	}

	stored, err := normalizeDefinitionPayloadForStorage(raw, domain.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("normalizeDefinitionPayloadForStorage: %v", err)
	}
	var envelope draftDefinitionEnvelope
	if err := json.Unmarshal(stored, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if len(envelope.Outcomes) != 1 || envelope.Outcomes[0].Code != "INTJ" || envelope.Outcomes[0].Name != "建筑师" {
		t.Fatalf("outcomes = %#v", envelope.Outcomes)
	}
	if envelope.Runtime == nil || len(envelope.Runtime.FactorGraph.QuestionMappings) != 1 {
		t.Fatalf("runtime mappings = %#v", envelope.Runtime)
	}
	if envelope.Runtime.FactorGraph.QuestionMappings[0].Dimension != "EI" {
		t.Fatalf("dimension = %s", envelope.Runtime.FactorGraph.QuestionMappings[0].Dimension)
	}
}
