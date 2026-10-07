# Statistics 模块

> 状态：**当前唯一架构**。Statistics V1 的实时 Projector、Scanner、Pending Reconcile、Journey/Episode 事实、三段同步和 V1 API 已退役。
> 对外版本仍保持 `/api/v2/statistics`，这是已发布的接口合同，不表示内部仍有两套 Statistics。

## 1. 30 秒结论

门店纯统计查询检查运营统计 read 权限、有效 Operator 与门店 Scope，服务概览、临床人员和计划执行按当前归属统计；详见 [门店专题分析](./40-统计指标与口径.md#20-门店专题分析)。后台专业统计查询在访问缓存或数据库前始终检查测评 statistics 权限。缓存命中不能绕过授权；原有组织范围与统计发布时间校验继续生效。

Statistics 不在业务请求内实时维护计数器，而是每天以上海时间执行一次可重跑的投影：

```text
权威业务数据
  → 可扩展 Data Collector
  → Access / Assessment / Plan / StoreActivity Fact
  → Typed Projection Engine
  → 五类 Daily + Organization Snapshot
  → Read Service / API / Generation-aware Cache
```

业务已确认“看到前一完整自然日”足以支撑当前决策，因此本模块优先保护：

- 指标口径唯一；
- 来源可追溯；
- 投影可确定性重建；
- 失败可判断停点；
- 重跑不重复增长；
- 查询明确告知数据新鲜度。

## 2. 模块负责什么

| 责任 | 保护的语义 |
| --- | --- |
| Data Collector | 把多个业务源的生命周期动作映射成标准 Fact |
| Fact Store | 以稳定 `fact_key` 幂等接收，核心字段冲突时失败关闭 |
| Projection Engine | 按强类型 Projection 编排结果重建，每张结果表只有一个写入者 |
| SyncRun | 持久化 validate/repair/publish 的阶段、计数、错误和缓存发布状态 |
| Read Service | 只读统计结果和必要的当前资源读模型，不在线扫描 MongoDB |
| Query Cache | 按机构 Generation 切换整批结果，Redis 故障时提供可解释的 L1 stale 或受限回源 |

## 3. 模块不负责什么

- 不拥有 Actor、Survey、Evaluation、Interpretation 和 Plan 的业务事实；
- 不从 `updated_at` 猜测“何时加入、终止、完成或失败”；
- 不将患者周期明细伪装成统计聚合，该视图由 Plan Enrollment API 提供；
- 不保证实时 `today`；
- 不建设 Metric DSL、独立统计服务、持久化扫描 Checkpoint 或通用事实大表。

## 4. 四类业务事实

### 4.1 Access Fact

表达入口打开、Intake 确认、受试者建立、医患关系建立和转移。Entry 与关系日志是权威来源，Statistics 只对其做归一化。

### 4.2 Assessment Fact

按阶段分开记录 AnswerSheet submitted、Assessment created/failed、Outcome committed 和 Report generated/failed。
新数据的 Clinician、Entry、Plan、Enrollment 和 Task 归属来自 AnswerSheet 可靠受理时冻结的 `AttributionSnapshot`。

### 4.3 Plan Fact

分开记录 Enrollment joined/closed/terminated 与 Task created/opened/completed/expired/canceled。Task 活动 Fact 保持“每个 Task、每类生命周期最多一次”；
履约另使用 revision-scoped 的 `task_schedule_defined` 和 `task_schedule_terminal`，避免 Plan 恢复后旧 canceled Fact 永久排除同一 Task。
`PlanEnrollment` 是持久化业务概念，一轮参与是统计履约的最小上下文。

### 4.4 StoreActivity Fact

独立记录答卷提交和首次测评成功的历史开展量。Survey AnsweringStart 先冻结开展门店、归属版本与作答意图；Collector 读取答卷/Assessment 中的这份冻结上下文，
不使用受试者当前门店倒推历史。未归属开始与旧客户端未捕获开始分别进入明确 unknown reason，不能回填成公司或当前门店。

## 5. 物理数据模型

Statistics 拥有十一张 canonical 表：四张 Fact、五张 Daily、一张 Organization Snapshot 和一张 SyncRun。

| 分层 | 表 |
| --- | --- |
| Fact | `statistics_access_fact` |
| Fact | `statistics_assessment_fact` |
| Fact | `statistics_plan_fact` |
| Fact | `statistics_store_activity_fact` |
| Result | `statistics_access_daily` |
| Result | `statistics_assessment_daily` |
| Result | `statistics_plan_activity_daily` |
| Result | `statistics_plan_fulfillment_daily` |
| Result | `statistics_store_activity_daily` |
| Result | `statistics_org_snapshot` |
| Run | `statistics_sync_run` |

前三类 Fact 保存 `DATETIME(3)`，StoreActivity 保存 `DATETIME(6)`；均保存上海业务日 `DATE`。Daily 未知维度使用技术桶 `0/''`，Fact 中未知业务身份保持 `NULL`。比率不落库，由 Read Service 根据分子和分母计算。

## 6. 运行模型

夜间调度默认上海时间 00:30 启动，按机构串行执行 `publish`：

1. 获取 Redis 分布式租约；
2. 创建 `statistics_sync_run`；
3. 四类 Collector 按来源时间与稳定身份采集；StoreActivity 的 Mongo cursor 和 MySQL keyset 各遵循来源合同；
4. 在单一 MySQL 结果事务内执行四个 Daily Engine Projection 和两个 Global Engine Projection；
5. 同一事务将 Run 标记为 `data_committed`；
6. 提交后切换机构缓存 Generation；
7. 预热 `latest_complete_day / 7d / 30d`，标记 `succeeded`。

Redis 锁不可用时失败关闭，不允许两个批次并发重建同一机构。缓存发布失败时 Run 保留在 `data_committed`，运维使用 `resume-cache` 续传，不重跑 Collector 和 Projection。

## 7. 运行与查询接口

### 7.1 对外查询

- `GET /api/v2/statistics/overview`
- `GET /api/v2/statistics/clinicians`
- `GET /api/v2/statistics/clinicians/{id}`
- `GET /api/v2/statistics/entries`
- `GET /api/v2/statistics/entries/{id}`
- `POST /api/v2/statistics/contents/batch`

运营门店查询使用 `/api/v2/statistics/operations/overview`、`operations/stores` 和 `operations/analysis/{overview,clinicians,entries}`，
按运营统计 read 与 action/StoreScope 校验。专业接口继续按各自管理员/测评统计权限保护。`/statistics/clinicians/me` 及其所有子路径已退役，返回 410。

每个响应包含 `freshness.as_of_date / snapshot_at / is_stale`。没有成功 publish 时返回 `statistics_not_ready`，不伪造零值。

### 7.2 内部运行

- `POST /internal/v2/statistics/runs`
- `GET /internal/v2/statistics/runs`
- `GET /internal/v2/statistics/runs/{id}`
- `POST /internal/v2/statistics/runs/{id}/resume-cache`

`validate` 只读取、映射、校验和计数；`repair` 重建指定窗口但不发布新水位；`publish` 完成 Snapshot 与缓存代际切换。

## 8. 文档地图

| 文档 | 阅读目的 |
| --- | --- |
| [10-领域模型.md](./10-领域模型.md) | 理解 Fact、Daily、Snapshot、SyncRun 及不变量 |
| [20-核心设计-业务数据、事实与统计分层.md](./20-核心设计-业务数据、事实与统计分层.md) | 理解三层数据所有权 |
| [21-核心设计-数据采集、幂等与补偿.md](./21-核心设计-数据采集、幂等与补偿.md) | 理解 Collector 扩展点和 FactKey |
| [22-核心设计-Projection-Engine、同步与最终一致性.md](./22-核心设计-Projection-Engine、同步与最终一致性.md) | 理解批次事务和失败恢复 |
| [30-关键链路-从业务数据到统计查询.md](./30-关键链路-从业务数据到统计查询.md) | 从来源追踪到 API |
| [40-统计指标与口径.md](./40-统计指标与口径.md) | 查阅指标定义、维度和分母 |
| [90-设计问题与重构清单.md](./90-设计问题与重构清单.md) | 当前风险、已实现证据与待完成的运行验收 |

旧的 V2-only 重构目标文档已在目标架构落实后退出 active 层。当前数据完成定义由 20 维护，Collector 规则由 21 维护，Projection/缓存/恢复由 22 维护，代码、运行与生产验收门槛统一由 90 维护；不得再从迁移期检查表反推当前完成状态。

独立开始事实、提交传递和 StoreActivity 已实现。业务开始/提交契约归 [Survey](../10-survey/README.md)，历史开展 Fact 的口径在本模块10/20维护；
[ST-011](./90-设计问题与重构清单.md#15-st-011作答开始记录开展门店与客户端兼容)只跟踪客户端兼容、迁移/发布与运行证据。代码接线不证明生产已部署或指标已发布。

## 9. 源码事实入口

- 应用编排：`internal/apiserver/application/statistics/`
- 领域合同：`internal/apiserver/domain/statistics/`
- Collector/Projection/Store：`internal/apiserver/infra/mysql/statistics/`
- 缓存：`internal/apiserver/cache/statistics/`
- 模块装配：`internal/apiserver/container/modules/statistics/`
- 路由：`internal/apiserver/transport/rest/routes_statistics.go`
- 夜间调度：`internal/apiserver/runtime/scheduler/statistics_sync.go`
- 人工重建：`scripts/oneoff/rebuild_statistics/`
