package assessmentmodel_test

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/assessmentmodel"
)

func TestBuildPersonalityPublishedSnapshot(t *testing.T) {
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
			"factor_graph":{"dimension_order":["EI"],"dimensions":{"EI":{"code":"EI","name":"EI"}},"roots":["EI"]},
			"decision":{"kind":"pole_composition"},
			"outcome_mapping":{"detail_kind":"mbti_type"},
			"report":{"kind":"template","adapter_key":"mbti_default"}
		}`),
	}, model.CreatedAt); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	snapshot, err := aminfra.BuildPersonalityPublishedSnapshot(model)
	if err != nil {
		t.Fatalf("BuildPersonalityPublishedSnapshot: %v", err)
	}
	if snapshot.PayloadFormat != domain.PayloadFormatPersonalityTypologyV1 {
		t.Fatalf("payload format = %s", snapshot.PayloadFormat)
	}
	if snapshot.Model.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("algorithm = %s", snapshot.Model.Algorithm)
	}
	legacy := aminfra.LegacySnapshotFromPublished(snapshot)
	if legacy.Definition.Kind != domain.KindMBTIMigration {
		t.Fatalf("legacy kind = %s", legacy.Definition.Kind)
	}
}
