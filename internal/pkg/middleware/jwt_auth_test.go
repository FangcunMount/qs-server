package middleware

import (
	"testing"
	"time"

	authnv2 "github.com/FangcunMount/iam/v2/api/grpc/iam/authn/v2"
	auth "github.com/FangcunMount/iam/v2/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestBuildUserClaimsIncludesSessionAndMetadata(t *testing.T) {
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
		},
		Metadata: &auth.VerifyMetadata{
			TokenType: authnv2.TokenType_TOKEN_TYPE_ACCESS,
			Status:    authnv2.TokenStatus_TOKEN_STATUS_VALID,
			IssuedAt:  now,
			ExpiresAt: now.Add(time.Hour),
		},
	}

	claims := buildUserClaims(result)
	if claims == nil {
		t.Fatal("expected claims")
		return
	}
	if claims.UserID != "1001" {
		t.Fatalf("unexpected user id: %s", claims.UserID)
	}
	if claims.AccountID != "2001" {
		t.Fatalf("unexpected account id: %s", claims.AccountID)
	}
	if claims.TenantDomain != "fangcun" {
		t.Fatalf("unexpected tenant domain: %s", claims.TenantDomain)
	}
	if claims.OrgID != "3001" {
		t.Fatalf("unexpected org id: %s", claims.OrgID)
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
	if claims.Metadata.Status != authnv2.TokenStatus_TOKEN_STATUS_VALID {
		t.Fatalf("unexpected metadata status: %v", claims.Metadata.Status)
	}
}

func TestBuildUserClaimsFallsBackToExtraIDs(t *testing.T) {
	result := &auth.VerifyResult{
		Claims: &auth.TokenClaims{
			SessionID: "session-2",
			TokenID:   "token-2",
			Extra: map[string]interface{}{
				"user_id":    "4001",
				"org_id":     "5001",
				"tenant_id":  "fangcun",
				"account_id": "6001",
			},
		},
	}

	claims := buildUserClaims(result)
	if claims == nil {
		t.Fatal("expected claims")
		return
	}
	if claims.UserID != "4001" {
		t.Fatalf("unexpected fallback user id: %s", claims.UserID)
	}
	if claims.TenantDomain != "fangcun" {
		t.Fatalf("unexpected fallback tenant domain: %s", claims.TenantDomain)
	}
	if claims.OrgID != "5001" {
		t.Fatalf("unexpected fallback org id: %s", claims.OrgID)
	}
	if claims.AccountID != "6001" {
		t.Fatalf("unexpected fallback account id: %s", claims.AccountID)
	}
}

func TestUserClaimsMapToSecurityPlaneOrgScope(t *testing.T) {
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
		},
	}

	claims := buildUserClaims(result)
	scope := securityplane.NewOrgScope(claims.TenantDomain, 3001, true, "")

	if claims.UserID != "1001" || claims.AccountID != "2001" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if !scope.HasOrgID || scope.OrgID != 3001 || scope.TenantDomain != "fangcun" {
		t.Fatalf("org scope = %#v, want fangcun org 3001", scope)
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
