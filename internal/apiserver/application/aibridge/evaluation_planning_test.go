package aibridge

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func (g *managementStub) PrepareEvaluation(_ context.Context, scope DraftScope, _ EvaluationPlanQuery) (EvaluationPlan, error) {
	g.calls++
	g.scope = EvaluationScope{OrganizationID: scope.OrganizationID, OperatorUserID: scope.OperatorUserID}
	return EvaluationPlan{}, nil
}

func TestPrepareRequiresCurrentAuditPermissionAndCompleteReferences(t *testing.T) {
	ref := FrozenEvaluationRef{ID: "asset", Version: "v1", Fingerprint: "sha256:" + strings.Repeat("a", 64)}
	query := EvaluationPlanQuery{Suite: ref, GenerationRoute: ref, SemanticRoute: ref}
	scope := DraftScope{OrganizationID: 7, OperatorUserID: 42}
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	if _, err := service.Prepare(context.Background(), scope, query); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if _, err := service.Prepare(adminContext(), scope, query); err != nil || gateway.calls != 1 || gateway.scope.OrganizationID != 7 || gateway.scope.OperatorUserID != 42 {
		t.Fatal(err, gateway)
	}
	if _, err := service.Prepare(context.Background(), scope, query); !errors.Is(err, ErrGovernanceDenied) || gateway.calls != 1 {
		t.Fatal("revoked read reached AI", err)
	}
	invalid := query
	invalid.SemanticRoute.Fingerprint = "bad"
	if _, err := service.Prepare(adminContext(), scope, invalid); !errors.Is(err, ErrInvalid) || gateway.calls != 1 {
		t.Fatal(err)
	}
	if _, err := service.Prepare(adminContext(), DraftScope{}, query); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := (&EvaluationAdministration{}).Prepare(adminContext(), scope, query); !errors.Is(err, ErrManagementUnavailable) {
		t.Fatal(err)
	}
}
