# 新增 Event 与 Signal SOP

## 1. 先选择正确机制

新增消息前先回答：

```text
是否表达已经发生的业务事实？
├─ 否：只是缓存/状态刷新提示 → Signal
└─ 是
   ├─ 丢失会造成必要流程或投影永久缺失 → durable_outbox Event
   └─ 允许丢失且不需要补偿 → best_effort Event
```

不要先选 MQ 或 Redis 再寻找业务理由。delivery 是业务失败语义，不是中间件偏好。

## 2. 新增 Event

### Step 1：定义事实和 owner

明确：

- 事件用过去时表达已经发生的事实。
- owner 是产生该事实的业务模块，不是消费它的模块。
- aggregate type/ID 和 occurredAt 语义稳定。
- 是否已经存在可复用事实，避免用新名字重复同一事件。

### Step 2：设计 wire contract

定义事件类型常量、DomainEvent 和 payload DTO，并增加 JSON 表征测试。检查：

- JSON 字段名、类型、空值和时间格式。
- event ID 是否在重试期间保持不变。
- payload 是否泄露不稳定的内部领域模型。
- 是否需要兼容旧消费者；破坏性变化应新增事件类型。

基础 envelope 直接使用 component-base，不建立 qs-server 本地 codec 或 event wrapper。

### Step 3：登记 YAML 路由

在 `configs/events.yaml` 中登记：

- topic ID / MQ Topic（已有 Topic 优先）。
- event type。
- `durable_outbox` 或 `best_effort`。
- worker 主 handler 名。

不要把 owner、priority 或幂等策略重复塞入部署配置。

### Step 4：登记 EventSpec

在 `internal/pkg/eventing/catalog` 登记：

- Owner。
- Outbox Profile（durable 必填，best-effort 必须为空）。
- Immediate。
- Priority。
- Idempotency policy。
- Settlement policy。
- AdditionalConsumers（如果需要）。

约束：

- immediate 只能用于 durable event。
- profile 必须是 `mongo_domain_events` 或 `assessment_mysql_events`。
- priority 是 claim 顺序，不是 SLA 标签。
- 当前 settlement 使用 `handler_error_nack`；若要改变全局 poison/unknown 行为，必须单独设计运输协议。

### Step 5：实现 producer

Durable producer：

1. 根据业务事实所在数据库选择 profile。
2. 在同一 Mongo session/MySQL transaction 中写事实并调用 Stager。
3. rollback 不调用 post-commit。
4. commit 成功后，每批事件只调用一次 `PostCommit.AfterCommit`。
5. 不直接构造 Store、ready-index、ImmediateDispatcher 或 relay。

Best-effort producer：

1. 使用 Container/EventSubsystem 派生的 RoutingPublisher。
2. 明确发布失败是否只记录或向调用方返回。
3. 不把 logging/nop 当作持久化成功。

### Step 6：实现主 handler

1. 把 handler 加入 worker 显式 registry。
2. 校验 payload，保留 event ID 和业务键。
3. 先设计重复投递，再写业务副作用。
4. 可重试错误返回 error → NACK。
5. 成功、已完成或安全重复返回 nil → ACK。
6. 不可重试业务失败应先落为明确终态，再决定是否 ACK；不要无限 NACK 一个永远无法成功的事实。

### Step 7：按副作用设计幂等

优先级通常是：

1. 已有业务唯一键/ensure。
2. 状态机 claim 或 CAS。
3. 可覆盖、可重复计算的投影。
4. event ID processed key。
5. 只有不可逆外部副作用确有证据时，再为单个事件设计 inbox/dedup record。

不要默认引入统一 ledger，也不要声称 exactly-once。

### Step 8：需要附加消费者时

附加消费者适用于同一事实的独立投影：

- 在 EventSpec 登记稳定 Consumer ID、runtime、channel、幂等、settlement。
- 使用同 Topic、独立 channel，形成独立消费进度和失败域。
- 在目标进程 Start 前完成 handler binding。
- 增加 enabled 配置、status、指标和重复/失败测试。
- 禁止用 `BeforePublishHook` 承载业务投影。

