package grpc

import (
	"context"
	"testing"
	"time"

	authnv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/authn/v1"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
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

	if got := ctx.Value("user_id"); got != "1001" {
		t.Fatalf("unexpected user_id: %v", got)
	}
	if got := ctx.Value("account_id"); got != "2001" {
		t.Fatalf("unexpected account_id: %v", got)
	}
	if got := ctx.Value("tenant_id"); got != "3001" {
		t.Fatalf("unexpected tenant_id: %v", got)
	}
	if got := ctx.Value("session_id"); got != "session-1" {
		t.Fatalf("unexpected session_id: %v", got)
	}
	if got := ctx.Value("token_id"); got != "token-1" {
		t.Fatalf("unexpected token_id: %v", got)
	}
	if got := ctx.Value("username"); got != "alice" {
		t.Fatalf("unexpected username: %v", got)
	}
	if got := ctx.Value("token_metadata"); got == nil {
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
