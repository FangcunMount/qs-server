package middleware

import (
	"reflect"
	"testing"
	"time"

	authnv3 "github.com/FangcunMount/iam/v5/api/grpc/iam/authn/v3"
	auth "github.com/FangcunMount/iam/v5/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestBuildUserClaimsIncludesSessionAndMetadata(t *testing.T) {
	now := time.Now().UTC()
	result := &auth.VerifyResult{
		Claims: &auth.TokenClaims{
			UserID:          "1001",
			LoginIdentityID: "2001",

			OrgID:     "3001",
			SessionID: "session-1",
			TokenID:   "token-1",
			Roles:     []string{"admin"},
			AMR:       []string{"pwd"},
		},
		Metadata: &auth.VerifyMetadata{
			TokenType: authnv3.TokenType_TOKEN_TYPE_ACCESS,
			Status:    authnv3.TokenStatus_TOKEN_STATUS_VALID,
			IssuedAt:  now,
			ExpiresAt: now.Add(time.Hour),
		},
	}

	claims, _ := buildUserClaims(result)
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
	if claims.OrgID != "3001" {
		t.Fatalf("unexpected org id: %s", claims.OrgID)
	}
	if claims.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %s", claims.SessionID)
	}
	if claims.TokenID != "token-1" {
		t.Fatalf("unexpected token id: %s", claims.TokenID)
	}
	if _, exists := reflect.TypeOf(*claims).FieldByName("Roles"); exists {
		t.Fatal("UserClaims must not expose JWT roles")
	}
	if claims.Metadata == nil {
		t.Fatal("expected metadata")
	}
	if claims.Metadata.Status != authnv3.TokenStatus_TOKEN_STATUS_VALID {
		t.Fatalf("unexpected metadata status: %v", claims.Metadata.Status)
	}
}

func TestBuildUserClaimsIgnoresExtraIdentityAliases(t *testing.T) {
	for _, tc := range []struct {
		name               string
		user, account, org string
		want               [3]string
	}{
		{name: "aliases alone", want: [3]string{}},
		{name: "canonical fields win", user: "1001", account: "2001", org: "3001", want: [3]string{"1001", "2001", "3001"}},
		{name: "canonical user with optional scope absent", user: "1001", want: [3]string{"1001", "", ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &auth.VerifyResult{Claims: &auth.TokenClaims{UserID: tc.user, LoginIdentityID: tc.account, OrgID: tc.org, SessionID: "sid", TokenID: "jti", Extra: map[string]interface{}{"user_id": "4001", "uid": 4002, "sub": "4003", "account_id": "6001", "accountId": 6002, "aid": "6003", "org_id": "5001", "organization_id": 5002}}}
			claims, sources := buildUserClaims(result)
			if got := [3]string{claims.UserID, claims.AccountID, claims.OrgID}; got != tc.want {
				t.Fatalf("mapped %v want %v", got, tc.want)
			}
			for i, v := range tc.want {
				want := "canonical"
				if v == "" {
					want = "missing"
				}
				if sources[i] != want {
					t.Fatalf("source[%d]=%q want %q", i, sources[i], want)
				}
			}
			if claims.SessionID != "sid" || claims.TokenID != "jti" {
				t.Fatal("session identity changed")
			}
		})
	}
}

func TestUserClaimsMapToSecurityPlaneOrgScope(t *testing.T) {
	result := &auth.VerifyResult{
		Claims: &auth.TokenClaims{
			UserID:          "1001",
			LoginIdentityID: "2001",

			OrgID:     "3001",
			SessionID: "session-1",
			TokenID:   "token-1",
			Roles:     []string{"qs:operator"},
			AMR:       []string{"pwd"},
		},
	}

	claims, _ := buildUserClaims(result)
	scope := securityplane.NewOrgScope(3001, true)

	if claims.UserID != "1001" || claims.AccountID != "2001" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if !scope.HasOrgID || scope.OrgID != 3001 {
		t.Fatalf("org scope = %#v, want fangcun org 3001", scope)
	}
}

func TestNormalizeVerifyOptionsPreservesForceRemoteAndForcesMetadata(t *testing.T) {
	opts := normalizeVerifyOptions(&auth.VerifyOptions{
		ForceRemote:       true,
		AllowedTokenTypes: []authnv3.TokenType{authnv3.TokenType(3)},
	})
	if !opts.ForceRemote {
		t.Fatal("expected ForceRemote to be preserved")
	}
	if !opts.IncludeMetadata {
		t.Fatal("expected IncludeMetadata to be forced on")
	}
	if len(opts.AllowedTokenTypes) != 1 || opts.AllowedTokenTypes[0] != authnv3.TokenType_TOKEN_TYPE_ACCESS {
		t.Fatalf("AllowedTokenTypes = %v, want access only", opts.AllowedTokenTypes)
	}
}