### Step 9：同步文档和观测

- 更新[领域事件设计](./02-领域事件设计.md)事件矩阵/附加消费者矩阵。
- 确认 publish、Outbox、consume 指标的 event type/outcome 可见。
- 如修改 status JSON，同时更新后端契约测试和 qs-operating-system 页面。
- 说明 backlog、handler error 和人工排障方式。

## 3. 新增 Signal

1. 确认它只是一次性唤醒，丢失后有 TTL、下次变更或查询兜底。
2. 在 `configs/signals.yaml` 登记稳定 name、publisher 和 subscribers。
3. 在 `internal/pkg/signalcatalog` 增加名称常量。
4. 在 transport-agnostic contract 包定义 SignalName、SignalKey 和 JSON payload。
5. 在所属 CacheSubsystem 或专属 runtime 构造 notifier/watcher；业务模块只依赖窄接口。
6. disabled 时不创建订阅、不发送通知。
7. Notify 失败保持 best-effort，不反向破坏已经提交的业务事务。
8. 更新[一次性信令链路](./06-一次性信令链路.md)拓扑和清单同步测试。

如果需求提出“Signal 必须不丢、必须重试、必须确认处理”，停止实现并重新评估它是否应成为 durable Event。

## 4. 变更现有契约

| 变更 | 要求 |
| --- | --- |
| payload 新增可选字段 | 更新 DTO/测试，确认旧消费者可忽略 |
| payload 删除/改名/改类型 | 视为破坏性变化，设计新事件或兼容窗口 |
| Topic/channel 变化 | 设计双运行或迁移，不直接替换生产值 |
| delivery 从 best-effort 改 durable | 补事务 staging、profile、relay 观测和恢复测试 |
| immediate/priority 改动 | 更新 EventSpec、矩阵、Registry/Profile 测试 |
| handler settlement 改动 | 同步 runtime、集成测试、outcome 与可观测文档 |
| 删除事件 | 先确认 producer、Outbox backlog、全部 channel 和旧部署均已退出 |

## 5. 必测场景

### Contract

- YAML、常量、EventSpec、主 handler、事件矩阵严格同步。
- envelope 和 payload JSON 兼容。
- 附加 consumer ID/channel/binding 唯一。

### Durable producer

- 事实与 Outbox 一起 commit/rollback。
- 无事务 context 时 staging 失败。
- post-commit 每批一次，rollback 不调用。
- logging/nop/MQ publisher 缺失不标记 published。
- MQ-backed immediate 成功和失败状态正确。
- ready-index 故障时 DB relay 可恢复。

### Consumer

- poison ACK。
- unknown ACK。
- handler error NACK。
- handled/duplicate ACK。
- 重复 event ID 不重复产生受保护副作用。
- 附加 channel 故障不影响主 channel。

### Lifecycle / Status

- EventSubsystem 只构造一次。
- Start 顺序为 consumers → all reconcilers → all relays。
- Close 逆序、幂等，并汇总全部错误。
- status 保持 `catalog/outboxes` 兼容并正确输出 events/profiles/consumers。

## 6. 验收命令

```bash
go test ./internal/pkg/eventing/... \
  ./internal/pkg/signalcatalog \
  ./internal/pkg/architecture \
  ./internal/apiserver/application/eventing \
  ./internal/apiserver/eventing/subsystem \
  ./internal/apiserver/outboxcore \
  ./internal/apiserver/infra/mongo/eventoutbox \
  ./internal/apiserver/infra/mysql/eventoutbox \
  ./internal/apiserver/infra/redis/outboxready \
  ./internal/worker/integration/eventing \
  ./internal/worker/integration/messaging \
  ./internal/worker/handlers

go test -race ./internal/pkg/eventing/runtime \
  ./internal/apiserver/eventing/subsystem

make docs-hygiene
git diff --check
```

最终 review 还应扫描：业务模块不得构造 relay/Store，不得出现 `BeforePublishHook` 或 `behavior_footprint` event，不得恢复已删除的旧 Event 包路径和兼容 API。
