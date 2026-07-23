# Runtime：配置、组合根与进程生命周期

Runtime Infrastructure 决定“一份代码最终以什么进程、什么依赖和什么故障语义运行”。它负责读取配置、构造基础设施、装配业务模块、启动 transport/consumer/scheduler，并在进程退出时按明确顺序停止接流量和释放资源。

## 1. 先看结论

- qs-server 不是一个进程，而是 `qs-apiserver`、`collection-server`、`qs-worker` 三个独立进程；它们共享基础设施包，但拥有不同的 composition root 和生命周期。
- 三个入口都经过 Cobra/Viper `App`：读取 config/flag/env、完成与校验 Options，再把同一 Options 包装为 runtime Config。当前没有另一份手工复制的配置真值。
- process 层按 Stage 顺序准备资源。业务模块不创建全局 DB/Redis/MQ/gRPC client，而从 container/composition root 接收 port/adapter。
- apiserver 的当前启动顺序是 resources → container → integrations → transports → background runtimes → shutdown callback；collection 和 worker 有各自不同阶段。
- Stage runner 只“遇错即停”，没有通用 rollback。若 shutdown callback 尚未注册，前序阶段已创建的资源不会由 Runner 自动回收；当前部分 stage 有局部清理，但不是统一事务。
- 正常 SIGINT/SIGTERM 会触发 component-base GracefulShutdown；服务监听失败、RunGroup 中某个 server 退出或 prepare 阶段 `Fatalf` 则不一定经过同一关闭链。
- apiserver 当前关闭顺序先停后台/清理 container 和数据库，最后才关闭 HTTP/gRPC。这与“先摘流量、再释放依赖”的理想顺序相反，是已实现的生命周期风险。
- gRPC `Close()` 使用无 deadline 的 `GracefulStop()`；存在永不结束的 RPC 时，SIGTERM 回调可能长期阻塞。
- collection-server 在业务 HTTP 之外还会按 profiling 配置启动一个 `127.0.0.1:6060` pprof server，该 server 未纳入 shutdown；通用 HTTP server 默认也可注册 pprof。
- worker 同时使用 GracefulShutdown 的 POSIX manager 和自己的 `signal.Notify` 等待同一信号，存在 `Run()` 返回与异步 shutdown callback 竞争的双重信号所有权。
- 配置校验覆盖了很多数值和交叉约束，但不是 production safety proof：ServerRunOptions 的 Validate 为空、middleware 名称不校验、IAM disabled/verifier nil 等危险装配不会统一 fail-fast。
- 当前 repo 中的 `configs/*.prod.yaml` 是版本化意图，不是“线上一定如此”的证据。实际运行真值是进程启动时合并后的 config + flags + env + secret mounts。

## 2. 三个进程的责任地图

| 进程 | 入口 | 对外/内部 transport | 主要依赖 | 长跑任务 |
| --- | --- | --- | --- | --- |
| apiserver | `cmd/qs-apiserver` | HTTP/HTTPS + gRPC | MySQL、Mongo、Redis profiles、IAM、MQ、OSS/WeChat | Outbox relay、本地 Event consumer、cache watcher、治理恢复、业务 scheduler |
| collection-server | `cmd/collection-server` | HTTP/HTTPS，向 apiserver 发 gRPC | Redis、IAM、apiserver gRPC、MQ authz sync | cache subsystem、authz version subscriber、report signaling/wait runtime |
| worker | `cmd/qs-worker` | MQ consumer，向 apiserver 发 gRPC；独立 metrics HTTP | MQ、apiserver gRPC、Redis、Mongo、MySQL retry/dead-letter store | Event handlers、retry-hold replayer、attention projection、metrics server |

collection-server 不直接拥有 MySQL/Mongo 业务 repository；答卷和查询通过 apiserver gRPC 进入权威数据层。worker 的 Mongo 用于特定投影，MySQL 则由 delivery dead-letter/retry-hold runtime 单独打开和持有，不是 apiserver DatabaseManager 的共享连接。

## 3. 从命令行到有效配置

```text
cmd/*/main.go
   ↓ NewApp(basename)
default Options + flags
   ↓ Viper config / env / flags
ValidateRawSettings → Unmarshal
   ↓
Complete → Validate
   ↓
config.Config{Options}
   ↓
process.Run → PrepareRun → Run
```

### 3.1 配置发现与覆盖

