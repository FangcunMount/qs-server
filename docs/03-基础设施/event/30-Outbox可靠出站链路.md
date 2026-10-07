# Outbox 与业务效果恢复

QS 的标准 Outbox 使用 SDK `rm_outbox`，保留业务原事务与恢复许可。它将待发送责任持久化，`published` 只证明发送结算；下游效果由 QS 业务事实或回执证明。

## 原事务与两个 Profile

| Profile | 权威存储 | 业务范围 |
| --- | --- | --- |
| `mongo_domain_events` | Mongo `rm_outbox` | 答卷提交、报告生成/失败、Interpretation 重试授权 |
| `assessment_mysql_events` | MySQL `rm_outbox` | Evaluation 请求/结果/失败/重试，启用后的 Task 开放提醒 |

每个已提供数据库都必须绑定完整标准 Profile。配置缺失、禁用标准 Profile、存储预检失败或非 NSQ/MQ-backed 装配会启动失败；当前源码不提供旧 Outbox 的配置回退。

```text
原业务事务：事实 + Stager → SDK Appender.Append 原意图
  ├─ rollback：共同回滚
  └─ commit：共同持久化
       → PostCommitWake（可丢唤醒）
       → SDK 周期扫描、claim、发送与条件结算
```

Mongo Stager 使用原活动 `mongo.SessionContext` 绑定 `sdkmongo.Bind`；MySQL 使用 `mysql.RequireTx` 和 `sdkmysql.BindGORM`。没有业务事务时必须拒绝暂存，不另开业务事务兜底。Mongo callback 重入仍须保持原事件 ID/bytes；原事务 runner 的耐久性、背压和重试边界归宿主。

标准意图在首次暂存前编码完整 Revision2 wire，保存原事件 UUID、metadata 和领域 envelope。SDK Publisher 原样发送 `Message.Payload`，不在重投时重新构造当前业务数据。`EvaluationRequested` 另在原 MySQL 事务保存 `qs_rm_evaluation_request_ref`，用于找回原评估关联；它是业务恢复证据，不是第二套 Outbox。

`AfterCommit` 没有错误返回且只在提交成功后调用：此时不能再回滚业务事实。它只向本地 Wake channel 发提示，不 claim、不 publish，不使用旧 ready-index、ImmediateDispatcher 或 reconciler。

## 标准状态与 QS 治理投影

| 持久状态 | 当前含义 |
| --- | --- |
| `pending` | 原业务事务已追加意图，等待到期领取 |
| `publishing` | 发送者获得有效 claim；不证明已发送 |
| `retry_wait` | 原意图等待下次技术投递 |
| `quarantined` | 自动扫描不再领取；保留损坏、拒绝或预算耗尽责任 |
| `published` | 当前 claim 的 Broker 确认已经条件写回 |

Store 用原记录、token、version 和数据库租约拒绝过期写回。PUB 接受与 Store.Confirm 是两次操作：MQ 接收但发布结算未持久化时，租约恢复可能再次投递，消费者必须幂等；响应错误本身也不能证明数据库未更新，应回查原状态。

SDKRetryPolicy 使用 QS `OutboxPolicy` 与 claim `FailureCount`，当前预算为 30，退避 10 秒起步、1 分钟封顶、20% deterministic jitter，策略版本 `outbox-publish-retry/v2`。预算不能与 claim 次数、NSQ 物理次数或业务 attempt 相加。

QS 状态 reader 按标准状态读积压，不再使用旧 `failed` 存储状态。治理层将 `quarantined + publish_unknown` 投影为 `manual_required`，其余隔离原因投影为 terminal；`retry_wait` 与原授权版本决定 automatic/authorized。处置必须读取实际错误及原事实，不能把所有 quarantined 都当作可自动重发。

旧记录已有的未来 `next_attempt_at` 不随政策升级自动改写。72 秒是默认退避含 jitter 的最大等待，不是端到端恢复时限；较长故障仍可能耗尽预算。

## published 后的业务效果由 QS 核对

