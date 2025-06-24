# Log Package

一个功能强大、高性能的Go日志包，基于 `go.uber.org/zap` 构建，提供企业级的日志解决方案。

## 🚀 核心特性

- ✅ **高性能** - 基于zap的零分配日志器
- ✅ **多级别** - Debug/Info/Warn/Error/Panic/Fatal
- ✅ **多格式** - Console/JSON，支持颜色输出
- ✅ **日志轮转** - 自动按大小/时间轮转日志文件
- ✅ **多输出** - 同时输出到文件、控制台、stderr
- ✅ **结构化** - 支持字段化日志记录
- ✅ **上下文支持** - 在context中传递logger
- ✅ **配置灵活** - 丰富的配置选项
- ✅ **兼容性强** - 支持klog、logrus等日志框架

## 📦 包结构

```text
pkg/log/
├── log.go          # 核心日志功能 (573行)
├── options.go      # 配置选项 (215行)
├── rotation.go     # 日志轮转 (107行)
├── types.go        # 类型定义 (109行)
├── context.go      # 上下文支持 (50行)
├── encoder.go      # 时间编码器 (33行)
├── klog/           # Kubernetes风格日志
├── logrus/         # Logrus兼容层
├── example/        # 使用示例
├── distribution/   # 分布式日志
└── cronlog/        # 定时任务日志
```

## 🔧 快速开始

### 基础使用

```go
package main

import (
    "github.com/yshujie/questionnaire-scale/pkg/log"
)

func main() {
    // 使用默认配置
    log.Info("Hello, World!")
    log.Infof("User %s logged in", "john")
    log.Infow("Request completed", "method", "GET", "path", "/api/users")
    
    // 不同级别的日志
    log.Debug("Debug message")
    log.Warn("Warning message")
    log.Error("Error message")
    
    // 字段化日志
    log.Info("User operation", 
        log.String("action", "login"),
        log.Int("user_id", 123),
        log.Duration("latency", time.Millisecond*15))
}
```

### 自定义配置

```go
package main

import (
    "github.com/yshujie/questionnaire-scale/pkg/log"
)

func main() {
    // 创建配置
    opts := &log.Options{
        Level:             "debug",
        Format:            "json",
        EnableColor:       false,
        DisableCaller:     false,
        DisableStacktrace: false,
        Development:       false,
        OutputPaths:       []string{"app.log", "stdout"},
        ErrorOutputPaths:  []string{"error.log", "stderr"},
        
        // 日志轮转配置
        MaxSize:    100,  // 100MB
        MaxAge:     30,   // 保留30天
        MaxBackups: 10,   // 保留10个备份
        Compress:   true, // 压缩旧文件
    }
    
    // 初始化全局logger
    log.Init(opts)
    defer log.Flush()
    
    log.Info("Application started with custom config")
}
```

## 📋 配置选项详解

### Options 结构体

```go
type Options struct {
    // 基础配置
    Level             string   // 日志级别: debug, info, warn, error, panic, fatal
    Format            string   // 输出格式: console, json
    DisableCaller     bool     // 是否禁用调用者信息
    DisableStacktrace bool     // 是否禁用堆栈跟踪
    EnableColor       bool     // 是否启用颜色输出（仅console格式）
    Development       bool     // 是否为开发模式
    Name              string   // Logger名称
    
    // 输出配置
    OutputPaths       []string // 输出路径: ["stdout", "app.log"]
    ErrorOutputPaths  []string // 错误输出路径: ["stderr", "error.log"]
    
    // 日志轮转配置
    MaxSize           int      // 单个文件最大大小（MB）
    MaxAge            int      // 保留旧文件最大天数
    MaxBackups        int      // 保留旧文件最大个数
    Compress          bool     // 是否压缩旧文件
}
```

### 默认配置

```go
&Options{
    Level:             "info",
    Format:            "console",
    EnableColor:       false,
    DisableCaller:     false,
    DisableStacktrace: false,
    Development:       false,
    OutputPaths:       []string{"stdout"},
    ErrorOutputPaths:  []string{"stderr"},
    MaxSize:           100,    // 100MB
    MaxAge:            30,     // 30天
    MaxBackups:        10,     // 10个备份
    Compress:          true,   // 压缩
}
```

