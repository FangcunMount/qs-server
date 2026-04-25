# Redis Lock、幂等与重复抑制

**本文回答**：当前 Redis lock 在哪些场景使用，哪些是 leader lock，哪些是 idempotency guard，哪些只是 best-effort duplicate suppression。

## 30 秒结论

| 场景 | 语义 | 代码 |
| ---- | ---- | ---- |
| Scheduler leader | 抢不到锁就跳过本轮 | [`runtime/scheduler`](../../../internal/apiserver/runtime/scheduler/) |
| Collection submit | in-flight lock + done marker | [`SubmitGuard`](../../../internal/collection-server/infra/redisops/submit_guard.go) |
| Worker answersheet | best-effort duplicate suppression，降级继续 | [`answersheet_handler.go`](../../../internal/worker/handlers/answersheet_handler.go) |
| Redis primitive | token-based lease，无自动续租，无 fencing token | [`redislock`](../../../internal/pkg/redislock/) |

## 模型图

```mermaid
flowchart TD
    redislock["redislock.Manager<br/>lease primitive"]
    leader["Leader Lock<br/>skip on contention"]
    submit["SubmitGuard<br/>done marker + in-flight lock"]
    duplicate["Worker Gate<br/>best-effort duplicate skip"]

    redislock --> leader
    redislock --> submit
    redislock --> duplicate
```

## SubmitGuard 时序

```mermaid
sequenceDiagram
    participant S as SubmissionService
    participant G as SubmitGuard
    participant R as Redis

    S->>G: Begin(key)
    G->>R: GET done marker
    alt done exists
        G-->>S: already submitted
    else no done
        G->>R: acquire in-flight lease
        alt acquired
            S->>S: submitSync
            S->>G: Complete(answerSheetID)
            G->>R: SET done marker
            G->>R: release lease
        else contention
            G-->>S: ResourceExhausted
        end
    end
```

## 不变量

- `redislock` 不自动续租；长任务要单独评估 TTL。
- wrong-token release 不能释放其他 owner 的锁。
- `SubmitGuard.Complete` 写 done marker 失败时保留 in-flight lock，等待 TTL 过期。
- worker gate 失败时继续处理，正确性依赖下游幂等和唯一约束。

## Verify

```bash
go test ./internal/pkg/redislock ./internal/collection-server/infra/redisops ./internal/worker/handlers ./internal/apiserver/runtime/scheduler
```
