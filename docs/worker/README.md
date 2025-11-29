# QS Worker 服务

## 概述

`qs-worker` 是问卷量表系统的后台事件处理服务，负责消费领域事件并执行异步任务处理。

## 架构

```
qs-worker
├── cmd/qs-worker/          # 入口点
│   └── main.go
├── internal/worker/        # Worker 服务实现
│   ├── app.go              # App 定义
│   ├── run.go              # 启动逻辑
│   ├── config/             # 运行时配置
│   ├── options/            # 启动选项
│   └── handlers/           # 事件处理器
│       ├── handler.go      # 处理器接口
│       ├── registry.go     # 处理器注册表
│       └── assessment_submitted.go  # 答卷提交处理器
└── configs/
    ├── worker.dev.yaml     # 开发环境配置
    └── worker.prod.yaml    # 生产环境配置
```

## 事件处理

### 支持的事件类型

| 事件类型 | Topic | 描述 |
|---------|-------|------|
| AssessmentSubmittedEvent | `assessment.submitted` | 答卷已提交，触发评估流程 |
| AssessmentInterpretedEvent | `assessment.interpreted` | 评估已完成，触发通知 |
| AssessmentFailedEvent | `assessment.failed` | 评估失败，触发告警 |

### 事件流程

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│ qs-apiserver│───▶│ Message Queue│───▶│  qs-worker  │
│             │    │  (NSQ/AMQP)  │    │             │
└─────────────┘    └──────────────┘    └─────────────┘
       │                                      │
       │ 发布事件                              │ 消费事件
       ▼                                      ▼
┌─────────────┐                        ┌─────────────┐
│   Domain    │                        │   Handler   │
│   Events    │                        │   Registry  │
└─────────────┘                        └─────────────┘
```

## 使用方式

### 构建

```bash
# 构建 worker 服务
make build-worker

# 或构建所有服务
make build-all
```

### 运行

```bash
# 启动 worker 服务（开发环境）
make run-worker

# 或直接运行
./bin/qs-worker --config=configs/worker.dev.yaml
```

### 管理

```bash
# 停止服务
make stop-worker

# 重启服务
make restart-worker
```

## 配置

### 消息队列配置

Worker 支持以下消息中间件：

- **NSQ**（默认）：高性能、分布式消息队列
- **RabbitMQ**：AMQP 协议消息队列

```yaml
messaging:
  provider: nsq  # 或 rabbitmq
  nsq-addr: 127.0.0.1:4150
  nsq-lookupd-addr: 127.0.0.1:4161
  rabbitmq-url: amqp://guest:guest@localhost:5672/
```

### Worker 配置

```yaml
worker:
  concurrency: 5      # 并发处理数
  max-retries: 3      # 最大重试次数
  service-name: qs-worker  # 服务名称（用作消息队列 channel）
```

## 扩展处理器

### 添加新的事件处理器

1. 在 `handlers/` 目录创建新文件：

```go
package handlers

import (
    "context"
    "log/slog"
)

type MyEventHandler struct {
    *BaseHandler
    logger *slog.Logger
}

func NewMyEventHandler(logger *slog.Logger) *MyEventHandler {
    return &MyEventHandler{
        BaseHandler: NewBaseHandler("my.event.topic", "my_event_handler"),
        logger:      logger,
    }
}

func (h *MyEventHandler) Handle(ctx context.Context, payload []byte) error {
    // 处理逻辑
    return nil
}
```

2. 在 `registry.go` 的 `RegisterDefaultHandlers` 中注册：

```go
func RegisterDefaultHandlers(registry *Registry, logger *slog.Logger) {
    registry.Register(NewAssessmentSubmittedHandler(logger))
    registry.Register(NewMyEventHandler(logger))  // 添加新处理器
}
```

## 与事件系统的关系

Worker 使用 `pkg/event` 和 `pkg/messaging` 两个共享包：

- **pkg/event**：定义领域事件接口（`DomainEvent`、`EventPublisher`、`EventSubscriber`）
- **pkg/messaging**：消息队列抽象层（`Publisher`、`Subscriber`、`Message`）

```
pkg/event          pkg/messaging
    │                    │
    │ 领域事件抽象        │ 消息队列抽象
    ▼                    ▼
┌─────────────────────────────────┐
│      internal/worker/run.go     │
│   (将 messaging 适配到 event)   │
└─────────────────────────────────┘
```
