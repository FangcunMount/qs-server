package aibridge

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func (g *managementStub) ReviewEvaluation(ctx context.Context, scope EvaluationScope, value EvaluationReview) (EvaluationState, error) {
	g.review = value
	return g.GetEvaluation(ctx, scope)
}

func reviewCommand() EvaluationReview {
	return EvaluationReview{ExpectedVersion: 7, Role: "assessment_semantics", Reviews: []CandidateReviewItem{{
		CandidateID: " candidate:1 ", Decision: "approve", Reason: " 核对事实 ",
		SemanticReview: &SemanticContradictionReview{PolicyVersion: "semantic-contradiction-dual-review/v1", ExecutionID: "semantic:1",
			OutputFingerprint: "sha256:" + strings.Repeat("a", 64), AssertionOrdinal: 1, OriginalDetail: "原判断", CandidateExcerpt: "原文", Reason: " 复核说明 "},
	}}}
}

func TestReviewValidatesCompleteBatchBeforeForwarding(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	for _, change := range []func(*EvaluationReview){
		func(c *EvaluationReview) { c.ExpectedVersion = 0 },
		func(c *EvaluationReview) { c.Role = "admin" },
		func(c *EvaluationReview) { c.Reviews = nil },
		func(c *EvaluationReview) { c.Reviews = make([]CandidateReviewItem, 36) },
		func(c *EvaluationReview) { c.Reviews = append(c.Reviews, c.Reviews[0]) },
		func(c *EvaluationReview) { c.Reviews[0].CandidateID = " " },
		func(c *EvaluationReview) { c.Reviews[0].Reason = strings.Repeat("汉", 334) },
		func(c *EvaluationReview) { c.Reviews[0].Reason = "<script>" },
		func(c *EvaluationReview) { c.Reviews[0].Decision = "reject" },
		func(c *EvaluationReview) { c.Reviews[0].SemanticReview.OutputFingerprint = "invalid" },
		func(c *EvaluationReview) { c.Reviews[0].SemanticReview.PolicyVersion = "v0" },
		func(c *EvaluationReview) { c.Reviews[0].SemanticReview.AssertionOrdinal = 0 },
		func(c *EvaluationReview) { c.Reviews[0].SemanticReview.CandidateExcerpt = "" },
	} {
		value := reviewCommand()
		change(&value)
		if _, err := service.Review(adminContext(), managementScope(), value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid batch: %v", err)
		}
	}
	if gateway.calls != 0 {
		t.Fatal("invalid review forwarded")
	}
	value := reviewCommand()
	if _, err := service.Review(adminContext(), managementScope(), value); err != nil {
		t.Fatal(err)
	}
	if gateway.review.Reviews[0].CandidateID != "candidate:1" || gateway.review.Reviews[0].Reason != "核对事实" || gateway.review.Reviews[0].SemanticReview.Reason != "复核说明" {
		t.Fatal("normalization missing")
	}
	if value.Reviews[0].Reason != " 核对事实 " || value.Reviews[0].SemanticReview.Reason != " 复核说明 " {
		t.Fatal("caller-owned command changed")
	}
	if _, err := service.Review(context.Background(), managementScope(), value); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if gateway.calls != 1 {
		t.Fatal("revoked review reached AI")
	}
}
