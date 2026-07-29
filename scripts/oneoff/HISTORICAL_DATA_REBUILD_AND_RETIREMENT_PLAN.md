# P. 代码分析报告：Testee 历史数据重建与回填能力退场计划

> 状态：规划改造
>
> 适用仓库：`iam`、`qs-server`、`seeddata-runner`
>
> 重建日期：`2025-01-01..2026-07-27`（Asia/Shanghai，含首尾）
>
> 数据边界：删除 Testee 主数据及其业务事实，保留医护人员、入口、计划和内容主数据。

## 1. 修订结论

本计划不再使用“按批次枚举每个资源 + 持久化 rollback operation + receipt + foreign-reference coordinator”的数据清理方案。

新方案是一次受控的 **Testee 事实域整体重置**：

1. 停止会写入 Testee 事实的进程。
2. 备份 IAM MySQL、QS MySQL 和 QS MongoDB。
3. 保存 QS `staff.user_id` 和 `testee.profile_id` 清单。
4. 在 QS MySQL 中按固定表顺序删除 Testee 主数据、业务事实、事件控制和 Statistics 投影。
5. 在 MongoDB 中对 Testee 事实 collections 执行 `deleteMany({})`，不 drop collection，保留索引。
6. 在 IAM MySQL 中删除已删 Testee 对应的 Profile 和 ProfileLink，默认不删 User/LoginIdentity/Credential。
7. 验证医护人员、Assessment Entry、Assessment Plan、Questionnaire、Model、Norm 和 Report Template 仍然完整。
8. 使用新 seeddata batch 重建 573 天数据。
9. 完成 Statistics 修复和 K6 验收后，退场一次性历史回填能力。

数据清理仅需三份经 DBA 复核的数据库脚本：

```text
01-reset-testee-facts-qs-mysql.sql
02-reset-testee-facts-qs-mongo.js
03-reset-testee-profiles-iam-mysql.sql
```

不新增生产删除 API，不新增数据清理账本表，不使用应用服务逐条删除。

## 2. 分析目标

- 明确哪些表/collections 属于 Testee 事实域。
- 明确哪些表是医护人员及业务配置主数据，不参与删除。
- 将旧历史数据整体重置，避免新回填与旧事实叠加。
- 将数据清理与“回填代码退场”分成两个独立发布阶段。

## 3. 分析范围

### 3.1 包含

- qs-server MySQL 的 Testee、Entry activity、Plan enrollment/task、Assessment/Outcome、Statistics 和事件控制数据。
- qs-server MongoDB 的 AnswerSheet、Report、Interpretation lifecycle 和 Mongo outbox。
- IAM MySQL 中与 QS Testee Profile ID 相关的 `profiles` / `profile_links`。
- seeddata-runner 重建和历史能力退场。

### 3.2 不包含

- 不删 IAM User、LoginIdentity、Credential 和 Token 数据。
- 不删 qs-server 的 `staff`、`clinician`、`assessment_entry`、`assessment_plan`。
- 不删 Questionnaire、Assessment Model、Norm、Report Template 等内容数据。
- 不通过 REST/gRPC 逐条删除。
- 不在本阶段 drop 任何业务表或 Mongo collection。

## 4. 入口与调用路径

当前 Testee 事实写入链是：

```text
IAM User/LoginIdentity
  -> IAM Profile/ProfileLink
  -> QS Testee
  -> Entry resolve/intake + ClinicianRelation
  -> PlanEnrollment/AssessmentTask
  -> Mongo AnswerSheet + Mongo Outbox
  -> MySQL Assessment + MySQL Outbox
  -> EvaluationOutcome
  -> Mongo Report lifecycle/artifact/catalog
  -> Statistics facts/daily/snapshot
```

删除顺序必须是反向依赖顺序，最后才删 `testee` 和 IAM `profiles`。

## 5. 当前职责与依赖

### 5.1 医护人员主数据

