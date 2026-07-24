package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	authnv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/authn/v2"
	auth "github.com/FangcunMount/iam/v2/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
)

func TestInjectUserContextIncludesSessionAndMetadata(t *testing.T) {
	interceptor := &IAMAuthInterceptor{}
	now := time.Now().UTC()
	result := &auth.VerifyResult{
		Claims: &auth.TokenClaims{
			UserID:          "1001",
			LoginIdentityID: "2001",
			TenantDomain:    "fangcun",
			OrgID:           "3001",
			SessionID:       "session-1",
			TokenID:         "token-1",
			Roles:           []string{"admin"},
			AMR:             []string{"pwd"},
			Extra: map[string]interface{}{
				"username": "alice",
			},
		},
		Metadata: &auth.VerifyMetadata{
			TokenType: authnv2.TokenType_TOKEN_TYPE_ACCESS,
			Status:    authnv2.TokenStatus_TOKEN_STATUS_VALID,
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
	if got := TenantDomainFromContext(ctx); got != "fangcun" {
		t.Fatalf("unexpected tenant_domain: %v", got)
	}
	if orgID, ok := OrgIDFromContext(ctx); ok {
		t.Fatalf("org_id should not come from JWT claims, got %d", orgID)
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

func TestInjectedUserContextMapsToSecurityPlanePrincipalAndOrgScope(t *testing.T) {
	interceptor := &IAMAuthInterceptor{}
	result := &auth.VerifyResult{
		Claims: &auth.TokenClaims{
			UserID:          "1001",
			LoginIdentityID: "2001",
			TenantDomain:    "fangcun",
			OrgID:           "3001",
			SessionID:       "session-1",
			TokenID:         "token-1",
			Roles:           []string{"qs:operator"},
			AMR:             []string{"pwd"},
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
	scope, ok := OrgScopeFromContext(ctx)
	if !ok {
		t.Fatal("expected org scope projection")
	}

	if principal.UserID != "1001" || principal.Username != "alice" {
		t.Fatalf("unexpected principal projection: %#v", principal)
	}
	if principal.Source != securityplane.PrincipalSourceGRPCJWT {
		t.Fatalf("principal source = %q, want grpc_jwt", principal.Source)
	}
	if scope.HasOrgID || scope.OrgID != 0 || scope.TenantDomain != "fangcun" {
		t.Fatalf("org scope = %#v, want tenant without JWT org", scope)
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

func TestServiceIdentityFromComponentBaseMTLSContextUsesCanonicalCollectionIdentity(t *testing.T) {
	ctx := basegrpc.ContextWithServiceIdentity(context.Background(), &basegrpc.ServiceIdentity{
		ServiceName: serviceidentity.CollectionServerCertificateCommonName,
		CommonName:  serviceidentity.CollectionServerCertificateCommonName,
	})

	identity, ok := ServiceIdentityFromMTLSContext(ctx)
	if !ok {
		t.Fatal("expected component-base mTLS service identity projection")
	}
	if identity.ServiceID != serviceidentity.CollectionServerServiceID {
		t.Fatalf("service ID = %q, want %q", identity.ServiceID, serviceidentity.CollectionServerServiceID)
	}
	if identity.CommonName != serviceidentity.CollectionServerCertificateCommonName {
		t.Fatalf("common name = %q, want %q", identity.CommonName, serviceidentity.CollectionServerCertificateCommonName)
	}
}

func TestLoadACLConfigUsesDefaultPolicyWhenFileMissing(t *testing.T) {
	denyACL := loadACLConfig("missing-acl.yaml", "deny")
	if services := denyACL.ListServices(); len(services) != 0 {
		t.Fatalf("loaded ACL services = %v, want none when file is missing", services)
	}
	if err := denyACL.CheckAccess("collection-server", "/qs.Internal/Submit"); err == nil {
		t.Fatal("deny default policy allowed unconfigured service")
	}

	allowACL := loadACLConfig("missing-acl.yaml", "allow")
	if err := allowACL.CheckAccess("collection-server", "/qs.Internal/Submit"); err != nil {
		t.Fatalf("allow default policy rejected unconfigured service: %v", err)
	}
}

func TestLoadACLConfigLoadsCanonicalCollectionRules(t *testing.T) {
	acl := loadACLConfig("../../../configs/grpc-acl.example.yaml", "deny")
	if err := acl.CheckAccess(serviceidentity.CollectionServerCertificateCommonName, "/interpretation.ParticipantReportService/GetAssessmentReport"); err != nil {
		t.Fatalf("collection caller should be allowed: %v", err)
	}
	if err := acl.CheckAccess(serviceidentity.CollectionServerCertificateCommonName, "/assessmentmodel.AssessmentModelCatalogService/ListHotPublishedModels"); err != nil {
		t.Fatalf("collection catalog caller should be allowed: %v", err)
	}
	if err := acl.CheckAccess(serviceidentity.CollectionServerCertificateCommonName, "/evaluation.TesteeEvaluationService/ListMyAssessments"); err != nil {
		t.Fatalf("collection evaluation caller should be allowed: %v", err)
	}
	if err := acl.CheckAccess("qs-worker.svc", "/interpretation.ParticipantReportService/GetAssessmentReport"); err == nil {
		t.Fatal("non-collection caller should be denied")
	}
	if err := acl.CheckAccess(serviceidentity.CollectionServerCertificateCommonName, "/interpretation.InterpretationAutomationService/GenerateReportFromOutcome"); err == nil {
		t.Fatal("collection caller should not access unrelated service methods")
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
