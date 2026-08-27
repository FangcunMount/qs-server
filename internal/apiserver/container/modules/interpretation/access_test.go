package interpretation

import (
	"context"
	"testing"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	aiexplanationadministration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	operations "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/operations"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
)

func TestOperationsAccessRequiresAuditCapabilityAndSameOrganization(t *testing.T) {
	adapter := operationsAccessAdapter{}
	actor := operations.Actor{OrgID: 7, OperatorUserID: 9}
	ctx := authzapp.WithSnapshot(context.Background(), &authzapp.Snapshot{
		EffectiveRoles: []string{"qs:admin"},
		Permissions: []authzapp.Permission{{
			Resource: "qs:*:*:*", Action: "*", Mode: authzapp.AuthorizationModeUnconditional,
		}},
	})
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

func TestAIAdministrationSeparatesAuditReadFromGlobalGovernance(t *testing.T) {
	adapter := aiAdministrationAccessAdapter{}
	actor := aiexplanationadministration.Actor{OrgID: 7, OperatorUserID: 9}
	auditContext := authzapp.WithSnapshot(context.Background(), &authzapp.Snapshot{Permissions: []authzapp.Permission{{
		Resource: "qs:evaluation:collection:reports", Action: "audit", Mode: authzapp.AuthorizationModeUnconditional,
	}}})
	if err := adapter.AuthorizeRead(auditContext, actor); err != nil {
		t.Fatal(err)
	}
	if err := adapter.AuthorizeGovernance(auditContext, actor); err == nil {
		t.Fatal("audit-only actor must not mutate global AI explanation releases")
	}

	adminContext := authzapp.WithSnapshot(context.Background(), &authzapp.Snapshot{Permissions: []authzapp.Permission{{
		Resource: "qs:*:*:*", Action: "*", Mode: authzapp.AuthorizationModeUnconditional,
	}}})
	if err := adapter.AuthorizeReview(adminContext, actor, domainevaluation.ReviewRoleAssessmentSemantics); err != nil {
		t.Fatal(err)
	}
	if err := adapter.AuthorizeGovernance(adminContext, actor); err != nil {
		t.Fatal(err)
	}
	if err := adapter.AuthorizeReview(adminContext, actor, domainevaluation.ReviewRole("caller_defined")); err == nil {
		t.Fatal("unknown review role must fail closed")
	}
}
