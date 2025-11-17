# RabbitMQ 详解

## 什么是 RabbitMQ？

RabbitMQ 是一个开源的消息代理（Message Broker），实现了 AMQP（Advanced Message Queuing Protocol）协议。它就像一个"邮局"，负责接收、存储和转发消息。

## 核心概念

### 1. 基本架构

```text
Producer → Exchange → Queue → Consumer
  生产者    交换机     队列     消费者
```

### 2. Exchange（交换机）

Exchange 接收消息并决定如何路由到队列。有 4 种类型：

#### Fanout（扇出/广播）

```text
        ┌─→ Queue1 → Consumer1
Exchange┼─→ Queue2 → Consumer2
        └─→ Queue3 → Consumer3
```

- **特点**：广播给所有绑定的队列
- **用途**：事件驱动、日志系统
- **例子**：用户注册后，通知邮件服务、统计服务、审计服务

#### Direct（直接路由）

```text
        routing_key="error" → Queue1 → Consumer1
Exchange routing_key="info"  → Queue2 → Consumer2
        routing_key="warn"  → Queue3 → Consumer3
```

- **特点**：根据 routing key 精确匹配
- **用途**：日志分级、消息分类
- **例子**：错误日志发到告警队列，普通日志发到普通队列

#### Topic（主题路由）

```text
        routing_key="user.*.created" → Queue1
Exchange routing_key="user.#"         → Queue2
        routing_key="order.*.paid"   → Queue3
```

- **特点**：支持通配符匹配
  - `*`：匹配一个单词
  - `#`：匹配零个或多个单词
- **用途**：复杂的消息分类
- **例子**：`user.email.created` 可以匹配 `user.*.created` 和 `user.#`

#### Headers（头部路由）

```text
Exchange → 根据消息头（headers）路由 → Queue
```

- **特点**：根据消息头属性路由
- **用途**：复杂的路由规则
- **例子**：根据消息的 `type` 和 `priority` 字段路由

### 3. Queue（队列）

队列存储消息，等待消费者消费。

**特性**：

- **持久化**：队列在 RabbitMQ 重启后仍然存在
- **独占**：队列只能被一个连接使用
- **自动删除**：没有消费者时自动删除

### 4. Binding（绑定）

连接 Exchange 和 Queue 的规则。

```go
// 绑定示例
queue.Bind(
    queueName,    // 队列名称
    routingKey,   // 路由键
    exchangeName, // 交换机名称
)
```

## 配置详解

### 基础配置

```yaml
messaging:
  provider: rabbitmq
  rabbitmq:
    # 方式1：使用 URL（推荐）
    url: amqp://guest:guest@localhost:5672/

    # 方式2：使用独立配置项
    host: localhost
    port: 5672
    username: guest
    password: guest
    vhost: /  # 虚拟主机，相当于命名空间
```

### VHost（虚拟主机）

VHost 是 RabbitMQ 中的命名空间，用于隔离不同应用的消息。

```text
RabbitMQ Server
├── VHost: /（默认）
│   ├── Exchange: user.events
│   ├── Queue: email-service
│   └── Queue: stat-service
├── VHost: /production
│   ├── Exchange: orders
│   └── Queue: payment-service
└── VHost: /development
    └── ...
```

**用途**：

- 开发环境和生产环境隔离
- 不同项目隔离
- 不同租户隔离（多租户系统）

### QoS（服务质量）配置

```yaml
rabbitmq:
  prefetch_count: 200  # 预取数量
  prefetch_size: 0     # 预取大小（0=不限制）
```

**prefetch_count**（重要）：

- 消费者一次最多预取多少条未确认的消息
- **值越大**：吞吐量越高，但内存占用也越大
- **值越小**：内存占用少，但吞吐量低
- **建议**：根据消息大小和处理速度调整

**例子**：

```go
// prefetch_count = 1（慢速处理）
Consumer1: [msg1] → 处理中...
Consumer2: [msg2] → 处理中...
// 其他消息在队列中等待

// prefetch_count = 100（快速处理）
Consumer1: [msg1, msg2, ..., msg100] → 批量处理
Consumer2: [msg101, msg102, ..., msg200] → 批量处理
```

### 持久化配置

```yaml
rabbitmq:
  durable: true              # Exchange 和 Queue 持久化
  persistent_messages: true  # 消息持久化
```

**持久化的意义**：

1. **durable: true**
   - Exchange 和 Queue 在 RabbitMQ 重启后不会丢失
   - 但队列中的消息可能丢失

2. **persistent_messages: true**
   - 消息会写入磁盘
   - RabbitMQ 重启后消息不会丢失
   - 性能会稍微降低（磁盘 IO）

**建议**：

- 生产环境：都设置为 `true`
- 开发环境：可以设置为 `false` 提高性能

### 连接配置

```yaml
rabbitmq:
  connection_timeout: 10s    # 连接超时
  heartbeat_interval: 10s    # 心跳间隔
  auto_reconnect: true       # 自动重连
  reconnect_delay: 5s        # 重连延迟
  max_reconnect_attempts: 0  # 最大重连次数（0=无限）
```

**heartbeat_interval**：

- 客户端和服务器之间的心跳检测
- 检测连接是否存活
- 建议：10-30 秒

**auto_reconnect**：

- 连接断开后自动重连
- 生产环境建议设置为 `true`

### Exchange 类型配置

```yaml
rabbitmq:
  exchange_type: fanout  # fanout, direct, topic, headers
```

**在我们的实现中，默认使用 fanout**：

- 实现事件驱动模式（广播）
- 简单易用
- 性能好

