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
- [`internal/pkg/redisruntime`](../../../internal/pkg/redisruntime)：Redis family、profile、namespace、availability；其中也包含 lock、rank、ops 等非缓存 workload。

Domain 不依赖 Cache。Cache-Aside 位于 Repository decorator、application query service 或其 consumer-owned port；业务模块只看到自己的窄接口。

版本基线：`2026-07-28`。本轮已核对 capability catalog、Policy/Registry、三进程 Redis runtime 装配、Cache subsystem、失效/信令与专题测试；真实 Redis 故障、多实例 L1 收敛和生产命中率仍需运行环境证据。

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
3. 业务 adapter 每次操作从 `PolicyProvider` 解析一次 Policy，不读取 Viper、process Options 或静态副本。
4. Redis key、payload、negative sentinel、TTL 与失效规则是兼容合同；结构重构不能顺带改写。
5. Pub/Sub signal 只做 best-effort 驱逐或预热唤醒，TTL 是最终收敛兜底，可靠一致性不能依赖 signal。

## 3. 当前能力范围

apiserver 登记七个普通 cache capability：

```text
survey.questionnaire
modelcatalog.published_model
evaluation.assessment_detail
evaluation.assessment_list
actor.testee
plan.detail
statistics.query
```

`report_status` 同样出现在 Registry 和三进程配置中，但它的 `kind` 是 `operational_state`，不是普通 Cache-Aside。collection-server 另有静态 L1 capability：`catalog.questionnaire` 与 `catalog.typology`。

IAM/JWKS/ProfileLink、WeChat SDK token 等私有缓存继续由各自 integration owner 维护，不纳入上述业务 Registry。

## 4. 事实源

本文档集的事实优先级为：

1. 上述 package 的源码和测试；
2. [`configs/apiserver.prod.yaml`](../../../configs/apiserver.prod.yaml)、[`configs/collection-server.prod.yaml`](../../../configs/collection-server.prod.yaml)、[`configs/worker.prod.yaml`](../../../configs/worker.prod.yaml)；
3. [`api/rest/apiserver.yaml`](../../../api/rest/apiserver.yaml) 的治理接口；
4. 本目录的说明。

旧的分散文档和重构计划不再属于 active truth layer。历史决策从 Git 追溯，不在现行目录继续维护“目标态”和“实施态”两套说法。

## 5. 快速验证

```bash
go test ./internal/pkg/cache/... ./internal/pkg/redisruntime/...
go test ./internal/apiserver/cache/... ./internal/collection-server/cache
go test ./internal/pkg/configcontract ./internal/pkg/architecture
make docs-hygiene
```