[`internal/apiserver/infra/mysql/actor/po.go`](../../internal/apiserver/infra/mysql/actor/po.go) 明确定义：

- `staff`：后台操作者，持有 IAM `user_id`。
- `clinician`：业务从业者，可通过 `operator_id` 关联 staff。
- `assessment_entry`：医护人员创建的扫码/测评入口。
- `clinician_relation`：医护人员与 Testee 的关系事实，不是医护人员主数据。

因此 `staff`、`clinician`、`assessment_entry` 保留，`clinician_relation` 随 Testee 删除。

### 5.2 Entry activity

- `assessment_entry_resolve_log` 记录入口扫描/解析行为。
- `assessment_entry_intake_log` 包含 `testee_id` 和 intake 事实。

这两张表不是入口主数据。重建后 seeddata 会重新产生 resolve/intake，所以两张 activity log 都要清空，否则 `entry_opened` 和 `intake_confirmed` 会重复。

### 5.3 Statistics

Statistics 是可从剩余主事实和新 seeddata 事实重建的读侧。旧 fact/daily/snapshot 必须整体清空，不做逐 Testee 减法。

`statistics_sync_run` 是旧统计运行记录，本次也清空，避免新基线与旧 run 证据混用。

### 5.4 IAM

IAM 不存在历史时钟或回填专用表。QS `testee.profile_id` 指向 IAM `profiles.id`，IAM `profile_links.profile_id` 建立 User 与该 Profile 的授权关系。

核心重置只删：

```text
profile_links WHERE profile_id IN (<deleted testee profile IDs>)
profiles      WHERE id         IN (<deleted testee profile IDs>)
```

IAM `users`、`auth_login_identities`、`auth_credentials`、`auth_token_audit` 默认保留。这可以避免在同一 `users` 表中误删医护人员或真实监护人身份。

如未来确实需要删除旧 seed guardian User，应作为独立 IAM 数据治理任务，以 `auth_login_identities.meta_json.source` 和 QS `staff.user_id` 保留清单为边界，不与本次 Testee 重置混合。

## 6. 行为 / 契约 / 不变量边界

### 6.1 必须保留

- `staff` 行数、ID、`user_id`、roles 不变。
- `clinician` 行数、ID、`operator_id` 和业务属性不变。
- `assessment_entry` 和 `assessment_plan` 不变。
- Questionnaire、Published Model、Norm、Report Template 不变。
- IAM 医护 User/LoginIdentity/Credential 不变。
- Mongo collections 的结构和索引不变。

### 6.2 必须清空

- QS Testee 及其关系、任务、答卷、测评、结果、报告。
- 旧 Entry resolve/intake activity。
- 旧 Statistics fact/daily/snapshot/run。
- 旧 Testee 事件的 outbox、checkpoint、retry/dead-letter。
- 旧 historical stage/attempt/rollback 控制数据。
- IAM Testee Profile 和指向该 Profile 的 ProfileLink。

### 6.3 交叉角色保护

如果某个 `testee.profile_id` 同时被 QS `staff.user_id` 对应的 IAM User 通过 ProfileLink 持有，医护人员保留规则优先：

- QS Testee 和业务事实仍然删除。
- IAM Profile/ProfileLink 暂不删除，输出冲突 ID 人工复核。
- 不允许为了“清空 Profile”而删医护 User。

## 7. 需要直接清理的数据表

### 7.1 QS MySQL：Testee 专属表整表删除，共享表按事件类型删除

下列表在本次“保留主数据、重置全部 Testee 事实”边界下属于删除范围。Testee 专属表可分批执行
等价于无 `WHERE` 的全表 `DELETE`；共享 runtime/event 表必须使用第 7.1 节所列 Testee 事件类型
或 Evaluation scope 过滤：

