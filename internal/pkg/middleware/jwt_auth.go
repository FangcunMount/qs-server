package middleware

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/gin-gonic/gin"

	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
)

// UserClaimsContextKey 用户声明上下文键
type UserClaimsContextKey struct{}

// UserClaims 简化的用户声明
type UserClaims struct {
	UserID   string
	TenantID string
	Roles    []string
}

// JWTAuthMiddleware JWT 认证中间件（使用 SDK TokenVerifier 本地 JWKS 验签）
func JWTAuthMiddleware(verifier *auth.TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
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
		token := extractToken(c)
		if token == "" {
			logger.L(c.Request.Context()).Errorw("JWTAuthMiddleware missing or invalid authorization token", "error", "missing or invalid authorization token")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "missing or invalid authorization token",
			})
			c.Abort()
			return
		}

		// 使用 SDK TokenVerifier 验证（本地 JWKS 优先，远程降级）
		result, err := verifier.Verify(c.Request.Context(), token, nil)
		if err != nil {
			logger.L(c.Request.Context()).Errorw("JWTAuthMiddleware token verification failed", "error", fmt.Sprintf("token verification failed: %v", err))
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": fmt.Sprintf("token verification failed: %v", err),
			})
			c.Abort()
			return
		}

		if !result.Valid {
			logger.L(c.Request.Context()).Errorw("JWTAuthMiddleware invalid token", "error", "invalid token")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token",
			})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		tokenClaims := result.Claims
		logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware tokenClaims", "tokenClaims", tokenClaims)
		if tokenClaims == nil {
			logger.L(c.Request.Context()).Errorw("JWTAuthMiddleware invalid token claims", "error", "invalid token claims")
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token claims",
			})
			c.Abort()
			return
		}

		claims := &UserClaims{
			UserID:   resolveUserID(tokenClaims.UserID, tokenClaims.Extra),
			TenantID: resolveTenantID(tokenClaims.TenantID, tokenClaims.Extra),
			Roles:    tokenClaims.Roles,
		}
		logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware claims", "claims", claims)
		logJWTClaimMapping(c, tokenClaims, claims)

		c.Set("user_claims", claims)
		ctx := context.WithValue(c.Request.Context(), UserClaimsContextKey{}, claims)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// OptionalJWTAuthMiddleware 可选的 JWT 认证中间件（使用 SDK TokenVerifier）
func OptionalJWTAuthMiddleware(verifier *auth.TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware started", "path", c.Request.URL.Path, "method", c.Request.Method)
		// 提取 Token
		token := extractToken(c)
		logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware token", "token", token)
		if token == "" {
			logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware token is empty", "path", c.Request.URL.Path, "method", c.Request.Method)
			// Token 缺失，继续执行但不设置用户信息
			c.Next()
			return
		}

		// 检查 verifier 是否可用
		if verifier == nil {
			logger.L(c.Request.Context()).Debugw("JWTAuthMiddleware verifier is nil", "path", c.Request.URL.Path, "method", c.Request.Method)
			c.Next()
			return
		}

		// 使用 SDK TokenVerifier 验证
		result, err := verifier.Verify(c.Request.Context(), token, nil)
		logger.L(c.Request.Context()).Debugw("OptionalJWTAuthMiddleware result", "result", result)
		logger.L(c.Request.Context()).Debugw("OptionalJWTAuthMiddleware err", "err", err)
		if err != nil || !result.Valid {
			// Token 无效，继续执行但不设置用户信息
			c.Next()
			return
		}

		// 将用户信息存入上下文
		tokenClaims := result.Claims
		logger.L(c.Request.Context()).Debugw("OptionalJWTAuthMiddleware tokenClaims", "tokenClaims", tokenClaims)
		if tokenClaims == nil {
			// Token 无效，继续执行但不设置用户信息
			c.Next()
			return
		}

		claims := &UserClaims{
			UserID:   resolveUserID(tokenClaims.UserID, tokenClaims.Extra),
			TenantID: resolveTenantID(tokenClaims.TenantID, tokenClaims.Extra),
			Roles:    tokenClaims.Roles,
		}
		logJWTClaimMapping(c, tokenClaims, claims)

		c.Set("user_claims", claims)
		ctx := context.WithValue(c.Request.Context(), UserClaimsContextKey{}, claims)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// RequireRole 要求特定角色的中间件
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetUserClaims(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			c.Abort()
			return
		}

		if !hasRole(claims.Roles, role) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": fmt.Sprintf("role '%s' required", role),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAnyRole 要求任意一个角色的中间件
func RequireAnyRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetUserClaims(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			c.Abort()
			return
		}

		for _, role := range roles {
			if hasRole(claims.Roles, role) {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"error": fmt.Sprintf("one of roles %v required", roles),
		})
		c.Abort()
	}
}

