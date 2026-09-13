package aibridge

import (
	"context"
	"errors"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"testing"
)

func (g *managementStub) ListEvaluationUnknowns(ctx context.Context, s EvaluationScope, version int64) (EvaluationUnknownIndex, error) {
	_, err := g.GetEvaluation(ctx, s)
	return EvaluationUnknownIndex{RunID: s.RunID, Version: version, Status: "blocked", Executions: []EvaluationUnknownExecution{}}, err
}

func TestUnknownReadRequiresCurrentAuditScopeAndPositiveVersion(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	scope := managementScope()
	if _, err := service.ListUnknowns(auditContext(), scope, 7); err != nil {
		t.Fatal(err)
	}
	if gateway.calls != 1 || gateway.scope != scope {
		t.Fatal("scope drift")
	}
	for _, ctx := range []context.Context{context.Background(), authz.WithSnapshot(context.Background(), &authz.Snapshot{})} {
		if _, err := service.ListUnknowns(ctx, scope, 7); !errors.Is(err, ErrGovernanceDenied) {
			t.Fatal(err)
		}
	}
	for _, version := range []int64{0, -1} {
		if _, err := service.ListUnknowns(auditContext(), scope, version); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	scope.OperatorUserID = 0
	if _, err := service.ListUnknowns(auditContext(), scope, 7); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := service.Resolve(auditContext(), managementScope(), UnknownResolution{}); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if gateway.calls != 1 {
		t.Fatal("denied query reached gateway")
	}
}
