package outboxruntime

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

func TestDefaultPolicyKeepsImmediateAndPriorityContracts(t *testing.T) {
	policy := DefaultPolicy()
	for _, eventType := range []string{
		eventcatalog.AnswerSheetSubmitted,
		eventcatalog.EvaluationRequested,
		eventcatalog.EvaluationOutcomeCommitted,
	} {
		if !policy.AllowsImmediate(eventType) {
			t.Fatalf("%q must remain immediate", eventType)
		}
	}
	if policy.AllowsImmediate(eventcatalog.EvaluationFailed) {
		t.Fatal("evaluation.failed must remain relay-only")
	}
	if len(policy.PriorityTiers) != 3 || len(policy.PriorityTiers[0]) == 0 || policy.PriorityTiers[2] != nil {
		t.Fatalf("priority tiers = %#v, want P0, P0+P1, fallback", policy.PriorityTiers)
	}
}
