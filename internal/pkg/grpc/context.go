package grpc

import (
	"context"
	"strings"

	auth "github.com/FangcunMount/iam-contracts/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
	"github.com/FangcunMount/qs-server/internal/pkg/securityprojection"
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

// AuthenticationMethodsFromContext returns IAM AMR values from a gRPC request context.
func AuthenticationMethodsFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}

	amr, _ := ctx.Value(authContextKeyAMR).([]string)
	return amr
}

// TokenMetadataFromContext returns IAM token metadata from a gRPC request context.
func TokenMetadataFromContext(ctx context.Context) *auth.VerifyMetadata {
	if ctx == nil {
		return nil
	}

	metadata, _ := ctx.Value(authContextKeyTokenMeta).(*auth.VerifyMetadata)
	return metadata
}

// PrincipalFromContext returns the Security Control Plane principal projection.
func PrincipalFromContext(ctx context.Context) (securityplane.Principal, bool) {
	if ctx == nil {
		return securityplane.Principal{}, false
	}
	userID := UserIDFromContext(ctx)
	accountID := AccountIDFromContext(ctx)
	tenantID := TenantIDFromContext(ctx)
	sessionID := SessionIDFromContext(ctx)
	tokenID := TokenIDFromContext(ctx)
	username := UsernameFromContext(ctx)
	roles := RolesFromContext(ctx)
	amr := AuthenticationMethodsFromContext(ctx)
	if userID == "" && accountID == "" && tenantID == "" && sessionID == "" && tokenID == "" && username == "" && len(roles) == 0 && len(amr) == 0 {
		return securityplane.Principal{}, false
	}
	return securityprojection.PrincipalFromInput(securityprojection.PrincipalInput{
		Kind:      securityplane.PrincipalKindUser,
		Source:    securityplane.PrincipalSourceGRPCJWT,
		UserID:    userID,
		AccountID: accountID,
		TenantID:  tenantID,
		SessionID: sessionID,
		TokenID:   tokenID,
		Username:  username,
		Roles:     roles,
		AMR:       amr,
	}), true
}

// TenantScopeFromContext returns the Security Control Plane tenant scope projection.
func TenantScopeFromContext(ctx context.Context) (securityplane.TenantScope, bool) {
	tenantID := TenantIDFromContext(ctx)
	if tenantID == "" {
		return securityplane.TenantScope{}, false
	}
	return securityprojection.TenantScopeFromTenantID(tenantID, ""), true
}

// ServiceIdentityFromMTLSContext returns the mTLS service identity projection when present.
func ServiceIdentityFromMTLSContext(ctx context.Context) (securityplane.ServiceIdentity, bool) {
	if ctx == nil {
		return securityplane.ServiceIdentity{}, false
	}
	mtlsIdentity, ok := ctx.Value(mtlsIdentityKey).(map[string]interface{})
	if !ok {
		mtlsIdentity, ok = ctx.Value("mtls.identity").(map[string]interface{})
	}
	if !ok {
		return securityplane.ServiceIdentity{}, false
	}
	commonName, _ := mtlsIdentity["common_name"].(string)
	namespace, _ := mtlsIdentity["namespace"].(string)
	serviceID := strings.TrimSuffix(commonName, ".svc")
	return securityprojection.ServiceIdentityFromInput(securityprojection.ServiceIdentityInput{
		ServiceID:  serviceID,
		Source:     securityplane.ServiceIdentitySourceMTLS,
		CommonName: commonName,
		Namespace:  namespace,
	}), true
}
