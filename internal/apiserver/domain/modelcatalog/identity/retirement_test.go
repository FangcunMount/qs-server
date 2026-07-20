package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
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
		AssessmentRetainedAlias: 2, MetricsRetainedReadOK: true, MetricsFallbackOK: true,
	})
	if got.Status != "FAIL" || got.Reasons[0] != "assessment_retained_alias>0" {
		t.Fatalf("got = %#v", got)
	}
	got = identity.EvaluateRetirementGate(identity.RetirementGateInputs{
		AssessmentEmptyAlgorithm: 3, MetricsRetainedReadOK: true, MetricsFallbackOK: true,
	})
	if got.Status != "FAIL" || got.Reasons[0] != "assessment_empty_algorithm>0" {
		t.Fatalf("got = %#v", got)
	}
}

func TestEvaluateDualIdentityRetirementGateIgnoresEmptyAlgorithm(t *testing.T) {
	t.Parallel()
	got := identity.EvaluateDualIdentityRetirementGate(identity.RetirementGateInputs{
		AssessmentEmptyAlgorithm: 99, MetricsRetainedReadOK: true, MetricsFallbackOK: true,
	})
	if got.Status != "PASS" {
		t.Fatalf("got = %#v", got)
	}
	got = identity.EvaluateDualIdentityRetirementGate(identity.RetirementGateInputs{
		AssessmentRetainedAlias: 1, MetricsRetainedReadOK: true, MetricsFallbackOK: true,
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

func TestIsRetainedReadAliasAlgorithm(t *testing.T) {
	t.Parallel()
	if !identity.IsRetainedReadAliasAlgorithm(binding.AlgorithmMBTI) {
		t.Fatal("mbti")
	}
	if identity.IsRetainedReadAliasAlgorithm(binding.AlgorithmBrief2) {
		t.Fatal("brief2 is canonical")
	}
	if identity.IsRetainedReadAliasAlgorithm("") {
		t.Fatal("empty is not retained alias")
	}
}

func TestRetirementDeleteChecklistNonEmpty(t *testing.T) {
	t.Parallel()
	if len(identity.DualIdentityDeleteChecklist()) != 0 {
		t.Fatal("dual-identity checklist should be empty after retirement")
	}
	if len(identity.RetirementDeleteChecklist()) < 2 {
		t.Fatal("full checklist too short")
	}
}
