package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	v1envelope "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/v1envelope"
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
	if published.Kind != domain.KindTypology || published.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("model = %#v", published)
	}
	legacy := v1envelope.V1FromPublished(published)
	if legacy.Definition.Kind != domain.KindTypology || legacy.Definition.Code != "MBTI_OEJTS" {
		t.Fatalf("legacy kind = %s code = %s", legacy.Definition.Kind, legacy.Definition.Code)
	}
}

func TestDecodeScaleFromPublishedPrefersDefinitionV2(t *testing.T) {
	model := &port.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        domain.PayloadFormatAssessmentScaleV1,
		Kind:                 domain.KindScale,
		Code:                 "SCL-V2",
		Version:              "2.0.0",
		Title:                "Definition Scale",
		Status:               "published",
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		Payload:              []byte(`not-json`),
		DefinitionV2: scalesnapshot.DefinitionFromScaleSnapshot(&scalesnapshot.ScaleSnapshot{
			Code:         "ignored",
			ScaleVersion: "ignored",
			Title:        "ignored",
			Status:       "published",
			Factors: []scalesnapshot.FactorSnapshot{{
				Code:            "total",
				Title:           "Total",
				IsTotalScore:    true,
				QuestionCodes:   []string{"q1"},
				ScoringStrategy: "sum",
				InterpretRules: []scalesnapshot.InterpretRuleSnapshot{{
					Min: 0, Max: 10, RiskLevel: "low", Conclusion: "低风险",
				}},
			}},
		}),
	}

	got, err := DecodeScaleFromPublished(model)
	if err != nil {
		t.Fatalf("DecodeScaleFromPublished: %v", err)
	}
	if got.Code != "SCL-V2" || got.ScaleVersion != "2.0.0" || got.Factors[0].Title != "Total" {
		t.Fatalf("decoded scale = %#v", got)
	}
	if len(got.Factors[0].InterpretRules) != 1 || got.Factors[0].InterpretRules[0].RiskLevel != "low" {
		t.Fatalf("interpret rules = %#v", got.Factors[0].InterpretRules)
	}
}

func TestRefFromPublishedPreservesPersonalityTypologyIdentity(t *testing.T) {
	published := &port.PublishedModel{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Kind:          domain.KindTypology,
		SubKind:       domain.SubKindTypology,
		Algorithm:     domain.AlgorithmPersonalityTypology,
		Code:          "ENNEAGRAM_45",
		Version:       "1.0.0",
		Title:         "九型人格",
		Status:        "published",
		Payload:       []byte(`{"algorithm":"personality_typology","code":"ENNEAGRAM_45","version":"1.0.0","status":"published"}`),
	}
	ref := RefFromPublished(published)
	if ref.Kind != domain.KindTypology || ref.SubKind != domain.SubKindTypology || ref.Algorithm != domain.AlgorithmPersonalityTypology {
		t.Fatalf("ref = %#v, want typology/typology/personality_typology", ref)
	}
}

func TestRefMatchesPublishedSupportsLegacyAndV2Refs(t *testing.T) {
	published := &port.PublishedModel{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Kind:          domain.KindTypology,
		SubKind:       domain.SubKindTypology,
		Algorithm:     domain.AlgorithmMBTI,
		Code:          "MBTI_OEJTS",
		Version:       "2.0.1",
		Title:         "MBTI",
		Status:        "published",
		Payload:       []byte(`{"algorithm":"mbti","code":"MBTI_OEJTS","version":"2.0.1","status":"published"}`),
	}
	v2Ref := port.Ref{
		Kind:      domain.KindTypology,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Code:      "MBTI_OEJTS",
		Version:   "2.0.1",
	}
	if !RefMatchesPublished(v2Ref, published) {
		t.Fatal("expected v2 ref to match published snapshot")
	}
}

func TestRefFromPublishedPreservesBigFiveIdentity(t *testing.T) {
	published := &port.PublishedModel{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Kind:          domain.KindTypology,
		SubKind:       domain.SubKindTypology,
		Algorithm:     domain.AlgorithmBigFive,
		Code:          "BIG5_IPIP_50",
		Version:       "1.0.0",
		Title:         "大五人格",
		Status:        "published",
		Payload:       []byte(`{"algorithm":"bigfive","code":"BIG5_IPIP_50","version":"1.0.0","status":"published"}`),
	}
	ref := RefFromPublished(published)
	if ref.Kind != domain.KindTypology || ref.SubKind != domain.SubKindTypology || ref.Algorithm != domain.AlgorithmBigFive {
		t.Fatalf("ref = %#v, want typology/typology/bigfive", ref)
	}
}