## 使用场景

### 场景 1：事件驱动（广播）

```go
// 配置
config := &messaging.Config{
    Provider: messaging.ProviderRabbitMQ,
    RabbitMQ: messaging.RabbitMQConfig{
        URL:          "amqp://guest:guest@localhost:5672/",
        ExchangeType: "fanout", // 广播模式
    },
}

bus, _ := messaging.NewEventBus(config)

// 订阅者 1：邮件服务
bus.Subscriber().Subscribe("user.created", "email-service", emailHandler)

// 订阅者 2：统计服务
bus.Subscriber().Subscribe("user.created", "stat-service", statHandler)

// 订阅者 3：审计服务
bus.Subscriber().Subscribe("user.created", "audit-service", auditHandler)

// 发布一条消息，三个服务都会收到
bus.Publisher().Publish(ctx, "user.created", data)
```

**RabbitMQ 内部结构**：

```text
Producer
   ↓
Exchange(user.created, type=fanout)
   ├→ Queue(email-service) → Consumer(邮件服务)
   ├→ Queue(stat-service)  → Consumer(统计服务)
   └→ Queue(audit-service) → Consumer(审计服务)
```

### 场景 2：任务队列（负载均衡）

```go
// 多个 worker 使用相同的队列名
bus.Subscriber().Subscribe("email.send", "worker-group", handler) // Worker 1
bus.Subscriber().Subscribe("email.send", "worker-group", handler) // Worker 2
bus.Subscriber().Subscribe("email.send", "worker-group", handler) // Worker 3

// 发布 1000 条消息，3 个 worker 会分担处理
for i := 0; i < 1000; i++ {
    bus.Publisher().Publish(ctx, "email.send", data)
}
```

**RabbitMQ 内部结构**：

```text
Producer
   ↓
Exchange(email.send, type=fanout)
   ↓
Queue(worker-group)
   ├→ Consumer1 (处理 msg1, msg4, msg7, ...)
   ├→ Consumer2 (处理 msg2, msg5, msg8, ...)
   └→ Consumer3 (处理 msg3, msg6, msg9, ...)
```

## 与 NSQ 的对比

| 特性 | RabbitMQ | NSQ |
|------|----------|-----|
| **架构** | 集中式（单点或集群） | 分布式（去中心化） |
| **部署复杂度** | 较高 | 低 |
| **学习曲线** | 陡峭（概念多） | 平缓 |
| **功能丰富度** | 非常丰富 | 简单实用 |
| **路由能力** | 强大（4 种 Exchange） | 简单（topic + channel） |
| **消息持久化** | 支持 | 支持 |
| **管理界面** | 功能强大 | 简单实用 |
| **性能** | 优秀（2-3 万 msg/s） | 极高（10 万+ msg/s） |
| **社区** | 成熟庞大 | 活跃 |
| **适用场景** | 复杂的企业应用 | 高性能、简单场景 |

## 最佳实践

### 1. 开发环境

```bash
# 使用 Docker 快速启动
docker run -d --name rabbitmq \
  -p 5672:5672 \
  -p 15672:15672 \
  rabbitmq:3-management

# 访问管理界面
open http://localhost:15672
# 用户名/密码：guest/guest
```

### 2. 生产环境

```yaml
rabbitmq:
  # 使用高可用集群
  url: amqp://user:pass@rabbitmq-cluster:5672/production
  
  # 持久化配置
  durable: true
  persistent_messages: true
  
  # QoS 配置（根据实际情况调整）
  prefetch_count: 100
  
  # 连接配置
  connection_timeout: 10s
  heartbeat_interval: 10s
  auto_reconnect: true
  reconnect_delay: 5s
  
  # Exchange 配置
  exchange_type: fanout
```

### 3. 监控指标

访问管理界面（<http://localhost:15672）监控：>

- **队列长度**：过长说明消费速度跟不上
- **消息速率**：发布/消费速率
- **未确认消息**：过多说明处理太慢或有问题
- **连接数**：监控连接泄漏

### 4. 常见问题

#### 问题1：消息丢失

**原因**：

- 持久化未开启
- 消费者崩溃前未确认

**解决**：

```yaml
durable: true
persistent_messages: true
```

#### 问题2：消息堆积

**原因**：

- 消费速度慢
- 消费者数量不足

**解决**：

```yaml
prefetch_count: 50  # 降低预取数量
# 增加消费者数量
```

#### 问题3：内存溢出

**原因**：

- 消息堆积太多
- prefetch_count 设置过大

**解决**：

```yaml
prefetch_count: 10  # 降低预取数量
# 增加消费者，加快消费速度
```

## 配置示例

### 高性能配置

```yaml
rabbitmq:
  url: amqp://guest:guest@localhost:5672/
  prefetch_count: 500           # 高吞吐量
  durable: false                # 不持久化
  persistent_messages: false    # 不持久化消息
  exchange_type: fanout
```

### 高可靠配置

```yaml
rabbitmq:
  url: amqp://guest:guest@localhost:5672/
  prefetch_count: 50            # 适中
  durable: true                 # 持久化
  persistent_messages: true     # 持久化消息
  auto_reconnect: true          # 自动重连
  heartbeat_interval: 10s       # 心跳检测
  exchange_type: fanout
```

### 平衡配置（推荐）

```yaml
rabbitmq:
  url: amqp://guest:guest@localhost:5672/
  prefetch_count: 200           # 平衡
  durable: true                 # 持久化
  persistent_messages: true     # 持久化消息
  auto_reconnect: true
  reconnect_delay: 5s
  exchange_type: fanout
```
