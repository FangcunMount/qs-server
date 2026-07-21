package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestMapperRoundTripPublishedModel(t *testing.T) {
	original := &port.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		Kind:                 domain.KindTypology,
		SubKind:              domain.SubKindTypology,
		Algorithm:            domain.AlgorithmPersonalityTypology,
		AlgorithmFamily:      domain.AlgorithmFamilyFactorClassification,
		Code:                 "MBTI_OEJTS",
		Version:              "1.0.0",
		Title:                "MBTI",
		Description:          "personality type",
		Category:             "personality",
		Stages:               []string{"intake"},
		ApplicableAges:       []string{"adult"},
		Reporters:            []string{"self"},
		Tags:                 []string{"demo"},
		Status:               "published",
		QuestionnaireCode:    "MBTI_OEJTS",
		QuestionnaireVersion: "1.0.0",
		DecisionKind:         domain.DecisionKindPoleComposition,
		Source:               map[string]any{"license": "CC BY-NC-SA 4.0"},
		DefinitionV2:         sampleDefinitionV2(),
	}

	mapper := NewMapper()
	po := mapper.ToPO(original)
	if po.AlgorithmFamily != string(domain.AlgorithmFamilyFactorClassification) {
		t.Fatalf("po algorithm_family = %q", po.AlgorithmFamily)
	}
	if po.DefinitionSchemaVersion != domain.SchemaVersionV2 {
		t.Fatalf("definition schema version = %q", po.DefinitionSchemaVersion)
	}
	if po.DefinitionV2 == nil || len(po.DefinitionV2.Measure.Scoring) != 1 {
		t.Fatalf("definition_v2 po = %#v", po.DefinitionV2)
	}
	got := mapper.ToPublished(po)
	if got.Code != original.Code || got.Algorithm != domain.AlgorithmPersonalityTypology {
		t.Fatalf("published round trip = %#v", got)
	}
	if got.Description != "personality type" || got.Category != "personality" || got.Stages[0] != "intake" ||
		got.ApplicableAges[0] != "adult" || got.Reporters[0] != "self" || got.Tags[0] != "demo" {
		t.Fatalf("published metadata round trip = %#v", got)
	}
	assertDefinitionV2RoundTrip(t, got.DefinitionV2)
}

func TestPublishedMapperLeavesMissingDefinitionV2Nil(t *testing.T) {
	po := &PublishedAssessmentModelPO{
		SchemaVersion:  domain.SchemaVersionV2,
		RecordRole:     recordRolePublishedSnapshot,
		Kind:           string(domain.KindBehavioralRating),
		Algorithm:      string(domain.AlgorithmBrief2),
		Code:           "brief2",
		ReleaseVersion: "v1",
		Title:          "BRIEF-2",
		Status:         "published",
		DecisionKind:   string(domain.DecisionKindNormLookup),
	}
	got := NewMapper().ToPublished(po)
	if got.DefinitionV2 != nil {
		t.Fatalf("definition v2 = %#v, want nil for old document", got.DefinitionV2)
	}
}