- 显式 `--config/-c` 指向文件；未指定时从当前目录、`configs/`、用户目录和 `/etc/<prefix>` 搜索以 binary basename 命名的配置。
- 配置文件读取失败会直接退出，不会静默使用全部默认值启动。
- 环境变量前缀由 binary basename 生成：
  - `qs-apiserver` → `QS_APISERVER_`
  - `collection-server` → `COLLECTION_SERVER_`
  - `qs-worker` → `QS_WORKER_`
- key 中 `.` 与 `-` 映射为 `_`；flag 名中的 `_` 会归一化为 `-`。
- legacy NSQ host/port 与 `REDIS_DB` env 会在正常绑定后通过 `viper.Set` 写入兼容值。
- 启动日志会输出 config file、有效设置和一组常用 env；结构化配置经过 `configmask` 递归脱敏。

配置文件、环境变量、flags 和默认值之间的优先级由 Viper/pflag 决定。排障时应看启动时的 masked effective config，而不是只看 repo YAML。

### 3.2 校验边界

当前流程包含三层：

1. `ValidateRawSettings`：目前主要对 `cache` section 的未知字段做 schema 校验，不是全配置严格 schema。
2. `Complete`：apiserver/collection 主要补全 secure serving；worker 当前无额外完成逻辑。
3. `Validate`：检查连接、并发、重试、Redis family、scheduler、Outbox worker 数等显式约束。

仍未覆盖：

- `server.mode` 是否为 Gin 支持值。
- `server.middlewares` 名称是否已注册。
- production 是否强制 IAM/verifier/snapshot/mTLS。
- metrics/pprof/governance 是否暴露到不可信网络。
- 所有跨组件容量预算是否匹配。

因此“Options.Validate 通过”只代表已编码约束成立，不代表可安全上线。

### 3.3 配置与 secret 日志

`configmask` 会按 password/secret/token/key 等字段名脱敏 Viper/Options 输出；但 `cliflag.PrintFlags` 在 Debug 级别逐个输出原始 flag value，没有调用 mask。worker 的当前 production YAML 又配置了 debug/console/development/color，因此如果通过 CLI flag 传 secret，存在泄露风险。

生产要求：

- secret 通过 secret file/env/secret manager 注入，不进入版本库。
- 禁止以明文 secret CLI flag 启动；或先修复 PrintFlags 脱敏。
- production 日志级别和格式必须由部署门禁校验，不能信任配置文件注释。
- 证书/key path 可以记录，private key 内容和完整 DSN 不能记录。

### 3.4 进程外部文件

并非所有契约都嵌入二进制：

- apiserver 当前固定从相对路径 `configs/events.yaml` 加载 Event catalog。
- worker 默认同样读取 `configs/events.yaml`，可由 worker option 覆盖。
- gRPC ACL 在启用时从配置文件读取；读取失败会退回 default policy。
- TLS/mTLS、OSS、migration 和其它文件路径必须在容器内真实存在。

所以 working directory 与 volume mount 是 runtime contract。仅复制 binary、不复制 `configs/events.yaml` 会在 prepare resources 阶段失败。

## 4. 通用分阶段启动模型

三个 process 都使用 `processruntime.Runner`：

```text
state = empty
for stage in stages:
    output = stage.Run(state)
    on error: stop immediately
after all stages:
    build preparedServer
preparedServer.Run()
```

优势是每个阶段有稳定名字、输入和输出，测试可验证顺序与 wiring；代价是 Runner 本身不理解资源所有权，也不自动回滚。

### 启动资源所有权原则

- 构造成功后立即把 handle 放进 stage output。
- 后续 stage 只能通过 output 取得依赖，避免 package global。
- 创建者或 process lifecycle 必须拥有 Close/Stop/Cancel。
- 一个 stage 内发生部分失败时，应在返回前清理本 stage 已创建资源。
- 若要跨 stage rollback，需要显式 startup lifecycle；当前通用 Runner 未提供。

当前 `PrepareRun()` 遇错会 `Fatalf`，进程退出会由 OS 回收 socket/连接，但这不等于执行了 subscriber stop、flush、lease release 或应用级 cleanup。

## 5. apiserver 组合根

### 5.1 启动阶段