## 🎯 核心API

### 1. 基本日志方法

```go
// 简单消息
log.Debug("Debug message")
log.Info("Info message")
log.Warn("Warning message")
log.Error("Error message")
log.Panic("Panic message")  // 记录后panic
log.Fatal("Fatal message")  // 记录后exit(1)

// 格式化消息
log.Debugf("User %s has %d items", username, count)
log.Infof("Request took %v", duration)
log.Errorf("Failed to connect to %s: %v", host, err)

// 键值对消息（推荐）
log.Infow("User login", "username", "john", "ip", "192.168.1.1")
log.Errorw("Database error", "table", "users", "error", err)
```

### 2. 结构化字段

```go
// 使用类型化字段（性能更好）
log.Info("Request processed",
    log.String("method", "POST"),
    log.String("path", "/api/users"),
    log.Int("status", 200),
    log.Duration("latency", time.Millisecond*15),
    log.Int64("user_id", 12345),
    log.Bool("cached", true),
    log.Any("headers", headers))

// 常用字段类型
log.String("key", "value")          // 字符串
log.Int("count", 10)                // 整数
log.Float64("score", 95.5)          // 浮点数
log.Bool("success", true)           // 布尔值
log.Duration("latency", duration)   // 时间间隔
log.Time("timestamp", time.Now())   // 时间
log.Err(err)                        // 错误
log.Any("data", complexObject)      // 任意类型
```

### 3. Logger实例方法

```go
// 创建具名logger
userLogger := log.WithName("user-service")
userLogger.Info("User service started")

// 创建带默认字段的logger
requestLogger := log.WithValues(
    "request_id", "req-123",
    "user_id", 456)
requestLogger.Info("Processing request")
requestLogger.Error("Request failed")

// 级别控制
if log.V(log.DebugLevel).Enabled() {
    log.V(log.DebugLevel).Info("Expensive debug operation")
}
```

### 4. 上下文支持

```go
// 将logger存入context
ctx := log.WithContext(context.Background())

// 从context获取logger
logger := log.FromContext(ctx)
logger.Info("Operation completed")

// 在HTTP处理器中使用
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    logger := log.FromContext(ctx)
    logger.Info("Handling request", 
        log.String("method", r.Method),
        log.String("path", r.URL.Path))
}
```

## 💡 使用场景和最佳实践

### 1. Web服务日志

```go
// 中间件记录请求日志
func LoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        
        // 创建请求专用logger
        requestLogger := log.WithValues(
            "request_id", generateRequestID(),
            "method", r.Method,
            "path", r.URL.Path,
            "remote_addr", r.RemoteAddr)
        
        // 将logger存入context
        ctx := requestLogger.WithContext(r.Context())
        r = r.WithContext(ctx)
        
        requestLogger.Info("Request started")
        
        // 处理请求
        next.ServeHTTP(w, r)
        
        requestLogger.Info("Request completed",
            log.Duration("latency", time.Since(start)))
    })
}
```

### 2. 错误处理和追踪

```go
func ProcessUser(userID int64) error {
    logger := log.WithValues("user_id", userID)
    
    logger.Info("Processing user")
    
    user, err := getUserFromDB(userID)
    if err != nil {
        logger.Error("Failed to get user from database", log.Err(err))
        return fmt.Errorf("database error: %w", err)
    }
    
    if err := validateUser(user); err != nil {
        logger.Warn("User validation failed", 
            log.Err(err),
            log.String("username", user.Username))
        return fmt.Errorf("validation error: %w", err)
    }
    
    logger.Info("User processed successfully",
        log.String("username", user.Username),
        log.String("email", user.Email))
    
    return nil
}
```

### 3. 生产环境配置

```go
// 生产环境推荐配置
func setupProductionLogger() {
    opts := &log.Options{
        Level:             "info",           // 生产环境通常用info
        Format:            "json",           // JSON格式便于日志收集
        EnableColor:       false,           // 生产环境禁用颜色
        DisableCaller:     false,           // 保留调用者信息
        DisableStacktrace: false,           // 保留堆栈信息
        Development:       false,           // 生产模式
        
        OutputPaths: []string{
            "/var/log/app/app.log",         // 应用日志
            "stdout",                       // 标准输出（容器环境）
        },
        ErrorOutputPaths: []string{
            "/var/log/app/error.log",       // 错误日志
            "stderr",                       // 标准错误输出
        },
        
        // 日志轮转配置
        MaxSize:    100,                    // 100MB轮转
        MaxAge:     7,                      // 保留7天
        MaxBackups: 5,                      // 保留5个备份
        Compress:   true,                   // 压缩旧文件
    }
    
    log.Init(opts)
}
```

