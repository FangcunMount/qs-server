package aibridge

import (
	"context"
	"errors"
	"testing"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

func (g *managementStub) ListEvaluationCandidates(ctx context.Context, s EvaluationScope) (EvaluationCandidateIndex, error) {
	_, err := g.GetEvaluation(ctx, s)
	return EvaluationCandidateIndex{RunID: s.RunID, Version: 7}, err
}
func (g *managementStub) GetEvaluationCandidate(ctx context.Context, s EvaluationScope, q CandidateQuery) (EvaluationCandidateEvidence, error) {
	_, err := g.GetEvaluation(ctx, s)
	return EvaluationCandidateEvidence{RunID: s.RunID, Version: q.ExpectedVersion, CandidateID: q.CandidateID}, err
}
func auditContext() context.Context {
	return authz.WithSnapshot(context.Background(), &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:evaluation:collection:reports", Action: "audit", Mode: authz.AuthorizationModeUnconditional}}})
}
func TestAuditorCanReadCandidateEvidenceButCannotWriteReviews(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	scope := managementScope()
	ctx := auditContext()
	if _, err := service.ListCandidates(ctx, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetCandidate(ctx, scope, CandidateQuery{CandidateID: "candidate:1", ExpectedVersion: 7}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review(ctx, scope, EvaluationReview{}); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if gateway.calls != 3 || gateway.scope != scope {
		t.Fatal("audit scope drift or mutation forwarded")
	}
	for _, ctx := range []context.Context{context.Background(), authz.WithSnapshot(context.Background(), &authz.Snapshot{})} {
		if _, err := service.ListCandidates(ctx, scope); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatal(err)
		}
		if _, err := service.GetCandidate(ctx, scope, CandidateQuery{CandidateID: "candidate:1", ExpectedVersion: 7}); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatal(err)
		}
	}
	if gateway.calls != 3 {
		t.Fatal("revoked audit reached gateway")
	}
}
func TestCandidateQueriesRequireValidVersionIdentityAndTrustedScope(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	for _, q := range []CandidateQuery{{CandidateID: "candidate:1"}, {CandidateID: "bad id", ExpectedVersion: 1}} {
		if _, err := service.GetCandidate(auditContext(), managementScope(), q); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	scope := managementScope()
	scope.OperatorUserID = 0
	if _, err := service.ListCandidates(auditContext(), scope); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if gateway.calls != 0 {
		t.Fatal("invalid query reached gateway")
	}
}