```text
prepare resources
  ├─ DatabaseManager: MySQL / Redis profiles / Mongo / migration
  ├─ Redis family runtime + cache subsystem
  ├─ resilience + lock subsystem
  ├─ governance action audit primary/fallback
  ├─ MQ publisher + Event catalog
  └─ EventSubsystem / Outbox stores
        ↓
initialize container
  ├─ IAM module
  └─ business modules + ports/adapters
        ↓
initialize integrations
  ├─ OSS / WeChat / QR code
  └─ IAM authz version subscriber
        ↓
initialize transports
  ├─ Generic HTTP/HTTPS + routes
  └─ gRPC server + services/interceptors
        ↓
start background runtimes
  ├─ EventSubsystem
  ├─ cache signal watcher
  └─ scheduler manager
        ↓
register shutdown callback
        ↓
start shutdown manager → HTTP and gRPC concurrently
```

### 5.2 业务模块安装顺序

`Container.Initialize()` 当前顺序：

1. EventSubsystem binding。
2. Survey。
3. ModelCatalog。
4. Actor。
5. Interpretation。
6. Evaluation，并把 Outcome/participant/admin/clinician access 绑定到 Interpretation。
7. Plan。
8. Statistics。
9. Cache warmup governance。
10. Platform。
11. resilience control 与 governance audit recovery runner。

这个顺序包含跨模块依赖，不能随意按目录字母排序。`modules/*/wire.go`/`assemble.go` 构建内部 adapter/service，`install.go` 把稳定 exports 安装回 Container；transport 最后只消费 `BuildRESTDeps/BuildGRPCDeps`。

### 5.3 后台 runtime

- EventSubsystem 拥有 Mongo/MySQL Outbox relay 和已启用的 apiserver local consumer。
- cache subsystem 启动 signal watcher/warmup/repair 等运行能力，关闭时由 Container cleanup 负责。
- scheduler manager 当前可装配 Plan、Statistics nightly sync、Evaluation consistency reconcile runner。
- scheduler 使用独立 cancel context，并向 runtime lifecycle 注册 `stop schedulers`。
- action audit recovery 与 resilience control 在 Container.Initialize 中启动，并由 Container.Cleanup 取消。

业务模块不应在 `Wire()` 内自行启动匿名 goroutine。长跑任务必须返回给 process/container 生命周期，否则无法测试关闭和避免重复启动。

## 6. collection-server 组合根

### 6.1 启动阶段

```text
prepare resources
  └─ Redis profiles/runtime + lock subsystem
        ↓
initialize container
  ├─ HTTP gates / rate limit / cache / report runtime
  └─ IAM module
        ↓
initialize integrations
  ├─ apiserver gRPC manager
  ├─ runtime client bundle
  ├─ Container.Initialize
  └─ IAM authz version subscriber
        ↓
initialize transport
  └─ Generic HTTP/HTTPS + REST routes
        ↓
register shutdown callback
        ↓
start cache subsystem
        ↓
start shutdown manager → serve HTTP
```

关键边界：

- gRPC manager 是 collection 到 apiserver 的唯一业务 RPC composition root，并注入 mTLS/service auth/delegated subject 和 inflight gate。
- Redis 承担 cache/lock/signal/control；业务权威数据仍在 apiserver。
- cache subsystem 在 `preparedServer.Run()` 中、shutdown manager 启动前启动。cache 启动失败会阻止 HTTP serve。
- authz version sync 初始化失败只记录 warning 并继续；collection 使用的 terminal handler 只有日志/指标，不是 apiserver/worker 的持久 Event delivery dead letter。

### 6.2 运行调优与 pprof

collection app 在 process.Run 之前应用：

- `runtime.go-mem-limit` → `debug.SetMemoryLimit`/`GOMEMLIMIT`。
- `runtime.go-gc` → `debug.SetGCPercent`/`GOGC`。

当 profiling enabled 时还启动独立 `127.0.0.1:6060` pprof server。与此同时 Generic server 的默认配置本身也是 health/metrics/profiling enabled；如果 production YAML 未显式覆盖 `server.*`，HTTP 主监听面也可能注册 `/metrics` 和 `/debug/pprof`。

独立 6060 server 没有保存在 process output，也没有 Shutdown hook；正常退出由进程终止回收，而不是受控 drain。

## 7. worker 组合根

### 7.1 启动阶段

