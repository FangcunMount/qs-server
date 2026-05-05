# 为什么使用 Outbox

**本文回答**：为什么 qs-server 不在业务事务里直接 publish MQ，也不把事件发布交给 worker 重试，而是在 AnswerSheet、Assessment、Report 等关键写入路径中使用 Outbox；Outbox 在系统里解决了什么一致性问题、带来了什么代价、如何与 Mongo/MySQL、EventCatalog、Relay、Worker 和治理状态协作。

---

## 30 秒结论

Outbox 解决的是一个核心问题：

```text
业务事实已经落库，但事件没有可靠发出去，怎么办？
```

如果没有 Outbox，以下任一场景都会造成业务链路断裂：

| 场景 | 后果 |
| ---- | ---- |
| AnswerSheet 保存成功，但 MQ publish 失败 | 评估永远不执行 |
| Assessment 状态更新成功，但后续事件没发出 | 统计/通知/行为投影缺失 |
| Report 保存成功，但事件丢失 | 前端/统计/后续链路不知道报告完成 |
| 进程在 DB commit 后、MQ publish 前崩溃 | 业务事实存在，但异步链路断掉 |
| MQ 短暂不可用 | 同步请求失败面扩大，或事件丢失 |

Outbox 的基本策略是：

```text
同一个业务事务里：
  保存业务事实
  Stage domain events 到 outbox

事务提交后：
  Relay 从 outbox claim due events
  Publish 到 MQ
  成功 MarkPublished
  失败 MarkFailed + nextAttemptAt
```

一句话概括：

> **Outbox 把“业务事实落库”和“事件可靠出站”绑定在同一个持久化事务里，把 MQ 不稳定性从主写链路中隔离出去。**

---

## 1. 为什么不能直接 publish MQ

最直觉的方案是：

```text
Save AnswerSheet
  -> Publish answersheet.submitted
```

问题是这两个动作不是一个原子操作。

### 1.1 先保存 DB，再 publish MQ

```text
DB save success
MQ publish failed
```

结果：

- AnswerSheet 已经存在。
- `answersheet.submitted` 没发出去。
- worker 不会创建 Assessment。
- 用户看到答卷提交成功，但报告永远不生成。

这就是典型的“双写不一致”。

### 1.2 先 publish MQ，再保存 DB

```text
MQ publish success
DB save failed
```

结果：

- worker 收到事件。
- 但 AnswerSheet 不存在。
- 后续创建 Assessment 失败。
- 还可能触发无意义重试。

这比先 DB 后 MQ 更糟。

### 1.3 同步 publish 成功也不够

即使 MQ publish 成功，也不能完全消除问题：

- 进程可能在 Mark/Response 前崩溃。
- MQ ack 不等于下游 worker 已处理。
- 后续事件链路仍需要状态可观测。
- 失败重试策略不应挤在用户请求里。

所以需要 Outbox。

---

## 2. Outbox 解决的核心一致性问题

Outbox 的关键不是“异步”，而是：

```text
业务事实和待发布事件同事务持久化
```

以 AnswerSheet 提交为例：

```text
Mongo transaction:
  Insert AnswerSheet
  Insert idempotency record
  Insert outbox events
```

事务成功后，系统至少有一个确定状态：

```text
答卷已保存，事件待发送
```

哪怕 MQ 暂时不可用，事件仍在 outbox 中，后续 relay 可以继续发送。

---

## 3. qs-server 中有哪些 Outbox

当前系统中至少有三类重要 outbox 语义：

| Outbox | 存储 | 典型事件 | 作用 |
| ------ | ---- | -------- | ---- |
| mongo-domain-events | Mongo | answersheet.submitted、behavior footprint 等 | 从 Survey 答卷事实出站 |
| assessment-mysql-outbox | MySQL | assessment/evaluation lifecycle 事件 | 从 Evaluation/Assessment 状态出站 |
| report mongo outbox | Mongo | report / interpretation 相关事件 | 从 Report durable save 出站 |

它们都不是“单独的消息表”那么简单，而是和各自业务事实存储绑定。

---

## 4. AnswerSheet 为什么使用 Mongo Outbox

AnswerSheet 是 Mongo 文档。

提交时需要保证：

1. AnswerSheet 已保存。
2. Idempotency 记录已保存。
3. Domain events 已 staged。

当前 durable submit 的关键动作是：

```text
CreateDurably
  -> withTransaction
     -> SaveSubmittedAnswerSheet
     -> outboxStore.StageEventsTx
```

这意味着：

```text
AnswerSheet + idempotency + outbox events
```

在同一个 Mongo transaction 中完成。

### 4.1 这解决什么

