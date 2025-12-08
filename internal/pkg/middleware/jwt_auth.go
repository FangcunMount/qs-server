package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	authnv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/authn/v1"
	sdk "github.com/FangcunMount/iam-contracts/pkg/sdk"
)

// UserClaimsContextKey 用户声明上下文键
type UserClaimsContextKey struct{}

// UserClaims 简化的用户声明
type UserClaims struct {
	UserID   string
	TenantID string
	Roles    []string
}

// JWTAuthMiddleware JWT 认证中间件
func JWTAuthMiddleware(client *sdk.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 提取 Token
		token := extractToken(c)
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "missing or invalid authorization token",
			})
			c.Abort()
			return
		}

		// 验证 Token
		resp, err := client.Auth().VerifyToken(c.Request.Context(), &authnv1.VerifyTokenRequest{
			AccessToken: token,
		})
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": fmt.Sprintf("token verification failed: %v", err),
			})
			c.Abort()
			return
		}

		if !resp.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token",
			})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		tokenClaims := resp.GetClaims()
		if tokenClaims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token claims",
			})
			c.Abort()
			return
		}

		// 从 Attributes 中提取角色（如果存在）
		roles := make([]string, 0)
		if attrs := tokenClaims.GetAttributes(); attrs != nil {
			if rolesStr, ok := attrs["roles"]; ok {
				roles = strings.Split(rolesStr, ",")
			}
		}

		claims := &UserClaims{
			UserID:   tokenClaims.GetUserId(),
			TenantID: tokenClaims.GetTenantId(),
			Roles:    roles,
		}

		c.Set("user_claims", claims)
		ctx := context.WithValue(c.Request.Context(), UserClaimsContextKey{}, claims)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// OptionalJWTAuthMiddleware 可选的 JWT 认证中间件
func OptionalJWTAuthMiddleware(client *sdk.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 提取 Token
		token := extractToken(c)
		if token == "" {
			// Token 缺失，继续执行但不设置用户信息
			c.Next()
			return
		}

		// 验证 Token
		resp, err := client.Auth().VerifyToken(c.Request.Context(), &authnv1.VerifyTokenRequest{
			AccessToken: token,
		})
		if err != nil || !resp.Valid {
			// Token 无效，继续执行但不设置用户信息
			c.Next()
			return
		}

		// 将用户信息存入上下文
		tokenClaims := resp.GetClaims()
		if tokenClaims == nil {
			// Token 无效，继续执行但不设置用户信息
			c.Next()
			return
		}

		// 从 Attributes 中提取角色（如果存在）
		roles := make([]string, 0)
		if attrs := tokenClaims.GetAttributes(); attrs != nil {
			if rolesStr, ok := attrs["roles"]; ok {
				roles = strings.Split(rolesStr, ",")
			}
		}

		claims := &UserClaims{
			UserID:   tokenClaims.GetUserId(),
			TenantID: tokenClaims.GetTenantId(),
			Roles:    roles,
		}

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
