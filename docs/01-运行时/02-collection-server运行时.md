# collection-server 运行时

## 1. 结论

`collection-server` 是前台 BFF 和保护层。它组合身份上下文、租户范围、入口限流、50ms Submit Gate、跨实例 SubmitGuard、下游背压、Assessment readiness 和 gRPC client，但不拥有 Survey/Evaluation 聚合。进程内 SubmitQueue 已删除。

## 2. 请求路径

```text
REST middleware
  -> identity / tenant scope
  -> rate limit / Submit Gate / advisory duplicate guard
  -> questionnaire + ProfileLink preflight
  -> apiserver SaveAnswerSheet gRPC
  -> Mongo transaction: AnswerSheet + idempotency + Outbox
  -> 202 accepted + answersheet_id
  -> assessment-readiness / report wait
```

具体端点以 `api/rest/collection.yaml` 为准；保护机制以 `internal/collection-server` 的实际装配和配置为准。

## 3. 失败语义

Gate 50ms 未取得槽位返回 `429 + Retry-After: 1`；前置依赖、gRPC、Mongo 或可靠事件不可用返回 `503 + Retry-After: 1`。只有 AnswerSheet、幂等记录和 Outbox 同事务可靠提交后才返回 `202`。
