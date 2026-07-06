package shared

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func TestSummaryFromSnapshotTypologyPayload(t *testing.T) {
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
	snapshot := &domain.Snapshot{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Definition: domain.Definition{
			Code:    "MBTI_OEJTS",
			Version: "1.0.0",
			Title:   "MBTI",
			Status:  "published",
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "MBTI_OEJTS",
			QuestionnaireVersion: "1.0.0",
		},
		Payload: payload,
	}
	decoded, err := modeltypology.DecodeFromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	summary := SummaryFromSnapshot(snapshot, decoded)
	if summary.Algorithm != string(domain.AlgorithmMBTI) || summary.QuestionCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}
