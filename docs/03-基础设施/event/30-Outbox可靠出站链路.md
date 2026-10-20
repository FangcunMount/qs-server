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

### M3 T13：未来业务调度的冻结计划（实现中）

`eventing/retryschedule` 已新增独立冻结模型：保存原失败 Run、业务授权、到期时间和首次消息/wire，不包含发布 attempt、delivered 或消费完成状态。评分仅绑定 qs.mysql，报告仅绑定 qs.mongo；重复同输入复用第一份计划，授权或业务输入变化必须冲突，不用新时钟/事件ID覆盖赢家。存储索引时间按数据库精度向上取整，原始 deadline 和 wire 保持不变，避免提前到期。

已新增借用原 GORM 根事务或活跃 Mongo session 的存储适配；构造不执行DDL，显式安装107/42，版本及绑定不符拒绝。事务计划的 Prepared 入口复用首次wire，不重新构造消息。合同测试及原生用例编译通过；真实双库原子性、到期 Sender、旧调度交接及恢复治理仍待 T13 后续验收。既有调度、Outbox与生产消息责任尚未切换。进度及证据统一维护在 [infra M3 总表](https://github.com/FangcunMount/infra/blob/main/docs/server/rocketmq-m3-execution.md)。

T13到期核心已新增 `application/eventing/retrydispatch`：单次串行有界扫描最多100条，在SDK准备半消息后取得原根调度guard，锁定/条件写入当前Run授权，并将dispatch事实与witness共同提交。冻结首次selectedAt，重复/Unknown只读恢复，不自动新建fence或再次调用业务；FinalUnknown保留本地已提交。原根密封上下文校验精确引用和计划，失效callback拒绝使用。该入口尚未接管活动调度：真实到期Broker、显式中止重试、自动/人工授权写入口、暂停/交接与旧Replay责任仍待验收。当前普通源码回归7144通过、60既有可选依赖跳过；required真实依赖门禁仍待新候选，不表示T13或生产已完成。

评分失败和报告失败的新私有计划采用V2：在原根内写Run/失败事实、未来调度及本次失败事件的witness，未来事件不进入发布型Outbox，也不提前准备未来半消息。V1赢家按其冻结版本继续原Outbox责任；新旧版本稳定业务input hash相同，重复请求读取原赢家，不能以版本升级覆盖旧wire/责任。恢复V2还核对精确未来计划。自动入口及原生用例源码已通过7147项全仓race（60既有可选跳过），真实到期Broker门禁待执行；人工/force、暂停、显式中止重试与旧计划交接尚未完成，不能激活生产切换。

人工/force授权入口在显式事务owner配置下改用未来计划：调用原权限/组织校验后，在根事务外冻结完整授权命令和首次wire；根内先竞争Ready计划，再校验原Run/Generation并提交授权事实。重复同输入读取第一份计划，变更理由/主体/动作产生冲突；根提交回复丢弃后按fresh原库事实恢复，不再次授权或调用旧Stage。授权正文只保存在私有计划，消费者wire不含这些审计字段；自动计划的旧codec字节保持不变。该批源码race7157通过（60既有可选依赖跳过），真实双库人工门禁尚待执行。暂停、中止后新fence授权、旧调度交接/Replay与最终宿主生命周期仍未完成，不能据此切换生产。

T13显式协议恢复入口 `RetryAborted` 先读取原Ready计划和精确Aborted证据，再调用固定SDK的原子Retry申请新fence；沿用原计划、首次selectedAt和wire，旧执行保留。常规扫描/Checker不调用此入口。自动暂停开关在reserve之前拒绝自动计划，也禁止其显式协议恢复；人工/force不受该自动开关影响。新增合同30项race通过，源码全仓7166通过/60既有可选跳过；required真实恢复仍待新候选。人工首轮原库发现初始Assessment夹具未显式插入，已修正并新增初始事实回读；不能以编译通过代替修正后原库验收。

T13新增显式原根 `TransferLegacy` 适配：仅匹配完整原消息、fingerprint、原due以及未尝试pending责任；SQL行锁/Mongo写冲突与原Run授权共同竞争，数据库时钟确认仍为未来后，在同一根内保存未来计划并将原行标为held，错误码绑定完整计划摘要。原wire/身份/due/旧数据不删除，重复只读核对原交接及计划，不重置终态；旧Relay不能领取held，旧Replay明确返回future_schedule_owned。旧MySQL Outbox的UTC字符串微秒截断独立于原Run精度，交接按其真实编码读取。89项合同race、原生标签编译及固定lint通过；真实交接验收待新候选，尚无生产交接工具执行（T25/T30以后）。

显式恢复验收首轮三档在新增helper内失败，保留脱敏报告和完整清理回执。原宿主把本地回调错误保守地视作Unknown是正确行为：测试现在等待实际lease并调用原库ReconcileOnce，取得持久化Aborted后再申请新fence；旧fence由精确StoreChecker读取，不拿“当前活动执行”查询替代历史证据。实际SQL与Mongo标签分别编译和固定lint通过，修正后的真实Broker门禁待重跑；前次使用了不存在的report标签，未覆盖该helper的报告独立构建，已校正为m3_report_integration。