如果 transaction 成功：

- 答卷存在。
- 幂等记录存在。
- 待发布事件存在。

如果 transaction 失败：

- 三者都不应该半成功。

### 4.2 这对异步评估非常关键

worker 评估依赖 `answersheet.submitted`。

如果 AnswerSheet 保存和 event staging 不能同事务，异步评估链路就可能断。

---

## 5. Assessment 为什么使用 MySQL Outbox

Evaluation 中 Assessment、Score 等结果更多落在 MySQL。

Assessment 状态变化和后续事件也需要可靠出站。

MySQL outbox store 使用表：

```text
domain_event_outbox
```

主要字段包括：

- event_id。
- event_type。
- aggregate_type。
- aggregate_id。
- topic_name。
- payload_json。
- status。
- attempt_count。
- next_attempt_at。
- last_error。
- published_at。

### 5.1 Claim 机制

MySQL outbox 使用：

```text
FOR UPDATE SKIP LOCKED
```

claim due events，并把它们标记为 `publishing`。

这支持多 relay / 多实例下避免重复 claim 同一批 event。

### 5.2 状态流转

```text
pending
  -> publishing
  -> published

failed
  -> publishing
  -> published

publishing stale
  -> publishing
```

失败会：

```text
MarkEventFailed
  -> status=failed
  -> attempt_count + 1
  -> next_attempt_at = now + retryDelay
```

---

## 6. Mongo Outbox 为什么有不同 claim 策略

Mongo 没有 MySQL 的 `FOR UPDATE SKIP LOCKED` 语法，所以 Mongo outbox 用 `findOneAndUpdate` 抢占单个事件：

```text
filter status/next_attempt_at
sort
set status=publishing
return document after update
```

它会分别处理：

- pending。
- failed。
- stale publishing。

并设置多个索引：

- unique event_id。
- status + next_attempt_at。
- pending created_at + next_attempt_at。
- failed next_attempt_at + created_at。
- publishing updated_at + created_at。

### 6.1 为什么要处理 stale publishing

如果 relay claim 到 event 后进程崩溃，event 可能一直停在 `publishing`。

stale publishing 机制允许后续 relay 在超过 `publishingStaleFor` 后重新 claim。

---

## 7. OutboxRelay 做什么

OutboxRelay 的职责是：

1. 从 store ClaimDueEvents。
2. 执行 BeforePublish hooks。
3. 调 EventPublisher.Publish。
4. 成功 MarkEventPublished。
5. 失败 MarkEventFailed。
6. 上报 outbox observability。
7. Report outbox status。

伪流程：

```text
DispatchDue:
  pendingEvents = store.ClaimDueEvents(batchSize, now)

  for pending in pendingEvents:
    run hooks
    publish pending.Event
    if success:
      store.MarkEventPublished(eventID)
    else:
      store.MarkEventFailed(eventID, error, now + retryDelay)
```

---

## 8. 为什么需要 durable publisher 检查

`NewDurableOutboxRelay` 要求 publisher 是 MQ-backed：

```text
RequireDurablePublisher = true
```

如果 publisher 不是 MQ-backed，relay 不应该启动。

这是为了避免一种误解：

```text
outbox relay started
但实际只是 logging/nop publisher
事件并没有真正进入 MQ
```

当前 runtime bootstrap 也只有在 `mqPublisher != nil` 时才启动 durable relay。

---

## 9. EventCatalog 的作用

Outbox 存储事件时需要知道：

```text
event_type -> topic_name
```

这个映射不应该散落在业务代码中。

EventCatalog / TopicResolver 负责将 DomainEvent 解析成 outbox record 的 topic_name。

这带来两个收益：

1. 事件契约集中。
2. Outbox 不需要业务模块手写 topic。

如果 EventCatalog 缺失或配置错误，事件出站就可能进入错误 topic 或无法构造记录。

---

## 10. BeforePublish Hook 的作用

OutboxRelay 支持 `BeforePublishHook`。

当前 Survey 的 answersheet submitted relay 使用 hook 做 Scale 热度投影之类的发布前副作用。

Hook 的语义是：

```text
在 publish 前执行；
失败则当前 event MarkFailed；
不 publish；
等待后续重试。
```

这说明 hook 是事件出站前的可重试前置动作，不能随意放不可重试或高副作用逻辑。

---

## 11. Outbox 和 MQ 的关系

Outbox 不替代 MQ。

| Outbox | MQ |
| ------ | -- |
| 负责可靠出站前的持久化 | 负责进程间消息投递 |
| 和业务数据库同事务 | 独立消息系统 |
| 解决 DB 与 MQ 双写不一致 | 解决生产者到消费者异步解耦 |
| 可重试 publish | 可重试 consume |
| 保存 pending/failed/publishing | 保存 topic/channel/message |
| 由 relay publish | 由 worker consume |

