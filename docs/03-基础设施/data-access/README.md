# Data Access：事实、事务与存储边界

Data Access 层解决的不是“如何调用数据库 SDK”，而是四个更重要的问题：业务事实归哪个存储所有、一次事务能保护哪些写入、查询模型如何与写模型解耦，以及存储变慢时如何阻止请求把连接池和数据库一起压垮。

## 1. 先看结论

- MySQL 与 MongoDB 都是权威事实源，但各自拥有不同聚合；Redis 只承担缓存、信令、限流、租约和 ready-index 等可重建能力。
- application 依赖 domain repository、read-model port 和 `transaction.Runner`；具体 GORM、Mongo Driver、Redis 与 OSS adapter 留在 infra/container。
- MySQL UoW 只覆盖一个 MySQL transaction；Mongo Runner 只覆盖一个 Mongo session transaction。系统没有 Mongo/MySQL 分布式事务，也不应通过调用顺序伪装原子性。
- 可靠事件必须与业务事实写入同一本地事务：Mongo 事实配 Mongo Outbox，MySQL 事实配 MySQL Outbox。跨存储后续动作依靠 at-least-once Event、业务幂等和可恢复状态机收敛。
- 数据库唯一键、claim/CAS 和事务提交是正确性机制；Redis guard、缓存与 ready-index 只能降低代价或加速恢复。
- backpressure 限制的是当前 apiserver 进程内、已经接入 limiter 的数据库操作并发，不等于数据库连接池容量，也不是全局 QPS 限流。
- migration 是 schema 演进证据，不是某张表已无读写者的证明；删除前仍要从 composition root、repository/read model、任务与运维脚本反向追踪。

## 2. 当前存储责任地图

| 存储 | 主要权威数据 | 典型 adapter | 事务/一致性角色 |
| --- | --- | --- | --- |
| MySQL | Actor、Plan、Evaluation、Statistics 投影、runtime checkpoint、治理审计与 retry/dead-letter 状态 | `internal/apiserver/infra/mysql/*` | 关系约束、行锁、唯一键、GORM UoW、MySQL Outbox |
| MongoDB | Questionnaire、AnswerSheet 与提交幂等、ModelCatalog/Ruleset/Norm、Interpretation 生命周期与报告 | `internal/apiserver/infra/mongo/*` | 文档聚合、Mongo session transaction、唯一索引、Mongo Outbox |
| Redis | cache、signal、rate limit、lease、Outbox ready-index、热度等运行时状态 | `internal/apiserver/infra/redis/*`、`internal/pkg/redisruntime` | 加速与协调；除明确的 operational state 外不拥有业务事实 |
| OSS | 评估素材、二维码等二进制对象 | `internal/apiserver/infra/objectstorage` | 对象读写；不与 MySQL/Mongo 业务事务原子提交 |

这张表描述当前所有权，不表示一个模块只能读一种数据库。例如 Evaluation 的执行可以读取 Mongo 中已发布的问卷和模型，但 Assessment、Run、Outcome 的权威写模型仍在 MySQL。

### 2.1 MySQL 主体

- `actor`：testee、operator、clinician、relation、assessment entry。
- `plan`：assessment plan、enrollment、task 及面向工作台的查询。
- `evaluation`：assessment、run、score、outcome 与一致性读模型。
- `checkpoint`：Evaluation/Statistics 等运行 checkpoint、claim、retry disposition。
- `statistics`：collector 运行记录、聚合事实和发布后的查询模型。
- `eventoutbox`、`eventdelivery`、`retrygovernance`、`systemgovernance`：可靠投递和运维治理事实。

### 2.2 MongoDB 主体

- `questionnaire`：问卷定义和发布查询。
- `answersheet`：答卷事实、提交幂等记录和 AnswerSheet 读模型。
- `modelcatalog`、`ruleset`：评估模型、常模、规则集和发布快照。
- `interpretation`：generation、run、report artifact、archive、query catalog、template 和 admission failure。
- `eventoutbox`：与上述 Mongo 业务事实同事务的可靠事件。