```text
prepare resources
  ├─ Redis profiles/runtime + locks
  ├─ optional Mongo registry
  └─ Event catalog
        ↓
initialize container
  ├─ application handlers
  └─ optional attention projection Mongo binding
        ↓
initialize integrations
  ├─ apiserver gRPC manager
  └─ runtime client bundle + Container.Initialize
        ↓
initialize runtime
  ├─ metrics/health/governance server
  ├─ best-effort NSQ topic ensure
  ├─ MySQL delivery dead-letter recorder
  ├─ MySQL retry-event-hold store
  ├─ MQ subscriber + handler registration
  └─ optional retry publisher + hold replayer
        ↓
register shutdown callback
        ↓
start shutdown manager → wait for signal
```

worker 不启动业务 HTTP API。`metrics.enable` 时启动独立 HTTP server；生产配置当前为 `0.0.0.0:9092`。

### 7.2 失败边界

- Event catalog、gRPC manager、metrics bind、dead-letter recorder、hold store、subscriber 或 handler subscription 失败会中止启动。
- NSQ topic ensure 失败只 warning；真正 subscribe 能否成功仍由后续步骤决定。
- Mongo 不能提供 attention projection 时可记录 warning 并禁用该投影，但其它 handler 是否可运行取决于 Container 初始化。
- automatic retry enabled 时创建 retry publisher 失败会中止启动；关闭该开关时不启动 hold replayer。
- delivery attempts 被 hard cap 为 8，hold publish attempts 被 hard cap 为 30；runtime 不会因为 YAML 写更大值而突破代码上限。

## 8. 启动时 fail-fast 与 degraded

| 场景 | 当前行为 | 含义 |
| --- | --- | --- |
| config file 不可读 | 退出 | 无法证明有效配置 |
| option/交叉约束不合法 | 退出 | 已编码的静态约束 fail-fast |
| apiserver MySQL/Mongo init 或 migration 失败 | 退出 | 业务事实源不可用 |
| collection/worker Redis registry connect 失败 | 退出 | 当前这两个进程要求初始 Redis 可连接 |
| Redis 单个 family/profile 运行期不可用 | snapshot degraded/fallback，依 family 能力而定 | 不是全局正确性失败 |
| apiserver MQ publisher 创建失败 | warning，退回环境对应 fallback/logging mode | HTTP 可能上线，但可靠跨进程投递能力需单独确认 |
| apiserver local EventSubsystem start 失败 | 退出 | relay/consumer runtime 未就绪 |
| IAM module 构造失败 | 退出 | 已启用集成无法装配 |
| IAM authz sync subscriber 创建/订阅失败 | warning，继续 | snapshot 仍可 TTL/拉取，但版本失效不实时 |
| OSS/WeChat 已启用且关键 adapter 初始化失败 | 退出 | 避免暴露未装配 handler |
| worker MQ subscriber/dead-letter/hold store 失败 | 退出 | 不在无终态治理的条件下消费 |
| worker metrics server 后续异常退出 | 只记录 error | 不自动停止 consumer |

“进程启动成功”只证明 prepare path 完成，不证明每条业务能力可用。例如 apiserver MQ publisher failure 会允许进程继续，必须结合 EventSubsystem status、Outbox backlog 和 synthetic flow 判断。

## 9. 服务启动与异常退出

apiserver 用 `RunGroup` 并发运行 HTTP 和 gRPC；collection 用同一抽象运行 HTTP。`RunGroup` 的真实语义是：

- 启动所有非 nil service。
- 任一 service 返回 error 时立即把该 error 返回。
- 不 cancel 其它 service。
- 不主动触发 GracefulShutdown callback。

随后顶层 `App.Run()` 在 Execute 返回 error 时调用 `os.Exit(1)`。因此 listener bind 失败、HTTP/gRPC runtime error 与 SIGTERM 不是同一退出路径；前者不能假定会执行全部 Close/Stop。

这也是 runtime 设计中需要区分的两类生命周期：

- operator shutdown：停止接流量、drain、flush、close，预期可控。
- process failure：尽快退出，由 supervisor 重启；只能依靠 durable state 恢复，不能依靠 defer 修复业务事实。

## 10. 当前正常关闭顺序

### 10.1 apiserver

SIGINT/SIGTERM 当前执行：

1. runtime shutdown hooks：停止 scheduler。
2. `Container.Cleanup`：停止 resilience/action audit，关闭 Event/Cache/IAM/modules。
3. 停 IAM authz subscriber。
4. 关闭 DatabaseManager。
5. HTTP `Shutdown`。
6. gRPC `GracefulStop`。

