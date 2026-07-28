# Event 模块

Event 模块负责把进程内已经发生的业务事实，按明确的可靠性契约传播给其他处理器。它不是单一的 MQ 封装，而是由事件契约、发布路由、Outbox、消费结算、幂等策略、可观测性和进程级生命周期共同组成的基础设施。

学习本专题时始终沿一条因果链，不按 package 逐个记忆：

```text
业务事实
  → 同事务 Outbox 意图
  → 发现与发布
  → broker/channel 投递
  → 业务副作用结算
  → 有界重试、终态与人工恢复
```

如果还没有读过根级 [统一模型与推理方法](../04-统一模型与推理方法.md)，建议先读。Event 是其中“持久事实 → 可靠传播 → 派生结果”这一段的展开。

版本基线：`2026-07-28`。本轮已核对 `configs/events.yaml`、`EventSpec`/`EffectiveRegistry`、apiserver EventSubsystem、Mongo/MySQL Outbox 与 worker settlement/replay 链；真实 broker 投递、进程崩溃恢复和积压清理速度仍需环境证据。

## 先看结论

- 跨事务边界且不能丢失的业务事实使用 `durable_outbox`：业务事实与 Outbox 在同一本地事务提交，提交后由 immediate 和 relay 共同推动投递。
- 允许丢失、只用于轻量后置动作的通知使用 `best_effort`：生产者直接交给 RoutingPublisher，不获得持久化保证。
- Redis Pub/Sub Signal 只负责一次性唤醒和缓存失效提示，不属于 EventSubsystem，不承担业务事实投递。
- MQ 消费语义固定为：无法解析的消息 NACK、unknown ACK、handler error NACK、handled ACK；NACK 只消耗有界运输预算。
- 运输预算耗尽后写入 MySQL `event_delivery_dead_letter`，由高风险治理动作 `events.replay_delivery` 做组织范围、带状态冲突检查的一次性人工重放。
- 系统提供的是可治理的 at-least-once，不提供 exactly-once、统一 event-id ledger、自动修复 poison payload 或 schema negotiation。
- Statistics 不再维护行为足迹事件链或后台 Scanner；夜间 Data Collector 直接从权威业务数据构建 Access、Assessment 和 Plan Fact。

## 三种传播语义

| 机制 | 适用场景 | 持久化 | 失败后的恢复 | 所有者 |
| --- | --- | --- | --- | --- |
| Durable Event | 不能因进程或 MQ 短暂故障丢失的业务事实 | Mongo/MySQL Outbox | immediate 失败后由 relay 重试 | EventSubsystem + 业务本地事务 |
| Best-effort Event | 可丢失的轻量通知和后置动作 | 无 | 无持久化补偿 | RoutingPublisher |
| Signal | 缓存失效、状态刷新等一次性唤醒 | 无 | TTL、下一次变更或主动查询 | 各进程 CacheSubsystem / report status runtime |

这里的“允许丢失”描述 producer 没有 Outbox；一旦消息已经进入 MQ，consumer 仍使用统一的有界运输结算和死信审计。不要把 producer delivery 与 consumer transport delivery 混为同一层保证。

## 文档地图

1. [架构与责任边界](./10-架构与责任边界.md)：模块边界、所有权、依赖方向和生命周期。
2. [从事实到结算：失败窗口与状态机](./15-从事实到结算-失败窗口与状态机.md)：先区分 T1–T5 完成点，再推导 Outbox、at-least-once、逐副作用幂等和审计化恢复。
3. [Event 契约与演进](./20-事件契约与演进.md)：契约来源、wire envelope、完整事件矩阵和演进规则。
4. [Outbox 可靠出站链路](./30-Outbox可靠出站链路.md)：事务、profile、ready-index、immediate、relay 和恢复语义。
5. [MQ 发布、消费与结算](./40-MQ发布消费与结算.md)：发布模式、消息封装、消费结算和逐事件幂等。
6. [可观测性与故障恢复](./50-可观测性与故障恢复.md)：指标、状态接口、治理页面和排障路径。
7. [Signal 一次性信令](./60-Signal一次性信令.md)：Signal 与 Event 的边界、拓扑和失效语义。
8. [扩展与验收](./70-扩展与验收.md)：新增、变更和验收清单。

前两篇建立概念和推理主轴；20–60 是当前实现事实；70 把新增事件重新带回“业务损失—失败窗口—状态机—证据”的决策闭环。不要从事件矩阵中抄一个相似配置就认为完成了设计。

## 事实来源

文档与实现冲突时，按以下顺序判断：

1. wire contract 与基础抽象：`component-base/pkg/event`、`eventcodec`、`eventmessaging`。
2. 事件路由清单：[`configs/events.yaml`](../../../configs/events.yaml)。
3. 工程契约：`internal/pkg/eventing/catalog` 中的 `EventSpec` 与 `EffectiveRegistry`。
4. 运行时行为：`internal/apiserver/eventing/subsystem`、Outbox Store/relay、worker eventing。
5. 本目录文档。

`EffectiveRegistry` 在启动时合并 YAML 与代码契约并做严格校验；[Event 契约与演进](./20-事件契约与演进.md)中的矩阵还有同步测试保护。因此，文档矩阵不是手工维护的旁路清单。

## 变更时更新哪里

| 变更 | 必须同步 |
| --- | --- |
| 新增或删除事件 | `configs/events.yaml`、EventSpec、handler registry、事件矩阵、测试 |
| 修改 delivery/profile/immediate/priority | EventSpec、事件矩阵、Outbox/Registry 测试 |
| 新增附加消费者 | EventSpec、运行时 binding、配置、消费者矩阵、status 测试 |
| 修改 envelope 或 payload JSON | component-base 或 payload DTO、兼容测试、领域事件文档 |
| 修改 ACK/NACK 或运输预算 | runtime settlement、provider subscriber、死信/重放、worker 集成测试、MQ 文档和可观测 outcome |
| 新增 Signal | `configs/signals.yaml`、代码常量/contract、拓扑测试、Signal 文档 |

## 验证入口

```bash
go test ./internal/pkg/eventing/... \
  ./internal/apiserver/application/eventing \
  ./internal/apiserver/eventing/subsystem \
  ./internal/worker/integration/eventing \
  ./internal/worker/integration/messaging

go test ./internal/pkg/signalcatalog ./internal/pkg/architecture
make docs-hygiene
```

## 学习检查

读完本目录后，应能独立回答：

1. 为什么“数据库 commit 后直接 Publish”不能保证 durable event？
2. 为什么 immediate 与 Redis ready-index 都可以失败，而 Outbox Store 不能被绕过？
3. 为什么 MQ 成功不等于消费者副作用 exactly-once？
4. unknown event 为什么 ACK，invalid payload 为什么 NACK，两者各自承担什么风险？
5. business、Outbox、retry-hold 和 transport 四类 attempt 为什么不能相加？
6. `events.replay_pending` 与 `events.replay_delivery` 分别恢复哪一层事实？
7. 什么时候应该新增独立 channel，什么时候仍应留在一个 handler 的事务内？
8. Signal 丢失为什么是设计语义，而不是尚未补齐的可靠性缺陷？
