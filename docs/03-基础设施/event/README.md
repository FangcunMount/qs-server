# QS Event 与 Signal

QS 保留业务事实、原事务、消费幂等和恢复许可；`reliable-messaging v0.5.0` 提供冻结计划、事务发送、精确 witness、Checker 与 RocketMQ 传输，继续以 SDK 嵌入宿主。

```text
冻结计划 → 全部分支半消息 → 原业务根事务（业务＋witness）
  → Broker 最终确认／新实例精确回查 → 消费者幂等处理
```

半消息确认不证明本地提交，`LocalCommitted + FinalUnknown` 是合法结果。事务提交不证明消费者或模型已经完成。尽力通知走独立 NORMAL Topic；Redis Signal 保持原职责。

`evaluation.failed`、`interpretation.report.generated` 与 `interpretation.report.failed` 的 report-status Redis 写入/Signal 唤醒是 best-effort；Reporter 吞掉写入与通知错误，handler 可以最终 ACK。ACK 不证明 report-status 投影或唤醒已成功，具体业务窗口见[从事实到结算](./15-从事实到结算-失败窗口与状态机.md)。

## 选择传播语义

| 机制 | QS 用法 | 恢复责任 |
| --- | --- | --- |
| `durable_outbox` Event | 保留目录分类名；当前通过原事务 owner 提交业务与 witness | 精确 Checker；业务效果与重试授权由 QS 核对 |
| `best_effort` Event | RoutingPublisher 直接发布可丢通知 | producer 无持久补偿；消费端仍按声明的结算合同处理 |
| `ephemeral_signal` | Redis Pub/Sub 提示缓存失效或报告状态刷新 | TTL、权威回读或下一次变更；无 ACK/Outbox |

最终装配不使用发布型 Outbox、Relay 或 NSQ；`immediate/priority`、Profile 分类名和旧表不代表活动扫描。历史合同/SDK 通用兼容测试及开发依赖按各自范围保留。

## 阅读地图

| 读者问题 | 权威页 |
| --- | --- |
| QS 与 SDK 分别负责什么，谁启动和关闭资源？ | [架构与责任边界](./10-架构与责任边界.md) |
| 答卷到测评、报告有哪些失败窗口？ | [从事实到结算](./15-从事实到结算-失败窗口与状态机.md) |
| 有哪些事件、路由、业务 owner 和消费者？ | [事件契约与演进](./20-事件契约与演进.md) |
| 如何接入原事务 witness，怎样保留业务恢复责任？ | [事务消息与业务效果恢复](./30-Outbox可靠出站链路.md) |
| Worker、附加投影和失败记录何时 ACK？ | [消费与结算](./40-MQ发布消费与结算.md) |
| AI workflow 的接收、准入维护和离线发布预检在哪里？ | [AI 消息运行与维护](./40-MQ发布消费与结算.md#ai-workflow-消息运行与维护) |
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
