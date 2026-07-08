package shared

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func TestSummaryFromPublishedModelTypologyPayload(t *testing.T) {
	payload, err := json.Marshal(&modeltypology.Payload{
		Code:                 "MBTI_OEJTS",
		Version:              "1.0.0",
		Title:                "MBTI",
		Algorithm:            domain.AlgorithmMBTI,
		QuestionnaireCode:    "MBTI_OEJTS",
		QuestionnaireVersion: "1.0.0",
		QuestionMappings:     []modeltypology.QuestionMapping{{QuestionCode: "q1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &domain.PublishedModelSnapshot{
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
		Payload:  payload,
	}
	summary, err := SummaryFromPublishedModel(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Algorithm != string(domain.AlgorithmMBTI) || summary.QuestionCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}
