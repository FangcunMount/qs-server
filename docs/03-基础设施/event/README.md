# Event 模块

Event 模块负责把进程内已经发生的业务事实，按明确的可靠性契约传播给其他处理器。它不是单一的 MQ 封装，而是由事件契约、发布路由、Outbox、消费结算、幂等策略、可观测性和进程级生命周期共同组成的基础设施。

## 先看结论

- 跨事务边界且不能丢失的业务事实使用 `durable_outbox`：业务事实与 Outbox 在同一本地事务提交，提交后由 immediate 和 relay 共同推动投递。
- 允许丢失、只用于轻量后置动作的通知使用 `best_effort`：生产者直接交给 RoutingPublisher，不获得持久化保证。
- Redis Pub/Sub Signal 只负责一次性唤醒和缓存失效提示，不属于 EventSubsystem，不承担业务事实投递。
- MQ 消费语义固定为：poison ACK、unknown ACK、handler error NACK、handled ACK。
- 系统提供的是可治理的 at-least-once，不提供 exactly-once、统一 event-id ledger、DLQ、replay 或 schema negotiation。
- `behavior_footprint` 不再有事件链；行为足迹仅由 `behavior_journey_scan` 后台扫描器从事实数据构建。

## 三种传播语义

| 机制 | 适用场景 | 持久化 | 失败后的恢复 | 所有者 |
| --- | --- | --- | --- | --- |
| Durable Event | 不能因进程或 MQ 短暂故障丢失的业务事实 | Mongo/MySQL Outbox | immediate 失败后由 relay 重试 | EventSubsystem + 业务本地事务 |
| Best-effort Event | 可丢失的轻量通知和后置动作 | 无 | 无持久化补偿 | RoutingPublisher |
| Signal | 缓存失效、状态刷新等一次性唤醒 | 无 | TTL、下一次变更或主动查询 | 各进程 CacheSubsystem / report status runtime |

## 文档地图

1. [事件模块整体架构](./01-事件模块整体架构.md)：模块边界、所有权、依赖方向和生命周期。
2. [领域事件设计](./02-领域事件设计.md)：契约来源、wire envelope、完整事件矩阵和演进规则。
3. [Outbox 可靠出站链路](./03-Outbox可靠出站链路.md)：事务、profile、ready-index、immediate、relay 和恢复语义。
4. [MQ 发布与消费链路](./04-MQ发布与消费链路.md)：发布模式、消息封装、消费结算和逐事件幂等。
5. [事件可观测性与故障处理](./05-事件可观测性与故障处理.md)：指标、状态接口、治理页面和排障路径。
6. [一次性信令链路](./06-一次性信令链路.md)：Signal 与 Event 的边界、拓扑和失效语义。
7. [新增 Event 与 Signal SOP](./07-新增Event与Signal-SOP.md)：新增、变更和验收清单。

## 事实来源

文档与实现冲突时，按以下顺序判断：

1. wire contract 与基础抽象：`component-base/pkg/event`、`eventcodec`、`eventmessaging`。
2. 事件路由清单：[`configs/events.yaml`](../../../configs/events.yaml)。
3. 工程契约：`internal/pkg/eventing/catalog` 中的 `EventSpec` 与 `EffectiveRegistry`。
4. 运行时行为：`internal/apiserver/eventing/subsystem`、Outbox Store/relay、worker eventing。
5. 本目录文档。

`EffectiveRegistry` 在启动时合并 YAML 与代码契约并做严格校验；[领域事件设计](./02-领域事件设计.md)中的矩阵还有同步测试保护。因此，文档矩阵不是手工维护的旁路清单。

## 变更时更新哪里

| 变更 | 必须同步 |
| --- | --- |
| 新增或删除事件 | `configs/events.yaml`、EventSpec、handler registry、事件矩阵、测试 |
| 修改 delivery/profile/immediate/priority | EventSpec、事件矩阵、Outbox/Registry 测试 |
| 新增附加消费者 | EventSpec、运行时 binding、配置、消费者矩阵、status 测试 |
| 修改 envelope 或 payload JSON | component-base 或 payload DTO、兼容测试、领域事件文档 |
| 修改 ACK/NACK | runtime settlement、worker 集成测试、MQ 文档和可观测 outcome |
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
