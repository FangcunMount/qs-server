# Statistics 模块

> 状态：**V2 核心代码已落地，V1/V2 并存迁移中**。V2 已具备 PlanEnrollment、AttributionSnapshot、三类 Fact、五个 Projection、SyncRun、缓存与查询接口；V1 的 `BehaviorFootprint`、`AssessmentEpisode`、旧 Daily 和查询接口仍保留用于生产回滚。历史回填、连续七天影子对账、切流和 V1 退役尚未执行。阅读时必须区分“代码能力已具备”和“生产迁移已验收”。

## 1. 30 秒结论

Statistics 解决的不是“对业务表做几次 `COUNT(*)`”，而是：

> 在不夺取 Actor、Survey、Evaluation、Interpretation 和 Plan 业务真值所有权的前提下，由可扩展 Data Collector 把权威业务数据构建为稳定事实，再由 Typed Projection Engine 以唯一口径产生可重建、可解释的统计结果。

重建后的模块采用三层模型：

1. **业务数据层**：各业务模块拥有的权威状态与持久事实；
2. **统计事实层**：Access、Assessment、Plan 三类标准化事实；
3. **统计结果层**：四类日聚合与一个机构快照。

Statistics V2 不追求实时投影。业务已经确认：前一完整自然日的数据足以满足当前统计场景。因此第一版采用上海时间下的 **T+1 批量采集、确定性投影和最终一致查询**。

```mermaid
flowchart LR
    B["权威业务数据"]
    C["可扩展 Data Collectors"]
    F["标准事实层\nAccess / Assessment / Plan"]
    E["Typed Projection Engine"]
    R["统计结果层\nDaily / Fulfillment / Snapshot"]
    Q["Statistics Read Service"]

    B --> C
    C --> F
    F --> E
    E --> R
    R --> Q
    B -.->|当前资源状态| E
```

## 2. 为什么需要重建

### 2.1 当前实现的真实问题

当前 Statistics 已经提供机构总览、医生、入口、内容、Plan 活动和履约查询，但工程实现经过多轮叠加后形成了多套统计方式：

- 接入行为来自 Resolve/Intake 日志和 `BehaviorFootprint`；
- 测评服务过程由 `AssessmentEpisode` 拼装；
- 日趋势写入 `statistics_journey_daily`；
- Plan 活动写入 `statistics_plan_daily`；
- Plan 履约又实时扫描 `assessment_task`；
- 机构总览同时读取 Snapshot、Daily 和业务表；
- 实时事件投影、Scanner、夜间同步和一次性脚本存在不同入口；
- Checkpoint、Pending、窗口重算、缓存版本和预热共同构成恢复链路。

这些机制分别有合理起因，但整体已经超过当前业务体量真正需要的复杂度。最突出的问题不是某一个 SQL 写错，而是：

> 同一个指标可能经历不同的事实采集、投影、补偿和查询方式，工程人员难以证明它们最终会得到相同结果。

### 2.2 当前业务不需要实时统计

Statistics 不参与答卷可靠受理、测评执行或报告提交。业务主链路是否成功，不能由统计是否及时更新决定。

当前统计使用场景主要是：

- 运营查看机构前一日及一段时间的服务量；
- 医生或管理者查看入口、测评和 Plan 履约趋势；
- 通过日期窗口了解变化，而不是秒级监控业务状态；
- 在数据异常后能够重跑、对账和解释差异。

因此，T+1 比实时投影更符合奥卡姆剃刀原则：它牺牲当前不需要的秒级新鲜度，换取统一事实、统一重建和更容易验证的一致性模型。

### 2.3 事实层仍然不可省略

虽然不再实时投影，但不能直接让所有统计 SQL 跨 MongoDB、MySQL 和多个业务模块随意聚合。事实层仍然承担三个价值：

1. 统一不同业务来源的身份、时间和维度；
2. 冻结历史归属，避免重建时被当前 Actor、Entry 或 Plan 状态污染；
3. 为日聚合提供稳定、可重复扫描的输入。

