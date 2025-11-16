package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/gin-gonic/gin"
)

// APILoggerConfig API日志配置
type APILoggerConfig struct {
	// LogRequestBody 是否记录请求体
	LogRequestBody bool
	// LogResponseBody 是否记录响应体
	LogResponseBody bool
	// MaxBodySize 最大记录的请求/响应体大小（字节）
	MaxBodySize int64
	// SkipPaths 跳过记录的路径
	SkipPaths []string
	// LogLevel 日志级别，0=INFO, 1=DEBUG
	LogLevel int
}

// DefaultAPILoggerConfig 默认API日志配置
func DefaultAPILoggerConfig() APILoggerConfig {
	return APILoggerConfig{
		LogRequestBody:  true,
		LogResponseBody: true,
		MaxBodySize:     10 * 1024, // 10KB
		SkipPaths:       []string{"/healthz", "/metrics", "/favicon.ico"},
		LogLevel:        0, // INFO level
	}
}

// responseBodyWriter 包装gin.ResponseWriter以捕获响应体
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// APILogger 详细API日志中间件
func APILogger() gin.HandlerFunc {
	return APILoggerWithConfig(DefaultAPILoggerConfig())
}

// APILoggerWithConfig 带配置的详细API日志中间件
func APILoggerWithConfig(config APILoggerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否跳过此路径
		for _, path := range config.SkipPaths {
			if c.Request.URL.Path == path {
				c.Next()
				return
			}
		}

		startTime := time.Now()
		ctx := c.Request.Context()

		// 记录请求开始
		log.L(ctx).Infof("=== API Request Started ===")
		log.L(ctx).Infof("Method: %s", c.Request.Method)
		log.L(ctx).Infof("Path: %s", c.Request.URL.Path)
		log.L(ctx).Infof("Query: %s", c.Request.URL.RawQuery)
		log.L(ctx).Infof("User-Agent: %s", c.Request.UserAgent())
		log.L(ctx).Infof("Client-IP: %s", c.ClientIP())

		// 记录请求头
		if config.LogLevel >= 1 { // DEBUG level
			log.L(ctx).V(1).Info("Request Headers:")
			for name, values := range c.Request.Header {
				// 隐藏敏感信息
				if strings.ToLower(name) == "authorization" {
					log.L(ctx).V(1).Infof("  %s: [REDACTED]", name)
				} else {
					log.L(ctx).V(1).Infof("  %s: %s", name, strings.Join(values, ", "))
				}
			}
		}

		// 记录请求体
		var requestBody []byte
		if config.LogRequestBody && c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

			if len(requestBody) > 0 && int64(len(requestBody)) <= config.MaxBodySize {
				if isJSON(requestBody) {
					log.L(ctx).Infof("Request Body (JSON): %s", formatJSON(requestBody))
				} else {
					log.L(ctx).Infof("Request Body: %s", string(requestBody))
				}
			} else if len(requestBody) > 0 {
				log.L(ctx).Infof("Request Body: [Too large: %d bytes]", len(requestBody))
			}
		}

		// 包装ResponseWriter以捕获响应体
		var responseBody *bytes.Buffer
		if config.LogResponseBody {
			responseBody = &bytes.Buffer{}
			c.Writer = &responseBodyWriter{
				ResponseWriter: c.Writer,
				body:           responseBody,
			}
		}

		// 处理请求
		c.Next()

		// 计算处理时间
		latency := time.Since(startTime)

		// 记录响应信息
		log.L(ctx).Infof("Status: %d", c.Writer.Status())
		log.L(ctx).Infof("Latency: %v", latency)

		// 记录响应头
		if config.LogLevel >= 1 { // DEBUG level
			log.L(ctx).V(1).Info("Response Headers:")
			for name, values := range c.Writer.Header() {
				log.L(ctx).V(1).Infof("  %s: %s", name, strings.Join(values, ", "))
			}
		}

		// 记录响应体
		if config.LogResponseBody && responseBody != nil {
			respBody := responseBody.Bytes()
			if len(respBody) > 0 && int64(len(respBody)) <= config.MaxBodySize {
				if isJSON(respBody) {
					log.L(ctx).Infof("Response Body (JSON): %s", formatJSON(respBody))
				} else {
					log.L(ctx).Infof("Response Body: %s", string(respBody))
				}
			} else if len(respBody) > 0 {
				log.L(ctx).Infof("Response Body: [Too large: %d bytes]", len(respBody))
			}
		}

		// 记录错误（如果有）
		if len(c.Errors) > 0 {
			log.L(ctx).Errorf("Errors: %s", c.Errors.String())
		}

		log.L(ctx).Infof("=== API Request Completed ===")
	}
}

// isJSON 检查数据是否为JSON格式
func isJSON(data []byte) bool {
	var js json.RawMessage
	return json.Unmarshal(data, &js) == nil
}

// formatJSON 格式化JSON数据（移除不必要的空格和换行）
func formatJSON(data []byte) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return string(data)
	}
	result := compact.String()
	if len(result) > 500 { // 限制长度
		return result[:500] + "..."
	}
	return result
}
