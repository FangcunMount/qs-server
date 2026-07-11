package registry_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
	pipeline "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationroute"
)

func TestReportRoutingContextFromOutcomeSetsReportProfileFromDecisionKind(t *testing.T) {
	outcome := evaloutcome.Outcome{
		RuntimeDescriptorKey: pipeline.RuntimeDescriptorKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
			DecisionKind:    modelcatalog.DecisionKindNormLookup,
		},
	}
	ctx, ok := registry.ReportRoutingContextFromOutcome(outcome)
	if !ok {
		t.Fatal("expected routing context")
	}
	if ctx.ReportProfile != policy.ReportProfileNorm {
		t.Fatalf("profile = %q, want %q", ctx.ReportProfile, policy.ReportProfileNorm)
	}
	if ctx.Audience != "" {
		t.Fatalf("audience should remain empty on v1 path, got %q", ctx.Audience)
	}
}