所以最终方案不是“删除 Statistics，直接查业务表”，而是“保留必要的标准化事实，删除不必要的实时投影运行时”。

## 3. 模块负责什么

| 职责 | Statistics V2 保护的语义 |
| --- | --- |
| 数据采集 | 由可扩展 Data Collector 将 Access、Assessment、Plan 的异构数据转成三类稳定 Fact |
| 统一投影 | 由 Typed Projection Engine 显式执行五个 Projection，一张结果表只有一个计算入口 |
| 历史归属 | 使用受理时冻结的医生、入口、Plan、Task 等上下文统计新数据 |
| 日统计 | 按上海自然日构建接入、测评、Plan 活动和履约结果 |
| 资源快照 | 保存机构最近一次资源规模和累计量快照 |
| T+1 同步 | 按机构执行事实补采、结果重建、缓存切换与运行记录 |
| 幂等与补偿 | 重跑相同窗口不重复计数；失败后可以从业务源重新执行 |
| 查询编排 | 组合 Daily、Snapshot 和必要的当前业务读模型形成 API |
| 新鲜度表达 | 返回 `as_of_date`、`snapshot_at`，让调用方知道结果完整到哪里 |
| 查询保护 | 使用机构级缓存 Generation、L1/L2 缓存、限流与降级保护查询侧 |

## 4. 模块不负责什么

| 问题 | 权威模块 | Statistics 的边界 |
| --- | --- | --- |
| 患者、家长、医生和关系是否有效 | Actor / IAM | 读取身份和授权结果，不修改关系 |
| 问卷与答案是否合法 | Survey | 只观察最终 AnswerSheet，不重新校验作答 |
| 测评模型如何计算和判定 | ModelCatalog / Calculation / Evaluation | 只观察模型身份和执行结果 |
| 报告内容和 Audience | Interpretation | 只记录报告生成/失败事实，不保存正文 |
| Plan 周期和 Task 状态迁移 | Plan | 读取 Enrollment/Task 事实，不推进状态 |
| 医学诊断 | 医生及外部医疗业务 | 统计结果只能提供运营和治疗观察辅助信息 |
| 自由取数和通用数仓 | 专门 BI / 数据平台 | 只实现 qs-server 内稳定、明确口径的查询 |

边界原则是：

> Statistics 可以重建“怎样观察业务”，不能重新决定“业务事实上是什么”。

## 5. 已确认的十条设计原则

1. 业务模块拥有权威数据，Statistics 只拥有派生事实与查询结果。
2. 事实层按 Access、Assessment、Plan 拆分，不按页面或接口拆表。
3. 结果层按业务语义拆分，Plan Activity 与 Fulfillment 必须分开。
4. Data Collector 是可扩展应用组件，不是独立数据层；新数据源通过强类型 Collector 和显式装配接入。
5. Typed Projection Engine 只管执行策略，具体 Projection 拥有统计口径，不引入动态 DSL 和 Metric Catalog。
6. 第一版采用 T+1 批量采集，不建设实时 Projector。
7. 上海时间是唯一业务日边界，所有 Daily 使用半开区间。
8. 新 Assessment 在 AnswerSheet 可靠受理时冻结归属；历史数据允许尽力推导。
9. Plan 正式引入持久化 `PlanEnrollment`，一条 Enrollment 表示一轮参与。
10. 同步必须可重跑、可对账、可判断停在哪一阶段，缓存失败不能回滚 MySQL 结果。

## 6. 九张目标表

