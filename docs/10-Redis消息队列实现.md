# Redis 消息队列实现

## 📋 概述

本文档描述了如何使用 Redis 实现消息队列，实现 collection-server 和 evaluation-server 之间的异步通信。

## 🏗️ 架构设计

```
┌─────────────────┐    Redis Pub/Sub    ┌─────────────────┐
│ collection-server│ ──────────────────→ │ evaluation-server│
│    (发布者)      │                     │    (订阅者)      │
└─────────────────┘                     └─────────────────┘
```

### 消息流程

1. **答卷提交**: 用户通过 collection-server 提交答卷
2. **数据保存**: collection-server 调用 apiserver 保存答卷数据
3. **消息发布**: collection-server 发布 "答卷已保存" 消息到 Redis
4. **消息订阅**: evaluation-server 订阅并接收消息
5. **业务处理**: evaluation-server 处理消息，生成解读报告

## 🔧 实现细节

### 1. 通用消息队列模块

位置: `pkg/pubsub/redis.go`

```go
// RedisPublisher Redis发布者
type RedisPublisher struct {
    client *redis.Client
    config *RedisConfig
}

// RedisSubscriber Redis订阅者
type RedisSubscriber struct {
    client   *redis.Client
    config   *RedisConfig
    handlers map[string]MessageHandler
}
```

### 2. 消息结构定义

位置: `pkg/pubsub/pubsub.go`

```go
// ResponseSavedMessage 答卷已保存消息
type ResponseSavedMessage struct {
    ResponseID      string `json:"response_id"`
    QuestionnaireID string `json:"questionnaire_id"`
    UserID          string `json:"user_id"`
    SubmittedAt     int64  `json:"submitted_at"`
}
```

### 3. Collection Server (发布者)

#### 配置
```yaml
# configs/collection-server.yaml
redis:
  host: 127.0.0.1
  port: 6379
  password: ""
  database: 0
```

#### 核心代码
```go
// 发布答卷已保存消息
message := &pubsub.ResponseSavedMessage{
    ResponseID:      strconv.FormatUint(resp.Id, 10),
    QuestionnaireID: req.QuestionnaireCode,
    UserID:          strconv.FormatUint(req.TesteeID, 10),
    SubmittedAt:     time.Now().Unix(),
}

err := h.publisher.Publish(ctx, "answersheet.saved", message)
```

### 4. Evaluation Server (订阅者)

#### 配置
```yaml
# configs/evaluation-server.yaml
message_queue:
  type: redis
  endpoint: localhost:6379
  topic: answersheet.saved
  group: evaluation_group
```

#### 核心代码
```go
// 消息处理器
func (h *handler) HandleAnswersheetSaved(ctx context.Context, message []byte) error {
    var savedMsg pubsub.ResponseSavedMessage
    if err := json.Unmarshal(message, &savedMsg); err != nil {
        return fmt.Errorf("failed to unmarshal message: %w", err)
    }
    
    // 处理业务逻辑
    log.Infof("Processing answersheet: %s", savedMsg.ResponseID)
    return nil
}
```

## 🚀 使用指南

### 1. 环境准备

```bash
# 启动 Redis 服务器
redis-server

# 或者使用 Docker
docker run -d -p 6379:6379 redis:latest
```

### 2. 编译服务

```bash
make build
```

### 3. 启动服务

#### 方式一: 使用测试脚本 (推荐)
```bash
# 启动所有服务
./test-message-queue.sh
```

#### 方式二: 手动启动
```bash
# 启动 apiserver
./qs-apiserver --config=configs/qs-apiserver.yaml &

# 启动 evaluation-server (订阅者)
./evaluation-server --config=configs/evaluation-server.yaml &

# 启动 collection-server (发布者)
./collection-server --config=configs/collection-server.yaml &
```

### 4. 测试消息队列

```bash
# 提交测试答卷
./test-answersheet-submit.sh
```

### 5. 查看日志