NSQ PUB 确认不证明同步刷盘、某 channel 消费或业务完成。标准 Relay 不自动重新扫描所有 published 行；无条件重投会重复正常完成的业务或外部副作用。

`answersheetgap.Scanner` 是 QS 的有界、只读跨库效果检查：读取 durable AnswerSheet 与冻结准入，核对原标准 Outbox 身份/指纹，再查 MySQL Assessment。

| 分类 | 判断与后续责任 |
| --- | --- |
| `assessment_present` | 原答卷已有正确组织的 Assessment，不能因缺少投递日志重建 |
| `not_required` | 原冻结准入为独立问卷，不要求 Assessment |
| `delivery_pending` | 原 Outbox 仍 pending/publishing/retry_wait，发送恢复责任尚未结束 |
| `missing_confirmed` | 原 Outbox published，但要求的 Assessment 缺失；形成审核候选 |
| `manual_required` | 原意图缺失、冻结事实不全、身份冲突或其他需治理状态 |
| `unknown` | 查询失败等证据不足；不能渲染成无缺口 |

这项 scanner 只核对答卷到 Assessment，不核验报告完成或未知模型执行。恢复准备由 `CaptureOriginalRecovery` 复查原 Mongo snapshot、原消息、冻结上下文及任何 Assessment（含软删除）；Mongo/MySQL 不共享快照，持久授权后仍须再次捕获，正常消费者并发由原 Intake 唯一键裁决。Scanner 本身不自动发消息。

当前 `ScanPage` 的调用仅见于集成合同，`CaptureOriginalRecovery` 提供恢复准备能力；二者尚未见生产 composition root、调度器或治理 API 接入。上界、grace period、cursor 持久化、授权 owner 和现场操作仍需明确装配；不能把包与测试存在写成后台自动发现/修复已上线。

已确认消息的恢复必须证明对应业务效果缺失、保存授权/审计并保留原身份、bytes、计数与确认历史；SDK 提供受限原行条件重排原语，宿主决定是否调用。Task 提醒另有原 Task、收件账本及时间窗合同，见[任务调度、入口与提醒](../../02-业务模块/60-plan/23-核心设计-任务调度、入口与提醒.md)。Evaluation/报告的原效果恢复也依各自业务证据，不能沿用答卷 scanner 的结论。

结果未知的外部调用不因新物理投递或消息重排而获得重新执行许可；模型调用应继续原任务/冻结配置恢复，提醒应核对逐收件状态。

## 运行与回退

EventSubsystem 启动附加消费者和标准 Profile Supervisor；退出时先取消并等待 Run 完成，再 drain Publisher。宿主保留数据库、schema/索引、退出预算和部署所有权，见[架构与责任边界](./10-架构与责任边界.md)。

旧 core/Store/ready-index 历史合同从固定旧提交运行，入口为 `scripts/testing/run-retired-outbox-contracts.sh`，清单为 [`retired-outbox-contracts.json`](../../retired-outbox-contracts.json)。它们只证明固定旧实现，不能替代当前 SDK 验证。部署回退使用核验过的旧镜像及其数据版本约束；删除旧源码不授权删除生产历史行、索引、NSQ channel 或回退镜像。

## 代码与验证

- 原事务：`infra/mongo/standardoutbox/stager.go`、`infra/mysql/standardoutbox/stager.go`
- Profile 装配：`process/standard_event_subsystem_m4.go`
- Wake/监督/政策/观测：`eventing/standardoutbox`
- 状态与授权：`infra/{mongo,mysql}/standardoutbox/{status,governance,replay}.go`
- 答卷效果：`infra/answersheetgap/{scanner,recovery}.go`
- 原评估/报告效果：`maintenance/originaleffect`

以上路径均以 `internal/apiserver/` 为前缀。普通合同及实库/故障验证入口见[扩展与验收](./70-扩展与验收.md)。SDK claim/时钟机制以[当前依赖版本的源码合同](./10-架构与责任边界.md#sdk-版本与参考文档)为准；本页不把单元测试、代码存在或 published 计数当作现场效果验收。
