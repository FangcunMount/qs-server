package middleware

import (
	"testing"
	"time"

	authnv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/authn/v1"
	auth "github.com/FangcunMount/iam-contracts/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestBuildUserClaimsIncludesSessionAndMetadata(t *testing.T) {
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
		},
		Metadata: &auth.VerifyMetadata{
			TokenType: authnv1.TokenType_TOKEN_TYPE_ACCESS,
			Status:    authnv1.TokenStatus_TOKEN_STATUS_VALID,
			IssuedAt:  now,
			ExpiresAt: now.Add(time.Hour),
		},
	}

	claims := buildUserClaims(result)
	if claims == nil {
		t.Fatal("expected claims")
	}
	if claims.UserID != "1001" {
		t.Fatalf("unexpected user id: %s", claims.UserID)
	}
	if claims.AccountID != "2001" {
		t.Fatalf("unexpected account id: %s", claims.AccountID)
	}
	if claims.TenantID != "3001" {
		t.Fatalf("unexpected tenant id: %s", claims.TenantID)
	}
	if claims.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %s", claims.SessionID)
	}
	if claims.TokenID != "token-1" {
		t.Fatalf("unexpected token id: %s", claims.TokenID)
	}
	if claims.Metadata == nil {
		t.Fatal("expected metadata")
	}
	if claims.Metadata.Status != authnv1.TokenStatus_TOKEN_STATUS_VALID {
		t.Fatalf("unexpected metadata status: %v", claims.Metadata.Status)
	}
}

func TestBuildUserClaimsFallsBackToExtraIDs(t *testing.T) {
	result := &auth.VerifyResult{
		Claims: &auth.TokenClaims{
			SessionID: "session-2",
			TokenID:   "token-2",
			Extra: map[string]interface{}{
				"user_id": "4001",
				"org_id":  "5001",
			},
		},
	}

	claims := buildUserClaims(result)
	if claims == nil {
		t.Fatal("expected claims")
	}
	if claims.UserID != "4001" {
		t.Fatalf("unexpected fallback user id: %s", claims.UserID)
	}
	if claims.TenantID != "5001" {
		t.Fatalf("unexpected fallback tenant id: %s", claims.TenantID)
	}
}

func TestUserClaimsMapToSecurityPlanePrincipalAndTenantScope(t *testing.T) {
	result := &auth.VerifyResult{
		Claims: &auth.TokenClaims{
			UserID:    "1001",
			AccountID: "2001",
			TenantID:  "3001",
			SessionID: "session-1",
			TokenID:   "token-1",
			Roles:     []string{"qs:operator"},
			AMR:       []string{"pwd"},
		},
	}

	claims := buildUserClaims(result)
	principal := securityplane.Principal{
		Kind:      securityplane.PrincipalKindUser,
		Source:    securityplane.PrincipalSourceHTTPJWT,
		UserID:    claims.UserID,
		AccountID: claims.AccountID,
		TenantID:  claims.TenantID,
		SessionID: claims.SessionID,
		TokenID:   claims.TokenID,
		Roles:     claims.Roles,
		AMR:       claims.AMR,
	}
	scope := securityplane.NewTenantScope(claims.TenantID, "")

	if principal.UserID != "1001" || principal.AccountID != "2001" || principal.TokenID != "token-1" {
		t.Fatalf("unexpected principal projection: %#v", principal)
	}
	if principal.Kind != securityplane.PrincipalKindUser || principal.Source != securityplane.PrincipalSourceHTTPJWT {
		t.Fatalf("unexpected principal kind/source: %#v", principal)
	}
	if len(principal.RoleNames()) != 1 || principal.RoleNames()[0] != "qs:operator" {
		t.Fatalf("principal roles = %#v, want [qs:operator]", principal.RoleNames())
	}
	if !scope.HasNumericOrg || scope.OrgID != 3001 {
		t.Fatalf("tenant scope = %#v, want numeric org 3001", scope)
	}
}

func TestNormalizeVerifyOptionsPreservesForceRemoteAndForcesMetadata(t *testing.T) {
	opts := normalizeVerifyOptions(&auth.VerifyOptions{ForceRemote: true})
	if !opts.ForceRemote {
		t.Fatal("expected ForceRemote to be preserved")
	}
	if !opts.IncludeMetadata {
		t.Fatal("expected IncludeMetadata to be forced on")
	}
}
