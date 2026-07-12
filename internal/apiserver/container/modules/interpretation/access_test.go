package interpretation

import (
	"context"
	"testing"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	operations "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/operations"
)

func TestOperationsAccessRequiresAuditCapabilityAndSameOrganization(t *testing.T) {
	adapter := operationsAccessAdapter{}
	actor := operations.Actor{OrgID: 7, OperatorUserID: 9}
	ctx := authzapp.WithSnapshot(context.Background(), &authzapp.Snapshot{Permissions: []authzapp.Permission{{Resource: "qs:interpretation_reports", Action: "audit"}}})
	if err := adapter.AuthorizeAudit(ctx, actor, 7); err != nil {
		t.Fatal(err)
	}
	if err := adapter.AuthorizeAudit(ctx, actor, 8); err == nil {
		t.Fatal("expected cross-organization denial")
	}
	if err := adapter.AuthorizeAudit(context.Background(), actor, 7); err == nil {
		t.Fatal("expected missing capability denial")
	}
}
