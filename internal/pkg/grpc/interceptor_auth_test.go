package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	authnv1 "github.com/FangcunMount/iam/api/grpc/iam/authn/v1"
	auth "github.com/FangcunMount/iam/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestInjectUserContextIncludesSessionAndMetadata(t *testing.T) {
	interceptor := &IAMAuthInterceptor{}
	now := time.Now().UTC()
	result := &auth.VerifyResult{
		Claims: &auth.TokenClaims{
			UserID:    "1001",
			AccountID: "2001",
			TenantID:  "3001",
			SessionID: "session-1",
			TokenID:   "token-1",
			Roles:     []string{"admin"},
			AMR:       []string{"pwd"},
			Extra: map[string]interface{}{
				"username": "alice",
			},
		},
		Metadata: &auth.VerifyMetadata{
			TokenType: authnv1.TokenType_TOKEN_TYPE_ACCESS,
			Status:    authnv1.TokenStatus_TOKEN_STATUS_VALID,
			IssuedAt:  now,
			ExpiresAt: now.Add(time.Hour),
		},
	}

	ctx := interceptor.injectUserContext(context.Background(), result)

	if got := UserIDFromContext(ctx); got != "1001" {
		t.Fatalf("unexpected user_id: %v", got)
	}
	if got := AccountIDFromContext(ctx); got != "2001" {
		t.Fatalf("unexpected account_id: %v", got)
	}
	if got := TenantIDFromContext(ctx); got != "3001" {
		t.Fatalf("unexpected tenant_id: %v", got)
	}
	if got := SessionIDFromContext(ctx); got != "session-1" {
		t.Fatalf("unexpected session_id: %v", got)
	}
	if got := TokenIDFromContext(ctx); got != "token-1" {
		t.Fatalf("unexpected token_id: %v", got)
	}
	if got := UsernameFromContext(ctx); got != "alice" {
		t.Fatalf("unexpected username: %v", got)
	}
	if got := TokenMetadataFromContext(ctx); got == nil {
		t.Fatal("expected token_metadata")
	}
}

func TestInjectedUserContextMapsToSecurityPlanePrincipalAndTenantScope(t *testing.T) {
	interceptor := &IAMAuthInterceptor{}
	result := &auth.VerifyResult{
		Claims: &auth.TokenClaims{
			UserID:    "1001",
			AccountID: "2001",
			TenantID:  "3001",
			SessionID: "session-1",
			TokenID:   "token-1",
			Roles:     []string{"qs:operator"},
			AMR:       []string{"pwd"},
			Extra: map[string]interface{}{
				"username": "alice",
			},
		},
	}

	ctx := interceptor.injectUserContext(context.Background(), result)
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		t.Fatal("expected principal projection")
	}
	scope, ok := TenantScopeFromContext(ctx)
	if !ok {
		t.Fatal("expected tenant scope projection")
	}

	if principal.UserID != "1001" || principal.Username != "alice" {
		t.Fatalf("unexpected principal projection: %#v", principal)
	}
	if principal.Source != securityplane.PrincipalSourceGRPCJWT {
		t.Fatalf("principal source = %q, want grpc_jwt", principal.Source)
	}
	if !scope.HasNumericOrg || scope.OrgID != 3001 {
		t.Fatalf("tenant scope = %#v, want numeric org 3001", scope)
	}
	if got := principal.AuthenticationMethods(); len(got) != 1 || got[0] != "pwd" {
		t.Fatalf("principal amr = %#v, want [pwd]", got)
	}
}

func TestVerifyIdentityMatchUsesLegacyMTLSIdentityMapContract(t *testing.T) {
	interceptor := &IAMAuthInterceptor{}
	claims := &auth.TokenClaims{Extra: map[string]interface{}{"service_id": "qs-worker"}}
	ctx := context.WithValue(context.Background(), mtlsIdentityKey, map[string]interface{}{
		"common_name": "qs-worker.svc",
	})

	if err := interceptor.verifyIdentityMatch(ctx, claims); err != nil {
		t.Fatalf("verifyIdentityMatch() unexpected error: %v", err)
	}
	identity, ok := ServiceIdentityFromMTLSContext(ctx)
	if !ok {
		t.Fatal("expected mTLS service identity projection")
	}
	if identity.Source != securityplane.ServiceIdentitySourceMTLS || identity.ServiceID != "qs-worker" || identity.CommonName != "qs-worker.svc" {
		t.Fatalf("identity = %#v, want qs-worker mTLS identity", identity)
	}

	mismatch := context.WithValue(context.Background(), mtlsIdentityKey, map[string]interface{}{
		"common_name": "collection-server.svc",
	})
	err := interceptor.verifyIdentityMatch(mismatch, claims)
	if err == nil || !strings.Contains(err.Error(), "service_id mismatch") {
		t.Fatalf("verifyIdentityMatch() error = %v, want service_id mismatch", err)
	}
}

func TestLoadACLConfigUsesDefaultPolicyOnlyUntilFileLoaderExists(t *testing.T) {
	denyACL := loadACLConfig("acl.yaml", "deny")
	if services := denyACL.ListServices(); len(services) != 0 {
		t.Fatalf("loaded ACL services = %v, want none while file loader is not implemented", services)
	}
	if err := denyACL.CheckAccess("collection-server", "/qs.Internal/Submit"); err == nil {
		t.Fatal("deny default policy allowed unconfigured service")
	}

	allowACL := loadACLConfig("acl.yaml", "allow")
	if err := allowACL.CheckAccess("collection-server", "/qs.Internal/Submit"); err != nil {
		t.Fatalf("allow default policy rejected unconfigured service: %v", err)
	}
}

func TestBuildVerifyOptionsHonorsForceRemote(t *testing.T) {
	opts := buildVerifyOptions(true)
	if !opts.ForceRemote {
		t.Fatal("expected ForceRemote to be enabled")
	}
	if !opts.IncludeMetadata {
		t.Fatal("expected IncludeMetadata to be enabled")
	}
}