| 层 | 表 | 粒度 | 主要职责 |
| --- | --- | --- | --- |
| Fact | `statistics_access_fact` | 一次接入事实 | Entry 打开、Intake、建档、建立/转移关系 |
| Fact | `statistics_assessment_fact` | 一个测评交付阶段事实 | AnswerSheet、Assessment、Outcome、Report 生命周期 |
| Fact | `statistics_plan_fact` | 一个 Enrollment/Task 生命周期事实 | 加入、关闭、终止及 Task 活动 |
| Result | `statistics_access_daily` | 机构 × 日期 × 医生 × Entry | 接入漏斗与趋势 |
| Result | `statistics_assessment_daily` | 机构 × 日期 ×归属 × 内容 | 测评交付与内容统计 |
| Result | `statistics_plan_activity_daily` | 机构 × 日期 × Plan | 事件发生量 |
| Result | `statistics_plan_fulfillment_daily` | 机构 × Cohort 日期 × Plan | 应履约、按时、逾期 |
| Result | `statistics_org_snapshot` | 每机构一行 | 当前资源和累计量快照 |
| Runtime | `statistics_sync_run` | 每机构每批次一行 | 运行阶段、范围、计数、错误和新鲜度 |

第一版明确不增加：

- Statistics Checkpoint；
- Statistics Pending；
- Statistics Dead Letter；
- 实时投影 Generation；
- 独立事实数据库实例；
- 通用指标 DSL 或动态 Cube。

## 7. 两个前置领域改造

### 7.1 Assessment AttributionSnapshot

当前事件并不稳定携带医生、Entry 等全部归属。如果 Statistics 在夜间根据当前关系推导，新数据的历史归属仍会漂移。因此需要在 AnswerSheet Admission 中持久化受理时快照：

```text
origin_type / origin_id
clinician_id / entry_id
plan_id / enrollment_id / task_id
captured_at / version
```

新数据必须使用 `frozen` 快照；历史数据允许标记为 `derived_legacy` 或 `unknown`。

### 7.2 持久化 PlanEnrollment

当前代码中的 `PlanEnrollment` 是领域服务，参与关系由 Task 集合隐式表达。目标设计将其提升为持久化实体：

```text
一个患者 + 一个 Plan + 一轮参与
```

状态为 `active / closed / terminated`，Task 增加 `enrollment_id`。Statistics 不应在 Plan 领域完成该改造前伪造 Enrollment 事实。

## 8. 核心查询能力

| 查询族 | V2 主要来源 | 说明 |
| --- | --- | --- |
| Organization Overview | Org Snapshot + 四类 Daily | 返回 `as_of_date` 与 `snapshot_at` |
| Access Funnel | Access Daily | 支持机构、医生和 Entry 过滤 |
| Assessment Service | Assessment Daily | 分开 AnswerSheet、Assessment、Outcome、Report 阶段 |
| Clinician Statistics | Actor 当前状态 + Daily | 当前资源归 Actor，窗口指标归 Statistics |
| Entry Statistics | Entry 当前状态 + Daily | Entry 元信息不复制进 Daily |
| Content Batch | Assessment Daily | Questionnaire 与 Model kind/code 正交 |
| Plan Activity | Plan Activity Daily | 按事件发生日期统计 |
| Plan Fulfillment | Plan Fulfillment Daily | 按 planned/due cohort 统计 |
| Testee Periodic | PlanEnrollment + Task 业务查询 | 患者任务明细不强行物化成统计聚合 |

## 9. 当前实现与目标设计对照

| 能力 | 当前实现 | V2 目标 |
| --- | --- | --- |
| 接入事实 | Resolve/Intake Log + BehaviorFootprint | Access Fact |
| 测评过程 | BehaviorFootprint + AssessmentEpisode | Assessment Fact |
| Plan 参与 | 由 Task 集合推导 | 持久化 PlanEnrollment + Plan Fact |
| 日聚合 | JourneyDaily / PlanDaily / 部分实时 SQL | 四类职责单一 Daily |
| 履约 | 实时扫描 `assessment_task` | 每夜全量重建 Fulfillment Daily |
| 机构概览 | Snapshot + 实时累计 + Daily | 完整 Org Snapshot + Daily |
| 数据采集 | Projector、Scanner、Checkpoint、Pending | 可扩展 Data Collector + T+1 幂等重跑 |
| 统计计算 | 综合 RebuildWriter 与实时 SQL | Typed Projection Engine + 五个显式 Projection |
| 运行记录 | 日志和部分指标 | 持久化 SyncRun |
| 时间语义 | `time.Local` 与隐式 `DATE()` | 显式 `Asia/Shanghai` |
| `today` | 混合实时与日聚合 | V2 不承诺实时 today |

