# Behavior Projection 深讲目录

**本文回答**：本目录把行为投影从专题长文中拆成可维护的 truth layer，说明 footprint 事件如何投影为 `assessment_episode` 和统计读模型，以及 pending/reconcile 为什么存在。

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 投影目标 | 把行为 footprint 和测评过程串成可统计的 `assessment_episode`，再更新 analytics projection |
| 主路径 | footprint durable outbox -> worker behavior handler -> apiserver internal gRPC -> statistics projector |
| 乱序处理 | 缺少归因条件时进入 pending；后续事件或 reconcile scheduler 再补偿 |
| 边界 | 这是统计读侧投影，不是业务写模型，不改变 AnswerSheet / Assessment / Report 权威状态 |
| 维护方式 | 模型、运行时、pending/reconcile 和排障分别维护，原专题文只保留入口判断 |

## 阅读顺序

```mermaid
flowchart LR
    model["00 整体模型"] --> runtime["01 运行时链路"]
    runtime --> pending["02 Pending 与 Reconcile"]
    pending --> episode["03 assessment_episode 边界"]
    episode --> ops["04 排障与演进边界"]
```

| 顺序 | 文档 | 先回答什么 |
| ---- | ---- | ---------- |
| 1 | [00-整体模型.md](./00-整体模型.md) | footprint、episode、projection、pending 的关系 |
| 2 | [01-运行时链路.md](./01-运行时链路.md) | 事件如何从 outbox 进入 projector |
| 3 | [02-Pending与Reconcile.md](./02-Pending与Reconcile.md) | 乱序和缺归因如何延迟处理 |
| 4 | [03-assessment_episode边界.md](./03-assessment_episode边界.md) | episode 到底记录什么、不记录什么 |
| 5 | [04-排障与演进边界.md](./04-排障与演进边界.md) | 如何定位 projector 问题，以及哪些能力当前不支持 |

## 代码锚点与测试锚点

| 能力 | 锚点 |
| ---- | ---- |
| 行为事件契约 | [configs/events.yaml](../../../configs/events.yaml) |
| worker handler | [internal/worker/handlers/behavior_handler.go](../../../internal/worker/handlers/behavior_handler.go) |
| projector 应用服务 | [internal/apiserver/application/statistics/journey.go](../../../internal/apiserver/application/statistics/journey.go) |
| 领域模型 | [internal/apiserver/domain/statistics/journey.go](../../../internal/apiserver/domain/statistics/journey.go) |
| MySQL 存储 | [internal/apiserver/infra/mysql/statistics/journey_repository.go](../../../internal/apiserver/infra/mysql/statistics/journey_repository.go)、[internal/apiserver/infra/mysql/statistics/po_journey.go](../../../internal/apiserver/infra/mysql/statistics/po_journey.go) |

## Verify

```bash
go test ./internal/apiserver/application/statistics ./internal/worker/handlers
python scripts/check_docs_hygiene.py
```

