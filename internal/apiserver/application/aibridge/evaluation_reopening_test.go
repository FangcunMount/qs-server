package aibridge

import (
	"context"
	"errors"
	"testing"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

func (g *managementStub) ReopenEvaluationReview(ctx context.Context, scope EvaluationScope, command EvaluationReopen) (EvaluationState, error) {
	g.reopen = command
	return g.GetEvaluation(ctx, scope)
}

func TestReviewReopeningRechecksAuthorizationAndConfirmation(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	command := EvaluationReopen{ExpectedVersion: 8, Reason: " 复核语义分歧 ", Confirm: true}
	if _, err := service.ReopenReview(context.Background(), managementScope(), command); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	for _, invalid := range []EvaluationReopen{
		{ExpectedVersion: 8, Reason: "复核"}, {ExpectedVersion: 8, Confirm: true}, {Reason: "复核", Confirm: true},
	} {
		if _, err := service.ReopenReview(adminContext(), managementScope(), invalid); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if gateway.calls != 0 {
		t.Fatal("invalid command reached AI")
	}
	if _, err := service.ReopenReview(adminContext(), managementScope(), command); err != nil {
		t.Fatal(err)
	}
	if gateway.calls != 1 || gateway.reopen.Reason != "复核语义分歧" || gateway.scope != managementScope() {
		t.Fatal("scope or normalized command changed")
	}
	revoked := authz.WithSnapshot(context.Background(), &authz.Snapshot{})
	if _, err := service.ReopenReview(revoked, managementScope(), command); !errors.Is(err, ErrGovernanceDenied) || gateway.calls != 1 {
		t.Fatal("revoked request reached AI", err)
	}
}