## 10. 文档地图

| 顺序 | 文档 | 回答的问题 |
| --- | --- | --- |
| 00 | 本文 | 为什么重建、最终边界和阅读路线是什么 |
| 10 | [领域模型](./10-领域模型.md) | Fact、Daily、Snapshot、SyncRun 分别是什么 |
| 20 | [业务数据、事实与统计分层](./20-核心设计-业务数据、事实与统计分层.md) | 三层怎样映射到九张表及业务所有权 |
| 21 | [数据采集、幂等与补偿](./21-核心设计-数据采集、幂等与补偿.md) | Data Collector 如何扩展，T+1 怎样采集、补采和重跑 |
| 22 | [Projection Engine、同步与最终一致性](./22-核心设计-Projection-Engine、同步与最终一致性.md) | 投影、批次、事务、缓存和失败恢复怎样闭环 |
| 30 | [从业务数据到统计查询](./30-关键链路-从业务数据到统计查询.md) | 三条业务链怎样进入 Fact、Daily 和 API |
| 40 | [统计指标与口径](./40-统计指标与口径.md) | 指标定义、时间归属、分母和来源是什么 |
| 80 | [重构目标架构与完成定义](./80-重构目标架构与完成定义.md) | Statistics V2 最终必须长什么样、满足什么条件才算重构完成 |
| 90 | [设计问题与重构清单](./90-设计问题与重构清单.md) | 如何从当前实现迁移到 V2 |

推荐阅读顺序：

```text
README
  -> 10 领域模型
  -> 20 三层数据
  -> 21 可扩展 Data Collector
  -> 22 Projection Engine 与同步
  -> 30 完整链路
  -> 40 指标词典
  -> 80 目标架构与完成定义
  -> 90 实施路线图
```

## 11. 与其他模块的关系

- Survey 的最终作答事实见 [Survey 文档](../10-survey/README.md)；
- 模型身份和发布版本见 [ModelCatalog 文档](../20-model-catalog/README.md)；
- Assessment、Outcome 和执行状态见 [Evaluation 文档](../30-evaluation/README.md)；
- Report 成品和查询授权见 [Interpretation 文档](../40-interpretation/README.md)；
- 医生、患者和 Entry 见 [Actor 文档](../50-actor/README.md)；
- Enrollment、Task 和履约业务真值见 [Plan 文档](../60-plan/README.md)；
- 缓存实现细节见 [Cache 基础设施文档](../../03-基础设施/cache/README.md)。

## 12. 事实来源与验证

当前代码事实入口：

- [`domain/statistics`](../../../internal/apiserver/domain/statistics/)
- [`application/statistics`](../../../internal/apiserver/application/statistics/)
- [`infra/mysql/statistics`](../../../internal/apiserver/infra/mysql/statistics/)
- [`runtime/scheduler/statistics_sync.go`](../../../internal/apiserver/runtime/scheduler/statistics_sync.go)
- [`domain/plan/plan_enrollment.go`](../../../internal/apiserver/domain/plan/plan_enrollment.go)
- [`configs/events.yaml`](../../../configs/events.yaml)
- [`internal/pkg/migration/migrations/mysql`](../../../internal/pkg/migration/migrations/mysql/)

文档验证：

```bash
make docs-hygiene
make docs-facts
git diff --check
```

这些命令只能验证结构和关键名称，不能证明生产迁移已经完成。V2 条目只有在代码、migration、测试、历史回填、影子对账和运行证据全部完成后，才能标记为“生产已验收”。