正确理解：

```text
Outbox 是 producer-side reliability；
MQ 是 transport；
Worker 是 consumer-side processing。
```

---

## 12. Outbox 和业务事务的关系

Outbox 的关键约束：

```text
Stage event 必须发生在业务事务内
```

MySQL store 的 `Stage(ctx, events...)` 要求 context 中有 active transaction。

Mongo store 的 `Stage(ctx, events...)` 要求 context 是 `mongo.SessionContext`。

这防止开发者在非事务上下文里偷偷 stage event，造成业务事实和事件不一致。

---

## 13. 为什么不是让 worker 扫业务表

另一种方案是：worker 定时扫 AnswerSheet / Assessment 表，看哪些未处理。

这个方案有几个问题：

| 问题 | 后果 |
| ---- | ---- |
| 业务表需要增加处理状态 | 领域模型被消息投递污染 |
| 扫描成本高 | 高数据量下慢 |
| 很难知道具体事件类型 | 只能推断，不是显式事件 |
| 顺序和幂等复杂 | 事件语义不清 |
| 多个下游订阅困难 | 所有逻辑集中在扫描任务中 |

Outbox 显式记录事件，语义更清楚。

---

## 14. 为什么不是同步调用下游服务

例如：

```text
AnswerSheet save
  -> 直接调用 EvaluationService.CreateAssessment
```

问题：

- 提交请求被 Evaluation 拖慢。
- Evaluation 失败会影响 AnswerSheet 提交。
- 下游服务不可用会扩大主链路失败面。
- 无法统一重试。
- 无法支持多个订阅者。
- 无法审计事件出站状态。

Outbox 把这些变成：

```text
主链路只负责 stage event
后续由 relay/worker 处理
```

---

## 15. 为什么不是只靠事务提交后重试 publish

有人会说：

```text
DB commit 成功后，publish 失败就在当前请求里重试几次
```

这仍然不够：

- 请求超时后重试无法继续。
- 进程崩溃后重试丢失。
- MQ 长时间不可用会阻塞用户请求。
- 无法统一观察 pending/failed/backlog。
- 多事件订阅者难管理。

Outbox 的优势是把 publish retry 变成持久化后台任务。

---

## 16. Outbox 带来的收益

### 16.1 可靠性

业务事实和待发事件同事务持久化。

### 16.2 可重试

失败进入 failed，设置 nextAttemptAt，后续重新 claim。

### 16.3 可观测

可以看：

- pending count。
- failed count。
- publishing count。
- oldest created_at。
- lag。
- last error。

### 16.4 可恢复

进程崩溃后，pending/failed/stale publishing 都能继续处理。

### 16.5 可扩展

后续新增事件订阅者，不需要把同步调用塞进主写链路。

---

## 17. Outbox 带来的代价

| 代价 | 说明 |
| ---- | ---- |
| 多一张表/集合 | MySQL/Mongo 都需要 outbox 存储 |
| 多一个 relay 运行时 | 需要调度、关闭、观测 |
| 最终一致 | 事件不会和业务响应同时被消费 |
| 排障链路变长 | 要查业务表、outbox、MQ、worker |
| 重复投递可能性 | consumer 必须幂等 |
| 状态清理问题 | published 事件后续是否归档/清理要设计 |

Outbox 提升可靠出站，但不保证 consumer exactly-once。

---

## 18. Outbox 不保证什么

Outbox 不保证：

- MQ 一定立即可用。
- worker 一定成功处理。
- consumer exactly-once。
- 业务不会重复消费。
- 事件顺序完全全局一致。
- 下游处理没有副作用。
- published 后业务一定完成。

所以 worker 端仍需要：

- duplicate suppression。
- idempotency。
- status check。
- retry。
- dead-letter / manual repair，未来可补。

---

## 19. Outbox 的状态排障

### 19.1 pending 增多

可能原因：

- relay 没启动。
- MQ publisher 不可用。
- durable relay 未启动，因为 mqPublisher nil。
- batch size 太小。
- relay goroutine 卡住。

### 19.2 failed 增多

可能原因：

- MQ publish 失败。
- EventCatalog topic 配置错误。
- BeforePublishHook 失败。
- payload decode 失败。
- 下游 topic 不存在。

### 19.3 publishing 长期存在

可能原因：

- relay claim 后进程崩溃。
- MarkPublished 失败。
- stale publishing 时间未到。
- DB/Mongo update 失败。

### 19.4 published 但 worker 没处理

可能原因：

