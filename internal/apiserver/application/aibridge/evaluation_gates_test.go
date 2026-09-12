package aibridge

import (
	"context"
	"errors"
	"testing"
)

func (g *managementStub) PreviewEvaluationGates(ctx context.Context, scope EvaluationScope, version int64) (EvaluationGatePreview, error) {
	_, err := g.GetEvaluation(ctx, scope)
	return EvaluationGatePreview{RunID: scope.RunID, Version: version}, err
}
func TestGatePreviewRequiresCurrentAuditPermissionAndVersion(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	scope := managementScope()
	result, err := service.PreviewGates(auditContext(), scope, 7)
	if err != nil || result.Version != 7 || gateway.scope != scope {
		t.Fatal(result, err)
	}
	if _, err := service.PreviewGates(context.Background(), scope, 7); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if _, err := service.PreviewGates(auditContext(), scope, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	scope.OrganizationID = 0
	if _, err := service.PreviewGates(auditContext(), scope, 7); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if gateway.calls != 1 {
		t.Fatal("invalid or unauthorized preview forwarded")
	}
}
