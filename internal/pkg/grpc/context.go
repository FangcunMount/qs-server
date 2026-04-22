package grpc

import (
	"context"

	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
)

type authContextKey string

const (
	authContextKeyUserID       authContextKey = "user_id"
	authContextKeyAccountID    authContextKey = "account_id"
	authContextKeyTenantID     authContextKey = "tenant_id"
	authContextKeySessionID    authContextKey = "session_id"
	authContextKeyTokenID      authContextKey = "token_id"
	authContextKeyRoles        authContextKey = "roles"
	authContextKeyAMR          authContextKey = "amr"
	authContextKeyCustomClaims authContextKey = "custom_claims"
	authContextKeyUsername     authContextKey = "username"
	authContextKeyTokenMeta    authContextKey = "token_metadata"
)

func contextStringValue(ctx context.Context, key authContextKey) string {
	if ctx == nil {
		return ""
	}

	value, _ := ctx.Value(key).(string)
	return value
}

// UserIDFromContext returns the IAM user ID from a gRPC request context.
func UserIDFromContext(ctx context.Context) string {
	return contextStringValue(ctx, authContextKeyUserID)
}

// AccountIDFromContext returns the IAM account ID from a gRPC request context.
func AccountIDFromContext(ctx context.Context) string {
	return contextStringValue(ctx, authContextKeyAccountID)
}

// TenantIDFromContext returns the IAM tenant ID from a gRPC request context.
func TenantIDFromContext(ctx context.Context) string {
	return contextStringValue(ctx, authContextKeyTenantID)
}

// SessionIDFromContext returns the IAM session ID from a gRPC request context.
func SessionIDFromContext(ctx context.Context) string {
	return contextStringValue(ctx, authContextKeySessionID)
}

// TokenIDFromContext returns the IAM token ID from a gRPC request context.
func TokenIDFromContext(ctx context.Context) string {
	return contextStringValue(ctx, authContextKeyTokenID)
}

// UsernameFromContext returns the IAM username from a gRPC request context.
func UsernameFromContext(ctx context.Context) string {
	return contextStringValue(ctx, authContextKeyUsername)
}

// RolesFromContext returns the IAM roles from a gRPC request context.
func RolesFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}

	roles, _ := ctx.Value(authContextKeyRoles).([]string)
	return roles
}

// TokenMetadataFromContext returns IAM token metadata from a gRPC request context.
func TokenMetadataFromContext(ctx context.Context) *auth.VerifyMetadata {
	if ctx == nil {
		return nil
	}

	metadata, _ := ctx.Value(authContextKeyTokenMeta).(*auth.VerifyMetadata)
	return metadata
}