### 4. 开发环境配置

```go
// 开发环境友好配置
func setupDevelopmentLogger() {
    opts := &log.Options{
        Level:             "debug",         // 开发环境显示详细日志
        Format:            "console",       // 易读的控制台格式
        EnableColor:       true,            // 启用颜色便于阅读
        DisableCaller:     false,           // 显示调用者
        DisableStacktrace: false,           // 显示堆栈
        Development:       true,            // 开发模式
        
        OutputPaths:       []string{"stdout"},
        ErrorOutputPaths:  []string{"stderr"},
    }
    
    log.Init(opts)
}
```

## 🔄 日志轮转

### 自动轮转配置

```go
opts := &log.Options{
    OutputPaths: []string{
        "/var/log/app/app.log",      // 自动轮转的日志文件
        "stdout",                    // 同时输出到控制台
    },
    
    MaxSize:    100,    // 单个文件最大100MB
    MaxAge:     30,     // 保留30天的日志
    MaxBackups: 10,     // 最多保留10个备份文件
    Compress:   true,   // 压缩旧的日志文件
}
```

### 轮转后的文件命名

```text
app.log              # 当前日志文件
app.log.2024-01-01   # 按日期命名的备份文件
app.log.2024-01-02.gz # 压缩的备份文件
```

## 🎛️ 命令行参数

log包支持通过命令行参数配置：

```bash
# 设置日志级别
--log.level=debug

# 设置输出格式
--log.format=json

# 启用颜色输出
--log.enable-color=true

# 设置输出路径
--log.output-paths=stdout,app.log

# 设置错误输出路径  
--log.error-output-paths=stderr,error.log

# 日志轮转配置
--log.max-size=200
--log.max-age=7
--log.max-backups=5
--log.compress=true
```

## 📊 性能优化

### 1. 条件日志记录

```go
// 避免昂贵的字符串格式化
if log.V(log.DebugLevel).Enabled() {
    log.Debug("Expensive debug info", 
        log.String("data", expensiveOperation()))
}
```

### 2. 使用类型化字段

```go
// 推荐：使用类型化字段
log.Info("User created", 
    log.Int64("user_id", 123),
    log.String("username", "john"))

// 避免：使用interface{}
log.Infow("User created", "user_id", 123, "username", "john")
```

### 3. 重用Logger实例

```go
// 创建带默认字段的logger并重用
userLogger := log.WithValues("service", "user-service")

func CreateUser() {
    userLogger.Info("Creating user")
}

func DeleteUser() {
    userLogger.Info("Deleting user")
}
```

## 📚 示例代码

完整的使用示例请参考 `pkg/log/example/` 目录：

- `example.go` - 基本使用示例
- `simple/` - 简单示例
- `context/` - 上下文使用示例
- `vlevel/` - 级别控制示例

## 🔗 依赖关系

- `go.uber.org/zap` - 高性能日志库
- `gopkg.in/natefinch/lumberjack.v2` - 日志轮转
- `github.com/spf13/pflag` - 命令行参数解析

## 🎯 设计特点

1. **高性能** - 基于zap的零分配设计
2. **类型安全** - 强类型字段避免运行时错误  
3. **结构化** - 支持JSON格式便于日志分析
4. **配置灵活** - 丰富的配置选项适应不同场景
5. **生产就绪** - 内置日志轮转和错误处理
6. **可扩展** - 支持多种日志框架兼容

## 📋 代码统计

- **总代码量**: 1,092 行
- **核心功能**: 高性能日志、轮转、上下文支持
- **子包**: klog、logrus、示例代码
- **测试覆盖**: 包含单元测试和基准测试

这个log包是一个企业级的日志解决方案，适用于从开发环境到生产环境的各种场景。
