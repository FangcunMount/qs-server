# Messaging - 统一消息抽象层

> **设计理念**：提供 Watermill 级别的消息传递抽象，而非简单封装某个具体的消息中间件。NSQ、RabbitMQ 只是底层实现的 Adapter。

## 📚 目录

- [核心概念](#核心概念)
- [架构设计](#架构设计)
- [快速开始](#快速开始)
- [核心组件](#核心组件)
- [中间件系统](#中间件系统)
- [配置指南](#配置指南)
- [最佳实践](#最佳实践)
- [示例代码](#示例代码)

---

## 核心概念

### 设计目标

Messaging 包的设计目标是提供一个**统一的消息传递抽象层**，它关注三个核心维度：

1. **消息中间件初始化**
   - 配置管理（Options）
   - 连接建立（Connection）
   - 优雅关停（Shutdown）

2. **消息中间件使用**
   - 发布订阅模型（Publisher/Subscriber）
   - 消息抽象（Message with Metadata）
   - 确认机制（Ack/Nack）

3. **横切关注点**
   - 中间件链（日志、重试、超时、限流...）
   - 路由管理（统一注册和调度）
   - 可观测性（健康检查、指标、追踪）

### 关键特性

- ✅ **统一抽象**：业务代码只依赖接口，不依赖具体实现
- ✅ **开闭原则**：通过 Provider 模式轻松扩展新的消息中间件
- ✅ **中间件支持**：提供 15+ 种内置中间件，支持自定义扩展
- ✅ **消息增强**：UUID、Metadata、Ack/Nack 完整支持
- ✅ **路由器**：统一管理消息处理器，支持批量注册
- ✅ **生产就绪**：健康检查、优雅关闭、错误恢复

---

## 架构设计

### 六边形架构（端口-适配器模式）

```text
┌───────────────────────────────────────────────────────────┐
│                  应用层（Business Logic）                   │
│                                                             │
│   • 只依赖 messaging.EventBus 接口                          │
│   • 使用 messaging.Message 统一消息模型                      │
│   • 通过配置切换底层实现（NSQ/RabbitMQ）                     │
└──────────────────────┬────────────────────────────────────┘
                       │
                       │ 依赖接口（Port）
                       ↓
┌───────────────────────────────────────────────────────────┐
│                messaging 包（端口层 - Port）                │
│                                                             │
│  核心抽象：                                                  │
│  ┌─────────────────────────────────────────────────────┐  │
│  │ • EventBus      - 事件总线接口                       │  │
│  │ • Publisher     - 发布者接口                         │  │
│  │ • Subscriber    - 订阅者接口                         │  │
│  │ • Message       - 消息模型（UUID/Metadata/Payload）  │  │
│  │ • Handler       - 消息处理函数                       │  │
│  │ • Middleware    - 中间件函数                         │  │
│  │ • Router        - 路由器                            │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                             │
│  工厂模式：                                                  │
│  • Provider        - 提供者枚举（NSQ/RabbitMQ）           │
│  • RegisterProvider - 自动注册机制                        │
│  • NewEventBus     - 统一创建入口                         │
└──────────────────────┬────────────────────────────────────┘
                       │
          ┌────────────┴─────────────┐
          │                          │
          ↓                          ↓
┌─────────────────────┐    ┌─────────────────────┐
│  nsq 包（适配器）     │    │ rabbitmq 包（适配器） │
│                     │    │                     │
│  • Publisher        │    │  • Publisher        │
│  • Subscriber       │    │  • Subscriber       │
│  • EventBus         │    │  • EventBus         │
│                     │    │                     │
│  实现细节：          │    │  实现细节：          │
│  • NSQ 协议封装     │    │  • AMQP 协议封装    │
│  • 消息转换         │    │  • Exchange/Queue   │
│  • 自动重连         │    │  • 持久化配置       │
└─────────────────────┘    └─────────────────────┘
```

### 消息流转

```text
发布流程：
Publisher.Publish() → Adapter 转换 → NSQ/RabbitMQ → 网络传输

订阅流程：
网络接收 → NSQ/RabbitMQ → Adapter 转换 → Middleware 链 → Handler
                                              ↓
                                         Ack/Nack
```

---

## 快速开始

### 安装

```bash
go get github.com/FangcunMount/qs-server/pkg/messaging
```

### 5 分钟上手

```go
package main

import (
    "context"
    "log"
    
    "github.com/FangcunMount/qs-server/pkg/messaging"
    _ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

func main() {
    // 1. 创建配置
    config := messaging.DefaultConfig()
    
    // 2. 创建事件总线
    bus, err := messaging.NewEventBus(config)
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()
    
    // 3. 订阅消息
    bus.Subscriber().Subscribe("user.created", "email-service", 
        func(ctx context.Context, msg *messaging.Message) error {
            log.Printf("收到消息: %s", string(msg.Payload))
            return msg.Ack() // 确认消息
        })
    
    // 4. 发布消息
    bus.Publisher().Publish(context.Background(), 
        "user.created", []byte(`{"user_id": 123}`))
    
    select {} // 保持运行
}
```

### 切换到 RabbitMQ

只需修改一行配置：

```go
config := messaging.DefaultConfig()
config.Provider = messaging.ProviderRabbitMQ  // 切换到 RabbitMQ
config.RabbitMQ.URL = "amqp://guest:guest@localhost:5672/"
```

---

## 核心组件

### 1. Message（消息模型）

**设计理念**：参考 Watermill，提供完整的消息抽象。

```go
type Message struct {
    // 核心字段
    UUID     string            // 全局唯一标识
    Payload  []byte            // 消息负载
    Metadata map[string]string // 元数据（链路追踪、业务标识）
    
    // 运行时字段
    Attempts  uint16    // 重试次数
    Timestamp int64     // 时间戳
    Topic     string    // 主题
    Channel   string    // 通道
}
```

**核心方法**：

```go
// 创建消息
msg := messaging.NewMessage("uuid-123", payload)
msg.Metadata["trace_id"] = "trace-abc"
msg.Metadata["user_id"] = "1001"

// 确认消息（处理成功）
msg.Ack()

// 拒绝消息（触发重试）
msg.Nack()
```

**为什么需要 Metadata？**

- **链路追踪**：传递 trace_id、span_id
- **业务标识**：传递 user_id、tenant_id
- **消息路由**：传递 priority、group
- **调试信息**：传递 source、version

### 2. Publisher（发布者）

```go
type Publisher interface {
    // 发布字节数组（快速）
    Publish(ctx context.Context, topic string, body []byte) error
    
    // 发布消息对象（支持 Metadata）
    PublishMessage(ctx context.Context, topic string, msg *Message) error
    
    // 关闭发布者
    Close() error
}
```

**使用示例**：

```go
// 方式 1：快速发布
publisher.Publish(ctx, "user.created", []byte(`{"id": 123}`))

// 方式 2：带 Metadata 发布
msg := messaging.NewMessage("", []byte(`{"id": 123}`))
msg.Metadata["trace_id"] = "trace-123"
publisher.PublishMessage(ctx, "user.created", msg)
```

### 3. Subscriber（订阅者）

```go
type Subscriber interface {
    // 订阅消息
    Subscribe(topic, channel string, handler Handler) error
    
    // 订阅消息（支持中间件）
    SubscribeWithMiddleware(topic, channel string, 
        handler Handler, middlewares ...Middleware) error
    
    // 停止订阅
    Stop()
    
    // 关闭订阅者
    Close() error
}
```

**Topic vs Channel**：

- **Topic**：消息主题（如 `user.created`）
- **Channel**：消费者分组
  - 相同 channel：负载均衡（任务队列模式）
  - 不同 channel：广播（事件驱动模式）

```go
// 事件驱动：每个服务使用不同的 channel
subscriber.Subscribe("user.created", "email-service", emailHandler)
subscriber.Subscribe("user.created", "stat-service", statHandler)
// → 每条消息都会被两个服务接收

// 任务队列：多个 worker 使用相同的 channel
subscriber.Subscribe("email.send", "email-workers", handler1)
subscriber.Subscribe("email.send", "email-workers", handler2)
// → 每条消息只会被一个 worker 接收
```

### 4. Router（路由器）

**设计理念**：统一管理消息处理器，支持中间件链。

```go
// 创建路由器
router := bus.Router()

// 添加全局中间件（应用到所有处理器）
router.AddMiddleware(messaging.LoggerMiddleware(logger))
router.AddMiddleware(messaging.RecoverMiddleware(logger))

// 注册处理器（不带中间件）
router.AddHandler("user.created", "email-service", emailHandler)

// 注册处理器（带局部中间件）
router.AddHandlerWithMiddleware(
    "order.payment", 
    "payment-service", 
    paymentHandler,
    messaging.RetryMiddleware(3, time.Second),
    messaging.TimeoutMiddleware(5 * time.Second),
)

// 启动路由器（批量订阅）
ctx, cancel := context.WithCancel(context.Background())
go router.Run(ctx)

// 优雅关闭
router.Stop()
```

### 5. EventBus（事件总线）

**设计理念**：组合 Publisher、Subscriber、Router，提供完整的消息总线功能。

```go
type EventBus interface {
    Publisher() Publisher      // 获取发布者
    Subscriber() Subscriber    // 获取订阅者
    Router() *Router          // 获取路由器
    Health() error            // 健康检查
    Close() error             // 关闭总线
}
```

---

## 中间件系统

### 设计理念

**中间件**是处理横切关注点的标准方式，采用**洋葱模型**：

```text
Request → MW1 → MW2 → MW3 → Handler → MW3 → MW2 → MW1 → Response
          ↓     ↓     ↓       ↓       ↑     ↑     ↑
        日志   重试  超时    业务    超时  重试  日志
```

### 中间件类型

```go
type Middleware func(Handler) Handler
```

### 内置中间件（15 种）

#### 1. 可靠性中间件

| 中间件 | 功能 | 使用场景 |
|--------|------|----------|
| **RetryMiddleware** | 自动重试（指数退避） | 网络抖动、临时故障 |
| **TimeoutMiddleware** | 超时控制 | 防止处理时间过长 |
| **RecoverMiddleware** | Panic 恢复 | 防止单个消息崩溃整个服务 |
| **CircuitBreakerMiddleware** | 熔断器 | 防止级联故障 |

```go
// 示例：组合可靠性中间件
router.AddHandlerWithMiddleware(
    "order.payment",
    "payment-service",
    handler,
    messaging.RecoverMiddleware(logger),        // 最外层：捕获 panic
    messaging.RetryMiddleware(3, time.Second),  // 重试 3 次
    messaging.TimeoutMiddleware(10*time.Second), // 超时 10 秒
)
```

#### 2. 流量控制中间件

| 中间件 | 功能 | 使用场景 |
|--------|------|----------|
| **RateLimitMiddleware** | 限流（令牌桶） | 防止系统过载 |
| **BatchMiddleware** | 批处理 | 提高吞吐量 |
| **FilterMiddleware** | 条件过滤 | 选择性处理消息 |
| **PriorityMiddleware** | 优先级排序 | VIP 消息优先处理 |

```go
// 示例：限流（每秒 100 个请求）
limiter := messaging.NewTokenBucketLimiter(100, 10*time.Millisecond)
router.AddMiddleware(messaging.RateLimitMiddleware(limiter, "drop"))

// 示例：过滤高价值订单
filterMW := messaging.FilterMiddleware(func(msg *messaging.Message) bool {
    var order Order
    json.Unmarshal(msg.Payload, &order)
    return order.Amount > 1000 // 只处理金额 > 1000 的订单
})
```

#### 3. 可观测性中间件

| 中间件 | 功能 | 使用场景 |
|--------|------|----------|
| **LoggerMiddleware** | 日志记录 | 调试、审计 |
| **MetricsMiddleware** | 指标收集 | 监控、告警 |
| **TracingMiddleware** | 链路追踪 | 分布式追踪 |
| **AuditMiddleware** | 审计日志 | 合规、安全 |

```go
// 示例：完整的可观测性栈
router.AddMiddleware(messaging.LoggerMiddleware(logger))
router.AddMiddleware(messaging.TracingMiddleware())
router.AddMiddleware(messaging.MetricsMiddleware(metricsCollector))
```

#### 4. 数据处理中间件

| 中间件 | 功能 | 使用场景 |
|--------|------|----------|
| **DeduplicationMiddleware** | 消息去重 | 防止重复处理 |
| **TransformMiddleware** | 消息转换 | 数据格式转换 |
| **ValidationMiddleware** | 消息校验 | 数据合法性检查 |

### 自定义中间件

```go
// 示例：自定义认证中间件
func AuthMiddleware(authService AuthService) messaging.Middleware {
    return func(next messaging.Handler) messaging.Handler {
        return func(ctx context.Context, msg *messaging.Message) error {
            // 从 Metadata 提取 token
            token := msg.Metadata["auth_token"]
            
            // 验证 token
            user, err := authService.ValidateToken(token)
            if err != nil {
                return fmt.Errorf("认证失败: %w", err)
            }
            
            // 将用户信息注入 context
            ctx = context.WithValue(ctx, "user", user)
            
            // 继续处理
            return next(ctx, msg)
        }
    }
}
```

---

## 配置指南

### 统一配置结构

```go
type Config struct {
    Provider Provider    // nsq | rabbitmq
    NSQ      NSQConfig
    RabbitMQ RabbitMQConfig
}
```

### NSQ 配置

```go
config := &messaging.Config{
    Provider: messaging.ProviderNSQ,
    NSQ: messaging.NSQConfig{
        LookupdAddrs: []string{"127.0.0.1:4161"},
        NSQdAddr:     "127.0.0.1:4150",
        MaxAttempts:  5,           // 最大重试次数
        MaxInFlight:  200,         // 并发处理数
        MsgTimeout:   time.Minute, // 消息超时
    },
}
```

### RabbitMQ 配置

```go
config := &messaging.Config{
    Provider: messaging.ProviderRabbitMQ,
    RabbitMQ: messaging.RabbitMQConfig{
        URL:               "amqp://guest:guest@localhost:5672/",
        PrefetchCount:     200,   // QoS
        Durable:           true,  // 持久化
        PersistentMessages: true, // 消息持久化
        AutoReconnect:     true,  // 自动重连
    },
}
```

### 默认配置

```go
config := messaging.DefaultConfig() // 使用 NSQ 默认配置
```

---

## 最佳实践

### 1. 消息设计

**✅ 推荐**：

```go
// 使用结构化的消息体
type UserCreatedEvent struct {
    UserID    int64     `json:"user_id"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`
}

// 发布时序列化
data, _ := json.Marshal(event)
msg := messaging.NewMessage("", data)
msg.Metadata["event_type"] = "user.created"
msg.Metadata["version"] = "v1"
publisher.PublishMessage(ctx, "user.created", msg)
```

**❌ 不推荐**：

```go
// 不要在消息中包含大量数据
// 不要使用二进制格式（除非必要）
// 不要在 Metadata 中放敏感信息
```

### 2. 错误处理

```go
func handler(ctx context.Context, msg *messaging.Message) error {
    // 可重试错误：返回 error，触发重试
    if err := processMessage(msg); err != nil {
        return fmt.Errorf("处理失败: %w", err)
    }
    
    // 不可重试错误：记录日志，返回 nil
    if err := validateMessage(msg); err != nil {
        log.Printf("消息格式错误，跳过: %v", err)
        return nil // 不重试
    }
    
    // 成功：确认消息
    return msg.Ack()
}
```

### 3. 中间件顺序

**推荐顺序**（从外到内）：

```go
router.AddHandlerWithMiddleware(
    "order.payment",
    "payment-service",
    handler,
    messaging.RecoverMiddleware(logger),     // 1. 最外层：捕获 panic
    messaging.LoggerMiddleware(logger),      // 2. 日志
    messaging.TracingMiddleware(),           // 3. 链路追踪
    messaging.TimeoutMiddleware(30*time.Second), // 4. 超时控制
    messaging.RetryMiddleware(3, time.Second),   // 5. 重试
    messaging.DeduplicationMiddleware(store, time.Hour), // 6. 去重
)
```

### 4. 性能优化

```go
// 1. 调整并发数
config.NSQ.MaxInFlight = 500 // 根据 CPU 核心数调整

// 2. 使用批量发布
bodies := [][]byte{data1, data2, data3}
publisher.(*nsq.Publisher).MultiPublish(ctx, "topic", bodies)

// 3. 启用限流（防止突发流量）
limiter := messaging.NewTokenBucketLimiter(1000, time.Millisecond)
router.AddMiddleware(messaging.RateLimitMiddleware(limiter, "wait"))
```

---

## 示例代码

### 示例 1：事件驱动架构

```go
// 场景：用户注册后，通知多个服务
publisher.Publish(ctx, "user.created", userData)

// 邮件服务（独立 channel）
subscriber.Subscribe("user.created", "email-service", emailHandler)

// 统计服务（独立 channel）
subscriber.Subscribe("user.created", "stat-service", statHandler)

// 审计服务（独立 channel）
subscriber.Subscribe("user.created", "audit-service", auditHandler)
```

### 示例 2：任务队列

```go
// 场景：10 个 worker 处理邮件发送任务
for i := 1; i <= 10; i++ {
    go func(workerID int) {
        // 所有 worker 使用相同 channel
        subscriber.Subscribe("email.send", "email-workers", 
            func(ctx context.Context, msg *messaging.Message) error {
                log.Printf("Worker %d 处理邮件", workerID)
                return sendEmail(msg)
            })
    }(i)
}
```

### 示例 3：中间件组合

```go
// 场景：支付服务需要高可靠性
router := bus.Router()

// 全局中间件
router.AddMiddleware(messaging.RecoverMiddleware(logger))
router.AddMiddleware(messaging.LoggerMiddleware(logger))

// 局部中间件（只用于支付）
breaker := messaging.NewSimpleCircuitBreaker(5, 30*time.Second)
router.AddHandlerWithMiddleware(
    "order.payment",
    "payment-service",
    paymentHandler,
    messaging.CircuitBreakerMiddleware(breaker),
    messaging.RetryMiddleware(3, 2*time.Second),
    messaging.TimeoutMiddleware(15*time.Second),
)

router.Run(ctx)
```

### 示例 4：链路追踪

```go
// 发布时注入 trace_id
msg := messaging.NewMessage("", payload)
msg.Metadata["trace_id"] = "trace-" + uuid.New().String()
msg.Metadata["span_id"] = "span-" + uuid.New().String()
publisher.PublishMessage(ctx, "user.created", msg)

// 消费时提取 trace_id
handler := func(ctx context.Context, msg *messaging.Message) error {
    traceID := msg.Metadata["trace_id"]
    spanID := msg.Metadata["span_id"]
    
    log.Printf("处理消息 [trace=%s, span=%s]", traceID, spanID)
    
    // 继续传播 trace_id
    nextMsg := messaging.NewMessage("", nextPayload)
    nextMsg.Metadata["trace_id"] = traceID
    nextMsg.Metadata["parent_span_id"] = spanID
    nextMsg.Metadata["span_id"] = "span-" + uuid.New().String()
    
    return nil
}
```

---

## 进阶主题

### Provider 扩展

如何添加新的消息中间件（如 Kafka）：

```go
// 1. 实现 Publisher、Subscriber、EventBus 接口
// 2. 在 init 函数中注册
func init() {
    messaging.RegisterProvider(messaging.ProviderKafka, NewEventBusFromConfig)
}

// 3. 业务代码无需修改，只需切换配置
config.Provider = messaging.ProviderKafka
```

### 健康检查集成

```go
// HTTP 健康检查接口
http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    if err := bus.Health(); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "unhealthy",
            "error":  err.Error(),
        })
        return
    }
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "healthy",
    })
})
```

### 优雅关闭

```go
func main() {
    bus, _ := messaging.NewEventBus(config)
    defer bus.Close()
    
    // 监听退出信号
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    router := bus.Router()
    // ... 注册处理器
    
    ctx, cancel := context.WithCancel(context.Background())
    go router.Run(ctx)
    
    // 等待退出信号
    <-sigChan
    
    log.Println("正在优雅退出...")
    
    // 1. 停止接收新消息
    cancel()
    router.Stop()
    
    // 2. 等待正在处理的消息完成（最多 30 秒）
    time.Sleep(30 * time.Second)
    
    // 3. 关闭连接
    bus.Close()
    
    log.Println("退出完成")
}
```

---

## 常见问题

### Q1: NSQ 和 RabbitMQ 如何选择？

| 特性 | NSQ | RabbitMQ |
|------|-----|----------|
| 部署复杂度 | ⭐⭐ 简单 | ⭐⭐⭐ 中等 |
| 性能 | ⭐⭐⭐⭐⭐ 极高 | ⭐⭐⭐⭐ 高 |
| 功能丰富度 | ⭐⭐⭐ 基础 | ⭐⭐⭐⭐⭐ 丰富 |
| 消息持久化 | ⭐⭐⭐ 有限 | ⭐⭐⭐⭐⭐ 强大 |
| 适用场景 | 高吞吐、简单队列 | 复杂路由、企业级 |

**推荐**：

- 开发环境 / 简单场景：NSQ
- 生产环境 / 复杂需求：RabbitMQ

### Q2: 消息会丢失吗？

**NSQ**：

- 默认内存队列，重启会丢失
- 可配置 `--mem-queue-size=0` 强制磁盘持久化

**RabbitMQ**：

- 设置 `Durable: true` + `PersistentMessages: true` 保证持久化
- 需要手动 Ack 确认

### Q3: 如何保证消息顺序？

**方案 1**：单 Worker（降低并发）

```go
config.NSQ.MaxInFlight = 1 // 一次只处理一条
```

**方案 2**：分区（按 key 路由）

```go
// 同一个 user_id 的消息发送到同一个队列
topic := fmt.Sprintf("user.%d.events", userID)
```

### Q4: 如何处理毒消息（Poison Message）？

```go
func handler(ctx context.Context, msg *messaging.Message) error {
    // 检查重试次数
    if msg.Attempts > 5 {
        // 发送到死信队列
        dlq.Publish(ctx, "dlq.user.created", msg.Payload)
        return nil // 不再重试
    }
    
    // 继续处理
    return processMessage(msg)
}
```

---

## 附录

### A. 完整 API 参考

查看源码注释：

- `port.go` - 核心接口定义
- `middleware.go` - 所有中间件
- `router.go` - 路由器实现
- `config.go` - 配置结构

### B. 示例代码目录

```text
example/
├── simple/              # 基础发布订阅
├── event-driven/        # 事件驱动架构
├── task-queue/          # 任务队列模式
├── middleware/          # 中间件基础使用
├── advanced-middleware/ # 高级中间件（限流、熔断）
├── unified/             # Provider 切换演示
├── semantic/            # 语义化辅助函数
└── rabbitmq/            # RabbitMQ 特定功能
```

### C. 性能基准

```bash
# NSQ
吞吐量：100,000 msg/s（单机）
延迟：P99 < 10ms

# RabbitMQ
吞吐量：50,000 msg/s（单机）
延迟：P99 < 50ms
```

---

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可

MIT License
