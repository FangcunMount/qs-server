package aibridge

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func (g *managementStub) CorrectEvaluationReview(_ context.Context, scope EvaluationScope, command EvaluationReviewCorrection) (EvaluationState, error) {
	g.calls++
	return EvaluationState{RunID: scope.RunID, Version: command.ExpectedVersion + 1, Status: "awaiting_review"}, nil
}
func TestCorrectionRequiresAdminAndBoundOriginalEvidence(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	command := EvaluationReviewCorrection{CommandID: "00000000-0000-4000-8000-000000000002", ExpectedVersion: 7,
		CandidateID: "candidate:1", Role: "safety_product", PreviousReviewFingerprint: "sha256:" + strings.Repeat("a", 64),
		CandidateOutputFingerprint: "sha256:" + strings.Repeat("b", 64), Decision: "approve", Reason: "核对原文后更正", Confirm: true}
	if _, err := service.CorrectReview(context.Background(), managementScope(), command); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	for _, modify := range []func(*EvaluationReviewCorrection){
		func(c *EvaluationReviewCorrection) { c.CommandID = "bad" }, func(c *EvaluationReviewCorrection) { c.ExpectedVersion = 0 },
		func(c *EvaluationReviewCorrection) { c.Confirm = false }, func(c *EvaluationReviewCorrection) { c.Role = "admin" },
		func(c *EvaluationReviewCorrection) { c.PreviousReviewFingerprint = "bad" }, func(c *EvaluationReviewCorrection) { c.Decision = "waive" },
	} {
		bad := command
		modify(&bad)
		if _, err := service.CorrectReview(adminContext(), managementScope(), bad); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	value, err := service.CorrectReview(adminContext(), managementScope(), command)
	if err != nil || value.Version != 8 || gateway.calls != 1 {
		t.Fatalf("%+v %v calls=%d", value, err, gateway.calls)
	}
}