## 3. 依赖方向：port 是边界，BaseRepository 不是

```text
handler / job
      ↓
application service
      ├─ domain repository interface
      ├─ read-model/query port
      └─ transaction.Runner
                 ↑
container module assembly
                 ↓
infra/mysql | infra/mongo | cache | objectstorage
```

### Domain repository

Repository 接口由 domain 或 application-facing port 定义，表达聚合保存、按业务键查找、claim 等业务语言。infra adapter 负责：

- PO/document 与 domain entity 映射；
- 驱动查询、索引和锁语义；
- 唯一键/找不到等数据库错误到业务错误的翻译；
- 审计字段和软删除等持久化细节。

`internal/pkg/database/mysql.BaseRepository` 与 `internal/apiserver/infra/mongo.BaseRepository` 只是共享 CRUD、context 和 backpressure 接入的实现工具。application 不应依赖它们；高级 repository 也不能因为能拿到 `DB()`/`Collection()` 就绕过自己的业务约束。

### Read model

read-model port 服务特定查询，不要求返回聚合，也不强制通过 domain repository。当前既有：

- 从权威表/集合直接 join 或 aggregate 的查询 adapter；
- Statistics、report query catalog、attention 等可重建投影；
- 缓存 decorator 包装的读链路。

因此“read model”不自动意味着“异步最终一致”。必须继续追到 adapter：直接读权威库时是该数据库当前可见状态；读异步投影时才需要声明 lag、checkpoint 和重建方式。

## 4. 本地事务模型

application 只看到统一的窄接口：

```go
type Runner interface {
    WithinTransaction(
        ctx context.Context,
        fn func(txCtx context.Context) error,
    ) error
}
```

container 决定实际 runner：

| Runner | 实现 | transaction context 如何传播 | 关键前提 |
| --- | --- | --- | --- |
| MySQL | component-base GORM UnitOfWork | `gormuow.WithContext` 从 context 取得当前 `*gorm.DB` transaction | repository 必须使用传入的 `txCtx` |
| Mongo | `mongo.Session.WithTransaction` | callback 收到 `mongo.SessionContext` | replica set/sharded deployment 支持 transaction |

Mongo transaction 显式使用 primary read preference、snapshot read concern 和 majority write concern。standalone Mongo 不具备这项跨文档原子发布能力，应失败，而不是悄悄退化成若干独立写。

### 4.1 事务内可以保证什么

- 同一 MySQL transaction 内的表写入、行锁、唯一键和 MySQL Outbox。
- 同一 Mongo session transaction 内的文档写入、唯一索引和 Mongo Outbox。
- callback 返回 error 时由 runner 回滚本地事务。

### 4.2 事务内不能保证什么

- Mongo 与 MySQL 一起 commit。
- 数据库与 Redis/OSS/MQ 一起 commit。
- 已 commit 后的网络响应一定到达调用方。
- Event consumer 只执行一次。

跨边界流程应记录可恢复的中间事实，再由 Event/claim/CAS/幂等推进；不能把“先写 A，再写 B”描述成原子事务。

## 5. Transactional Outbox 的数据访问责任

```text
local transaction
  ├─ write aggregate / lifecycle fact
  └─ stage outbox in the same database
          │
          ├─ rollback → both absent
          └─ commit   → both durable
                           ↓
                 post-commit hint + relay
```

两个代表性链路：

- AnswerSheet：Mongo AnswerSheet、`(writer_id, idempotency_key)` 提交幂等事实和 `answersheet.submitted` Outbox 共享 Mongo transaction。
- Evaluation：Assessment/Run/Outcome 等 MySQL 事实与对应 lifecycle Outbox 共享 MySQL transaction。

