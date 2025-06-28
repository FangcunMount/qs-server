// 增强日志中间件使用示例
// 这是一个独立的示例文件，展示如何使用增强日志中间件

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	// 导入项目的中间件包
	// "github.com/yshujie/questionnaire-scale/internal/pkg/middleware"
)

// 模拟中间件（实际使用时导入真实的中间件）
func mockEnhancedLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 记录请求开始
		fmt.Printf("📨 [%s] %s %s - Request Started\n",
			time.Now().Format("15:04:05"), c.Request.Method, c.Request.URL.Path)

		// 记录请求头（示例）
		if auth := c.GetHeader("Authorization"); auth != "" {
			fmt.Printf("   Authorization: %s\n", auth[:min(len(auth), 20)]+"...")
		}

		// 处理请求
		c.Next()

		// 记录请求结束
		duration := time.Since(start)
		status := c.Writer.Status()

		var emoji string
		if status >= 500 {
			emoji = "❌"
		} else if status >= 400 {
			emoji = "⚠️"
		} else {
			emoji = "✅"
		}

		fmt.Printf("%s [%s] %s %s - %d (%v)\n",
			emoji, time.Now().Format("15:04:05"), c.Request.Method,
			c.Request.URL.Path, status, duration)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	// 创建Gin路由器
	r := gin.New()

	// 使用增强日志中间件（模拟）
	r.Use(mockEnhancedLogger())

	// 定义API路由
	r.POST("/api/users", func(c *gin.Context) {
		var user map[string]interface{}
		if err := c.ShouldBindJSON(&user); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		// 模拟业务处理
		fmt.Println("   Processing user creation...")
		time.Sleep(50 * time.Millisecond) // 模拟处理时间

		c.JSON(http.StatusCreated, gin.H{
			"id":       123,
			"username": user["username"],
			"message":  "User created successfully",
		})
	})

	r.GET("/api/users/:id", func(c *gin.Context) {
		id := c.Param("id")
		fmt.Printf("   Fetching user with ID: %s\n", id)

		c.JSON(http.StatusOK, gin.H{
			"id":       id,
			"username": "john_doe",
			"email":    "john@example.com",
		})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	fmt.Println("🚀 Server starting on :8080")
	fmt.Println("📋 Test endpoints:")
	fmt.Println("   POST /api/users")
	fmt.Println("   GET  /api/users/123")
	fmt.Println("   GET  /health")
	fmt.Println()

	// 启动一个goroutine来发送测试请求
	go func() {
		time.Sleep(2 * time.Second) // 等待服务器启动

		fmt.Println("🧪 Sending test requests...")

		// 测试 POST 请求
		userData := map[string]interface{}{
			"username": "john_doe",
			"email":    "john@example.com",
			"password": "secretpassword123",
		}

		jsonData, _ := json.Marshal(userData)
		resp, err := http.Post("http://localhost:8080/api/users",
			"application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("POST request failed: %v", err)
		} else {
			resp.Body.Close()
		}

		// 测试 GET 请求
		resp, err = http.Get("http://localhost:8080/api/users/123")
		if err != nil {
			log.Printf("GET request failed: %v", err)
		} else {
			resp.Body.Close()
		}

		// 测试健康检查
		resp, err = http.Get("http://localhost:8080/health")
		if err != nil {
			log.Printf("Health check failed: %v", err)
		} else {
			resp.Body.Close()
		}
	}()

	// 启动服务器
	r.Run(":8080")
}

/*
运行这个示例：

1. 将此文件保存为 main.go
2. 在终端运行： go run main.go
3. 查看控制台输出，观察日志格式

预期的日志输出示例：
📨 [15:04:05] POST /api/users - Request Started
   Authorization: Bearer eyJhbGciOiJIU...
   Processing user creation...
✅ [15:04:05] POST /api/users - 201 (52ms)

📨 [15:04:05] GET /api/users/123 - Request Started
   Fetching user with ID: 123
✅ [15:04:05] GET /api/users/123 - 200 (1ms)

实际项目中的使用：

1. 在配置文件中添加中间件：
   server:
     middlewares:
       - enhanced_logger

2. 或在代码中直接使用：
   r.Use(middleware.EnhancedLogger())

3. 自定义配置：
   config := middleware.EnhancedLoggerConfig{
     LogRequestHeaders:   true,
     LogRequestBody:      true,
     LogResponseHeaders:  false,
     LogResponseBody:     false,
     MaxBodySize:        512 * 1024,
     MaskSensitiveFields: true,
   }
   r.Use(middleware.EnhancedLoggerWithConfig(config))
*/
