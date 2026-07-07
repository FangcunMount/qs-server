package personality_test

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	personalitydomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestBuildPublishedSnapshot(t *testing.T) {
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:      "personality_mbti_test",
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Title:     "MBTI Test",
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode: "Q_MBTI", QuestionnaireVersion: "1.0.0",
	}, model.CreatedAt); err != nil {
		t.Fatalf("BindQuestionnaire: %v", err)
	}
	if err := model.UpdateDefinition(domain.DefinitionPayload{
		Format: domain.PayloadFormatPersonalityTypologyV1,
		Data: []byte(`{
			"algorithm":"mbti",
			"outcomes":[{"code":"INTJ","name":"建筑师","summary":"独立战略家"}],
			"runtime":{
				"factor_graph":{"dimension_order":["EI"],"dimensions":{"EI":{"code":"EI","name":"EI"}},"roots":["EI"]},
				"decision":{"kind":"pole_composition"},
				"outcome_mapping":{"detail_kind":"personality_type","detail_adapter_key":"personality_type"},
				"report":{"kind":"personality_type","adapter_key":"personality_type"}
			}
		}`),
	}, model.CreatedAt); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	snapshot, err := personalitydomain.BuildPublishedSnapshot(model)
	if err != nil {
		t.Fatalf("BuildPublishedSnapshot: %v", err)
	}
	if snapshot.PayloadFormat != domain.PayloadFormatPersonalityTypologyV1 {
		t.Fatalf("payload format = %s", snapshot.PayloadFormat)
	}
	if snapshot.Model.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("algorithm = %s", snapshot.Model.Algorithm)
	}
	if snapshot.Model.Version != "v3" {
		t.Fatalf("model version = %s, want v3", snapshot.Model.Version)
	}
	if snapshot.Model.Version == snapshot.Binding.QuestionnaireVersion {
		t.Fatalf("model version should not reuse questionnaire version %s", snapshot.Binding.QuestionnaireVersion)
	}
	var payload modeltypology.Payload
	if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
		t.Fatalf("decode snapshot payload: %v", err)
	}
	if payload.Runtime == nil || payload.Runtime.Decision.Kind != domain.DecisionKindPoleComposition {
		t.Fatalf("snapshot runtime = %#v, want pole_composition", payload.Runtime)
	}
	if len(payload.Outcomes) != 1 || payload.Outcomes[0].Code != "INTJ" {
		t.Fatalf("snapshot payload outcomes = %#v, want INTJ preserved", payload.Outcomes)
	}
	legacy := domain.LegacyFromPublished(snapshot)
	if legacy.Definition.Kind != domain.KindMBTIMigration {
		t.Fatalf("legacy kind = %s", legacy.Definition.Kind)
	}
}

func TestBuildPublishedSnapshotUsesMatchingSpecDecisionKind(t *testing.T) {
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:      "personality_mbti_trait_profile",
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Title:     "MBTI Trait Profile Override",
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode: "Q_MBTI", QuestionnaireVersion: "1.0.0",
	}, model.CreatedAt); err != nil {
		t.Fatalf("BindQuestionnaire: %v", err)
	}
	if err := model.UpdateDefinition(domain.DefinitionPayload{
		Format: domain.PayloadFormatPersonalityTypologyV1,
		Data: []byte(`{
			"algorithm":"mbti",
			"matching_spec":{"kind":"trait_profile"},
			"dimension_order":["EI"],
			"dimensions":{"EI":{"code":"EI","name":"EI"}},
			"outcomes":[{"code":"INTJ","name":"建筑师"}],
			"runtime":{
				"factor_graph":{"dimension_order":["EI"],"dimensions":{"EI":{"code":"EI","name":"EI"}},"roots":["EI"]},
				"outcome_mapping":{"detail_kind":"personality_trait","detail_adapter_key":"trait_profile"},
				"report":{"kind":"trait_profile","adapter_key":"trait_profile"}
			}
		}`),
	}, model.CreatedAt); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	snapshot, err := personalitydomain.BuildPublishedSnapshot(model)
	if err != nil {
		t.Fatalf("BuildPublishedSnapshot: %v", err)
	}
	if snapshot.Decision.Kind != domain.DecisionKindTraitProfile {
		t.Fatalf("decision kind = %s, want trait_profile from matching_spec", snapshot.Decision.Kind)
	}
}

func TestBuildPublishedSnapshotUsesExplicitRuntimeDecisionKind(t *testing.T) {
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:      "personality_mbti_explicit_decision",
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Title:     "MBTI Explicit Decision",
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode: "Q_MBTI", QuestionnaireVersion: "1.0.0",
	}, model.CreatedAt); err != nil {
		t.Fatalf("BindQuestionnaire: %v", err)
	}
	if err := model.UpdateDefinition(domain.DefinitionPayload{
		Format: domain.PayloadFormatPersonalityTypologyV1,
		Data: []byte(`{
			"algorithm":"mbti",
			"outcomes":[{"code":"INTJ","name":"建筑师"}],
			"runtime":{
				"factor_graph":{"dimension_order":["EI"],"dimensions":{"EI":{"code":"EI","name":"EI"}},"roots":["EI"]},
				"decision":{"kind":"pole_composition"},
				"outcome_mapping":{"detail_kind":"personality_type","detail_adapter_key":"personality_type"},
				"report":{"kind":"personality_type","adapter_key":"personality_type"}
			}
		}`),
	}, model.CreatedAt); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	snapshot, err := personalitydomain.BuildPublishedSnapshot(model)
	if err != nil {
		t.Fatalf("BuildPublishedSnapshot: %v", err)
	}
	if snapshot.Decision.Kind != domain.DecisionKindPoleComposition {
		t.Fatalf("decision kind = %s, want explicit pole_composition (no publish-time algorithm fallback)", snapshot.Decision.Kind)
	}
}