| 分组 | 表 |
|---|---|
| Entry activity | `assessment_entry_resolve_log`, `assessment_entry_intake_log` |
| Testee relation | `clinician_relation` |
| Plan facts | `assessment_task`, `plan_enrollment` |
| Evaluation facts | `assessment_score`, `evaluation_outcome`, `assessment` |
| Testee master | `testee` |
| Statistics facts | `statistics_access_fact`, `statistics_assessment_fact`, `statistics_plan_fact` |
| Statistics projections | `statistics_access_daily`, `statistics_assessment_daily`, `statistics_plan_activity_daily`, `statistics_plan_fulfillment_daily`, `statistics_org_snapshot` |
| Statistics audit | `statistics_sync_run` |
| Runtime control | `runtime_checkpoint` 的 Evaluation rows；`retry_event_hold`、`event_delivery_dead_letter`、`domain_event_outbox` 中属于 Testee 事件目录的 rows |
| Historical control | `seed_backfill_stage_attempt`, `seed_backfill_stage`, `seed_backfill_rollback_phase_attempt`, `seed_backfill_rollback_resource`, `seed_backfill_rollback_operation` |

Testee 事实表和 Statistics 的“整表 DELETE”结论仅适用于当前明确的环境重置目标。共享事件表
不扩大到 Questionnaire、AssessmentModel 或未知事件类型。如果未来只删某个 Org 或某部分
Testee，不得复用这个方案。

### 7.2 QS MySQL：明确保留

| 分组 | 表/资源 |
|---|---|
| 医护主数据 | `staff`, `clinician` |
| 入口主数据 | `assessment_entry` |
| Plan 定义 | `assessment_plan` |
| 系统与治理 | 机构、配置、migration 水位、`system_governance_action_runs` |
| 内容与版本 | 问卷/模型/常模/报告模板等对应主数据 |

### 7.3 QS MongoDB：Testee collection 全删，共享 Outbox 过滤删除

使用 `deleteMany({})`，不使用 `drop()`：

| 分组 | Collection |
|---|---|
| AnswerSheet | `answersheets`, `answersheet_submit_idempotency` |
| Interpretation lifecycle | `report_generations`, `interpretation_runs` |
| Report | `interpret_report_artifacts`, `report_query_catalog`, `archived_reports` |
| Failure/projection | `interpretation_admission_failures`, `interpretation_attention_projections` |
| Durable events | `domain_event_outbox` 中 AnswerSheet、Evaluation、Interpretation、Plan 事件；保留 Questionnaire、AssessmentModel 和未知事件类型 |
| Transient repair | `interpretation_catalog_repair_plans` |
| Legacy（若存在） | `interpret_reports` |

明确保留：

- `questionnaires`
- Assessment Model draft/published collections
- `assessment_norms`
- Report Template collections
- `schema_migrations`
- 其他目录、字典、配置和内容 collections

### 7.4 IAM MySQL：按 Profile ID 删除

IAM 不做整表清空。在删 QS `testee` 之前导出非空 `profile_id`，导入 IAM 临时表：

```text
tmp_delete_testee_profile_ids(profile_id PRIMARY KEY)
tmp_preserve_staff_user_ids(user_id PRIMARY KEY)
```

先找出冲突 Profile：

```text
profile_links.profile_id IN tmp_delete_testee_profile_ids
AND profile_links.user_id IN tmp_preserve_staff_user_ids
```

冲突集非空时，IAM 脚本在任何持久化删除前整体失败，并输出冲突 ID 供人工复核。冲突为零后按顺序执行：

```text
DELETE profile_links BY profile_id
DELETE guardianships BY child_id  -- 仅旧桥接表存在时
DELETE profiles      BY id
DELETE children      BY id        -- 仅旧桥接表存在时
```

不触及 `users`、`auth_login_identities`、`auth_credentials`、`auth_token_audit`。

## 8. 删除顺序

### 8.1 维护窗口前置

