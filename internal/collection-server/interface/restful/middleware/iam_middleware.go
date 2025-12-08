package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
)

// IAMContext 上下文键常量
const (
	// UserIDKey 用户ID（uint64）
	UserIDKey = "user_id"
	// ChildIDKey 儿童ID（uint64）
	ChildIDKey = "child_id"
	// TesteeIDKey 受试者ID（需要通过业务查询获取）
	TesteeIDKey = "testee_id"
)

// UserIdentityMiddleware 用户身份解析中间件
// 将 JWT claims 中的 UserID（string）转换为 uint64 并存入 context
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

		// 解析 UserID（IAM 返回的是 string 类型）
		userID, err := strconv.ParseUint(claims.UserID, 10, 64)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": fmt.Sprintf("invalid user id format: %s", claims.UserID),
			})
			c.Abort()
			return
		}

		// 将 uint64 类型的 user_id 存入 context
		c.Set(UserIDKey, userID)

		c.Next()
	}
}

// GuardianshipVerifier 监护关系验证器接口
type GuardianshipVerifier interface {
	IsGuardian(ctx gin.Context, userID, childID string) (bool, error)
}

// GuardianshipMiddleware 监护关系验证中间件
// 验证当前用户是否是指定儿童的监护人
// childIDParam: 从 URL query 或 body 中获取 child_id 的参数名
func GuardianshipMiddleware(guardianshipSvc *iam.GuardianshipService, childIDParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查服务是否可用
		if guardianshipSvc == nil || !guardianshipSvc.IsEnabled() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "guardianship service not available",
			})
			c.Abort()
			return
		}

		// 获取用户 claims
		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "user not authenticated",
			})
			c.Abort()
			return
		}

		// 获取 child_id（从 query 参数或 path 参数）
		childIDStr := c.Query(childIDParam)
		if childIDStr == "" {
			childIDStr = c.Param(childIDParam)
		}
		if childIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("missing required parameter: %s", childIDParam),
			})
			c.Abort()
			return
		}

		// 验证监护关系
		isGuardian, err := guardianshipSvc.IsGuardian(c.Request.Context(), claims.UserID, childIDStr)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("failed to verify guardianship: %v", err),
			})
			c.Abort()
			return
		}

		if !isGuardian {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "you are not the guardian of this child",
			})
			c.Abort()
			return
		}

		// 解析并存储 child_id
		childID, err := strconv.ParseUint(childIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("invalid child id format: %s", childIDStr),
			})
			c.Abort()
			return
		}
		c.Set(ChildIDKey, childID)

		c.Next()
	}
}

// OptionalGuardianshipMiddleware 可选的监护关系验证中间件
// 如果提供了 child_id 则验证监护关系，否则跳过
func OptionalGuardianshipMiddleware(guardianshipSvc *iam.GuardianshipService, childIDParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取 child_id
		childIDStr := c.Query(childIDParam)
		if childIDStr == "" {
			childIDStr = c.Param(childIDParam)
		}
		if childIDStr == "" {
			// 没有提供 child_id，跳过验证
			c.Next()
			return
		}

		// 检查服务是否可用
		if guardianshipSvc == nil || !guardianshipSvc.IsEnabled() {
			// 服务不可用时跳过验证（降级策略）
			c.Next()
			return
		}

		// 获取用户 claims
		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil {
			// 用户未认证，跳过验证
			c.Next()
			return
		}

		// 验证监护关系
		isGuardian, err := guardianshipSvc.IsGuardian(c.Request.Context(), claims.UserID, childIDStr)
		if err != nil {
			// 验证失败时记录日志但不阻断请求（降级策略）
			c.Next()
			return
		}

		if isGuardian {
			// 解析并存储 child_id
			childID, _ := strconv.ParseUint(childIDStr, 10, 64)
			c.Set(ChildIDKey, childID)
		}

		c.Next()
	}
}

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

// GetChildID 从 gin.Context 获取儿童ID（uint64）
func GetChildID(c *gin.Context) uint64 {
	val, exists := c.Get(ChildIDKey)
	if !exists {
		return 0
	}
	if id, ok := val.(uint64); ok {
		return id
	}
	return 0
}
