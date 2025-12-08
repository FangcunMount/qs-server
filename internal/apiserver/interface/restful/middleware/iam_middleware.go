package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
)

// Context 键常量
const (
	// UserIDKey 用户ID（uint64）
	UserIDKey = "user_id"
	// UserIDStrKey 用户ID（string，原始值）
	UserIDStrKey = "user_id_str"
	// OrgIDKey 组织/租户ID（uint64）
	OrgIDKey = "org_id"
	// TenantIDKey 租户ID（string，原始值）
	TenantIDKey = "tenant_id"
	// RolesKey 用户角色列表
	RolesKey = "roles"
)

// UserIdentityMiddleware 用户身份解析中间件
// 将 JWT claims 中的用户信息解析并存入 context
// 依赖于 JWTAuthMiddleware 已执行
func UserIdentityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "user not authenticated",
			})
			c.Abort()
			return
		}

		// 存储原始 string 类型的 UserID
		c.Set(UserIDStrKey, claims.UserID)

		// 解析 UserID（IAM 返回的是 string 类型）为 uint64
		if claims.UserID != "" {
			userID, err := strconv.ParseUint(claims.UserID, 10, 64)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": fmt.Sprintf("invalid user id format: %s", claims.UserID),
				})
				c.Abort()
				return
			}
			c.Set(UserIDKey, userID)
		}

		// 存储 TenantID（原始值）
		c.Set(TenantIDKey, claims.TenantID)

		// 解析 TenantID 为 OrgID（uint64）
		if claims.TenantID != "" {
			orgID, err := strconv.ParseUint(claims.TenantID, 10, 64)
			if err == nil {
				c.Set(OrgIDKey, orgID)
			}
			// 如果解析失败，不阻断请求，OrgID 可能不是数字格式
		}

		// 存储角色列表
		if len(claims.Roles) > 0 {
			c.Set(RolesKey, claims.Roles)
		}

		c.Next()
	}
}

// OptionalUserIdentityMiddleware 可选的用户身份解析中间件
// 如果有 JWT claims 则解析，没有则跳过
func OptionalUserIdentityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			// 没有认证信息，继续执行
			c.Next()
			return
		}

		// 存储原始值
		c.Set(UserIDStrKey, claims.UserID)
		c.Set(TenantIDKey, claims.TenantID)

		// 解析 UserID
		if claims.UserID != "" {
			if userID, err := strconv.ParseUint(claims.UserID, 10, 64); err == nil {
				c.Set(UserIDKey, userID)
			}
		}

		// 解析 OrgID
		if claims.TenantID != "" {
			if orgID, err := strconv.ParseUint(claims.TenantID, 10, 64); err == nil {
				c.Set(OrgIDKey, orgID)
			}
		}

		// 存储角色
		if len(claims.Roles) > 0 {
			c.Set(RolesKey, claims.Roles)
		}

		c.Next()
	}
}

// RequireRoleMiddleware 角色验证中间件
// 验证用户是否拥有指定角色
func RequireRoleMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles := GetRoles(c)
		if len(roles) == 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "no roles found",
			})
			c.Abort()
			return
		}

		for _, role := range roles {
			if role == requiredRole {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"error": fmt.Sprintf("role '%s' required", requiredRole),
		})
		c.Abort()
	}
}

// RequireAnyRoleMiddleware 任一角色验证中间件
// 验证用户是否拥有任一指定角色
func RequireAnyRoleMiddleware(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles := GetRoles(c)
		if len(roles) == 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "no roles found",
			})
			c.Abort()
			return
		}

		for _, role := range roles {
			for _, required := range requiredRoles {
				if role == required {
					c.Next()
					return
				}
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"error": fmt.Sprintf("one of roles %v required", requiredRoles),
		})
		c.Abort()
	}
}

// ============= Helper Functions =============

// GetUserID 从 gin.Context 获取用户ID（uint64）
func GetUserID(c *gin.Context) uint64 {
	val, exists := c.Get(UserIDKey)
	if !exists {
		return 0
	}
	if id, ok := val.(uint64); ok {
		return id
	}
	return 0
}

// GetUserIDStr 从 gin.Context 获取用户ID（string）
func GetUserIDStr(c *gin.Context) string {
	val, exists := c.Get(UserIDStrKey)
	if !exists {
		return ""
	}
	if id, ok := val.(string); ok {
		return id
	}
	return ""
}

// GetOrgID 从 gin.Context 获取组织ID（uint64）
func GetOrgID(c *gin.Context) uint64 {
	val, exists := c.Get(OrgIDKey)
	if !exists {
		return 0
	}
	if id, ok := val.(uint64); ok {
		return id
	}
	return 0
}

// GetTenantID 从 gin.Context 获取租户ID（string）
func GetTenantID(c *gin.Context) string {
	val, exists := c.Get(TenantIDKey)
	if !exists {
		return ""
	}
	if id, ok := val.(string); ok {
		return id
	}
	return ""
}

// GetRoles 从 gin.Context 获取角色列表
func GetRoles(c *gin.Context) []string {
	val, exists := c.Get(RolesKey)
	if !exists {
		return nil
	}
	if roles, ok := val.([]string); ok {
		return roles
	}
	return nil
}

// HasRole 检查用户是否拥有指定角色
func HasRole(c *gin.Context, role string) bool {
	roles := GetRoles(c)
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