1. 停止 seeddata-runner、qs-worker、Outbox relay、Plan scheduler 和 Statistics scheduler。
2. 禁止 collection/apiserver 接受新 Testee、AnswerSheet 和 Plan 写入，或进入整体维护窗口。
3. 确认 MySQL/Mongo outbox 没有 `pending/publishing/failed/manual_required` 记录。
4. 完成 IAM MySQL、QS MySQL、QS MongoDB 的同一维护窗口备份。
5. 从 QS 导出 `staff.user_id` 和 `testee.profile_id`，并记录行数与 SHA-256。

### 8.2 数据操作顺序

```text
QS MySQL 备份与导出 ID
  -> QS MySQL runtime/historical control
  -> QS MySQL Statistics
  -> QS MySQL Entry activity / relation
  -> QS MySQL Task / Enrollment / Score / Outcome / Assessment
  -> QS MySQL Testee
  -> QS Mongo deleteMany
  -> IAM ProfileLink / Profile
  -> 三库验证
```

QS MySQL 内部建议的 `DELETE` 顺序：

```text
seed_backfill_rollback_phase_attempt
seed_backfill_rollback_resource
seed_backfill_rollback_operation
seed_backfill_stage_attempt
seed_backfill_stage
event_delivery_dead_letter
retry_event_hold
runtime_checkpoint
domain_event_outbox
statistics_org_snapshot
statistics_plan_fulfillment_daily
statistics_plan_activity_daily
statistics_assessment_daily
statistics_access_daily
statistics_plan_fact
statistics_assessment_fact
statistics_access_fact
statistics_sync_run
assessment_entry_resolve_log
assessment_entry_intake_log
clinician_relation
assessment_task
plan_enrollment
assessment_score
evaluation_outcome
assessment
testee
```

如数据量较大，使用固定主键批次 `DELETE ... ORDER BY id LIMIT N`，每批独立提交；不使用一个跨所有大表的长事务。当删除完整表数据时仍使用 `DELETE`，不使用 `TRUNCATE`，以便保留更明确的 binlog/审计边界。

## 9. 清理 SQL 的安全形态

三份脚本都必须包含：

- 环境/数据库名白名单断言。
- 执行前行数快照。
- 医护主数据基线断言。
- 事件队列已 drain 断言。
- 按固定顺序的删除语句。
- 删除后行数断言。
- 明确的非零错误与立即停止。

这些断言不需要新的业务表或应用编排器；MySQL 使用临时表、变量和 `SIGNAL SQLSTATE`，MongoDB 脚本使用执行前/后 `countDocuments` 断言即可。

## 10. 清理后验收

### 10.1 应为 0

- `testee`
- `clinician_relation`
- `assessment_entry_resolve_log`
- `assessment_entry_intake_log`
- `plan_enrollment`
- `assessment_task`
- `assessment`
- `assessment_score`
- `evaluation_outcome`
- 所有 Statistics fact/daily/snapshot/run
- runtime checkpoint 的 Evaluation rows，以及三类共享 event control 表中的 Testee 事件 rows
- 五类 historical control 表
- 第 7.3 节列出的 Mongo collections
- IAM 本次删除集中的 Profile/ProfileLink

### 10.2 应与删除前一致

- `staff`
- `clinician`
- `assessment_entry`
- `assessment_plan`
- QS 机构和业务配置
- Mongo Questionnaire/Model/Norm/Template 数量和发布版本
- IAM `users`、`auth_login_identities`、`auth_credentials`
- IAM 医护 User 对应 Profile/ProfileLink

### 10.3 应不可见

- 任一旧 Testee 查询结果。
- 任一旧 AnswerSheet/Assessment/Outcome/Report。
- 任一旧 Statistics Testee/Assessment/Plan 指标。
- Retry governance 中的旧 Testee 候选项。

## 11. 测试与可观测性现状

当前 [`cleanup_perf_testee_data`](cleanup_perf_testee_data/main.go) 已给出 MySQL/Mongo 的资源范围、删除顺序和索引关系，可作为直接数据库脚本的代码依据，但本次不调用它的 batch rollback coordinator。

