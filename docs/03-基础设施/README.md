# 基础设施

本层按系统问题组织，不按 Redis、NSQ、MySQL 等组件平均分栏。

## 1. 三条主线

| 系统问题 | 现行入口 | 主要机制 |
| --- | --- | --- |
| 高频读与缓存一致性 | [Cache](./cache/README.md) | capability registry、L1/L2、失效、预热、治理 |
| 异步执行与可靠协作 | [Event](./event/README.md) | Outbox、MQ、Redis signal、handler |
| 突发流量与故障隔离 | [Concurrency / Resilience](./concurrency/README.md) | 准入、限流、防重、背压、租约、治理操作 |

## 2. 支撑能力

| 能力 | 入口 |
| --- | --- |
| 数据访问、事务与读写模型 | [Data Access](./data-access/README.md) |
| IAM、服务认证与资源范围 | [Security](./security/README.md) |
| 日志、指标、状态与故障定位 | [Observability](./observability/README.md) |
| 配置、装配和资源生命周期 | [Runtime](./runtime/README.md) |

## 3. 机制边界

- Outbox 保证业务事件可靠出站；MQ 解耦异步消费；Redis signal 只做可丢失唤醒。
- cache miss/失效不能改变业务事实，只影响读性能和新鲜度。
- 限流、有界等待、防重、背压分别解决不同压力，不应合并成一个“失败重试”。
- 治理操作必须带审计、幂等和恢复路径，不能绕过业务边界直接修改主事实。
