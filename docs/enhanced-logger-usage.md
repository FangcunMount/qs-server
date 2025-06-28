# 🔍 增强日志中间件使用指南

## 📋 功能概述

增强日志中间件提供了完整的 HTTP 请求/响应日志记录功能，包括：

1. **📨 请求开始日志** - 记录请求头、请求体
2. **📤 请求结束日志** - 记录响应头、响应体、处理时间
3. **🔒 敏感信息脱敏** - 自动脱敏密码等敏感字段
4. **⚡ 性能优化** - 限制大请求体记录，避免性能影响
5. **🎯 灵活配置** - 支持自定义配置选项

## 🚀 使用方法

### 1. 基础使用（默认配置）

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/yshujie/questionnaire-scale/internal/pkg/middleware"
)

func main() {
    r := gin.New()
    
    // 使用默认配置的增强日志中间件
    r.Use(middleware.EnhancedLogger())
    
    // 或者从中间件管理器获取
    r.Use(middleware.Middlewares["enhanced_logger"])
    
    // 路由定义...
    r.Run(":8080")
}
```

### 2. 自定义配置使用

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/yshujie/questionnaire-scale/internal/pkg/middleware"
)

func main() {
    r := gin.New()
    
    // 自定义配置
    config := middleware.EnhancedLoggerConfig{
        LogRequestHeaders:   true,
        LogRequestBody:      true,
        LogResponseHeaders:  true,
        LogResponseBody:     false, // 不记录响应体
        SkipPaths:          []string{"/health", "/metrics", "/favicon.ico"},
        MaxBodySize:        512 * 1024, // 512KB
        MaskSensitiveFields: true,
    }
    
    r.Use(middleware.EnhancedLoggerWithConfig(config))
    
    // 路由定义...
    r.Run(":8080")
}
```

## 📊 配置选项详解

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `LogRequestHeaders` | bool | true | 是否记录请求头 |
| `LogRequestBody` | bool | true | 是否记录请求体 |
| `LogResponseHeaders` | bool | true | 是否记录响应头 |
| `LogResponseBody` | bool | true | 是否记录响应体 |
| `SkipPaths` | []string | `["/health", "/healthz", "/metrics"]` | 跳过记录的路径 |
| `MaxBodySize` | int64 | 1MB | 最大记录的请求/响应体大小 |
| `MaskSensitiveFields` | bool | true | 是否脱敏敏感字段 |

## 📈 日志输出示例

### 请求开始日志
```json
{
  "timestamp": "2024-01-15T10:30:15Z",
  "level": "info",
  "message": "HTTP Request Started",
  "event": "request_start",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "method": "POST",
  "path": "/api/users",
  "query": "page=1&size=10",
  "client_ip": "127.0.0.1",
  "user_agent": "Mozilla/5.0...",
  "request_headers": {
    "Content-Type": "application/json",
    "Authorization": "Bear****1234",
    "X-Request-ID": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

### 业务处理日志（正常记录）
```json
{
  "timestamp": "2024-01-15T10:30:15.050Z",
  "level": "info",
  "message": "Starting user creation",
  "username": "john",
  "email": "john@example.com",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### 请求结束日志
```json
{
  "timestamp": "2024-01-15T10:30:15.125Z",
  "level": "info",
  "message": "HTTP Request Completed Successfully",
  "event": "request_end",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "method": "POST",
  "path": "/api/users",
  "status_code": 201,
  "duration_ms": 125,
  "response_size": 156,
  "request_body": "{\"username\":\"john\",\"email\":\"john@example.com\",\"password\":\"***\"}",
  "response_headers": {
    "Content-Type": "application/json",
    "X-Request-ID": "550e8400-e29b-41d4-a716-446655440000"
  },
  "response_body": "{\"id\":123,\"username\":\"john\",\"email\":\"john@example.com\"}"
}
```

## 🔒 敏感信息脱敏

### 自动脱敏的字段
- `password`, `passwd`, `pwd`
- `token`, `access_token`, `refresh_token`
- `secret`, `key`
- `authorization`

### 脱敏示例

**原始数据：**
```json
{
  "username": "john",
  "password": "secretpassword123",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**脱敏后：**
```json
{
  "username": "john", 
  "password": "***",
  "token": "***"
}
```

## 🎯 使用场景与最佳实践

### 1. 开发环境配置
```go
// 开发环境 - 记录完整信息
config := middleware.EnhancedLoggerConfig{
    LogRequestHeaders:   true,
    LogRequestBody:      true,
    LogResponseHeaders:  true,
    LogResponseBody:     true,
    MaxBodySize:        2 * 1024 * 1024, // 2MB
    MaskSensitiveFields: false, // 开发时可以关闭脱敏
}
```

### 2. 生产环境配置
```go
// 生产环境 - 平衡安全和性能
config := middleware.EnhancedLoggerConfig{
    LogRequestHeaders:   true,
    LogRequestBody:      true,
    LogResponseHeaders:  false, // 减少日志量
    LogResponseBody:     false, // 减少日志量
    MaxBodySize:        512 * 1024, // 512KB
    MaskSensitiveFields: true, // 必须开启脱敏
    SkipPaths:          []string{"/health", "/metrics", "/favicon.ico"},
}
```

### 3. 调试模式配置
```go
// 调试特定问题时
config := middleware.EnhancedLoggerConfig{
    LogRequestHeaders:   true,
    LogRequestBody:      true,
    LogResponseHeaders:  true,
    LogResponseBody:     true,
    MaxBodySize:        10 * 1024 * 1024, // 10MB
    MaskSensitiveFields: false,
    SkipPaths:          []string{}, // 记录所有路径
}
```

## ⚡ 性能考虑

### 1. 请求体大小限制
- 默认限制 1MB，避免大文件上传影响性能
- 超过限制的部分不会记录到日志

### 2. 路径跳过
- 健康检查、指标等高频路径默认跳过
- 可自定义跳过路径列表

### 3. 内存使用
- 使用缓冲区暂存响应体，请求结束后释放
- 大响应体会占用额外内存，建议限制记录大小

### 4. 日志量控制
- 生产环境建议关闭响应体记录
- 使用合适的日志级别和轮转策略

## 🛠️ 与现有日志中间件的对比

| 特性 | 基础Logger | 增强Logger |
|------|------------|------------|
| 请求基本信息 | ✅ | ✅ |
| 处理时间 | ✅ | ✅ |
| 请求头 | ❌ | ✅ |
| 请求体 | ❌ | ✅ |
| 响应头 | ❌ | ✅ |
| 响应体 | ❌ | ✅ |
| 敏感信息脱敏 | ❌ | ✅ |
| 结构化日志 | ❌ | ✅ |
| 配置灵活性 | 基础 | 高度可配置 |

## 🔄 中间件协调

增强日志中间件与其他中间件的协调使用：

```go
r.Use(
    gin.Recovery(),                    // 1. 崩溃恢复
    middleware.RequestID(),            // 2. 生成请求ID
    middleware.Context(),              // 3. 上下文增强
    middleware.EnhancedLogger(),       // 4. 增强日志记录
    // middleware.Logger(),            // 5. 不要同时使用基础日志
    middleware.Cors(),                 // 6. 其他中间件
)
```

通过这套增强日志中间件，您可以获得完整的 HTTP 请求链路可观测性，既满足开发调试需求，又符合生产环境的安全和性能要求。 