// 辅助函数

// extractToken 从请求中提取 Token
func extractToken(c *gin.Context) string {
	// 1. Authorization Header
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		// Bearer token
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimPrefix(authHeader, "Bearer ")
		}
		// 直接是 token
		return authHeader
	}

	// 2. Query Parameter
	if token := c.Query("access_token"); token != "" {
		return token
	}

	// 3. Cookie
	if token, err := c.Cookie("access_token"); err == nil && token != "" {
		return token
	}

	return ""
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

// GetTenantID 从上下文获取租户 ID
func GetTenantID(c *gin.Context) string {
	claims := GetUserClaims(c)
	if claims != nil {
		return claims.TenantID
	}
	return ""
}

// GetRoles 从上下文获取角色列表
func GetRoles(c *gin.Context) []string {
	claims := GetUserClaims(c)
	if claims != nil {
		return claims.Roles
	}
	return nil
}

// HasRole 检查用户是否拥有特定角色
func HasRole(c *gin.Context, role string) bool {
	claims := GetUserClaims(c)
	if claims == nil {
		return false
	}
	return hasRole(claims.Roles, role)
}

// hasRole 检查角色列表中是否包含指定角色
func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// resolveTenantID 优先使用 SDK 的 TenantID，缺失时从 Extra 兼容（IAM 常把自定义声明放在 Extra）。
func resolveTenantID(tenantID string, extra map[string]interface{}) string {
	if s := strings.TrimSpace(tenantID); s != "" {
		return s
	}
	if len(extra) == 0 {
		return ""
	}
	for _, key := range []string{"tenant_id", "org_id", "organization_id", "tid"} {
		if v, ok := extra[key]; ok {
			if s := claimValueToString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// logJWTClaimMapping 在 tenant/user 映射后仍为空时打 Debug（只记录 Extra 的键名，不记录值）。
func logJWTClaimMapping(c *gin.Context, raw *auth.TokenClaims, mapped *UserClaims) {
	if mapped == nil {
		logger.L(c.Request.Context()).Debugw("jwt claims mapped is nil", "path", c.Request.URL.Path, "method", c.Request.Method)
		return
	}
	if mapped.TenantID != "" && mapped.UserID != "" {
		logger.L(c.Request.Context()).Debugw("jwt claims mapped with tenant_id and user_id", "path", c.Request.URL.Path, "method", c.Request.Method, "mapped_tenant_id", mapped.TenantID, "mapped_user_id", mapped.UserID)
		return
	}
	keys := sortedExtraKeys(raw)
	logger.L(c.Request.Context()).Debugw("jwt claims mapped with missing tenant_id or user_id",
		"path", c.Request.URL.Path,
		"method", c.Request.Method,
		"mapped_tenant_empty", mapped.TenantID == "",
		"mapped_user_empty", mapped.UserID == "",
		"raw_tenant_empty", strings.TrimSpace(raw.TenantID) == "",
		"raw_user_empty", strings.TrimSpace(raw.UserID) == "",
		"extra_keys", keys,
	)
}

func sortedExtraKeys(raw *auth.TokenClaims) []string {
	if raw == nil || len(raw.Extra) == 0 {
		return nil
	}
	keys := make([]string, 0, len(raw.Extra))
	for k := range raw.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// resolveUserID 优先使用标准 UserID 字段，缺失时从 Extra 兼容提取。
func resolveUserID(userID string, extra map[string]interface{}) string {
	if userID != "" {
		return userID
	}
	if len(extra) == 0 {
		return ""
	}
	for _, key := range []string{"user_id", "uid", "sub"} {
		if v, ok := extra[key]; ok {
			if s := claimValueToString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func claimValueToString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	case int:
		return strconv.Itoa(x)
	case int8, int16, int32, int64:
		return strconv.FormatInt(reflect.ValueOf(x).Int(), 10)
	case uint, uint8, uint16, uint32, uint64:
		return strconv.FormatUint(reflect.ValueOf(x).Uint(), 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}