三份新脚本需要先在本地/隔离数据库上运行 characterization test：

- 准备医护人员 + Entry + Plan + Testee + Assessment + AnswerSheet + Report 混合数据。
- 执行三份脚本。
- 断言 Testee 事实全部为 0。
- 断言医护人员、Entry、Plan 和内容数据完整。
- 断言 Mongo 索引仍存在。
- 断言第二次执行仍成功，结果不变。

## 12. 分析指标与判定

| 指标 | 判定 | 依据 |
|---|---|---|
| 入口清晰度 | 绿 | 删除入口收敛为 3 份数据库脚本。 |
| 责任内聚 | 绿 | 数据库脚本只负责数据重置，seeddata 只负责重建。 |
| 依赖扩散 | 黄 | QS 事实跨 MySQL/Mongo，IAM Profile 是另一数据库。 |
| 行为边界 | 绿 | 保留主数据和删除 Testee 事实的表级边界可枚举。 |
| 变更放大 | 绿 | 数据清理不再修改 IAM/QS 生产服务。 |
| 测试保护 | 黄 | 现有 cleanup 提供范围依据，但 3 份直接脚本仍需隔离数据库 characterization test。 |
| 可观测性 | 绿 | 执行前/后 count、备份和主数据基线可直接判定结果。 |
| 安全抽离性 | 绿 | 数据重置完成后，回填控制面可与业务事实分批退场。 |

## 13. 主要风险点

### 13.1 只运行 MySQL SQL

不够。AnswerSheet、Report 和 Mongo outbox 会留下孤儿数据，Statistics 后续 repair 仍可能从 Mongo 重新采集出旧 fact。因此 QS MySQL 和 MongoDB 必须同一维护窗口一起清理。

### 13.2 保留 resolve log

会让旧 `entry_opened` 在 Statistics repair 后重现，并与新 seeddata 扫描重复。本次保留 `assessment_entry`，但不保留旧 resolve/intake activity。

### 13.3 整表删 IAM users

会误删医护人员和其他真实身份。核心重置只删 Testee Profile/ProfileLink。

### 13.4 服务未停止

删除期间 Worker/Outbox/Scheduler 可能重新生成 Assessment、Report、Task 或 Statistics fact。事件必须先 drain，再停服执行。

### 13.5 整表长事务

大表单事务可造成 lock wait、binlog 压力和长时回滚。应按主键批次删除，每批提交。

## 14. 初步发现或假设

### 已确认

- 数据清理无需修改 IAM 和 qs-server 生产 API。
- 医护人员主数据与 Testee 关系表在存储上可分离。
- `assessment_entry` 与 resolve/intake activity 是不同责任；可保留入口定义并清空旧活动。
- Statistics 为可重建读侧，可在本次整体归零。
- IAM Testee Profile 可由 QS `testee.profile_id` 精确确定。

### 待执行前确认

- 目标环境确实要删除全部 Org 的 Testee 数据，而不是只删某一 Org。
- 目标环境 migration 水位已至少到 62，并不存在已退役 V1 表。
- MongoDB 实际 collection 清单与第 7.3 节一致，没有环境特有 Testee collection。
- 医护 User 与 Testee Profile 不存在未审批的交叉角色。

## 15. 使用 seeddata 重建

### 15.1 使用新 batch

不复用旧 batch ID。正式重建使用唯一新 ID，例如：

```text
hist-20250101-20260727-v2
```

pilot 使用独立 batch ID。

### 15.2 回填次序

1. 采集重置后的 Statistics 零基线。
2. 冻结 Plan、Questionnaire、Published Model 精确版本。
3. Staging 运行 3 天全 journey。
4. Staging 运行 `2025-01-01..2025-01-31`。
5. Production 用独立 batch 运行 3 天 pilot，完成表级和 Statistics 验收后清理 pilot。
6. 运行正式 573 天批次，每日通过 verify 后才推进 checkpoint。
7. 执行 Statistics repair/validate/catch-up/latest-complete-day publish。
8. 运行 `perf-preflight` 和 `perf-smoke`，必须发现真实 Report 样本且不 degraded。

