package aibridge

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

func TestCancellationRequiresCurrentAuthorityAndExplicitDecision(t *testing.T) {
	gateway := &managementStub{}
	messages := &submissionCommands{}
	service := &EvaluationAdministration{Gateway: gateway, Messages: messages}
	discard := false
	valid := EvaluationCancel{CommandID: "00000000-0000-4000-8000-000000000005", ExpectedVersion: 8, Reason: " 停止后续工作 ", Confirm: true, Discard: &discard}
	for _, ctx := range []context.Context{context.Background(), authz.WithSnapshot(context.Background(), &authz.Snapshot{})} {
		if err := service.SubmitCancel(ctx, managementScope(), valid); !errors.Is(err, ErrGovernanceDenied) {
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
		if err := service.SubmitCancel(adminContext(), managementScope(), command); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid cancellation accepted", err)
		}
	}
	if gateway.calls != 0 || messages.calls != 0 {
		t.Fatal("invalid request reached AI")
	}
	for _, decision := range []bool{false, true} {
		valid.Discard = &decision
		if err := service.SubmitCancel(adminContext(), managementScope(), valid); err != nil {
			t.Fatal(err)
		}
		if messages.cancel.Discard == nil || *messages.cancel.Discard != decision || messages.cancel.Reason != "停止后续工作" || messages.scope != managementScope() {
			t.Fatal("trusted scope or explicit false lost")
		}
	}
}