Stager 要求活跃 transaction context；缺少 transaction 时必须报错，不能单独写 Outbox。`AfterCommit` 只做 ready-index/immediate 等提交后推动，失败不能推翻已经 commit 的业务事实。完整状态机见 [Outbox 可靠出站链路](../event/03-Outbox可靠出站链路.md)。

## 6. 并发正确性落在哪里

| 问题 | 最终保护 | Redis/进程内机制的角色 |
| --- | --- | --- |
| 同一 AnswerSheet 意图重复提交 | Mongo 唯一键 + fingerprint + transaction | SubmitCoalescer 降低重复工作 |
| 同一事件重复消费 | 业务唯一键、ensure、claim/CAS 或可重复投影 | processed key 只用于特定可重建投影 |
| 多实例竞争任务 | DB row/document claim、lease token、状态条件更新 | Redis lease 只在明确协议中作为协调 |
| 查询缓存陈旧 | 权威 DB + version/TTL/失效协议 | Redis/L1 提高读取性能 |
| Outbox ready-index 丢失 | Mongo/MySQL Outbox + DB polling | Redis ZSet 仅加速发现 |

这也是“Redis SubmitCoalescer 不是正确性机制”的根本原因：只要 Redis 故障、过期或两个请求绕开同一 coalescer，正确性仍必须由持久化约束成立；否则系统只是把竞态隐藏在正常路径里。

## 7. Backpressure 与连接池

MySQL/Mongo BaseRepository 可注入同一个进程级 `backpressure.Acquirer`。一次操作先等待 slot，成功后持有到数据库调用返回，最后 release：

```text
request
   ↓ Acquire(max_inflight, timeout)
slot obtained ──> DB operation ──> release
   └─ timeout/cancel ──> fail fast with evidence
```

需要同时区分三个预算：

- `max_inflight`：应用愿意同时放行到该依赖的操作数。
- DB driver pool：连接创建、复用和排队的上限。
- 请求 QPS：单位时间到达量。

它们不能互换。一次请求可能执行多次 DB 操作，事务会持有连接，不同 SQL 的服务时间也不同；因此 `150 max_inflight` 不代表 `150 QPS`，更不代表多实例的全局安全吞吐。

当前 limiter 是每个 apiserver 实例本地状态，并通过 module assembly 注入普通 repository 与 Event Store。新增 raw GORM/Mongo adapter 时必须确认是否显式接入 limiter；只要直接使用裸 `*gorm.DB`/`*mongo.Collection`，就不能假定 BaseRepository 的保护自动生效。容量推导和故障恢复见 [下游背压与容量预算](../concurrency/40-下游背压与容量预算.md)。

## 8. 错误与幂等映射

- MySQL helper 能识别常见 duplicate/unique violation；只有 repository 安装 translator 后，才会变成对应业务错误。
- Mongo repository 应依据 error code、matched/modified count 和唯一索引解释冲突；不能只匹配错误字符串。
- “先查是否存在，再插入”不是并发幂等；最终仍要依赖唯一约束，并在 duplicate 后回读/比较业务意图。
- `not found`、`conflict`、`dependency unavailable` 和 `backpressure timeout` 是不同语义，不应全部翻译成 500。
- 日志可记录表/集合、操作、耗时和错误分类，但不得输出答卷、令牌、数据库密码或完整敏感 payload。

## 9. 连接、启动与关闭

apiserver 的 `DatabaseManager` 负责：

1. 根据配置注册 MySQL、Redis profiles 和 MongoDB。
2. 初始化连接；取得 MySQL 时会 ping 并记录连接池快照。
3. 在 migration 启用时依次运行 MySQL/Mongo migration，并校验部分关键 Mongo 索引。
4. 向后续 Redis runtime、EventSubsystem 和 module assembly 交付具体 client。
5. 健康检查时检查 registry 与 Redis profiles；关闭时释放全部连接。

当前 composition 要求 module assembly 取得 MySQL 与 MongoDB；不要仅根据 `initMySQL/initMongoDB` 中“未配置则 skip”的日志，就断言生产进程可以缺少该数据库正常提供完整 API。

