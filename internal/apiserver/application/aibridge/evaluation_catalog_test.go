package aibridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

func (g *managementStub) ListEvaluations(_ context.Context, scope DraftScope, query EvaluationCatalogQuery) (EvaluationCatalogPage, error) {
	g.calls++
	g.scope = EvaluationScope{OrganizationID: scope.OrganizationID, OperatorUserID: scope.OperatorUserID}
	return EvaluationCatalogPage{Items: []EvaluationSummary{}}, nil
}

func TestEvaluationCatalogChecksReadPermissionOnEveryCall(t *testing.T) {
	gateway := &managementStub{}
	service := &EvaluationAdministration{Gateway: gateway}
	scope := DraftScope{OrganizationID: 7, OperatorUserID: 42}
	if _, err := service.List(context.Background(), scope, EvaluationCatalogQuery{}); !errors.Is(err, ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if _, err := service.List(auditContext(), scope, EvaluationCatalogQuery{}); err != nil || gateway.calls != 1 || gateway.scope.OrganizationID != 7 || gateway.scope.OperatorUserID != 42 {
		t.Fatal(err, gateway)
	}
	if _, err := service.List(context.Background(), scope, EvaluationCatalogQuery{}); !errors.Is(err, ErrGovernanceDenied) || gateway.calls != 1 {
		t.Fatal("revoked read forwarded", err)
	}
	for _, query := range []EvaluationCatalogQuery{{Status: "failed"}, {Limit: -1}, {Limit: 101}, {Cursor: "bad"}} {
		if _, err := service.List(auditContext(), scope, query); !errors.Is(err, ErrInvalid) || gateway.calls != 1 {
			t.Fatal(query, err)
		}
	}
	if _, err := service.List(auditContext(), DraftScope{}, EvaluationCatalogQuery{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestEvaluationCatalogCursorBindsFilterAndScope(t *testing.T) {
	raw, _ := json.Marshal([]any{1, "evaluation", 7, "requested", "2026-09-13T08:00:00.123456+00:00", "00000000-0000-4000-8000-000000000001"})
	query := EvaluationCatalogQuery{Status: "requested", Limit: 20, Cursor: base64.URLEncoding.EncodeToString(raw)}
	if !query.Valid(7) || query.Valid(8) {
		t.Fatal("scope not bound")
	}
	query.Status = ""
	if query.Valid(7) {
		t.Fatal("filter not bound")
	}
}
