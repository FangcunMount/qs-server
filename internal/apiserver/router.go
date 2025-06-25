package apiserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// initRouter 初始化API路由
func initRouter(g *gin.Engine, dbManager *DatabaseManager) {
	// 健康检查路由（虽然genericapiserver已经有了，但这里可以自定义）
	g.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "Questionnaire Scale API服务运行正常",
		})
	})

	// 数据库健康检查路由
	g.GET("/health/db", func(c *gin.Context) {
		if err := dbManager.HealthCheck(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"message": "数据库连接异常",
				"error":   err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "数据库连接正常",
			"databases": gin.H{
				"registered": dbManager.GetRegistry().ListRegistered(),
			},
		})
	})

	// API版本路由组
	v1 := g.Group("/api/v1")
	{
		// 基础测试路由
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "pong",
			})
		})

		// 数据库连接测试路由
		v1.GET("/db-test", func(c *gin.Context) {
			response := gin.H{
				"message":   "数据库连接测试",
				"databases": gin.H{},
			}

			// 测试MySQL连接
			if mysqlDB, err := dbManager.GetMySQLDB(); err == nil {
				if sqlDB, err := mysqlDB.DB(); err == nil {
					if err := sqlDB.Ping(); err == nil {
						response["databases"].(gin.H)["mysql"] = "connected"
					} else {
						response["databases"].(gin.H)["mysql"] = "error: " + err.Error()
					}
				} else {
					response["databases"].(gin.H)["mysql"] = "error: " + err.Error()
				}
			} else {
				response["databases"].(gin.H)["mysql"] = "not configured"
			}

			// 测试Redis连接
			if redisClient, err := dbManager.GetRedisClient(); err == nil {
				if err := redisClient.Ping().Err(); err == nil {
					response["databases"].(gin.H)["redis"] = "connected"
				} else {
					response["databases"].(gin.H)["redis"] = "error: " + err.Error()
				}
			} else {
				response["databases"].(gin.H)["redis"] = "not configured"
			}

			// 测试MongoDB连接
			if mongoSession, err := dbManager.GetMongoSession(); err == nil {
				if err := mongoSession.Ping(); err == nil {
					response["databases"].(gin.H)["mongodb"] = "connected"
				} else {
					response["databases"].(gin.H)["mongodb"] = "error: " + err.Error()
				}
			} else {
				response["databases"].(gin.H)["mongodb"] = "not configured"
			}

			c.JSON(http.StatusOK, response)
		})

	}
}
