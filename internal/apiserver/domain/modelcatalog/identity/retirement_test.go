package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestEvaluateRetirementGateFailOnInventory(t *testing.T) {
	t.Parallel()
	got := identity.EvaluateRetirementGate(identity.RetirementGateInputs{
		PublishedRetainedRead: 1, MetricsRetainedReadOK: true, MetricsFallbackOK: true,
	})
	if got.Status != "FAIL" {
		t.Fatalf("got = %#v", got)
	}
	got = identity.EvaluateRetirementGate(identity.RetirementGateInputs{
		AssessmentRetainedRead: 2, MetricsRetainedReadOK: true, MetricsFallbackOK: true,
	})
	if got.Status != "FAIL" {
		t.Fatalf("got = %#v", got)
	}
}

func TestEvaluateRetirementGateWarnWithoutMetrics(t *testing.T) {
	t.Parallel()
	got := identity.EvaluateRetirementGate(identity.RetirementGateInputs{})
	if got.Status != "WARN" || len(got.Reasons) != 2 {
		t.Fatalf("got = %#v", got)
	}
}

func TestEvaluateRetirementGatePass(t *testing.T) {
	t.Parallel()
	got := identity.EvaluateRetirementGate(identity.RetirementGateInputs{
		MetricsRetainedReadOK: true, MetricsFallbackOK: true,
	})
	if got.Status != "PASS" {
		t.Fatalf("got = %#v", got)
	}
}

func TestRetirementDeleteChecklistNonEmpty(t *testing.T) {
	t.Parallel()
	if len(identity.RetirementDeleteChecklist()) < 5 {
		t.Fatal("checklist too short")
	}
}
