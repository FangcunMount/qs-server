# Cache 读侧治理

qs-server 的 Cache 不是 Redis 工具箱，而是以 **canonical capability** 为治理单位的读侧保护层。数据库和业务读模型始终是事实源；Cache 负责缩短热点读取路径、合并并发回源、控制失效与预热，并向运维侧暴露可解释的运行证据。

本专题沿一条读取因果链组织：

```text
权威事实
  → 可缓存派生值与 identity
  → L1/L2 命中或 miss
  → singleflight / LoadGuard 控制回源放大
  → delete / version / generation 失效
  → Signal 加速、TTL 最终收敛
  → warmup 与 capability 证据
```

如果还没有读过根级 [统一模型与推理方法](../04-统一模型与推理方法.md)，建议先读。Cache 是其中“持久事实 → 派生读取 → 可丢协调 → 证据”的展开。

当前实现已经收敛为四个边界：

- [`internal/pkg/cache`](../../../internal/pkg/cache)：与业务无关的 L1、Redis payload、object/query kernel、Policy 与 Registry；
- [`internal/apiserver/cache`](../../../internal/apiserver/cache)：apiserver 的业务 capability、adapter、治理和 subsystem；
- [`internal/collection-server/cache`](../../../internal/collection-server/cache)：collection-server 的目录 L1、信令监听、启动预热和生命周期；
- [`internal/pkg/redisruntime`](../../../internal/pkg/redisruntime)：Redis family、profile、namespace、availability；
  其中也包含 lock、rank、ops 等非缓存 workload。

Domain 不依赖 Cache。Cache-Aside 位于 Repository decorator、application query service 或其 consumer-owned port；业务模块只看到自己的窄接口。

逐篇源码基线和复核状态由 [`document-closure.json`](../../document-closure.json) 维护。
稳定文档只声明 capability catalog、Policy/Registry、Redis runtime、Cache subsystem 与失效/信令契约；真实 Redis 故障、多实例 L1 收敛和命中率必须单列环境证据。

## 1. 阅读路径

| 顺序 | 文档 | 回答的问题 |
| --- | --- | --- |
| 1 | [10-架构与责任边界.md](10-架构与责任边界.md) | Cache 在系统里保护什么，package 和业务 owner 如何分工 |
| 2 | [15-从事实到派生读取-一致性与容量模型.md](15-从事实到派生读取-一致性与容量模型.md) | 从事实、identity、回源放大和新鲜度推导 Cache-Aside、singleflight、失效、Signal 与预热 |
| 3 | [20-Capability-Registry与配置.md](20-Capability-Registry与配置.md) | 有哪些 capability，最终 Policy 从哪里来，如何 reload |
| 4 | [30-缓存内核与读写链路.md](30-缓存内核与读写链路.md) | L1、L2、object、query、loadguard 如何工作 |
| 5 | [40-一致性失效与降级.md](40-一致性失效与降级.md) | 写后如何失效，信令丢失或 Redis 异常时如何收敛 |
| 6 | [50-预热与运行时治理.md](50-预热与运行时治理.md) | startup、publish、manual、repair warmup 如何执行和判定成功 |
| 7 | [60-可观测性与运营页面.md](60-可观测性与运营页面.md) | 指标怎样投影到 canonical capability，后台页面应如何判读 |
| 8 | [70-扩展与验收.md](70-扩展与验收.md) | 新增能力要补哪些合同，如何验证没有破坏当前架构边界 |

前两篇建立问题分类和推理主轴；20–60 是当前实现事实；70 要求新 capability 重新说明事实源、stale SLO、回源容量和验证证据，而不是只复制一个 Redis key。

## 2. 五条不变式

1. 数据库、读模型和业务服务是事实源，Cache 不是第二份业务事实。
2. 一个 apiserver capability 只有一个 `Spec`、一个配置 source 和一个 effective Policy。
3. 普通 object/query read-through 每次只从 `PolicyProvider` 解析一次 Policy，不读取 Viper 或 process Options；
   published-model L1+L2 当前会在外层 enabled、L2 read-through 和 L1 Set 多次 Resolve，且 L1 jitter 保留启动时副本，这是已记录的例外而非应被扩张的契约。
4. Redis key、payload、negative sentinel、TTL 与失效规则是兼容合同；结构重构不能顺带改写。
5. Pub/Sub signal 只做 best-effort 驱逐或预热唤醒，TTL 是最终收敛兜底，可靠一致性不能依赖 signal。

## 3. 当前能力范围

apiserver 登记七个普通 cache capability：

```text
survey.questionnaire
modelcatalog.published_model
evaluation.assessment_access
evaluation.assessment_detail
actor.testee
plan.detail
statistics.query
```

`evaluation.assessment_list` 的历史实现未接入实际读取路径，已连同 version bump 和配置入口退役；assessment list 继续直接读取既有 read model。
`report_status` 同样出现在 Registry 和三进程配置中，但它的 `kind` 是 `operational_state`，不是普通 Cache-Aside。
collection-server 另有独立 L1 capability：
`catalog.questionnaire`、`catalog.published_model`、`catalog.typology`、`evaluation.assessment_access` 与 `evaluation.assessment_detail`。

`modelcatalog.published_model` 当前是有意收窄的 L1+L2：immutable exact-by-ref 运行快照，
以及 key 包含全局 catalog version 的 list/by-questionnaire 读取进入 apiserver L1，以复用已经解码的 `DefinitionV2`。发布后 bump 全局版本，旧目录 L1 key 会跨实例变为不可达；
latest-by-code、algorithms 等非版本化可变目录仍只使用 L2，Active admission 仍绕过缓存读取 Mongo。它的 L1 TTL 在 Set 时动态重取，jitter 则在构造时固定；
status 的 effective jitter 不证明 L1 已热生效该值。

IAM/JWKS/ProfileLink、WeChat SDK token 等私有缓存继续由各自 integration owner 维护，不纳入上述业务 Registry。

## 4. 事实源

本文档集的事实优先级为：

1. 上述 package 的源码和测试；
2. [`configs/cache/apiserver.prod.yaml`](../../../configs/cache/apiserver.prod.yaml)、[`configs/cache/collection-server.prod.yaml`](../../../configs/cache/collection-server.prod.yaml)
   与三进程主配置中的 `cache.policy_file` / `runtime_state.report_status`；
3. [`api/rest/apiserver.yaml`](../../../api/rest/apiserver.yaml) 的治理接口；
4. 本目录的说明。

旧的分散文档和重构计划不再属于 active truth layer。历史决策从 Git 追溯，不在现行目录继续维护“目标态”和“实施态”两套说法。

## 5. 快速验证

```bash
go test -count=1 ./internal/pkg/cache/... ./internal/pkg/redisruntime/...
go test -count=1 ./internal/apiserver/cache/... ./internal/collection-server/cache
go test -count=1 ./internal/pkg/configcontract ./internal/pkg/architecture
make docs-hygiene
```
