package aibridge

import (
	"context"
	"errors"
	"testing"
)

func (g *managementStub) FinalizeEvaluation(ctx context.Context, scope EvaluationScope, command EvaluationFinalize) (EvaluationState, error) {
	g.finalize = command
	return g.GetEvaluation(ctx, scope)
}

func TestFinalizeRequiresCurrentAdminAndExplicitOutcome(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	passed := false
	command := EvaluationFinalize{ExpectedVersion: 7, ExpectedPassed: &passed, Reason: " 核对后拒绝 ", Confirm: true}
	if _, err := service.Finalize(context.Background(), managementScope(), command); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	for _, mutate := range []func(*EvaluationFinalize){
		func(c *EvaluationFinalize) { c.ExpectedVersion = 0 },
		func(c *EvaluationFinalize) { c.ExpectedPassed = nil },
		func(c *EvaluationFinalize) { c.Confirm = false },
		func(c *EvaluationFinalize) { c.Reason = " " },
		func(c *EvaluationFinalize) { c.Reason = "<script>" },
	} {
		invalid := command
		mutate(&invalid)
		if _, err := service.Finalize(adminContext(), managementScope(), invalid); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if gateway.calls != 0 {
		t.Fatal("invalid finalization reached AI")
	}
	if _, err := service.Finalize(adminContext(), managementScope(), command); err != nil {
		t.Fatal(err)
	}
	if gateway.calls != 1 || gateway.finalize.ExpectedPassed == nil || *gateway.finalize.ExpectedPassed || gateway.finalize.Reason != "核对后拒绝" {
		t.Fatal("explicit false or audited reason lost")
	}
}
