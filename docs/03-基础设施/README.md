# 基础设施

本层按“系统要解决的问题”组织，不按 Redis、NSQ、MySQL 等组件平均分栏。一个组件可以参与多个机制，但业务事实、容量预算、恢复动作和安全边界必须有唯一 owner。

## 1. 建议阅读顺序

1. [基础设施总览](./00-基础设施总览.md)：先建立三进程、四类事实和失败边界。
2. [基础设施能力地图](./01-基础设施能力地图.md)：从系统问题定位到 composition root、代码和配置。
3. [基础设施设计原则](./02-基础设施设计原则.md)：理解正确性、优化、降级和生命周期的不变式。
4. [核心链路全景](./03-核心链路全景.md)：把同步提交、异步处理、缓存、报告等待和治理串起来。
5. 按专题进入 Cache、Event、Concurrency/Resilience 与四项支撑能力。

## 2. 三条主线

| 系统问题 | 入口 | 主要机制 | 最终事实 |
| --- | --- | --- | --- |
| 高频读与缓存一致性 | [Cache](./cache/README.md) | capability registry、L1/L2、singleflight、失效、预热 | DB/业务 read model |
| 异步执行与可靠协作 | [Event](./event/README.md) | Event contract、Transactional Outbox、MQ、Signal、dead letter/replay | 本地事务 + Outbox + 业务状态 |
| 突发流量与故障隔离 | [Concurrency / Resilience](./concurrency/README.md) | Gate、RateLimit、Backpressure、SubmitGuard、LockLease、治理 | DB 唯一约束、claim/CAS、事务 |

## 3. 支撑能力

| 能力 | 入口 | 核心问题 |
| --- | --- | --- |
| 数据访问 | [Data Access](./data-access/README.md) | 谁拥有事实、事务边界在哪里、跨存储如何收敛 |
| 安全 | [Security](./security/README.md) | 凭证、主体、组织、capability 与资源归属如何逐层证明 |
| 可观测性 | [Observability](./observability/README.md) | 日志、指标、状态和持久审计分别能证明什么 |
| 运行时 | [Runtime](./runtime/README.md) | 配置如何生效、依赖如何装配、后台任务和资源如何启停 |

## 4. 贯穿全层的边界

- Outbox 保证已提交业务事件的可靠出站；MQ 提供有界 at-least-once；Redis Signal 只做可丢失唤醒。
- Cache miss、失效或 Redis 故障不能改变业务事实，只影响性能、新鲜度和部分运行时协调。
- Gate、RateLimit、Backpressure 控制不同的量；SubmitGuard/LockLease 降低竞态代价，但不能替代数据库约束。
- AuthN、OrgScope、capability 和资源归属是连续的授权链，任意一层成功都不能替代其它层。
- `healthy`、`ready`、指标为零和 durable state 已完成不是同一结论。
- 每个 client、subscriber、scheduler、goroutine 都必须有构造、启动、停止和关闭 owner。
- 治理操作必须带组织范围、确认、幂等 request ID、审计和状态冲突检查，不能绕过业务边界直接改主事实。

## 5. 如何判断文档与代码冲突

事实优先级：

1. 当前 source、composition root 与运行时 handler。
2. `configs/events.yaml`、`configs/signals.yaml`、migration、API/proto 等机器契约。
3. 当前 `configs/*.prod.yaml` 和部署注入的配置。
4. 本目录 active docs。
5. 历史文档、重构计划和 Git 记录。

文档中的状态词：

- `已实现`：当前代码与装配可追踪。
- `当前风险/限制`：代码真实存在但边界不完整。
- `规划改造`：只描述目标，不得作为现行能力承诺。

## 6. 验证入口

```bash
make docs-hygiene docs-facts
go test ./internal/pkg/architecture ./internal/pkg/configcontract
```

专题文档末尾列出了更窄的测试入口。unit/contract test 不能替代真实 Mongo transaction、MySQL migration、MQ delivery、Redis 故障、跨实例并发和 SIGTERM drain 演练。
