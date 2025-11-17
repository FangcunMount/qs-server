# RabbitMQ 实现

基于 `github.com/rabbitmq/amqp091-go` 的 RabbitMQ 实现。

## 特性

- ✅ 支持事件驱动模式（广播）
- ✅ 支持任务队列模式（负载均衡）
- ✅ 自动声明 exchange 和 queue
- ✅ 消息持久化
- ✅ 失败自动重试
- ✅ 优雅退出

## 快速开始

### 1. 启动 RabbitMQ

```bash
# 使用 Docker
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management
```

访问管理界面：http://localhost:15672 (用户名/密码：guest/guest)

### 2. 基本使用

```go
import "github.com/FangcunMount/qs-server/pkg/messaging/rabbitmq"

bus, _ := rabbitmq.NewEventBus("amqp://guest:guest@localhost:5672/")
defer bus.Close()

// 发布消息
bus.Publisher().Publish(ctx, "user.created", data)

// 订阅消息
bus.Subscriber().Subscribe("user.created", "email-service", handler)
```

## 运行示例

```bash
go run pkg/messaging/example/rabbitmq/main.go
```
