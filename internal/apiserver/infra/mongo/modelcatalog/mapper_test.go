package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestMapperRoundTripPublishedModel(t *testing.T) {
	original := &port.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        domain.PayloadFormatPersonalityTypologyV1,
		Kind:                 domain.KindTypology,
		SubKind:              domain.SubKindTypology,
		Algorithm:            domain.AlgorithmMBTI,
		Code:                 "MBTI_OEJTS",
		Version:              "1.0.0",
		Title:                "MBTI",
		Status:               "published",
		QuestionnaireCode:    "MBTI_OEJTS",
		QuestionnaireVersion: "1.0.0",
		DecisionKind:         domain.DecisionKindPoleComposition,
		Source:               map[string]any{"license": "CC BY-NC-SA 4.0"},
		Payload:              []byte(`{"code":"MBTI_OEJTS","algorithm":"mbti"}`),
	}

	mapper := NewMapper()
	po := mapper.ToPO(original)
	got := mapper.ToPublished(po)
	if got.Code != original.Code || got.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("published round trip = %#v", got)
	}
}
