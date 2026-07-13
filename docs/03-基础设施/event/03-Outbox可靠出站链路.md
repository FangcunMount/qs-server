# Outbox 可靠出站链路

## 1. Outbox 解决什么问题

业务数据库提交成功、MQ 发布失败时，如果只在事务后直接 Publish，业务事实会永久缺少对应事件。Transactional Outbox 把“业务事实”和“待发送事件”写入同一个本地事务，消除这个提交缝隙。

```text
业务事务
  ├─ 写业务事实
  └─ Stage Outbox(pending)
          │
          ├─ rollback：二者一起回滚
          └─ commit：二者一起成为事实
                         │
                 AfterCommit（无错误返回）
                  ├─ ready-index enqueue
                  └─ immediate async try（仅声明为 immediate）
                         │
                   relay 持续兜底
```

Outbox 保证的是事件事实不会因进程退出或 MQ 短暂不可用而消失，不保证消费者只收到一次。

## 2. Profile 与运行时所有权

EventSubsystem 拥有两个 profile：

| Profile | Store | 事件范围 |
| --- | --- | --- |
| `mongo_domain_events` | Mongo `domain_event_outbox` | 答卷提交、Interpretation terminal report |
| `assessment_mysql_events` | MySQL `domain_event_outbox` | Evaluation 请求、结果提交和失败 |

每个 profile 构造且只构造一个：

- Outbox Store。
- Redis ready-index。
- ImmediateDispatcher。
- Ready-index reconciler。
- Relay。

Mongo collection、MySQL table、Topic 和业务事务边界属于兼容不变量。业务模块不拥有这些 runtime，也不能从 repository 导出 Store/relay/status 兼容代理。

## 3. 业务模块只依赖 ProfileBinding

```go
type ProfileBinding struct {
    Stager     EventStager
    PostCommit PostCommitDispatcher
}

type PostCommitDispatcher interface {
    AfterCommit(ctx context.Context, events []event.DomainEvent, readyAt time.Time)
}
```

职责边界：

- `Stager` 只能在活跃业务事务内调用。
- Mongo staging 要求 `mongo.SessionContext`；MySQL staging 要求 context 中存在 `*sql.Tx`。
- 没有事务 context 时必须失败，禁止偷偷降级为独立写 Outbox。
- `AfterCommit` 只在 commit 成功后调用，每批事件通知一次。
- `AfterCommit` 没有错误返回。此时业务事务已经提交，返回错误不能创造“调用方还能回滚”的错觉。

rollback 路径不调用 post-commit；即使 post-commit 内部 ready-index 或 immediate 失败，Outbox 的 pending 事实仍然存在。

## 4. Outbox 状态机

```text
pending ──claim──> publishing ──publish + mark──> published
   ▲                    │
   │                    ├─ publish failed ───────> failed
   │                    └─ stale claim / due retry
   └──────────────────────── failed（到期后重新 claim）
```

核心状态为 `pending`、`publishing`、`published`、`failed`。

- Store 原子 claim，避免同一时刻多个 worker 同时取得同一行。
- publish 失败或 mark 失败会记录重试状态。
- 到期的 failed 和陈旧 publishing 会重新进入候选集合。
- 没有最大重试次数和自动 DLQ；不可恢复错误需要依赖指标、日志和人工处置。

如果 MQ 已经接收消息、进程却在 mark published 前退出，同一事件可能再次发布。这是标准 at-least-once 窗口，不能用 Store 状态推导 exactly-once。

## 5. PostCommit：ready-index 与 immediate

事务提交后，dispatcher 按以下顺序执行：

1. 尽力把本批全部事件写入 ready-index。
2. 对 Registry 标记为 immediate 的事件异步尝试 MQ。

### Ready-index

ready-index 是 Redis ZSet 加速层，不是事件事实来源。key 按 store 和 priority bucket 隔离：

```text
outbox:ready:<store>:p0
outbox:ready:<store>:p1
outbox:ready:<store>:p2
```

relay 优先消费 ready-index 给出的 ID，同时保留数据库轮询。Redis 写入或读取失败不会删除 Outbox，也不会阻止数据库扫描恢复。

