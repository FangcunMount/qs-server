package modelcatalog

import (
	"context"
	"testing"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestSnapshotAuthorizerRequiresActionCapability(t *testing.T) {
	t.Parallel()
	actor := ActorContext{
		Principal: securityplane.Principal{Kind: securityplane.PrincipalKindUser},
		Scope:     securityplane.NewOrgScope("tenant", 1, true, "tenant"),
	}
	ctx := appauthz.WithSnapshot(context.Background(), &appauthz.Snapshot{
		Permissions: []appauthz.Permission{{Resource: "qs:assessment_model_definitions", Action: "update"}},
	})
	if err := (SnapshotAuthorizer{}).Authorize(ctx, actor, ActionEditDefinition, Resource{Code: "brief2"}); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if err := (SnapshotAuthorizer{}).Authorize(ctx, actor, ActionPublishCatalog, Resource{Code: "brief2"}); err == nil {
		t.Fatal("Authorize() succeeded without publication capability")
	}
}

func TestSnapshotAuthorizerRequiresServicePrincipalForRuntimeResolution(t *testing.T) {
	t.Parallel()
	ctx := appauthz.WithSnapshot(context.Background(), &appauthz.Snapshot{
		Permissions: []appauthz.Permission{{Resource: "qs:assessment_models", Action: "resolve"}},
	})
	err := (SnapshotAuthorizer{}).Authorize(ctx, ActorContext{Principal: securityplane.Principal{Kind: securityplane.PrincipalKindUser}}, ActionResolvePublished, Resource{})
	if err == nil {
		t.Fatal("Authorize() succeeded for user runtime resolver")
	}
	actor := ActorContext{Principal: securityplane.Principal{Kind: securityplane.PrincipalKindService}}
	if err := (SnapshotAuthorizer{}).Authorize(ctx, actor, ActionResolvePublished, Resource{}); err != nil {
		t.Fatalf("Authorize() service actor error = %v", err)
	}
}

func TestSnapshotAuthorizerRejectsCommandWithoutOrganizationScope(t *testing.T) {
	t.Parallel()
	ctx := appauthz.WithSnapshot(context.Background(), &appauthz.Snapshot{
		Permissions: []appauthz.Permission{{Resource: "qs:assessment_models", Action: "create"}},
	})
	err := (SnapshotAuthorizer{}).Authorize(ctx, ActorContext{Principal: securityplane.Principal{Kind: securityplane.PrincipalKindUser}}, ActionManageCatalog, Resource{})
	if err == nil {
		t.Fatal("Authorize() succeeded without organization scope")
	}
}

func TestSnapshotAuthorizerAllowsTrustedServiceCatalogCommandWithoutOrganizationScope(t *testing.T) {
	t.Parallel()
	ctx := appauthz.WithSnapshot(context.Background(), &appauthz.Snapshot{
		Permissions: []appauthz.Permission{{Resource: "qs:assessment_models", Action: "create"}},
	})
	actor := ActorContext{Principal: securityplane.Principal{
		Kind:   securityplane.PrincipalKindService,
		Source: securityplane.PrincipalSourceServiceAuth,
	}}
	if err := (SnapshotAuthorizer{}).Authorize(ctx, actor, ActionManageCatalog, Resource{}); err != nil {
		t.Fatalf("Authorize() trusted service actor error = %v", err)
	}
}

func TestSnapshotAuthorizerAllowsTrustedServiceResolvePublishedWithoutSnapshot(t *testing.T) {
	t.Parallel()
	actor := ActorContext{Principal: securityplane.Principal{
		Kind:   securityplane.PrincipalKindService,
		Source: securityplane.PrincipalSourceMTLS,
	}}
	if err := (SnapshotAuthorizer{}).Authorize(context.Background(), actor, ActionResolvePublished, Resource{}); err != nil {
		t.Fatalf("Authorize() trusted resolve without snapshot error = %v", err)
	}
	if err := (SnapshotAuthorizer{}).Authorize(context.Background(), actor, ActionReadCatalog, Resource{}); err == nil {
		t.Fatal("Authorize() trusted read_catalog without snapshot should fail")
	}
}
