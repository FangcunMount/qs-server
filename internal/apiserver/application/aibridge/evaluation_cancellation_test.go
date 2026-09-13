package aibridge

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

func (g *managementStub) CancelEvaluation(ctx context.Context, scope EvaluationScope, command EvaluationCancel) (EvaluationState, error) {
	g.cancel = command
	return g.GetEvaluation(ctx, scope)
}

func TestCancellationRequiresCurrentAuthorityAndExplicitDecision(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	discard := false
	valid := EvaluationCancel{ExpectedVersion: 8, Reason: " 停止后续工作 ", Confirm: true, Discard: &discard}
	for _, ctx := range []context.Context{context.Background(), authz.WithSnapshot(context.Background(), &authz.Snapshot{})} {
		if _, err := service.Cancel(ctx, managementScope(), valid); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatal("missing or revoked authority reached AI", err)
		}
	}
	for _, mutate := range []func(*EvaluationCancel){
		func(c *EvaluationCancel) { c.Discard = nil },
		func(c *EvaluationCancel) { c.Confirm = false },
		func(c *EvaluationCancel) { c.ExpectedVersion = 0 },
		func(c *EvaluationCancel) { c.ExpectedVersion = math.MaxInt64 },
		func(c *EvaluationCancel) { c.Reason = " " },
		func(c *EvaluationCancel) { c.Reason = strings.Repeat("中", 334) },
		func(c *EvaluationCancel) { c.Reason = "<invalid>" },
	} {
		command := valid
		mutate(&command)
		if _, err := service.Cancel(adminContext(), managementScope(), command); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid cancellation accepted", err)
		}
	}
	if gateway.calls != 0 {
		t.Fatal("invalid request reached AI")
	}
	for _, decision := range []bool{false, true} {
		valid.Discard = &decision
		if _, err := service.Cancel(adminContext(), managementScope(), valid); err != nil {
			t.Fatal(err)
		}
		if gateway.cancel.Discard == nil || *gateway.cancel.Discard != decision || gateway.cancel.Reason != "停止后续工作" || gateway.scope != managementScope() {
			t.Fatal("trusted scope or explicit false lost")
		}
	}
}