```bash
# 查看 evaluation-server 日志
tail -f logs/evaluation-server.log

# 查看 collection-server 日志
tail -f logs/collection-server.log
```

## 📊 监控和调试

### 1. 健康检查

```bash
# 检查服务状态
curl http://localhost:8081/healthz  # collection-server
curl http://localhost:8082/healthz  # evaluation-server
```

### 2. Redis 监控

```bash
# 连接 Redis CLI
redis-cli

# 监控发布订阅
MONITOR

# 查看订阅者
PUBSUB CHANNELS
PUBSUB NUMSUB answersheet.saved
```

### 3. 消息调试

```bash
# 手动发布消息
redis-cli PUBLISH answersheet.saved '{"response_id":"123","questionnaire_id":"PHQ9","user_id":"456","submitted_at":1640995200}'

# 手动订阅消息
redis-cli SUBSCRIBE answersheet.saved
```

## 🔍 故障排除

### 常见问题

1. **Redis 连接失败**
   - 检查 Redis 服务是否启动
   - 确认端口和地址配置正确
   - 检查防火墙设置

2. **消息未被接收**
   - 确认订阅者已启动
   - 检查主题名称是否一致
   - 查看日志中的错误信息

3. **消息处理失败**
   - 检查消息格式是否正确
   - 确认处理器逻辑无误
   - 查看详细错误日志

### 日志级别

```yaml
# 开发环境
log:
  level: debug

# 生产环境
log:
  level: info
```

## 🎯 性能优化

### 1. 连接池配置

```go
redis.NewClient(&redis.Options{
    Addr:         "localhost:6379",
    PoolSize:     10,
    MinIdleConns: 5,
    MaxRetries:   3,
})
```

### 2. 批量处理

```go
// 批量订阅多个主题
topics := []string{"answersheet.saved", "questionnaire.updated"}
subscriber.SubscribeMultiple(ctx, topics)
```

### 3. 错误重试

```go
// 添加重试逻辑
for attempts := 0; attempts < 3; attempts++ {
    if err := publisher.Publish(ctx, topic, message); err == nil {
        break
    }
    time.Sleep(time.Second * time.Duration(attempts+1))
}
```

## 📈 扩展功能

### 1. 消息持久化

使用 Redis Streams 替代 Pub/Sub 以获得消息持久化：

```go
// 使用 XADD 发布消息
client.XAdd(&redis.XAddArgs{
    Stream: "answersheet_stream",
    Values: map[string]interface{}{
        "data": messageJSON,
    },
})
```

### 2. 消息确认机制

```go
// 消息处理成功后发送确认
func (h *handler) HandleMessage(msg []byte) error {
    if err := h.processMessage(msg); err != nil {
        return err
    }
    
    // 发送确认消息
    return h.sendAck(msg)
}
```

### 3. 死信队列

```go
// 处理失败的消息
func (h *handler) HandleFailedMessage(msg []byte, err error) {
    deadLetterTopic := "answersheet.failed"
    h.publisher.Publish(ctx, deadLetterTopic, msg)
}
```

## 🔒 安全考虑

### 1. Redis 认证

```yaml
redis:
  password: "your-redis-password"
```

### 2. 网络安全

```yaml
redis:
  addr: "redis.internal:6379"  # 内网地址
  use_tls: true                # 启用 TLS
```

### 3. 消息加密

```go
// 消息加密
func (p *RedisPublisher) PublishEncrypted(ctx context.Context, topic string, message interface{}) error {
    data, _ := json.Marshal(message)
    encrypted := encrypt(data)
    return p.client.Publish(topic, encrypted).Err()
}
```

## 📚 参考资料

- [Redis Pub/Sub 官方文档](https://redis.io/docs/interact/pubsub/)
- [Go Redis 客户端](https://github.com/go-redis/redis)
- [消息队列最佳实践](https://redis.io/docs/interact/pubsub/#pattern-matching) 