### Immediate

ImmediateDispatcher 只在 RoutingPublisher 明确 `IsMQBacked() == true` 时启用。logging、nop 或 MQ publisher 缺失时：

- durable 事件不会被伪装成已发布。
- Outbox 保持 pending，等待真正的 MQ-backed relay。
- best-effort direct logging/nop 行为不受影响。

immediate 有独立并发上限和单次超时。成功时按合法状态迁移标记 published；失败时只记录观测结果，不改变 Outbox 作为最终事实的地位。

## 6. Relay 与优先级

Relay 的工作循环：

1. 从 Registry 派生的 priority tiers 取得 claim 策略。
2. 优先尝试 ready-index 中已提交事件 ID。
3. 对数据库执行轮询兜底，claim pending、到期 failed 和 stale publishing。
4. 编码并发布 MQ message。
5. 标记 published；失败则记录失败状态和下一次重试时间。

当前层级顺序是：

1. p0。
2. p0 + p1。
3. fallback all，覆盖 p2 或未命中的候选。

Priority 仅影响 claim 顺序，不改变事件 delivery、重试语义或 handler 结算。Store 和 ready-index 不得维护独立的默认 P0/P1 切片。

具体 interval、batch size、workers、immediate workers 属于部署调优参数，继续使用 `outbox_relay.mongo.*` 和 `outbox_relay.assessment.*`，不在文档中固化生产数值。

## 7. Reconciler

ready-index 是 best-effort，因此 EventSubsystem 为每个 profile 启动 reconciler，周期性从数据库列出仍需投递的事件引用并回填 ZSet。

Reconciler 解决的是“Outbox 已提交但 post-commit enqueue 丢失”的加速索引缺口。它不取代 relay 的数据库兜底，也不改变 Outbox 状态。

## 8. 失败场景与结算

| 失败位置 | Outbox 事实 | 系统行为 | 是否会丢 durable event |
| --- | --- | --- | --- |
| 业务事务 rollback | 未提交 | 事实与 Outbox 一起回滚 | 不产生事件 |
| ready-index enqueue 失败 | pending 已提交 | 记录失败，relay DB 扫描/reconciler 回填 | 否 |
| immediate publish 失败 | pending/可重试状态 | relay 后续重试 | 否 |
| MQ 不可用 | backlog 增长 | relay 重试；logging/nop 不标记 published | 否 |
| MQ 成功、mark published 失败 | 可能仍可重试 | 后续可能重复发布 | 不丢，但可能重复 |
| Redis ready-index 不可用 | Outbox 不变 | DB polling 继续工作 | 否 |
| Store 读取失败 | Outbox 不变 | status/relay 记录错误并下轮重试 | 否 |

## 9. 保证边界

提供：

- 单数据库内业务事实 + Outbox 原子提交。
- MQ 故障期间可积压，恢复后继续 relay。
- ready-index 丢失时数据库可恢复。
- immediate 与 relay 共享合法 Store 状态迁移。
- 可观测 backlog、oldest age、publish/mark 失败。

不提供：

- 跨 Mongo/MySQL 原子事务。
- exactly-once。
- 对不可逆外部副作用的统一 inbox/dedup record。
- 自动 DLQ、人工 replay 和无限积压的自动治理。

消费者必须按[领域事件设计](./02-领域事件设计.md)中的逐事件策略实现幂等。

## 10. 代码与验证

- Ports：`internal/apiserver/application/eventing`
- Orchestration：`internal/apiserver/eventing/subsystem`
- Shared relay core：`internal/apiserver/outboxcore`
- Mongo Store：`internal/apiserver/infra/mongo/eventoutbox`
- MySQL Store：`internal/apiserver/infra/mysql/eventoutbox`
- Ready-index：`internal/apiserver/infra/redis/outboxready`

```bash
go test ./internal/apiserver/application/eventing \
  ./internal/apiserver/eventing/subsystem \
  ./internal/apiserver/outboxcore \
  ./internal/apiserver/infra/mongo/eventoutbox \
  ./internal/apiserver/infra/mysql/eventoutbox \
  ./internal/apiserver/infra/redis/outboxready
```
