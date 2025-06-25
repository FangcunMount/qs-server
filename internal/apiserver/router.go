package apiserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/store"
	v1 "github.com/yshujie/questionnaire-scale/pkg/api/apiserver/v1"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	metav1 "github.com/yshujie/questionnaire-scale/pkg/meta/v1"
)

// initRouter 初始化路由
func initRouter(g *gin.Engine, dbManager *DatabaseManager) {
	installMiddleware(g)
	installController(g, dbManager)
}

func installMiddleware(g *gin.Engine) {
	// 安装中间件
}

func installController(g *gin.Engine, dbManager *DatabaseManager) {
	// 自定义健康检查路由（避免与系统路由冲突）
	g.GET("/health/db", healthCheck(dbManager))

	// API 版本组
	v1Group := g.Group("/v1")

	// 用户管理接口（使用存储系统）
	users := v1Group.Group("/users")
	users.GET("", listUsersWithStore())
	users.POST("", createUserWithStore())
	users.GET("/:username", getUserWithStore())
	users.PUT("/:username", updateUserWithStore())
	users.DELETE("/:username", deleteUserWithStore())

	// 问卷相关路由
	questionnaires := v1Group.Group("/questionnaires")
	questionnaires.GET("", listQuestionnaires(dbManager))
	questionnaires.POST("", createQuestionnaire(dbManager))
	questionnaires.GET("/:id", getQuestionnaire(dbManager))
	questionnaires.PUT("/:id", updateQuestionnaire(dbManager))
	questionnaires.DELETE("/:id", deleteQuestionnaire(dbManager))

	// 量表相关路由
	scales := v1Group.Group("/scales")
	scales.GET("", listScales(dbManager))
	scales.POST("", createScale(dbManager))
	scales.GET("/:id", getScale(dbManager))
	scales.PUT("/:id", updateScale(dbManager))
	scales.DELETE("/:id", deleteScale(dbManager))

	// 答卷相关路由
	responses := v1Group.Group("/responses")
	responses.GET("", listResponses(dbManager))
	responses.POST("", createResponse(dbManager))
	responses.GET("/:id", getResponse(dbManager))
	responses.DELETE("/:id", deleteResponse(dbManager))
}

// healthCheck 健康检查处理函数
func healthCheck(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		message := "OK"

		// 检查数据库连接状态
		if err := dbManager.HealthCheck(); err != nil {
			log.Warnf("Database health check failed: %v", err)
			message = "Database unhealthy"
		}

		c.JSON(http.StatusOK, gin.H{
			"status": message,
			"time":   time.Now().Format(time.RFC3339),
		})
	}
}

// 问卷相关处理函数
func listQuestionnaires(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "List questionnaires - 功能待实现",
			"data":    []interface{}{},
		})
	}
}

func createQuestionnaire(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Create questionnaire - 功能待实现",
		})
	}
}

func getQuestionnaire(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		c.JSON(http.StatusOK, gin.H{
			"message": "Get questionnaire - 功能待实现",
			"id":      id,
		})
	}
}

func updateQuestionnaire(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		c.JSON(http.StatusOK, gin.H{
			"message": "Update questionnaire - 功能待实现",
			"id":      id,
		})
	}
}

func deleteQuestionnaire(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		c.JSON(http.StatusOK, gin.H{
			"message": "Delete questionnaire - 功能待实现",
			"id":      id,
		})
	}
}

// 量表相关处理函数
func listScales(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "List scales - 功能待实现",
			"data":    []interface{}{},
		})
	}
}

func createScale(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Create scale - 功能待实现",
		})
	}
}

func getScale(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		c.JSON(http.StatusOK, gin.H{
			"message": "Get scale - 功能待实现",
			"id":      id,
		})
	}
}

func updateScale(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		c.JSON(http.StatusOK, gin.H{
			"message": "Update scale - 功能待实现",
			"id":      id,
		})
	}
}

func deleteScale(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		c.JSON(http.StatusOK, gin.H{
			"message": "Delete scale - 功能待实现",
			"id":      id,
		})
	}
}

// 答卷相关处理函数
func listResponses(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "List responses - 功能待实现",
			"data":    []interface{}{},
		})
	}
}

func createResponse(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Create response - 功能待实现",
		})
	}
}

func getResponse(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		c.JSON(http.StatusOK, gin.H{
			"message": "Get response - 功能待实现",
			"id":      id,
		})
	}
}

func deleteResponse(dbManager *DatabaseManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		c.JSON(http.StatusOK, gin.H{
			"message": "Delete response - 功能待实现",
			"id":      id,
		})
	}
}

// 使用存储系统的用户管理接口示例
func listUsersWithStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		userStore := store.Client().Users()
		users, err := userStore.List(ctx, metav1.ListOptions{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to list users: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "用户列表获取成功",
			"total":   users.TotalCount,
			"data":    users.Items,
		})
	}
}

func createUserWithStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		var user v1.User
		if err := c.ShouldBindJSON(&user); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid JSON: " + err.Error(),
			})
			return
		}

		ctx := c.Request.Context()
		userStore := store.Client().Users()

		if err := userStore.Create(ctx, &user, metav1.CreateOptions{}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to create user: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "用户创建成功",
			"data":    user,
		})
	}
}

func getUserWithStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.Param("username")
		ctx := c.Request.Context()

		userStore := store.Client().Users()
		user, err := userStore.Get(ctx, username, metav1.GetOptions{})
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "User not found: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "用户信息获取成功",
			"data":    user,
		})
	}
}

func updateUserWithStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.Param("username")
		var user v1.User
		if err := c.ShouldBindJSON(&user); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid JSON: " + err.Error(),
			})
			return
		}

		// 确保用户名一致
		user.Name = username

		ctx := c.Request.Context()
		userStore := store.Client().Users()

		if err := userStore.Update(ctx, &user, metav1.UpdateOptions{}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to update user: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "用户更新成功",
			"data":    user,
		})
	}
}

func deleteUserWithStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.Param("username")
		ctx := c.Request.Context()

		userStore := store.Client().Users()
		if err := userStore.Delete(ctx, username, metav1.DeleteOptions{}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to delete user: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "用户删除成功",
		})
	}
}
