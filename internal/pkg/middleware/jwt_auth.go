package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	authnv3 "github.com/FangcunMount/iam/v5/api/grpc/iam/authn/v3"
	"github.com/FangcunMount/qs-server/internal/pkg/authobservability"
	"github.com/gin-gonic/gin"

	auth "github.com/FangcunMount/iam/v5/pkg/sdk/auth/verifier"
)

// UserClaimsContextKey 用户声明上下文键
type UserClaimsContextKey struct{}

// UserClaims 简化的用户声明
type UserClaims struct {
	UserID    string
	AccountID string // IAM canonical LoginIdentityID; retains the existing context key.
	OrgID     string // IAM 业务组织 ID（JWT org_id 透传）
	SessionID string
	TokenID   string
	AMR       []string
	Metadata  *auth.VerifyMetadata
}

func normalizeVerifyOptions(opts *auth.VerifyOptions) *auth.VerifyOptions {
	if opts == nil {
		return &auth.VerifyOptions{
			IncludeMetadata:   true,
			AllowedTokenTypes: []authnv3.TokenType{authnv3.TokenType_TOKEN_TYPE_ACCESS},
		}
	}
	merged := *opts
	merged.IncludeMetadata = true
	// HTTP 用户入口的 Token Profile 固定为 Access Token，调用方不得放宽为 Service Token。
	merged.AllowedTokenTypes = []authnv3.TokenType{authnv3.TokenType_TOKEN_TYPE_ACCESS}
	return &merged
}

// JWTAuthMiddlewareWithOptions JWT 认证中间件（显式控制 VerifyOptions）。
func JWTAuthMiddlewareWithOptions(verifier *auth.TokenVerifier, opts *auth.VerifyOptions) gin.HandlerFunc {
	verifyOpts := normalizeVerifyOptions(opts)
	return func(c *gin.Context) {
		source, outcome := "unavailable", "rejected"
		defer func() { authobservability.RecordTokenVerification(source, outcome) }()
		logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware started", "path", c.Request.URL.Path, "method", c.Request.Method)
		// 检查 verifier 是否可用
		if verifier == nil {
			logger.L(c.Request.Context()).Errorw("JWTAuthMiddleware token verifier not configured", "error", "token verifier not configured")
			logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware token verifier not configured", "path", c.Request.URL.Path, "method", c.Request.Method)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "token verifier not configured",
			})
			c.Abort()
			return
		}

		// 提取 Token
		token, tokenSource := extractToken(c)
		source = tokenSource
		logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware token extracted", "has_token", token != "", "token_length", len(token))
		if token == "" {
			logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware token is empty", "path", c.Request.URL.Path, "method", c.Request.Method)
			logger.L(c.Request.Context()).Errorw("JWTAuthMiddleware missing or invalid authorization token", "error", "missing or invalid authorization token")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "missing or invalid authorization token",
			})
			c.Abort()
			return
		}

		// 使用调用方配置的验证模式；默认本地 JWKS 验签，显式 ForceRemote 时在线校验。
		result, err := verifier.Verify(c.Request.Context(), token, verifyOpts)
		if err != nil {
			logger.L(c.Request.Context()).Errorw("JWTAuthMiddleware token verification failed", "error", err, "force_remote", verifyOpts.ForceRemote)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "token verification failed",
			})
			c.Abort()
			return
		}

		if result == nil || !result.Valid {
			logger.L(c.Request.Context()).Errorw("JWTAuthMiddleware invalid token", "error", "invalid token")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token",
			})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		tokenClaims := result.Claims
		if tokenClaims == nil {
			logger.L(c.Request.Context()).Errorw("JWTAuthMiddleware invalid token claims", "error", "invalid token claims")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token claims",
			})
			c.Abort()
			return
		}

		claims, sources := buildUserClaims(result)
		authobservability.RecordVerifiedClaimSources(sources)
		if claims.UserID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "canonical user_id claim is required"})
			c.Abort()
			return
		}
		outcome = "accepted"

		c.Set("user_claims", claims)
		if claims.Metadata != nil {
			c.Set("token_metadata", claims.Metadata)
		}
		ctx := context.WithValue(c.Request.Context(), UserClaimsContextKey{}, claims)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// 辅助函数

// extractToken 从请求中提取 Token
func extractToken(c *gin.Context) (string, string) {
	// 1. Authorization Header
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		// Bearer token
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimPrefix(authHeader, "Bearer "), "bearer"
		}
		// An explicit unsupported header is rejected; do not fall back to a
		// query or cookie token when the caller supplied Authorization.
		return "", "bare"
	}

	// 2. Query Parameter
	if token := c.Query("access_token"); token != "" {
		return token, "query"
	}

	// 3. Cookie
	if token, err := c.Cookie("access_token"); err == nil && token != "" {
		return token, "cookie"
	}

	return "", "missing"
}

// GetUserClaims 从上下文获取用户声明
func GetUserClaims(c *gin.Context) *UserClaims {
	if val, exists := c.Get("user_claims"); exists {
		if claims, ok := val.(*UserClaims); ok {
			return claims
		}
	}
	return nil
}

// GetUserIDFromContext 从标准 context.Context 获取用户 ID（uint64）
func GetUserIDFromContext(ctx context.Context) uint64 {
	if ctx == nil {
		return 0
	}
	claims, ok := ctx.Value(UserClaimsContextKey{}).(*UserClaims)
	if !ok || claims == nil || claims.UserID == "" {
		return 0
	}
	userID, err := strconv.ParseUint(claims.UserID, 10, 64)
	if err != nil {
		return 0
	}
	return userID
}

// GetUserID 从上下文获取用户 ID
func GetUserID(c *gin.Context) string {
	claims := GetUserClaims(c)
	if claims != nil {
		return claims.UserID
	}
	return ""
}

// canonicalClaim maps only a verified SDK canonical identity field. Attributes
// may contain user-defined names, but cannot supply or override an identity.
func canonicalClaim(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "missing"
	}
	return value, "canonical"
}

func buildUserClaims(result *auth.VerifyResult) (*UserClaims, [3]string) {
	if result == nil || result.Claims == nil {
		return nil, [3]string{}
	}
	tokenClaims := result.Claims
	userID, userSource := canonicalClaim(tokenClaims.UserID)
	accountID, accountSource := canonicalClaim(tokenClaims.LoginIdentityID)
	orgID, orgSource := canonicalClaim(tokenClaims.OrgID)
	return &UserClaims{
		UserID:    userID,
		AccountID: accountID,

		OrgID:     orgID,
		SessionID: tokenClaims.SessionID,
		TokenID:   tokenClaims.TokenID,
		AMR:       tokenClaims.AMR,
		Metadata:  result.Metadata,
	}, [3]string{userSource, accountSource, orgSource}
}
