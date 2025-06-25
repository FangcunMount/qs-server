package apiserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// initRouter 初始化API路由
func initRouter(g *gin.Engine) {
	// 健康检查路由（虽然genericapiserver已经有了，但这里可以自定义）
	g.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "Questionnaire Scale API服务运行正常",
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

	}
}
