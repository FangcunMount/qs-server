# 新增 Event 与 Signal SOP

## 1. 先判断用哪一种契约

| 问题 | Event | Signal |
| --- | --- | --- |
| 是否表示业务事实 | 是 | 否，只是唤醒或缓存协同 |
| 丢失是否影响正确性 | durable Event 不能丢 | 允许，必须有 TTL、轮询或事实源兜底 |
| 是否需要 Outbox/MQ | 按 delivery 选择 | 否，使用 Redis Pub/Sub best-effort |

不要把业务事实放入 Signal，也不要为了一次性唤醒而新增 durable Event。

## 2. 新增 Event

1. 在 `internal/pkg/eventing/catalog/types.go` 增加稳定 event type 常量。
2. 在 `configs/events.yaml` 增加 Topic、delivery、domain、aggregate 和主 worker handler。不要改变既有 Topic 或 envelope。
3. 在 `internal/pkg/eventing/catalog/spec.go` 的 `DefaultSpecs` 增加 EventSpec：
   - durable event 必须指定 Outbox profile、priority、idempotency 和 settlement；
   - best-effort event 不得指定 profile、priority 或 immediate；
   - immediate 只能用于 durable event。
4. 在 `internal/pkg/eventing/payload` 或业务所有的 payload 位置定义 wire DTO，并增加 JSON 表征测试。
5. 如果有主 worker handler，将它注册到 `internal/worker/handlers` 的显式 registry，并验证 poison ACK、unknown ACK、handler error NACK 不变。
6. 如果有附加消费者，在 EventSpec 声明唯一 Consumer ID、稳定 channel、runtime、幂等和 settlement，再由相应进程 EventSubsystem 绑定 handler。
7. 在 [09-事件契约矩阵.md](09-事件契约矩阵.md) 同步 producer、delivery、store、immediate、handler、idempotency 和 failure settlement。

最小验证：

```bash
go test ./internal/pkg/eventing/catalog \
  ./internal/pkg/eventing/runtime \
  ./internal/worker/integration/eventing \
  ./internal/worker/integration/messaging \
  ./internal/worker/handlers
```

## 3. 新增 Cache Signal

1. 在 `internal/pkg/signalcatalog/types.go` 增加稳定 signal name。
2. 在 `configs/signals.yaml` 增加同名清单项，delivery 保持 `ephemeral_signal`，transport 保持 `redis_pubsub`。
3. 在 `internal/pkg/cache/signal/contract.go` 增加 transport-agnostic 契约：只包含 wire 字段、`SignalName` 和 `SignalKey`，不得依赖 Redis、options、reportstatus 或指标。
4. 在 `internal/apiserver/cache/subsystem` 增加 notifier/watcher，或在 `internal/collection-server/cache` 增加 L1 watcher。运输直接使用 component-base Redis Signaler 和 `redis.UniversalClient`。
5. 保持 disabled 时不订阅、不发送；Notify 失败只记录指标和日志，不向业务返回错误。
6. 增加 SignalName、SignalKey、JSON、prefix/channel 和 Start/Close 表征测试。

最小验证：

```bash
go test ./internal/pkg/signalcatalog \
  ./internal/pkg/cache/signal \
  ./internal/apiserver/cache/subsystem \
  ./internal/collection-server/cache
```

## 4. 提交前门禁

```bash
go test ./internal/pkg/architecture
make docs-hygiene
git diff --check
```

如果变更了 wire contract、Topic、Redis channel、ACK/NACK 或 Outbox 事务边界，这就不再是普通契约扩展，必须单独评审兼容性和发布顺序。
