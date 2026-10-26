# QS Event 与 Signal

QS 保留业务事实、原事务、消费幂等和恢复许可；`reliable-messaging v0.5.0` 提供冻结计划、事务发送、精确 witness、Checker 与 RocketMQ 传输，继续以 SDK 嵌入宿主。

```text
冻结计划 → 全部分支半消息 → 原业务根事务（业务＋witness）
  → Broker 最终确认／新实例精确回查 → 消费者幂等处理
```

半消息确认不证明本地提交，`LocalCommitted + FinalUnknown` 是合法结果。事务提交不证明消费者或模型已经完成。尽力通知走独立 NORMAL Topic；Redis Signal 保持原职责。

`evaluation.failed`、`interpretation.report.generated` 与 `interpretation.report.failed` 的 report-status Redis 写入/Signal 唤醒是 best-effort；Reporter 吞掉写入与通知错误，handler 可以最终 ACK。ACK 不证明 report-status 投影或唤醒已成功，具体业务窗口见[从事实到结算](./README.md#当前业务失败窗口)。

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
| 答卷到测评、报告有哪些失败窗口？ | [从事实到结算](./README.md#当前业务失败窗口) |
| 有哪些事件、路由、业务 owner 和消费者？ | [事件契约与演进](./20-事件契约与演进.md) |
| 如何接入原事务 witness，怎样保留业务恢复责任？ | [事务消息与业务效果恢复](./30-Outbox可靠出站链路.md) |
| Worker、附加投影和失败记录何时 ACK？ | [消费与结算](./README.md#当前消费与结算) |
| AI workflow 的接收、准入维护和离线发布预检在哪里？ | [AI 消息运行与维护](./README.md#ai-workflow-当前运行与维护) |
| 哪些指标确实有接线，如何定位失败与人工恢复？ | [可观测性与故障恢复](./README.md#当前观测与恢复) |
| 五条业务 Signal 怎样装配、丢失后谁兜底？ | [Signal](./60-Signal一次性信令.md) |
| 增加或改变一个事件需要同步什么？ | [扩展与验收](./README.md#当前扩展与验收) |

通用消息算法与 SDK API 以[当前依赖版本的 SDK 合同](./10-架构与责任边界.md#sdk-版本与参考文档)为准，本专题记录 QS 如何使用它们。重写前的旧机制教程与阶段记录已保存历史快照，可通过 Git 追溯，不能用于解释当前运行时。

## 事实来源与变更入口

当前事实优先看实际 composition root、原事务与 handler，再看 `go.mod`、`configs/events.yaml`、`EventSpec/EffectiveRegistry` 和契约测试。`configs/signals.yaml` 是说明清单，不是启动时加载的控制配置。

- 业务事件值归 `internal/pkg/event`；业务 payload 归 `internal/pkg/eventing/payload`、`outcome`。
- 通用 envelope 与历史 wire 归 SDK `wire/domain`、`wire/legacy`；不要重新依赖 component-base 消息包。
- 新增事件须同步 YAML、EventSpec、handler registry 和唯一事件矩阵。
- 修改 ACK/错误返回须同步当前 Worker/投影适配器、持久审计、outcome 与测试。
- 修改 Signal 须同时核对常量、清单及真实 publisher/watcher 接线。

逐篇源码基线和复核状态由 [`document-closure.json`](../../document-closure.json) 维护。代码、普通测试、实库/真实 Broker 验证、目标环境部署与业务验收分别记录；缺少现场证据时不能用源码或健康容器补成完成。

验证入口集中在[扩展与验收](./README.md#当前扩展与验收)，当前观测与恢复限制集中在[故障恢复](./README.md#当前观测与恢复)。

## 当前业务失败窗口

当前可靠发布先冻结完整计划并准备全部半消息，再进入宿主原根事务；guard、业务事实、业务证据与全部 witness 同一事务提交。半消息失败或未知时不启动业务。`LocalCommitted + FinalUnknown` 保留已提交事实，等待精确 Checker，不重跑业务。取消或 lease 到期不证明回滚。

| 窗口 | 当前责任及证据 |
| --- | --- |
| 答卷受理 | 原 Mongo 事务提交答卷、原幂等结果及 witness；`202` 需要业务持久化证据，半消息 ACK 不足以返回成功 |
| 原事务回滚或提交未知 | 明确回滚才中止；未知按精确原 witness/幂等结果查询，不换身份重试 |
| 本地已提交、Broker 确认丢失或进程退出 | 相同 owner 的新实例依据原存储完成回查；不依赖扫描发布型 Outbox |
| 测评/报告重复消费 | 保留原业务关联、Run/Generation claim、CAS、Outcome/Report 和幂等；技术重投不授予业务重做许可 |
| 未来重试 | RetryDecision、due、attempt、授权和冻结事件继续持久化；到期 dispatcher 取得资格后准备半消息并在原事务提交 dispatch/witness |
| 报告状态、提醒或外部调用结果未知 | 分别核对最终业务事实、投影和外部收件账本；ACK、事务提交和最终效果不能互相替代 |

QS Mongo 答卷与 MySQL 测评分别提交，不承诺跨库全局事务。幂等冲突、Prepared 或缺 witness 不能伪装成已提交或零积压。更多原事务约束见[事务消息与业务效果恢复](./30-Outbox可靠出站链路.md)。

## 当前消费与结算

最终装配显式使用 RocketMQ；可靠发布使用 TRANSACTION，可丢通知使用 NORMAL，物理 Topic 分离。`durable_outbox` 等目录分类名保留，不表示生产扫描旧表。SDK 通用 NSQ 包和独立兼容测试不进入当前生产 composition root。

- handler 返回错误时保留原失败语义；不能在持久化失败时确认投递。
- Worker unknown 先持久审计，成功后才确认；原始物理身份和业务消息身份分开保留。
- 暂停的业务 retry 只有成功转交 durable hold 才能确认；到期与重试授权仍归 QS。
- Hot-rank 等附加投影保留自己的逻辑订阅与过滤；不能用一个消费者成功代表全部 fanout 已收敛。
- 同一逻辑订阅的多物理输入共享预算，三个 worker 各48并发保持，不因拆 Topic 扩容。
- SDK 的失败转交、持久 recorder 与最终 ACK 是分别取证的边界；旧 NSQ PUB/FIN/节点拓扑不能用来解释当前 RocketMQ 状态。

## AI workflow 当前运行与维护

QS command 冻结首次 ID、Seal 和 wire，原根事务提交 operation、档案与 witness。接收 AI 事件时，同根提交 Inbox、投影、首次 ACK 档案及 witness；缺原 Committed 档案拒绝，当前 Payload/Operation 不回退读取旧发布型 Outbox。首次 wire、租户/收件方/mTLS/hash 校验和权限保护查询保持。

`ai.mysql` 的业务状态、receipt 和 witness 原子提交。模型调用与事务消息分离；已持久 response/checkpoint/artifact 是确定性 finalization 输入。MQ 故障不增加 provider invocation，不允许重做原 unknown 请求。当前名字仍为 result_outbox 的表承担业务 ACK、状态和历史责任，不能按名称删除。

当前维护只使用具名的准入 revision/CAS、只读审计及正式离线发布预检；旧切换、Relay、Stage 和 rearm 入口已退役。工具存在不代表获得生产重投或数据处置授权。实际命令以 `scripts/maintenance/qs-ai-messaging` 与 `scripts/cd` 当前入口为准，旧页面中的阶段命令不可用于生产。

## 当前观测与恢复

`internal/pkg/messagingruntime/transaction_metrics.go` 在原 registry 导出 `reliable_messaging_transaction_` 指标。采样访问原主库，scrape 读取缓存；DB预算2秒、采样周期5秒。错误/陈旧不能当零积压，`confirmation_gap_branches` 只是正向确认审计缺口，不等于 Broker 实际积压。

| 信号 | 处理边界 |
| --- | --- |
| 读取失败或样本陈旧 | 关闭新 Reserve/Execute，保留 Checker |
| 未决或确认缺口增龄120秒 | 告警、精确核对责任 |
| 缺口或过期准备增龄300秒、回查持续饱和120秒 | 保护该 owner 新写，不扩大预算 |
| 绑定矛盾、无有效证据、原生事务超限 | 拒绝新执行并保留精确回查；不清零或重做业务解除 |

原生回查窗口有限。高级手工恢复仍由 infra 独立台账维护，不因为文档清理宣称通过。Mac mini 继续采集监控，serverD 不新增代理。源码、CI、镜像、运行版和 system 业务验收分别记账。

## 当前扩展与验收

新增可靠事件先明确 owner/原事务、幂等输入、固定路由和最多16分支的冻结计划，再接入原根 guard/witness；不能新增发布型 Outbox 或提交后 Relay。尽力通知走独立 NORMAL；Redis Signal 仍由宿主管理连接和生命周期。UUID、时间、签名加密及网络调用放在数据库 callback 外；Mongo callback 重入只做确定性数据库操作。

构造不启动任务、不执行 DDL。Start 注册全部本 owner Topic，即使没有新发送也保持 Checker；关闭封准入后等待真实在途与回查，再关闭所属客户端，宿主最后关闭 DB/loop。`ShutdownIncomplete` 需要继续等待，不提前释放容量。

验证应包含原事务共同提交/回滚、首次 wire、未知提交、重复、错误 owner/Topic/证据拒绝、真实进程退出/跨实例回查、共享预算和完整关闭。真实依赖由 infra 隔离 CI 执行，缺依赖失败；普通单测不能替代实库/Broker、生产或 system 验收。当前报告见 [M4 总表](https://github.com/FangcunMount/infra/blob/main/docs/server/rocketmq-m4-execution.md)。

## 历史参考与保留例外

原15/40/50/70页的 NSQ、扫描型 Outbox、PUB/FIN、旧状态指标及阶段命令仅为历史/阶段参考，不能作为当前运行指南。保留旧链接和证据不表示运行兼容；当前行为以上述最终协议和10/20/30页为准。SDK 通用兼容测试、Mac mini 开发依赖、原 schema/表/备份按各自范围保留；当前业务 receipt、调度、first-wire、模型 checkpoint 与未决事务证据持续受保护。