- MQ 投递后 worker 未消费。
- topic/channel 配置错误。
- worker handler 未注册。
- worker Nack/retry。
- consumer-side 幂等跳过。

---

## 20. 设计不变量

后续演进应坚持：

1. 关键业务事件必须和业务事实同事务 stage。
2. 不允许业务保存成功但事件只在内存里。
3. Outbox relay 只在 durable publisher 可用时启动。
4. Outbox store 必须支持 claim due、mark published、mark failed。
5. Failed event 必须有 nextAttemptAt。
6. Publishing stale 必须可恢复。
7. EventID 必须唯一。
8. EventCatalog 是 topic 解析真值。
9. Consumer 必须幂等。
10. Governance endpoint 默认只读，不应随意 mark/skip/delete event。

---

## 21. 常见误区

### 21.1 “用了 MQ 就不需要 Outbox”

错误。MQ 解决进程间投递，不解决 DB 与 MQ 双写一致性。

### 21.2 “Outbox 能保证 exactly-once”

不能。Outbox 只能让 producer-side 出站可靠；consumer 仍要幂等。

### 21.3 “直接 publish 失败重试几次就够了”

不够。进程崩溃和 MQ 长时间不可用时仍会丢事件或阻塞请求。

### 21.4 “Outbox pending 多就是 worker 慢”

不一定。pending 多可能是 relay/MQ/publisher 问题；worker 慢更多体现在 MQ backlog/channel depth。

### 21.5 “published 代表业务全链路完成”

不代表。published 只说明事件已进入 MQ，worker 是否处理成功要看 consumer 侧。

---

## 22. 替代方案分析

### 22.1 直接同步调用

优点：

- 实现简单。
- 立即知道下游结果。

缺点：

- 主链路变慢。
- 下游失败影响提交。
- 不支持多订阅者。
- 无持久重试。

结论：不适合 AnswerSheet -> Evaluation 这种链路。

### 22.2 DB commit 后直接 publish MQ

优点：

- 比同步调用轻。
- 实现简单。

缺点：

- DB commit 后、publish 前崩溃会丢事件。
- MQ 失败时无持久重试。
- 难观测 backlog。

结论：仍有双写不一致。

### 22.3 Worker 扫业务表

优点：

- 不依赖 MQ publish。

缺点：

- 扫描成本高。
- 事件语义不清。
- 领域表被处理状态污染。
- 多订阅者扩展差。

结论：不适合作为事件系统主路径。

### 22.4 当前 Outbox 方案

优点：

- 业务事实和事件同事务。
- 后台持久重试。
- 可观测。
- 可恢复。
- 扩展多事件类型。

缺点：

- 实现复杂。
- 最终一致。
- consumer 仍需幂等。
- 需要 relay 和治理状态。

结论：当前系统复杂度下更稳妥。

---

## 23. 代码锚点

### Outbox 通用应用层

- `internal/apiserver/application/eventing/outbox.go`
- `internal/apiserver/port/outbox/outbox.go`
- `internal/apiserver/outboxcore`

### Mongo Outbox

- `internal/apiserver/infra/mongo/eventoutbox/store.go`
- `internal/apiserver/infra/mongo/answersheet/durable_submit.go`

### MySQL Outbox

- `internal/apiserver/infra/mysql/eventoutbox/store.go`

### 模块装配

- `internal/apiserver/container/assembler/survey.go`
- `internal/apiserver/container/assembler/evaluation.go`

### Runtime Relay

- `internal/apiserver/process/runtime_bootstrap.go`

---

## 24. Verify

```bash
go test ./internal/apiserver/application/eventing
go test ./internal/apiserver/infra/mongo/eventoutbox
go test ./internal/apiserver/infra/mysql/eventoutbox
go test ./internal/apiserver/infra/mongo/answersheet
go test ./internal/apiserver/process
```

如果修改事件目录：

```bash
go test ./internal/pkg/eventcatalog
```

如果修改 worker 消费：

```bash
go test ./internal/worker/handlers
```

如果修改文档：

```bash
make docs-hygiene
git diff --check
```

---

## 25. 下一跳

| 目标 | 文档 |
| ---- | ---- |
| 为什么需要读侧统计聚合 | `05-为什么需要读侧统计聚合.md` |
| 为什么同步提交但异步评估 | `02-为什么同步提交但异步评估.md` |
| Event Publish / Outbox 深讲 | `../03-基础设施/event/02-Publish与Outbox.md` |
| Worker Ack/Nack | `../03-基础设施/event/03-Worker消费与AckNack.md` |
| AnswerSheet 提交 | `../02-业务模块/survey/02-AnswerSheet提交与校验.md` |
