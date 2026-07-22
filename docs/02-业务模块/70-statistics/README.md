# Statistics 模块

> 状态：**当前唯一架构**。Statistics V1 的实时 Projector、Scanner、Pending Reconcile、Journey/Episode 事实、三段同步和 V1 API 已退役。对外版本仍保持 `/api/v2/statistics`，这是已发布的接口合同，不表示内部仍有两套 Statistics。

## 1. 30 秒结论

Statistics 不在业务请求内实时维护计数器，而是每天以上海时间执行一次可重跑的投影：

```text
权威业务数据
  → 可扩展 Data Collector
  → Access / Assessment / Plan Fact
  → Typed Projection Engine
  → 四类 Daily + Organization Snapshot
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

## 4. 三类业务事实

### 4.1 Access Fact

表达入口打开、Intake 确认、受试者建立、医患关系建立和转移。Entry 与关系日志是权威来源，Statistics 只对其做归一化。

### 4.2 Assessment Fact

按阶段分开记录 AnswerSheet submitted、Assessment created/failed、Outcome committed 和 Report generated/failed。新数据的 Clinician、Entry、Plan、Enrollment 和 Task 归属来自 AnswerSheet 可靠受理时冻结的 `AttributionSnapshot`。

### 4.3 Plan Fact

分开记录 Enrollment joined/closed/terminated 与 Task created/opened/completed/expired/canceled。`PlanEnrollment` 是持久化业务概念，一轮参与是统计履约的最小上下文。

## 5. 物理数据模型

Statistics 拥有九张 canonical 表：

| 分层 | 表 |
| --- | --- |
| Fact | `statistics_access_fact` |
| Fact | `statistics_assessment_fact` |
| Fact | `statistics_plan_fact` |
| Result | `statistics_access_daily` |
| Result | `statistics_assessment_daily` |
| Result | `statistics_plan_activity_daily` |
| Result | `statistics_plan_fulfillment_daily` |
| Result | `statistics_org_snapshot` |
| Run | `statistics_sync_run` |

Fact 保存发生时刻 `DATETIME(3)` 和上海业务日 `DATE`。Daily 未知维度使用技术桶 `0/''`，Fact 中未知业务身份保持 `NULL`。比率不落库，由 Read Service 根据分子和分母计算。

## 6. 运行模型

夜间调度默认上海时间 00:30 启动，按机构串行执行 `publish`：

1. 获取 Redis 分布式租约；
2. 创建 `statistics_sync_run`；
3. 三类 Collector 按 `(occurred_at,id)` 稳定分页采集 Fact；
4. 在单一 MySQL 结果事务内执行五个 Projection；
5. 同一事务将 Run 标记为 `data_committed`；
6. 提交后切换机构缓存 Generation；
7. 预热 `latest_complete_day / 7d / 30d`，标记 `succeeded`。

Redis 锁不可用时失败关闭，不允许两个批次并发重建同一机构。缓存发布失败时 Run 保留在 `data_committed`，运维使用 `resume-cache` 续传，不重跑 Collector 和 Projection。

## 7. 运行与查询接口

### 7.1 对外查询

- `GET /api/v2/statistics/overview`
- `GET /api/v2/statistics/clinicians`
- `GET /api/v2/statistics/clinicians/{id}`
- `GET /api/v2/statistics/clinicians/me/overview`
- `GET /api/v2/statistics/clinicians/me/entries`
- `GET /api/v2/statistics/clinicians/me/testees-summary`
- `GET /api/v2/statistics/entries`
- `GET /api/v2/statistics/entries/{id}`
- `POST /api/v2/statistics/contents/batch`

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
| [80-重构目标架构与完成定义.md](./80-重构目标架构与完成定义.md) | 核对当前架构和验收门槛 |
| [90-设计问题与重构清单.md](./90-设计问题与重构清单.md) | 只跟踪尚未解决的优化项 |

## 9. 源码事实入口

- 应用编排：`internal/apiserver/application/statistics/`
- 领域合同：`internal/apiserver/domain/statistics/`
- Collector/Projection/Store：`internal/apiserver/infra/mysql/statistics/`
- 缓存：`internal/apiserver/cache/statistics/`
- 模块装配：`internal/apiserver/container/modules/statistics/`
- 路由：`internal/apiserver/transport/rest/routes_statistics.go`
- 夜间调度：`internal/apiserver/runtime/scheduler/statistics_sync.go`
- 人工重建：`scripts/oneoff/rebuild_statistics/`