问题是 2–4 发生时 HTTP/gRPC 仍可能接收新请求。目标顺序应是：先 readiness false/摘流量，再停止 listeners 并 drain，接着停 consumer/scheduler，最后关闭下游 client 和存储。

HTTP shutdown 默认最多等待 10 秒；gRPC GracefulStop 没有 timeout/fallback Stop，可能无限等待。当前 shutdown callback 错误只记录并继续后续步骤。

### 10.2 collection-server

1. 先 HTTP shutdown，drain in-flight reliable submit。
2. 关闭 apiserver gRPC manager。
3. 关闭 Redis DatabaseManager。
4. 停 authz sync。
5. 关闭 IAM。
6. cleanup container。

这比 apiserver 更接近“先停入口”，但 Redis 在 authz/container cleanup 前关闭是否安全仍取决于各 Close 不再访问 Redis。独立 `127.0.0.1:6060` pprof 不在该链路。

### 10.3 worker

1. 停 hold replayer 和 MQ subscriber，关闭 publisher/dead-letter/hold store。
2. 关闭 apiserver gRPC manager。
3. 关闭 Redis/Mongo DatabaseManager。
4. 以 5 秒 timeout 关闭 metrics server。
5. cleanup container。

这条顺序先阻止新消息再释放依赖，方向正确。但 `preparedServer.Run()` 还单独创建了第二个 `signal.Notify(SIGINT,SIGTERM)`；同一信号会同时唤醒 Run 和 GracefulShutdown manager，主流程可能在异步 callback 完成前返回。应收敛为一个信号所有者和一个可等待的 shutdown completion。

component-base POSIX manager 在 callbacks 完成后 flush log 并 `os.Exit(0)`。因此正常信号路径也不能依赖 app 层 defer；所有重要 flush/close 必须在 callback 或 manager 中。

## 11. Runtime 与正确性

Runtime 只负责让正确性机制被实际装配，不能自己替代它：

- 加载到 Event catalog 不代表 handler 已注册并成功订阅。
- MQ publisher 创建成功不代表 Outbox 已发送；必须看 durable Outbox 状态。
- Redis family ready 不代表 AnswerSheet/Assessment 数据正确。
- scheduler goroutine 已启动不代表只有一个实例执行；还需 DB claim/lock protocol。
- container 中存在 repository 不代表 handler 使用了同一 transaction context。
- shutdown 日志“complete”不代表所有 gRPC 已在预算内 drain，尤其当前 GracefulStop 无 timeout。

Composition test 应证明“正确 adapter 被注入”；integration test/故障演练再证明事务、网络与生命周期语义。

## 12. 扩展与验收清单

新增 module、client、consumer 或 runner 时：

1. 指定唯一 owner：谁构造、谁启动、谁停止、谁关闭。
2. 放入明确 Stage output，不使用业务 package global。
3. 区分 mandatory、optional 和 degraded dependency，并为每类失败写启动测试。
4. 若 stage 内部分成功后失败，立即回滚本 stage 创建物。
5. 若需要 background goroutine，提供 context cancel/Stop，并注册到 lifecycle。
6. 先停止新流量/新消息，再 drain in-flight，最后关闭 downstream。
7. 为 Close 设置 deadline；不可控的 GracefulStop 要有超时 fallback 和指标。
8. 更新 health/readiness，使其表达真实装配和同步状态，但不要把所有依赖塞进 liveness。
9. 审查配置 schema、默认值、env/flag 映射、secret mask 和外部文件 mount。
10. 用两个实例验证 scheduler/consumer/lease/Outbox，而不是只做单进程 happy path。

验证入口：

```bash
go test ./pkg/app ./pkg/configmask ./pkg/flag \
  ./internal/pkg/options ./internal/pkg/server ./internal/pkg/grpc

go test ./internal/apiserver/process ./internal/apiserver/container/... \
  ./internal/collection-server/process ./internal/collection-server/container \
  ./internal/worker/process ./internal/worker/container
```

生产验收还应包含：

- 用最终 image、working directory、config/secret mounts 启动三个进程。
- 分别注入 DB/Redis/IAM/MQ 不可用，核对 fail-fast/degraded/status。
- 在有 in-flight HTTP、gRPC、MQ handler 时发送 SIGTERM，测量 drain 时间和未完成事实。
- 让 HTTP/gRPC/metrics listener 发生 bind error，确认 supervisor、日志和 durable recovery。
- 检查实际监听端口、metrics/pprof/governance 网络 ACL，以及启动日志是否泄密。
