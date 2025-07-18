# 发布订阅模型实现

本项目提供了企业级的发布订阅模型，底层基于 Redis Stream 实现。

## 发布订阅模型 (`pubsub.go`)

提供了简洁的发布订阅接口，底层使用 Redis Stream 实现企业级特性。

### 特点
- ✅ 消息持久化
- ✅ 消费组支持
- ✅ 消息确认机制
- ✅ 更好的错误处理和重试机制
- ✅ 中间件支持
- ✅ 指数退避重试
- ✅ 丰富的监控和日志
- ✅ 消息路由
- ✅ 插件化架构

### 使用场景
- 企业级消息处理
- 需要复杂错误处理的场景
- 需要中间件和监控的场景
- 微服务架构中的事件驱动
- 需要消息持久化和确认的场景
- 多消费者负载均衡

### 配置选项

```go
type Config struct {
    // Redis 连接配置
    Addr     string // Redis 地址
    Password string // Redis 密码
    DB       int    // Redis 数据库
    
    // 消费者配置
    ConsumerGroup string // 消费组名称
    Consumer      string // 消费者名称
    
    // 性能配置
    MaxLen          int64         // Stream 最大长度
    ClaimInterval   time.Duration // 声明间隔
    BlockTime       time.Duration // 阻塞时间
    ReadBatchSize   int64         // 批量读取大小
    
    // 重试配置
    MaxRetries      int           // 最大重试次数
    InitialInterval time.Duration // 初始重试间隔
    MaxInterval     time.Duration // 最大重试间隔
}
```

### 基本使用示例

```go
// 创建配置
config := pubsub.DefaultConfig()
config.Addr = "localhost:6379"
config.ConsumerGroup = "my-group"
config.Consumer = "consumer-1"

// 创建发布订阅实例
ps, err := pubsub.NewPubSub(config)
if err != nil {
    log.Fatal(err)
}
defer ps.Close()

// 定义消息处理器
handler := func(topic string, data []byte) error {
    fmt.Printf("Received: %s\n", string(data))
    return nil
}

// 订阅消息
ps.Subscriber().Subscribe(ctx, "my-topic", handler)

// 启动订阅者
go ps.Subscriber().Run(ctx)

// 发布消息
message := map[string]interface{}{
    "id": "123",
    "content": "Hello PubSub",
    "timestamp": time.Now(),
}
ps.Publisher().Publish(ctx, "my-topic", message)
```

### 带重试机制的使用示例

```go
// 配置重试参数
config := DefaultWatermillConfig()
config.MaxRetries = 3
config.InitialInterval = time.Millisecond * 100
config.MaxInterval = time.Second * 10

pubsub, err := NewWatermillPubSub(config)
if err != nil {
    log.Fatal(err)
}
defer pubsub.Close()

// 可能失败的处理器
retryHandler := func(topic string, data []byte) error {
    // 模拟业务逻辑，可能失败
    if rand.Float32() < 0.3 { // 30% 失败率
        return fmt.Errorf("processing failed")
    }
    fmt.Printf("Successfully processed: %s\n", string(data))
    return nil
}

// 使用带重试的订阅
pubsub.Subscriber.SubscribeWithRetry(ctx, "retry-topic", retryHandler)
go pubsub.Subscriber.Run(ctx)

// 发布消息
pubsub.Publisher.Publish(ctx, "retry-topic", "test message")
```

### 高级特性

#### 1. 消息元数据
```go
// 发布消息时自动添加元数据
pubsub.Publisher.Publish(ctx, "topic", message)
// 消息会包含 timestamp 和 source 等元数据
```

#### 2. 消费组管理
```go
// 多个消费者可以属于同一个消费组
config1 := DefaultWatermillConfig()
config1.ConsumerGroup = "group1"
config1.Consumer = "consumer-1"

config2 := DefaultWatermillConfig()
config2.ConsumerGroup = "group1"
config2.Consumer = "consumer-2"

// 消息会在消费组内的消费者之间负载均衡
```

#### 3. 指数退避重试
```go
// 自动计算重试间隔：100ms, 200ms, 400ms, 800ms, ...
// 最大不超过 MaxInterval
```

#### 4. 健康检查
```go
// 检查订阅者健康状态
if err := pubsub.Subscriber.HealthCheck(ctx); err != nil {
    log.Printf("Subscriber unhealthy: %v", err)
}
```

## 消息抽象

### 消息接口

```go
// Message 通用消息接口
type Message interface {
    GetType() string        // 获取消息类型
    GetSource() string      // 获取消息来源
    GetTimestamp() time.Time // 获取消息时间戳
    GetData() interface{}   // 获取消息数据
    Marshal() ([]byte, error) // 序列化消息
}

// BaseMessage 基础消息实现
type BaseMessage struct {
    Type      string      `json:"type"`
    Source    string      `json:"source"`
    Timestamp time.Time   `json:"timestamp"`
    Data      interface{} `json:"data"`
}
```

### 业务层消息示例

```go
// 在业务层定义具体的消息类型
type OrderCreatedMessage struct {
    *pubsub.BaseMessage
    OrderID    string  `json:"order_id"`
    CustomerID string  `json:"customer_id"`
    Amount     float64 `json:"amount"`
}

// 创建业务消息
func NewOrderCreatedMessage(orderID, customerID string, amount float64) *OrderCreatedMessage {
    data := map[string]interface{}{
        "order_id":    orderID,
        "customer_id": customerID,
        "amount":      amount,
    }
    
    return &OrderCreatedMessage{
        BaseMessage: pubsub.NewBaseMessage("order.created", "order-service", data),
        OrderID:     orderID,
        CustomerID:  customerID,
        Amount:      amount,
    }
}
```

## 运行示例

```bash
# 启动 Redis（如果还没有运行）
redis-server

# 运行基本示例
go run -c "
import \"github.com/yshujie/questionnaire-scale/pkg/pubsub\"
pubsub.RunExample()
"

# 运行带重试的示例
go run -c "
import \"github.com/yshujie/questionnaire-scale/pkg/pubsub\"
pubsub.RunRetryExample()
"

# 运行业务层示例
go run -c "
import \"github.com/yshujie/questionnaire-scale/pkg/pubsub\"
pubsub.RunBusinessExample()
"
```

## 依赖

确保在 `go.mod` 中添加了以下依赖：

```go
require (
    github.com/ThreeDotsLabs/watermill v1.4.7
    github.com/ThreeDotsLabs/watermill-redisstream v1.4.3
    github.com/redis/go-redis/v9 v9.11.0
)
```

## 监控和日志

Watermill 提供了丰富的日志输出，包括：
- 消息发布和接收日志
- 错误和重试日志
- 性能指标
- 连接状态信息

日志会自动集成到项目的日志系统中，便于监控和调试。

## 最佳实践

1. **合理设置消费组**：根据业务需求设置合适的消费组和消费者数量
2. **配置重试策略**：根据业务容错需求配置合适的重试次数和间隔
3. **监控消息积压**：定期检查 Stream 长度，避免消息积压
4. **优雅关闭**：确保在应用关闭时正确关闭发布者和订阅者
5. **错误处理**：实现合适的错误处理逻辑，避免消息丢失 