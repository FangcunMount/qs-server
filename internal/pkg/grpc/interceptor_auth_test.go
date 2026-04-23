package grpc

import (
	"context"
	"testing"
	"time"

	authnv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/authn/v1"
	auth "github.com/FangcunMount/iam-contracts/pkg/sdk/auth/verifier"
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

func TestBuildVerifyOptionsHonorsForceRemote(t *testing.T) {
	opts := buildVerifyOptions(true)
	if !opts.ForceRemote {
		t.Fatal("expected ForceRemote to be enabled")
	}
	if !opts.IncludeMetadata {
		t.Fatal("expected IncludeMetadata to be enabled")
	}
}
