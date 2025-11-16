package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/FangcunMount/iam-contracts/pkg/log"
	"github.com/gin-gonic/gin"
)

const (
	// 最大记录的请求/响应体大小 (1MB)
	MaxLogBodySize = 1024 * 1024
)

// 敏感字段列表，需要脱敏处理
var sensitiveFields = []string{
	"password", "token", "secret", "key", "authorization",
	"passwd", "pwd", "access_token", "refresh_token",
}

// EnhancedLoggerConfig 增强日志配置
type EnhancedLoggerConfig struct {
	// 是否记录请求头
	LogRequestHeaders bool
	// 是否记录请求体
	LogRequestBody bool
	// 是否记录响应头
	LogResponseHeaders bool
	// 是否记录响应体
	LogResponseBody bool
	// 跳过记录的路径
	SkipPaths []string
	// 最大请求体记录大小
	MaxBodySize int64
	// 敏感字段脱敏
	MaskSensitiveFields bool
}

// DefaultEnhancedLoggerConfig 默认配置
func DefaultEnhancedLoggerConfig() EnhancedLoggerConfig {
	return EnhancedLoggerConfig{
		LogRequestHeaders:   true,
		LogRequestBody:      true,
		LogResponseHeaders:  true,
		LogResponseBody:     true,
		SkipPaths:           []string{"/health", "/healthz", "/metrics"},
		MaxBodySize:         MaxLogBodySize,
		MaskSensitiveFields: true,
	}
}

// responseWriter 包装原始的ResponseWriter以捕获响应数据
type responseWriter struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (w *responseWriter) Write(data []byte) (int, error) {
	// 同时写入原始响应和缓冲区
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// EnhancedLogger 创建增强的日志中间件
func EnhancedLogger() gin.HandlerFunc {
	return EnhancedLoggerWithConfig(DefaultEnhancedLoggerConfig())
}

// EnhancedLoggerWithConfig 使用配置创建增强日志中间件
func EnhancedLoggerWithConfig(config EnhancedLoggerConfig) gin.HandlerFunc {
	// 构建跳过路径的映射
	skipPaths := make(map[string]struct{})
	for _, path := range config.SkipPaths {
		skipPaths[path] = struct{}{}
	}

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		// 检查是否跳过此路径
		if _, ok := skipPaths[path]; ok {
			c.Next()
			return
		}

		requestID := c.GetString(XRequestIDKey)

		// === 1. 记录请求开始信息 ===
		logRequestStart(c, config, requestID)

		// 读取并缓存请求体
		var requestBody []byte
		if config.LogRequestBody && c.Request.Body != nil {
			requestBody = readAndRestoreRequestBody(c, config.MaxBodySize)
		}

		// 包装ResponseWriter以捕获响应
		responseBuffer := &bytes.Buffer{}
		wrappedWriter := &responseWriter{
			ResponseWriter: c.Writer,
			body:           responseBuffer,
			statusCode:     http.StatusOK,
		}
		c.Writer = wrappedWriter

		// 处理请求
		c.Next()

		// === 2. 记录请求结束信息 ===
		logRequestEnd(c, config, requestID, start, requestBody, responseBuffer.Bytes(), wrappedWriter.statusCode)
	}
}

// logRequestStart 记录请求开始信息
func logRequestStart(c *gin.Context, config EnhancedLoggerConfig, requestID string) {
	fields := []interface{}{
		"event", "request_start",
		"request_id", requestID,
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"query", c.Request.URL.RawQuery,
		"client_ip", c.ClientIP(),
		"user_agent", c.Request.UserAgent(),
		"timestamp", time.Now(),
	}

	// 记录请求头
	if config.LogRequestHeaders {
		headers := make(map[string]string)
		for name, values := range c.Request.Header {
			if len(values) > 0 {
				value := values[0]
				if config.MaskSensitiveFields && isSensitiveHeader(name) {
					value = maskSensitiveValue(value)
				}
				headers[name] = value
			}
		}
		fields = append(fields, "request_headers", headers)
	}

	log.Infow("HTTP Request Started", fields...)
}

