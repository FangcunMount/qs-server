package service

import (
	"testing"

	appTypologyModel "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer"
)

func TestToProtoTypologyModelSummaryIncludesRoutingFields(t *testing.T) {
	t.Parallel()

	summary := toProtoTypologyModelSummary(&appTypologyModel.TypologyModelSummaryResult{
		Code:            "MBTI",
		Kind:            "personality",
		SubKind:         "typology",
		ProductChannel:  "behavior_ability",
		AlgorithmFamily: "typology",
		PayloadFormat:   "typology_v1",
		DecisionKind:    "outcome_code",
	})
	if summary.GetKind() != "personality" ||
		summary.GetSubKind() != "typology" ||
		summary.GetProductChannel() != "behavior_ability" ||
		summary.GetAlgorithmFamily() != "typology" ||
		summary.GetPayloadFormat() != "typology_v1" ||
		summary.GetDecisionKind() != "outcome_code" {
		t.Fatalf("routing fields not mapped: %+v", summary)
	}
}
