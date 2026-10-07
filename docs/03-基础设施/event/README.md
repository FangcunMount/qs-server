# QS Event 与 Signal

QS 保留业务事件、原事务、消费幂等和恢复许可；`reliable-messaging` 提供消息身份、wire、标准 Outbox、Relay、NSQ 传输和 Redis Signal 机制。抽出消息库后，Outbox 仍在 QS 的原业务数据库中，EventSubsystem 仍由 QS 进程装配和监督。

```text
QS 业务事实 + 原事务内 Outbox
  → SDK Relay / NSQ 发布
  → QS 消费者持久处理
  → SDK 消费结算
  → QS 核对业务效果与恢复责任
```

`published` 证明发送结算，ACK 证明某 channel 的这次投递结算；二者都不能证明报告完成。NSQ 发布确认也不提供数据库与 Broker 的共同事务。

`evaluation.failed`、`interpretation.report.generated` 与 `interpretation.report.failed` 的 report-status Redis 写入/Signal 唤醒是 best-effort；Reporter 吞掉写入与通知错误，handler 可以最终 ACK。ACK 不证明 report-status 投影或唤醒已成功，具体业务窗口见[从事实到结算](./15-从事实到结算-失败窗口与状态机.md)。

## 选择传播语义

| 机制 | QS 用法 | 恢复责任 |
| --- | --- | --- |
| `durable_outbox` Event | 业务事实与 Mongo/MySQL `rm_outbox` 在同一本地事务提交 | SDK 扫描与租约恢复；业务效果缺口由 QS 核对 |
| `best_effort` Event | RoutingPublisher 直接发布可丢通知 | producer 无持久补偿；消费端仍按声明的结算合同处理 |
| `ephemeral_signal` | Redis Pub/Sub 提示缓存失效或报告状态刷新 | TTL、权威回读或下一次变更；无 ACK/Outbox |

提交后的 `PostCommitWake` 只唤醒 SDK 的下次扫描。周期扫描以持久 Outbox 为依据。旧 Redis ready-index、ImmediateDispatcher、reconciler 已退役；`immediate/priority` 仅保留为兼容契约字段，不能据此推断当前有对应执行器。

## 阅读地图

| 读者问题 | 权威页 |
| --- | --- |
| QS 与 SDK 分别负责什么，谁启动和关闭资源？ | [架构与责任边界](./10-架构与责任边界.md) |
| 答卷到测评、报告有哪些失败窗口？ | [从事实到结算](./15-从事实到结算-失败窗口与状态机.md) |
| 有哪些事件、路由、业务 owner 和消费者？ | [事件契约与演进](./20-事件契约与演进.md) |
| 如何在原事务接入标准 Outbox，怎样核对已发布但效果缺失？ | [Outbox 与业务效果恢复](./30-Outbox可靠出站链路.md) |
| Worker、附加投影和失败记录何时 ACK？ | [消费与结算](./40-MQ发布消费与结算.md) |
| 哪些指标确实有接线，如何定位失败与人工恢复？ | [可观测性与故障恢复](./50-可观测性与故障恢复.md) |
| 五条业务 Signal 怎样装配、丢失后谁兜底？ | [Signal](./60-Signal一次性信令.md) |
| 增加或改变一个事件需要同步什么？ | [扩展与验收](./70-扩展与验收.md) |

通用消息算法与 SDK API 以[当前依赖版本的 SDK 合同](./10-架构与责任边界.md#sdk-版本与参考文档)为准，本专题记录 QS 如何使用它们。重写前的旧机制教程与阶段记录已保存历史快照，可通过 Git 追溯，不能用于解释当前运行时。

## 事实来源与变更入口

当前事实优先看实际 composition root、原事务与 handler，再看 `go.mod`、`configs/events.yaml`、`EventSpec/EffectiveRegistry` 和契约测试。`configs/signals.yaml` 是说明清单，不是启动时加载的控制配置。

- 业务事件值归 `internal/pkg/event`；业务 payload 归 `internal/pkg/eventing/payload`、`outcome`。
- 通用 envelope 与历史 wire 归 SDK `wire/domain`、`wire/legacy`；不要重新依赖 component-base 消息包。
- 新增事件须同步 YAML、EventSpec、handler registry 和唯一事件矩阵。
- 修改 ACK/错误返回须同步当前 Worker/投影适配器、持久审计、outcome 与测试。
- 修改 Signal 须同时核对常量、清单及真实 publisher/watcher 接线。

逐篇源码基线和复核状态由 [`document-closure.json`](../../document-closure.json) 维护。代码、普通测试、实库/真实 Broker 验证、目标环境部署与业务验收分别记录；缺少现场证据时不能用源码或健康容器补成完成。

验证入口集中在[扩展与验收](./70-扩展与验收.md)，当前观测与恢复限制集中在[故障恢复](./50-可观测性与故障恢复.md)。