### 15.3 最终数据不变量

- 新 Testee 全部关联新 IAM Profile/ProfileLink。
- 旧医护 `staff`/`clinician`/Entry/Plan ID 未变。
- 新 ClinicianRelation 指向保留的 clinician 和新 Testee。
- 每个 Assessment 场景有唯一 AnswerSheet、Assessment、Outcome、Report。
- 时间线满足既定顺序，且不跨对应上海自然日。
- Statistics 与重置后基线 + 新批次增量一致。

## 16. 历史回填能力退场

数据重置简化不改变代码退场的原则：新数据验收后先关闭能力，再删代码，最后删控制表。

### 16.1 立即关闭

- 关闭 apiserver/collection-server `historical_seed.enabled`。
- 轮换并移除 `QS_HISTORICAL_CONTEXT_SECRET`。
- 封存三库 HEAD、runner build、配置 hash、最终 manifest 和 Statistics/K6 证据。

### 16.2 seeddata-runner

- 删除 `historical-backfill`、`historical-verify`、`historical-manifest`。
- 删除 HistoricalDaySnapshot、manifest/checkpoint、历史 HMAC/context 和 stage 查询。
- 保留普通 daemon、Journey 状态机、IAM mock consumer 和 AnswerSheet 可靠提交。

### 16.3 qs-server

- 删除 `X-QS-Historical-*` 签名中间件和 `historical_seed` 配置。
- 删除 proto/event payload 的 `HistoricalExecutionContext`，原 proto 字段号/名称必须 `reserved`。
- 删除 Entry/Plan/AnswerSheet/Assessment/Outcome/Report 中的 historical context/stage/attempt 分支。
- 删除 stage REST API 和 historical Statistics 编排模式。
- 把 Historical Backfill Cross-Store E2E 改写为普通 AnswerSheet -> Report required E2E，不删跨存储保护。
- 保留 `assessment_task.business_created_at`，确保已回填任务在未来 Statistics repair 时仍使用正确业务日期。

### 16.4 IAM

- 不删 EnsureMockConsumer、Profile/Meta 传输和 ProfileLink 授权。
- 将测试中的 `seeddata_historical` 示例改为中性 seeddata Meta。
- 不新增历史时钟、回填表或删除 API。

### 16.5 控制表

所有应用版本已不读写后，通过新的 forward migration drop：

```text
seed_backfill_rollback_phase_attempt
seed_backfill_rollback_resource
seed_backfill_rollback_operation
seed_backfill_stage_attempt
seed_backfill_stage
```

不修改已执行的 migration 59/61/62。

## 17. 立即停止条件

- 目标数据库名与审批环境不一致。
- 任一 MySQL/Mongo outbox 还有未完成事件。
- IAM/QS/Mongo 备份不完整或未做恢复验证。
- `staff.user_id` 保留清单或 `testee.profile_id` 删除清单导出失败。
- 医护 User 与待删 Profile 存在未审批冲突。
- 任一保留表的执行后行数/hash 与基线不同。
- 任一 Testee/AnswerSheet/Assessment/Outcome/Report/Statistics 旧事实未清空。
- 重建时出现版本/payload conflict、Report 缺失、时间越界或 Statistics validate 失败。

## 18. 下一步建议

三份脚本已落在 `scripts/oneoff/reset_testee_historical_data`，仍不修改 IAM/qs-server 生产代码。下一步是：

1. 在空库应用最新 migration，并装载包含医护控制组与 Testee 事实的小型 fixture。
2. 运行三份脚本，证明“Testee 事实全部为 0，医护主数据全部保留”。
3. 注入一次批次中断，以 resume 模式证明不会扩大删除范围。
4. 人工复核脚本、备份恢复结果和行数报告后，再安排数据库维护窗口。
