# Worker 生命周期

## 1. 解决什么问题

Worker 生命周期解决异步消费者如何启动、注册 handler、消费事件、停止和暴露指标的问题。

## 2. 所在位置

它位于 worker process、worker container、event runtime、handler registry 和 observability 之间。

## 3. 设计目标

handler 注册清晰；消费并发可控；ACK / NACK 语义稳定；停止时不丢失已确认状态；指标可查询。

## 4. 正常流程

worker 启动后加载配置和资源；注册事件 handler；订阅 MQ；消费事件；处理成功 ACK，失败 NACK 或重试。

## 5. 异常流程

handler panic 或错误时按消费语义处理；MQ 断开时重连或退出；停止时等待当前 handler 到安全点。

## 6. 观测指标

worker running、handler duration、ACK / NACK count、consume error、reconnect count、worker shutdown duration。

## 7. 代码事实源

- [../../../internal/worker/process](../../../internal/worker/process)
- [../../../internal/worker/handlers](../../../internal/worker/handlers)
- [../../../internal/worker/observability](../../../internal/worker/observability)
- [../../../internal/pkg/eventing/runtime](../../../internal/pkg/eventing/runtime)
