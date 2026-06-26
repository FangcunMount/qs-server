package assessmentmodel

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

func TestBuildMBTIPublishedSnapshotUsesTypologyPayload(t *testing.T) {
	published, err := BuildMBTIPublishedSnapshot(&modeltypology.MBTILegacyModel{
		Code:              "MBTI_OEJTS",
		Version:           "1.0.0",
		QuestionnaireCode: "MBTI_OEJTS",
		Status:            "published",
	})
	if err != nil {
		t.Fatalf("BuildMBTIPublishedSnapshot: %v", err)
	}
	if published.PayloadFormat != domain.PayloadFormatPersonalityTypologyV1 {
		t.Fatalf("format = %s", published.PayloadFormat)
	}
	if published.Model.Kind != domain.KindPersonality || published.Model.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("model = %#v", published.Model)
	}
	legacy := LegacySnapshotFromPublished(published)
	if legacy.Definition.Kind != domain.KindMBTIMigration {
		t.Fatalf("legacy kind = %s", legacy.Definition.Kind)
	}
}
