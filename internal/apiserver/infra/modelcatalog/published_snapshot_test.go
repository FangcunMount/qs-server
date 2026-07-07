package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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
	if legacy.Definition.Kind != domain.KindPersonality || legacy.Definition.Code != "MBTI_OEJTS" {
		t.Fatalf("legacy kind = %s code = %s", legacy.Definition.Kind, legacy.Definition.Code)
	}
}

func TestRefFromSnapshotPreservesPersonalityTypologyIdentity(t *testing.T) {
	legacy := domain.LegacyFromPublished(&domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Model: domain.ModelDefinition{
			Kind:      domain.KindPersonality,
			SubKind:   domain.SubKindTypology,
			Algorithm: domain.AlgorithmPersonalityTypology,
			Code:      "ENNEAGRAM_45",
			Version:   "1.0.0",
			Title:     "九型人格",
			Status:    "published",
		},
		Payload: []byte(`{"algorithm":"personality_typology","code":"ENNEAGRAM_45","version":"1.0.0","status":"published"}`),
	})
	ref := RefFromSnapshot(legacy)
	if ref.Kind != domain.KindPersonality || ref.SubKind != domain.SubKindTypology || ref.Algorithm != domain.AlgorithmPersonalityTypology {
		t.Fatalf("ref = %#v, want personality/typology/personality_typology", ref)
	}
}

func TestRefMatchesSnapshotSupportsLegacyAndV2Refs(t *testing.T) {
	legacy := domain.LegacyFromPublished(&domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Model: domain.ModelDefinition{
			Kind:      domain.KindPersonality,
			SubKind:   domain.SubKindTypology,
			Algorithm: domain.AlgorithmMBTI,
			Code:      "MBTI_OEJTS",
			Version:   "2.0.1",
			Title:     "MBTI",
			Status:    "published",
		},
		Payload: []byte(`{"algorithm":"mbti","code":"MBTI_OEJTS","version":"2.0.1","status":"published"}`),
	})
	v2Ref := port.Ref{
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Code:      "MBTI_OEJTS",
		Version:   "2.0.1",
	}
	if !RefMatchesSnapshot(v2Ref, legacy) {
		t.Fatal("expected v2 ref to match legacy snapshot")
	}
}

func TestRefFromSnapshotPreservesBigFiveIdentity(t *testing.T) {
	legacy := domain.LegacyFromPublished(&domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Model: domain.ModelDefinition{
			Kind:      domain.KindPersonality,
			SubKind:   domain.SubKindTypology,
			Algorithm: domain.AlgorithmBigFive,
			Code:      "BIG5_IPIP_50",
			Version:   "1.0.0",
			Title:     "大五人格",
			Status:    "published",
		},
		Payload: []byte(`{"algorithm":"bigfive","code":"BIG5_IPIP_50","version":"1.0.0","status":"published"}`),
	})
	ref := RefFromSnapshot(legacy)
	if ref.Kind != domain.KindPersonality || ref.SubKind != domain.SubKindTypology || ref.Algorithm != domain.AlgorithmBigFive {
		t.Fatalf("ref = %#v, want personality/typology/bigfive", ref)
	}
}
