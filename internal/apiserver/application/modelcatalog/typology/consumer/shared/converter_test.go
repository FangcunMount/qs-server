package shared

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
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
	snapshot := &port.PublishedModel{
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
		Payload:              payload,
	}
	summary, err := SummaryFromPublishedModel(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Algorithm != string(domain.AlgorithmMBTI) || summary.QuestionCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}