// logRequestEnd 记录请求结束信息
func logRequestEnd(c *gin.Context, config EnhancedLoggerConfig, requestID string, start time.Time, requestBody, responseBody []byte, statusCode int) {
	duration := time.Since(start)

	fields := []interface{}{
		"event", "request_end",
		"request_id", requestID,
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"status_code", statusCode,
		"duration_ms", duration.Milliseconds(),
		"response_size", len(responseBody),
		"timestamp", time.Now(),
	}

	// 记录请求体
	if config.LogRequestBody && len(requestBody) > 0 {
		bodyStr := string(requestBody)
		if config.MaskSensitiveFields {
			bodyStr = maskSensitiveJSON(bodyStr)
		}
		fields = append(fields, "request_body", bodyStr)
	}

	// 记录响应头
	if config.LogResponseHeaders {
		headers := make(map[string]string)
		for name, values := range c.Writer.Header() {
			if len(values) > 0 {
				headers[name] = values[0]
			}
		}
		fields = append(fields, "response_headers", headers)
	}

	// 记录响应体
	if config.LogResponseBody && len(responseBody) > 0 {
		bodyStr := string(responseBody)
		if config.MaskSensitiveFields {
			bodyStr = maskSensitiveJSON(bodyStr)
		}
		fields = append(fields, "response_body", bodyStr)
	}

	// 记录错误信息
	if len(c.Errors) > 0 {
		fields = append(fields, "errors", c.Errors.String())
	}

	// 根据状态码选择日志级别
	if statusCode >= 500 {
		log.Errorw("HTTP Request Completed with Server Error", fields...)
	} else if statusCode >= 400 {
		log.Warnw("HTTP Request Completed with Client Error", fields...)
	} else {
		log.Infow("HTTP Request Completed Successfully", fields...)
	}
}

// readAndRestoreRequestBody 读取请求体并恢复到请求中
func readAndRestoreRequestBody(c *gin.Context, maxSize int64) []byte {
	if c.Request.Body == nil {
		return nil
	}

	// 限制读取大小
	reader := io.LimitReader(c.Request.Body, maxSize)
	body, err := io.ReadAll(reader)
	if err != nil {
		log.Warnw("Failed to read request body", "error", err)
		return nil
	}

	// 恢复请求体
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	return body
}

// isSensitiveHeader 检查是否为敏感请求头
func isSensitiveHeader(name string) bool {
	lowerName := strings.ToLower(name)
	for _, sensitive := range sensitiveFields {
		if strings.Contains(lowerName, sensitive) {
			return true
		}
	}
	return false
}

// maskSensitiveValue 脱敏敏感值
func maskSensitiveValue(value string) string {
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "***" + value[len(value)-4:]
}

// maskSensitiveJSON 脱敏JSON中的敏感字段
func maskSensitiveJSON(jsonStr string) string {
	if jsonStr == "" {
		return jsonStr
	}

	// 尝试解析为JSON
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// 如果不是JSON，使用正则表达式脱敏
		return maskSensitiveWithRegex(jsonStr)
	}

	// 递归脱敏JSON数据
	maskedData := maskSensitiveInData(data)

	masked, err := json.Marshal(maskedData)
	if err != nil {
		return jsonStr
	}

	return string(masked)
}

// maskSensitiveWithRegex 使用正则表达式脱敏
func maskSensitiveWithRegex(text string) string {
	for _, field := range sensitiveFields {
		// 匹配 "field":"value" 或 field=value 模式
		patterns := []string{
			fmt.Sprintf(`"%s"\s*:\s*"[^"]*"`, field),
			fmt.Sprintf(`%s\s*=\s*[^\s&]*`, field),
		}

		for _, pattern := range patterns {
			re := regexp.MustCompile(`(?i)` + pattern)
			text = re.ReplaceAllStringFunc(text, func(match string) string {
				parts := strings.Split(match, ":")
				if len(parts) == 2 {
					return parts[0] + `: "***"`
				}
				parts = strings.Split(match, "=")
				if len(parts) == 2 {
					return parts[0] + "=***"
				}
				return "***"
			})
		}
	}
	return text
}

// maskSensitiveInData 递归脱敏数据结构中的敏感字段
func maskSensitiveInData(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			if isSensitiveField(key) {
				result[key] = "***"
			} else {
				result[key] = maskSensitiveInData(value)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = maskSensitiveInData(item)
		}
		return result
	default:
		return v
	}
}

// isSensitiveField 检查字段名是否敏感
func isSensitiveField(fieldName string) bool {
	lowerField := strings.ToLower(fieldName)
	for _, sensitive := range sensitiveFields {
		if strings.Contains(lowerField, sensitive) {
			return true
		}
	}
	return false
}
