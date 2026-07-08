package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestMapperRoundTripPublishedModel(t *testing.T) {
	original := &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Model: domain.ModelDefinition{
			Kind:      domain.KindTypology,
			SubKind:   domain.SubKindTypology,
			Algorithm: domain.AlgorithmMBTI,
			Code:      "MBTI_OEJTS",
			Version:   "1.0.0",
			Title:     "MBTI",
			Status:    "published",
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "MBTI_OEJTS",
			QuestionnaireVersion: "1.0.0",
		},
		Decision: domain.DecisionSpec{Kind: domain.DecisionKindPoleComposition},
		Source:   domain.SourceRef{"license": "CC BY-NC-SA 4.0"},
		Payload:  []byte(`{"code":"MBTI_OEJTS","algorithm":"mbti"}`),
	}

	mapper := NewMapper()
	po := mapper.ToPO(original)
	got := mapper.ToPublished(po)
	if got.Model.Code != original.Model.Code || got.Model.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("published round trip = %#v", got.Model)
	}
}