## 10. Migration 与一次性数据修复

MySQL 与 Mongo migration 各有独立版本序列，文件嵌入二进制：

- MySQL：`internal/pkg/migration/migrations/mysql`
- MongoDB：`internal/pkg/migration/migrations/mongodb`
- migration runner：`internal/pkg/migration`

约束：

- schema 变更使用成对的 `up/down` migration，并用 contract/integration test 固化关键列、索引和退场顺序。
- dirty version 会阻止继续自动迁移，需要人工判断；不能盲目 force。
- 大数据 backfill、审计或不可重复的一次性修复放在 `scripts/oneoff`，需要单独的 dry-run、范围、checkpoint 和回滚说明。
- migration 中出现 `DROP` 只能证明部署时会执行删除；安全性还取决于旧版本进程、灰度窗口、查询脚本、worker 和回滚版本是否仍访问它。
- `AutoSeed` 是配置字段，不应据此推断当前 migration 自动载入全部业务种子；以实际 runner 调用链为准。

## 11. OSS 边界

`internal/apiserver/infra/objectstorage` 把 Aliyun OSS 适配为窄 ObjectStore，用于评估素材和二维码等对象。对象 visibility 由消费它的 HTTP proxy/应用协议控制，不依赖把 bucket ACL 当作业务鉴权。

OSS 写入不参加 MySQL/Mongo transaction。需要“元数据 + 对象”一致性时，应设计 pending/ready 状态、幂等 key 和垃圾回收/补偿；当前不能把两个独立调用描述为原子发布。

## 12. 排障顺序

| 现象 | 先看 | 再看 | 不应直接下结论 |
| --- | --- | --- | --- |
| DB 请求超时 | backpressure wait/timeout、request deadline | pool `InUse/WaitCount`、DB slow query | “连接池太小” |
| duplicate 增长 | 业务键、唯一索引、调用重试 | translator 和 duplicate 后回读 | “Redis 锁失效导致数据坏了” |
| Outbox backlog | Store 状态、retry disposition、oldest age | MQ、relay、mark failure | “Redis ready-index 丢了事件” |
| read model 与写模型不一致 | adapter 数据源和 checkpoint | projection consumer/collector | “缓存没删” |
| migration 启动失败 | backend/version/dirty state | 当前 schema 与 migration SQL/JSON | “重跑容器就会好” |
| Mongo transaction 失败 | replica set、write concern、session error | keyfile/权限、transaction integration test | “代码编译通过所以 durable submit 可用” |

## 13. 扩展与验收清单

新增 repository/read model 时：

1. 先声明权威存储、业务键、组织范围和一致性需求。
2. 在 domain/application 定义最窄 port；infra 实现映射与数据库语义。
3. 明确是否必须加入现有 transaction；不要在 repository 内偷偷开启另一个 transaction。
4. 为并发写设计唯一键、claim/CAS，并测试 duplicate/race/failure。
5. 判断是否需要 Outbox；若需要，必须选择与事实相同的 profile。
6. 判断查询是直接权威读取、缓存装饰，还是异步投影，并记录 freshness/重建路径。
7. 注入并验证 backpressure；审计 raw DB 调用是否绕过 limiter。
8. 增加 migration、索引契约和真实数据库 integration test。

验证入口：

```bash
go test ./internal/pkg/database/mysql \
  ./internal/pkg/resilience/backpressure \
  ./internal/apiserver/container/internal/transaction \
  ./internal/apiserver/infra/mysql/... \
  ./internal/apiserver/infra/mongo/...

go test ./internal/pkg/migration \
  ./internal/pkg/architecture
```

带 build tag 或外部数据库的 integration test 只有在 MySQL/Mongo 环境真实可用时才算通过；普通 `go test` 的跳过、编译成功或 mock 通过不能替代 replica-set transaction、唯一索引和 migration 的端到端验证。
