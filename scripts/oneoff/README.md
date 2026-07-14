# One-off Scripts

`scripts/oneoff/` 放只在特定修复、补录、回填场景下手工执行的脚本。这些脚本不是常规服务启动流程的一部分，执行前应先确认目标环境、时间窗口和影响范围。

## 执行原则

- 先在只读或 dry-run 模式确认候选数据，再执行带 `--apply` 的写入操作。
- 生产执行前先备份 MySQL / MongoDB / Redis，或使用脚本内置备份能力。
- 不要把真实密码、token 写入命令历史或提交到仓库，优先通过 shell 变量传入。
- 日期窗口里 `--start-date` 通常是包含边界，`--end-date` 通常是排除边界；具体以脚本参数说明为准。
- 建议从仓库根目录执行，避免相对路径和 Go module 解析问题。

常用变量示例：

```bash
export MYSQL_DSN='app_user:***@tcp(127.0.0.1:3306)/qs?parseTime=true'
export MONGO_URI='mongodb://app_user:***@127.0.0.1:27017/qs'
export QS_TOKEN='***'
export REDIS_PASSWORD='***'
```

如果 MongoDB 只监听远端服务器的 `127.0.0.1:27017`，先在执行脚本的机器上建立 SSH 隧道，再把 URI 指向本地转发端口。URI 必须带有效账号密码；如果账号创建在 `admin` 库，需要追加 `authSource=admin`。

```bash
ssh -N -L 27017:127.0.0.1:27017 yangshujie@81.70.102.15

export MONGO_URI='mongodb://app_user:***@127.0.0.1:27017/qs?directConnection=true'
# 或：export MONGO_URI='mongodb://app_user:***@127.0.0.1:27017/qs?authSource=admin&directConnection=true'
```

## 脚本总览

| 脚本 | 用途 | 主要写入对象 |
| ---- | ---- | ------------ |
| `audit_evaluation_cleanup.sql` | Batch E 清理前只读审计 Outcome/Run/Assessment 一致性、schema 版本与 legacy payload 存量 | 无写入 |
| `cleanup_deleted_assessment_orphans.go` | 清理物理删除 assessment 后遗留的行为、统计和 Mongo 文档引用 | MySQL `behavior_footprint` / `assessment_episode`，Mongo `answersheets` |
| `cleanup_orphaned_assessment_documents/` | 直接扫描 Mongo，清理不存在 MySQL Assessment 的历史报告和旧答卷 | Mongo `archived_reports` / `report_query_catalog` / `answersheets` / `answersheet_submit_idempotency` |
| `cleanup_perf_testee_data/main.go` | 按压测受试者 ID 物理清理 MySQL / MongoDB 垃圾数据 | MySQL testee/assessment/统计事实/outbox，Mongo answersheets/outbox |
| `rewrite_seeddata_assessment_times/main.go` | 将 seeddata plan task 测评从错误集中日期改回任务计划日期 | MySQL `assessment` / `assessment_task` / `assessment_score` / `testee` |
| `rebuild_access_funnel_from_sources/main.go` | 从接入业务源重建接入漏斗统计源和聚合 | MySQL `assessment_entry_intake_log` / `statistics_journey_daily.access_*` |
| `rebuild_statistics_facts_from_sources/main.go` | 从业务源表重建统计事实层 | MySQL `behavior_footprint` / `assessment_episode` |
| `rebuild_statistics_aggregates_and_cache/main.go` | 重建统计聚合表并刷新统计查询缓存 | MySQL 统计聚合表，Redis 统计查询缓存 |
| `rebuild_seeddata_access_statistics/main.go` | 一站式修复 seeddata 接入统计历史数据 | MySQL intake/resolve log、`behavior_footprint`、`statistics_journey_daily` |
| `enroll_testees_after_date.py` | 通过 REST API 将指定日期后创建的受试者批量加入计划 | REST `/plans/enroll` 对应的业务数据 |
| `seed_personality_typology/` | **统一入口**：重初始化 MBTI@2.0.1、MBTI_FC_93、SBTI、Big5、九型 问卷与解释模型 | Mongo `questionnaires` + `assessment_models` + `published_assessment_models` |
| `repair_sbti_profiles/` | 通过受保护 DefinitionV2 API 定点补齐 SBTI Pattern 和特殊结果标记；默认 dry-run，不自动发布 | `assessment_models` 草稿 |
| `seed_brief2/` | 从历史 BRIEF-2 常模 PHP 和经过核验的题目-因子映射，初始化 BRIEF-2 家长版行为能力模型 | Mongo `assessment_norms` + `assessment_models` + `published_assessment_models` |
| `seed_spm_sensory/` | 从历史 SPM 感觉统合常模 PHP 和经过核验的题目-因子映射，初始化 `spm_sensory` 行为能力模型 | Mongo `assessment_norms` + `assessment_models` + `published_assessment_models` |
| `audit_scale_models/` | 只读审计所有 canonical Scale 草稿、发布快照、绑定问卷、DefinitionV2 与 payload 投影一致性 | 无写入 |
| `observe_outbox_by_event_type/` | 只读观测 outbox 按 `event_type` 积压与近期写入，输出 legacy 退役 Gate | 无写入 |

`__pycache__/` 是 Python 运行产物，不是脚本入口。

### 已退役（ModelCatalog 迁移完成，脚本已删）

以下脚本仅用于一次性数据迁移/审计，生产已跑通后从仓库移除；需复现可查 git 历史：

| 脚本 | 原用途 |
| ---- | ------ |
| `backfill_draft_assessment_models_from_scales/` | `scales` head → `assessment_models` draft |
| `audit_scale_assessment_model_gap/` | scales head 与 assessment_models 差集审计 |
| `backfill_published_assessment_models/` | legacy `evaluation_rule_sets` → `published_assessment_models` |
| `seed_evaluation_rule_sets/` | 内置 MBTI/SBTI + 量表 published 种子（已由 `seed_personality_typology` 取代） |
| `audit_published_model_payload_formats/` | legacy payload_format 审计 |
| `audit_personality_kind_values/` / `migrate_personality_kind_values/` | `personality` → `typology` kind 迁移 |
| `migrate_modelcatalog_definition_v2/` | DefinitionV2 + norm 行回填 |

## observe_outbox_by_event_type

### 做什么

只读统计 MySQL / Mongo `domain_event_outbox`：

- **未完成积压**：`status IN (pending, publishing, failed)`，按 `event_type, status` 聚合
- **近期写入**：`created_at` 在最近 N 天内，按 `event_type` 聚合（含 `published`）
- **Legacy 高亮**：`assessment.interpreted.v2`、`report.generated.v2`（待排空的旧 outcome wire）
- **Canonical**：`assessment.interpreted`、`report.generated`（当前 outcome 投影 wire）
- **Gate**：`PASS` 当 deprecated wire 未完成 = 0 且近期 deprecated wire 写入 = 0；否则 `WARN`

用于 legacy 测评/报告事件退役 Phase 0 观测。

### 如何调用

```bash
export MYSQL_DSN='app_user:***@tcp(127.0.0.1:3306)/qs?parseTime=true'
export MONGO_URI='mongodb://app_user:***@127.0.0.1:27017/qs?authSource=admin&directConnection=true'

# 人类可读表格
go run ./scripts/oneoff/observe_outbox_by_event_type/ \
  --mysql-dsn "$MYSQL_DSN" \
  --mongo-uri "$MONGO_URI" \
  --mongo-db qs \
  --recent-days 7

# JSON（便于 CI / 告警采集）
go run ./scripts/oneoff/observe_outbox_by_event_type/ \
  --mysql-dsn "$MYSQL_DSN" \
  --mongo-uri "$MONGO_URI" \
  --json
```

至少提供 `--mysql-dsn` 或 `--mongo-uri` 之一；生产建议双查。

> **注意**：包目录，使用 `go run ./scripts/oneoff/observe_outbox_by_event_type/`（带尾部 `/`）。

> **注意**：`seed_personality_typology/` 是包目录，必须用 `go run ./scripts/oneoff/seed_personality_typology/`（带尾部 `/`），**不要** `go run .../main.go`。

## seed_personality_typology

### 做什么

一次性重初始化以下人格测评：

| 模型 | 问卷 | 算法 | 决策 |
| ---- | ---- | ---- | ---- |
| `MBTI_OEJTS@2.0.1` | 32题 | mbti | pole_composition |
| `MBTI_FC_93@1.0.0` | 93题强迫选择 | mbti | pole_composition |
| `SBTI_FUN@1.0.0` | 30题 | sbti | nearest_pattern |
| `BIG5_IPIP_50@1.0.0` | 50题 IPIP | bigfive | trait_profile |
| `ENNEAGRAM_45@1.0.0` | 45题自研 | personality_typology | trait_profile |

1. 发布问卷快照到 `questionnaires`
2. 写入带 **explicit factor graph** 的解释模型到 `assessment_models`（draft）与 `published_assessment_models`

模型 payload 包含：`question_mappings`、`factor.contributions`、`report.kind=personality_type`、`adapter_key=mbti|sbti`、正确的 `questionnaire_binding`。

### 如何调用

```bash
# dry-run
go run ./scripts/oneoff/seed_personality_typology/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs

# 覆盖已有脏数据并写入
go run ./scripts/oneoff/seed_personality_typology/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs --force --apply
```

仅发布问卷、不写解释模型（替代原 `seed_*_questionnaire`）：

```bash
go run ./scripts/oneoff/seed_personality_typology/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs --skip-models --force --apply
```

仅重初始化 Big5 / 九型：

```bash
go run ./scripts/oneoff/seed_personality_typology/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs \
  --skip-mbti --skip-sbti --force --apply
```

仅重初始化 MBTI 93 题版：

```bash
go run ./scripts/oneoff/seed_personality_typology/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs \
  --skip-mbti --skip-sbti --skip-big5 --skip-enneagram --force --apply
```

仅重初始化 MBTI 32 题版：

```bash
go run ./scripts/oneoff/seed_personality_typology/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs --skip-mbti93 --skip-sbti --skip-big5 --skip-enneagram --force --apply
```

验收：

```javascript
db.questionnaires.find(
  { code: { $in: ["MBTI_OEJTS", "MBTI_FC_93", "SBTI_FUN", "BIG5_IPIP_50", "ENNEAGRAM_45"] }, deleted_at: null },
  { code: 1, version: 1, status: 1, question_count: 1 }
)
db.assessment_models.find(
  { code: { $in: ["MBTI_OEJTS", "MBTI_FC_93", "SBTI_FUN", "BIG5_IPIP_50", "ENNEAGRAM_45"] }, deleted_at: null },
  { code: 1, questionnaire_code: 1, questionnaire_version: 1, status: 1, version: 1 }
)
```

题库 JSON 位于 `seed_personality_typology/data/`：

| 文件 | 说明 |
| ---- | ---- |
| `mbti_questionnaire.json` | MBTI 32 题（OEJTS） |
| `mbti_fc_93_questionnaire.json` | MBTI 93 题强迫选择（含 stem/placeholder/factor/left/right/options） |
| `big5_ipip_50_questionnaire.json` | 大五 IPIP-50 |
| `enneagram_45_questionnaire.json` | 九型 45 题 |
| `sbti_questionnaire.json` | SBTI 娱乐版 |
| `gen_seed.py` | 重新生成 MBTI 32 题 JSON |
| `gen_trait_questionnaires.py` | 重新生成 Big5 / 九型 JSON |

## cleanup_perf_testee_data/main.go

### 做什么

按明确给出的 `testee_id` 集合清理压测产生的垃圾数据。脚本会先在 MySQL 中创建临时作用域，补齐这些受试者关联的 `assessment_id`、`answersheet_id`、`report_id` 和 outbox `event_id`，然后清理：

- MySQL `testee`、`assessment`、`assessment_score`、`assessment_task`。
- MySQL `clinician_relation`、`assessment_entry_intake_log`。
- MySQL `behavior_footprint`、`assessment_episode`、旧版 testee 维度 `statistics_daily` / `statistics_accumulated`。
- MySQL `domain_event_outbox`、`analytics_pending_event`、`runtime_checkpoint`。
- MongoDB `answersheets`、`answersheet_submit_idempotency`、`domain_event_outbox`。

脚本默认 dry-run，只输出命中数量和受试者预览。执行 `--apply` 时会先创建备份表和备份集合，除非显式传入 `--skip-backup`。

脚本会在跑 MySQL 作用域查询前，对 MongoDB `answersheets`、`answersheet_submit_idempotency`、`domain_event_outbox` 做只读权限预检。`ping` 成功但 `find requires authentication` 失败时，说明地址已经连通，但 `MONGO_URI` 缺少账号密码或认证库不对。

MongoDB 单条命令有 16MB BSON 限制。脚本会把大量 `domain_id` / `aggregate_id` 的 `$in` 查询分批执行，避免几十万压测数据一次性塞进一条 `find`、`count`、`backup` 或 `delete` 命令。

旧版 MySQL `statistics_daily` / `statistics_accumulated` 已在新迁移中删除的环境里，脚本会自动跳过这些 legacy 统计表。

MySQL / MongoDB outbox 的清理都按 `aggregate_type + aggregate_id` 收窄；不要手工改成只按 `aggregate_id` 删除，因为不同聚合根可能共用相同的数字 ID 字符串。

### 解决什么问题

用于清理最近几天压测产生的大量答卷、测评、报告、行为事件和 outbox 积压数据。它不会跨库删除 IAM 用户/档案，也不会直接扣减新统计聚合表；清理源数据后，应按受影响日期窗口重建统计聚合和 Redis 查询缓存。

### 如何调用

先 dry-run：

```bash
go run scripts/oneoff/cleanup_perf_testee_data/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --mongo-uri "$MONGO_URI" \
  --mongo-db qs \
  --testee-ids '624047162266759726,623766863960093230,623932256825651758,623929287728181806,623917211471327790,623906818086679086,623905256379527726,623920684539589166,623902104913719854,623922208565178926' \
  --preview-limit 20
```

确认命中范围后执行：

```bash
go run scripts/oneoff/cleanup_perf_testee_data/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --mongo-uri "$MONGO_URI" \
  --mongo-db qs \
  --testee-ids '624047162266759726,623766863960093230,623932256825651758,623929287728181806,623917211471327790,623906818086679086,623905256379527726,623920684539589166,623902104913719854,623922208565178926' \
  --backup-suffix 20260616_perf_testee_cleanup \
  --apply
```

如果 ID 很多，可以放入文件：

```bash
go run scripts/oneoff/cleanup_perf_testee_data/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --mongo-uri "$MONGO_URI" \
  --mongo-db qs \
  --testee-ids-file /tmp/perf-testee-ids.txt \
  --apply
```

执行后按 dry-run 输出的 affected source date window 重建统计聚合和缓存，例如：

```bash
go run scripts/oneoff/rebuild_statistics_aggregates_and_cache/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2026-06-01 \
  --end-date 2026-07-06 \
  --apply
```

关键参数：

- `--testee-ids` / `--testee-ids-file`：二选一或同时使用，逗号、空格、换行都可分隔。
- `--testee-created-after`：安全边界，默认 `2026-05-01 00:00:00`；命中的受试者必须晚于该时间创建。
- `--allow-old-testees`：绕过创建时间保护，只应在人工确认这些 ID 确实是压测数据后使用。
- `--derive-ids-from-facts`：额外从 MySQL `behavior_footprint` / `assessment_episode` 反查关联 ID；大事实表上较慢，默认关闭。事实表本身仍会按 `testee_id` 清理。
- `--scan-event-payloads`：额外扫描 MySQL outbox / pending 的 `payload_json` 兜底匹配 `testee_id`；大 outbox 表上很慢，默认关闭。
- `--skip-counts`：跳过行数统计和 affected source date window 计算；在已有外部备份保护、只想快速执行清理时使用。
- `--skip-mongo-outbox-event-scope`：跳过从 Mongo `domain_event_outbox` 读取 `event_id` 并回灌 MySQL 临时表。Mongo outbox 文档仍会按聚合 ID 分批删除，但 MySQL `analytics_pending_event` / `runtime_checkpoint`（`scope=analytics_projector`）只会清理 MySQL outbox 已发现的事件 ID；这属于快速清理模式，不是“零残留”模式。如需清理 Mongo outbox 对应的 pending/checkpoint，可后续按事件类型和时间窗单独处理。
- `--backup-suffix`：备份表/集合后缀，只允许字母、数字和下划线。
- `--skip-backup`：跳过内置备份，只应在已有外部备份时使用。

大数据量快速执行示例：

```bash
go run scripts/oneoff/cleanup_perf_testee_data/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --mongo-uri "$MONGO_URI" \
  --mongo-db qs \
  --testee-ids-file /tmp/perf-testee-ids.txt \
  --backup-suffix 20260616_perf_testee_cleanup \
  --skip-counts \
  --skip-mongo-outbox-event-scope \
  --preview-limit 0 \
  --apply
```

如果已经在云厂商侧做了 MySQL/Mongo 快照，且明确接受脚本内不再二次备份，可以再加 `--skip-backup`。没有外部备份时不要跳过备份。

## cleanup_deleted_assessment_orphans.go

### 做什么

扫描 `behavior_footprint` 和 `assessment_episode` 中引用了已物理删除 `assessment` 的记录，将这些孤儿引用加入临时队列。执行 `--apply` 后：

- 软删除匹配的 MySQL `behavior_footprint` 行。
- 软删除匹配的 MySQL `assessment_episode` 行。
- 按 `answersheet_id` 软删除 Mongo `answersheets` 文档。
- 默认先创建备份表和备份集合，除非显式传入 `--skip-backup`。

### 解决什么问题

用于修复 assessment 被物理删除后，统计事实、行为足迹、测评 episode 或 Mongo 答卷/报告文档仍保留引用，导致统计、报告查询或治理数据看到已删除测评的问题。

### 如何调用

先 dry-run：

```bash
go run scripts/oneoff/cleanup_deleted_assessment_orphans.go \
  --mysql-dsn "$MYSQL_DSN" \
  --mongo-uri "$MONGO_URI" \
  --mongo-db qs \
  --org-id 1 \
  --preview-limit 50
```

限制扫描窗口：

```bash
go run scripts/oneoff/cleanup_deleted_assessment_orphans.go \
  --mysql-dsn "$MYSQL_DSN" \
  --mongo-uri "$MONGO_URI" \
  --mongo-db qs \
  --org-id 1 \
  --source-created-start '2026-05-01 00:00:00' \
  --source-created-end '2026-06-01 00:00:00'
```

确认后执行：

```bash
go run scripts/oneoff/cleanup_deleted_assessment_orphans.go \
  --mysql-dsn "$MYSQL_DSN" \
  --mongo-uri "$MONGO_URI" \
  --mongo-db qs \
  --org-id 1 \
  --backup-suffix 20260604_deleted_assessment_orphans \
  --batch-size 1000 \
  --apply
```

关键参数：

- `--org-id` / `--all-orgs`：二选一，限定组织范围。
- `--source-created-start` / `--source-created-end`：按源行 `created_at` 缩小扫描窗口。
- `--backup-suffix`：备份表/集合后缀，只允许字母、数字和下划线。
- `--batch-size`：每批处理的孤儿引用数。
- `--skip-backup`：跳过脚本内置备份，只应在已有外部备份时使用。

## rewrite_seeddata_assessment_times/main.go

### 做什么

修复 `seeddata-runner` 因系统问题把前几天计划测评集中提交到最后一天的问题。脚本只处理 linked plan task：

- `assessment_task.assessment_id` 能关联到 `assessment.id`。
- `assessment.origin_type = 'plan'`。
- 默认要求 `testee.source = 'daily_simulation'`，避免误改真实业务数据。
- 使用 `assessment_task.planned_at` 的日期作为目标日期，保留原始时分秒。

执行 `--apply` 后会按候选范围改写：

- `assessment.created_at` / `updated_at` / `submitted_at` / `evaluated_at` / `failed_at`
- `assessment_task.completed_at` / `updated_at`
- 默认同时改写 `assessment_task.open_at`，可用 `--rewrite-task-open-at=false` 关闭。
- 可选改写 `assessment_task.expire_at`，默认关闭。
- 默认同步改写 `assessment_score.created_at` / `updated_at`。
- 默认刷新受影响 `testee` 的 `total_assessments` / `last_assessment_at` / `last_risk_level`。

脚本默认 dry-run。执行写入时会先创建备份表，除非显式传入 `--skip-backup`。
备份表名前缀为 `seed_bak_assessment_`、`seed_bak_task_`、`seed_bak_score_`、`seed_bak_testee_`。

### 解决什么问题

用于修复 seeddata 测评源表日期错误导致统计趋势集中在最后一天的问题。修正源表后，应继续运行事实层和聚合缓存重建脚本，否则统计事实、统计聚合和 Redis 查询缓存仍可能保留旧值。

### 如何调用

先 dry-run 预览候选数据：

```bash
go run scripts/oneoff/rewrite_seeddata_assessment_times/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --collapsed-date 2026-06-03 \
  --target-start-date 2026-05-28 \
  --target-end-date 2026-06-03 \
  --preview-limit 50
```

限定计划范围：

```bash
go run scripts/oneoff/rewrite_seeddata_assessment_times/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --collapsed-date 2026-06-03 \
  --target-start-date 2026-05-28 \
  --target-end-date 2026-06-03 \
  --plan-id 614333603412718126 \
  --plan-id 614187067651404334
```

确认后执行：

```bash
go run scripts/oneoff/rewrite_seeddata_assessment_times/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --collapsed-date 2026-06-03 \
  --target-start-date 2026-05-28 \
  --target-end-date 2026-06-03 \
  --backup-suffix 20260603_seeddata_time_rewrite \
  --apply
```

执行后重建统计事实和聚合缓存：

```bash
go run scripts/oneoff/rebuild_statistics_facts_from_sources/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2026-05-28 \
  --end-date 2026-06-04 \
  --reset-window \
  --apply

go run scripts/oneoff/rebuild_statistics_aggregates_and_cache/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2026-05-28 \
  --end-date 2026-06-04 \
  --redis-addr "$REDIS_ADDR" \
  --redis-query-namespace 'qs:cache:query' \
  --redis-username "$REDIS_USERNAME" \
  --redis-password "$REDIS_PASSWORD" \
  --apply
```

关键参数：

- `--collapsed-date`：错误集中到的那一天，格式 `YYYY-MM-DD`。
- `--target-start-date` / `--target-end-date`：按 `assessment_task.planned_at` 限定目标日期窗口，end 是排除边界。
- `--testee-source`：受试者来源安全边界，默认 `daily_simulation`；传空字符串可关闭，但生产环境不建议关闭。
- `--plan-id`：限定计划 ID，可重复传入或用逗号分隔。
- `--rewrite-task-open-at`：是否同步改写任务开放时间，默认开启。
- `--rewrite-task-expire-at`：是否同步改写任务过期时间，默认关闭。
- `--rewrite-score-times`：是否同步改写测评分行时间，默认开启。
- `--refresh-testee-stats`：是否刷新受试者冗余测评统计字段，默认开启。
- `--backup-suffix`：备份表后缀，只允许字母、数字和下划线。
- `--skip-backup`：跳过内置备份，只应在已有外部备份时使用。

## rebuild_access_funnel_from_sources/main.go

### 做什么

从接入相关业务源重建统计中心概览里的“接入漏斗”数据：

- 保留并重放窗口内已有的 `assessment_entry_intake_log`。
- 从 `clinician_relation.source_type = 'assessment_entry'` 的照护关系推导缺失的 intake log，默认只处理 `testee.source = 'daily_simulation'`，避免误把真实人工分配算进 seeddata 接入。
- 重新计算 `statistics_journey_daily` 机构维度的 `access_entry_opened_count` / `access_intake_confirmed_count` / `access_testee_created_count` / `access_care_relationship_established_count`。

脚本不会删除 `assessment_entry_resolve_log`。入口打开只能从该日志读取，删除后无法从业务源完整还原。

### 解决什么问题

用于修复 `seeddata-runner` 曾经绕过 public `/assessment-entries/:token/intake`，直接创建 testee 并调用后台关系分配接口，导致接入漏斗“完成接入 / 新建档案 / 建立照护”缺失的问题。

脚本默认 dry-run。执行 `--apply` 时会先备份窗口内 active 的 `assessment_entry_intake_log` 和机构维度 `statistics_journey_daily` 行，除非显式传入 `--skip-backup`。

### 如何调用

先 dry-run 预览：

```bash
go run scripts/oneoff/rebuild_access_funnel_from_sources/main.go \
  --mysql-dsn 'fcm_admin:RfDtf6SGkGFeB9qZQtX@tcp(rm-2zet3fx250176qq8jko.mysql.rds.aliyuncs.com:3306)/qs?parseTime=true' \
  --org-id 1 \
  --start-date 2025-01-01 \
  --end-date 2026-06-06 \
  --preview-limit 50
```

确认后执行：

```bash
go run scripts/oneoff/rebuild_access_funnel_from_sources/main.go \
  --mysql-dsn 'fcm_admin:RfDtf6SGkGFeB9qZQtX@tcp(rm-2zet3fx250176qq8jko.mysql.rds.aliyuncs.com:3306)/qs?parseTime=true' \
  --org-id 1 \
  --start-date 2025-01-01 \
  --end-date 2026-06-06 \
  --backup-suffix 20260605_access_funnel_rebuild \
  --apply
```

如果确认 `imported` 也是 seeddata-runner 造出来并且应该纳入接入漏斗，可显式扩大来源范围：

```bash
go run scripts/oneoff/rebuild_access_funnel_from_sources/main.go \
  --mysql-dsn 'fcm_admin:RfDtf6SGkGFeB9qZQtX@tcp(rm-2zet3fx250176qq8jko.mysql.rds.aliyuncs.com:3306)/qs?parseTime=true' \
  --org-id 1 \
  --start-date 2025-01-01 \
  --end-date 2026-06-06 \
  --testee-source daily_simulation,imported \
  --backup-suffix 20260605_access_funnel_rebuild \
  --apply
```

执行后如果页面仍读到旧值，只刷新统计查询缓存：

```bash
go run scripts/oneoff/rebuild_statistics_aggregates_and_cache/main.go \
  --mysql-dsn 'fcm_admin:RfDtf6SGkGFeB9qZQtX@tcp(rm-2zet3fx250176qq8jko.mysql.rds.aliyuncs.com:3306)/qs?parseTime=true' \
  --org-id 1 \
  --start-date 2025-01-01 \
  --end-date 2026-06-06 \
  --skip-aggregate \
  --apply
```

关键参数：

- `--org-id` / `--all-orgs`：二选一，限定组织范围。
- `--start-date`：包含边界，默认 `2025-01-01`。
- `--end-date`：排除边界，不传默认到明天零点。
- `--testee-source`：推导缺失 intake log 的受试者来源，默认 `daily_simulation`；传空字符串可关闭来源过滤，但生产环境不建议关闭。
- `--inferred-testee-created`：推导出来的 intake log 是否记为新建档案，默认开启。
- `--backup-suffix`：备份表后缀，只允许字母、数字和下划线。
- `--skip-backup`：跳过内置备份，只应在已有外部备份时使用。

## rebuild_seeddata_access_statistics/main.go

### 做什么

一站式修复 `seeddata-runner` 历史数据导致的接入统计偏低，串联以下阶段：

1. 从 `clinician_relation` 推导缺失的 `assessment_entry_intake_log`：`source_type = assessment_entry` 走入口关联；`manual/import` 后台直挂主责/主治走该医生活跃入口（默认仅 `testee.source = daily_simulation`）。
2. 从 intake log 推导缺失的 `assessment_entry_resolve_log`（供 `entry_opened`）。
3. 将 resolve/intake log 投影到 `behavior_footprint`（`entry_opened` / `intake_confirmed` / `testee_profile_created` / `care_relationship_established`）。
4. 重建 `statistics_journey_daily`（含临床人员维度 `intake_count` 等）与组织快照。
5. 可选刷新 Redis 统计查询缓存。

### 解决什么问题

旧 seeddata 绕过 public intake、直接 assign-attending，导致漏斗事件缺失，临床人员维度 `window.intake_count` 远低于 `snapshot.primary_testee_count`。本脚本一次执行完成源日志补录、事实投影和聚合重算。

### 如何调用

先 dry-run：

```bash
go run scripts/oneoff/rebuild_seeddata_access_statistics/main.go \
  --mysql-dsn 'fcm_admin:RfDtf6SGkGFeB9qZQtX@tcp(rm-2zet3fx250176qq8jko.mysql.rds.aliyuncs.com:3306)/qs?parseTime=true' \
  --org-id 1 \
  --start-date 2025-01-01 \
  --end-date 2026-06-06 \
  --preview-limit 50
```

确认后执行（默认跳过 Redis，只写 MySQL）：

```bash
go run scripts/oneoff/rebuild_seeddata_access_statistics/main.go \
  --mysql-dsn 'fcm_admin:RfDtf6SGkGFeB9qZQtX@tcp(rm-2zet3fx250176qq8jko.mysql.rds.aliyuncs.com:3306)/qs?parseTime=true' \
  --org-id 1 \
  --start-date 2025-01-01 \
  --end-date 2026-06-06 \
  --backup-suffix 20260605_seeddata_access \
  --apply
```

需要同时刷新缓存时，追加 Redis 参数并关闭 `--skip-cache`：

```bash
go run scripts/oneoff/rebuild_seeddata_access_statistics/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2025-01-01 \
  --end-date 2026-06-06 \
  --backup-suffix 20260605_seeddata_access \
  --redis-addr "$REDIS_ADDR" \
  --redis-query-namespace 'qs:cache:query' \
  --redis-password "$REDIS_PASSWORD" \
  --skip-cache=false \
  --apply
```

验收 SQL（临床人员 30 天窗口 intake 应明显上升）：

```sql
SELECT clinician_id, SUM(intake_confirmed_count) AS intake_count
FROM statistics_journey_daily
WHERE org_id = 1 AND subject_type = 'clinician' AND deleted_at IS NULL
  AND stat_date >= DATE_SUB(CURDATE(), INTERVAL 30 DAY)
GROUP BY clinician_id
ORDER BY intake_count DESC
LIMIT 20;
```

关键参数：

- `--org-id` / `--all-orgs`：二选一。
- `--testee-source`：推导 intake 的受试者来源，默认 `daily_simulation`。
- `--skip-intake` / `--skip-resolve` / `--skip-footprint` / `--skip-aggregate` / `--skip-cache`：分阶段跳过；默认 `--skip-cache=true`。
- `--backup-suffix`：备份 intake/resolve/footprint/journey 窗口数据。

## rebuild_statistics_facts_from_sources/main.go

### 做什么

从源业务表重建统计事实层，写入或更新：

- `behavior_footprint`
- `assessment_episode`

脚本会从 `testee`、`clinician_relation`、`assessment`、`assessment_task`、`assessment_score` 等源表推导事件，包括 testee profile 创建、intake confirmed、照护关系建立/转移、答卷提交、测评创建、报告生成和 assessment episode。

### 解决什么问题

用于统计事实缺失、历史事件未投影、统计链路上线前已有数据需要补录，或事实层被错误清理后需要从源表回填的场景。

### 如何调用

先 dry-run 统计候选行：

```bash
go run scripts/oneoff/rebuild_statistics_facts_from_sources/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2026-05-01 \
  --end-date 2026-06-05
```

确认后写入事实层：

```bash
go run scripts/oneoff/rebuild_statistics_facts_from_sources/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2026-05-01 \
  --end-date 2026-06-05 \
  --apply
```

重建窗口前先删除该窗口已有事实：

```bash
go run scripts/oneoff/rebuild_statistics_facts_from_sources/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2026-05-01 \
  --end-date 2026-06-05 \
  --reset-window \
  --apply
```

关键参数：

- `--org-id` / `--all-orgs`：二选一，限定组织范围。
- `--start-date`：包含边界，默认 `2025-01-01`。
- `--end-date`：排除边界，不传则无上界。
- `--reset-window`：先删除窗口内 `behavior_footprint.occurred_at` 和 `assessment_episode.submitted_at` 匹配的数据，再重建。
- `--attribution-days`：将 assessment 归因到 `clinician_relation` 的回看天数，默认 30 天。

## rebuild_statistics_aggregates_and_cache/main.go

### 做什么

基于事实层重建统计聚合，并可刷新统计查询缓存：

- 重建每日、内容、journey、组织快照和 plan 统计聚合。
- 清理 Redis 中统计 query cache 和 version token。
- 预热 overview `today` / `7d` / `30d`、system、questionnaire、plan 统计查询缓存。

### 解决什么问题

用于事实层已经修复后，统计聚合表和 Redis 查询缓存仍是旧值的场景。通常应在 `rebuild_statistics_facts_from_sources/main.go` 或其他事实修复脚本之后执行。

### 如何调用

只重建 MySQL 聚合，跳过 Redis：

```bash
go run scripts/oneoff/rebuild_statistics_aggregates_and_cache/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2026-01-01 \
  --end-date 2026-06-01 \
  --skip-cache \
  --apply
```

重建聚合并刷新 Redis 查询缓存：

```bash
go run scripts/oneoff/rebuild_statistics_aggregates_and_cache/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2026-05-01 \
  --end-date 2026-06-05 \
  --redis-addr "$REDIS_ADDR" \
  --redis-query-namespace 'qs:cache:query' \
  --redis-username "$REDIS_USERNAME" \
  --redis-password "$REDIS_PASSWORD" \
  --apply
```

只刷新缓存，不重建聚合：

```bash
go run scripts/oneoff/rebuild_statistics_aggregates_and_cache/main.go \
  --mysql-dsn "$MYSQL_DSN" \
  --org-id 1 \
  --start-date 2026-01-01 \
  --end-date 2026-06-01 \
  --skip-aggregate \
  --redis-addr 127.0.0.1:6379 \
  --redis-query-db 4 \
  --redis-meta-db 7 \
  --redis-query-namespace cache:query \
  --apply
```

关键参数：

- `--org-id` / `--all-orgs`：二选一，限定组织范围；`--all-orgs` 会从窗口内源数据自动发现组织。
- `--start-date`：包含边界，默认 `2025-01-01`。
- `--end-date`：排除边界，不传默认到明天零点。
- `--skip-aggregate`：跳过 MySQL 聚合重建。
- `--skip-cache`：跳过 Redis 查询缓存清理和预热。
- `--questionnaire-code` / `--plan-id`：限定预热对象，可重复传入或用逗号分隔。
- `--max-questionnaires` / `--max-plans`：自动发现预热对象时限制数量。
- `--redis-query-addr` / `--redis-meta-addr`：查询缓存与 version token 使用不同 Redis 时分别指定。

## enroll_testees_after_date.py

### 做什么

通过 REST API 分页查询指定创建日期范围内的受试者，并调用 `/plans/enroll` 将这些受试者加入指定测评计划。

### 解决什么问题

用于计划创建或规则调整后，需要把某段时间之后已经登记的受试者补加入计划的场景。脚本走 REST API，因此会复用线上接口的认证、授权和业务校验。

### 如何调用

先 dry-run 预览匹配受试者：

```bash
python3 scripts/oneoff/enroll_testees_after_date.py \
  --base-url https://qs.fangcunmount.cn/api/v1 \
  --token "$QS_TOKEN" \
  --plan-id 1001 \
  --created-start-date 2026-05-01 \
  --created-end-date 2026-06-01 \
  --dry-run
```

确认后执行补录：

```bash
python3 scripts/oneoff/enroll_testees_after_date.py \
  --base-url https://qs.fangcunmount.cn/api/v1 \
  --token "$QS_TOKEN" \
  --plan-id 1001 \
  --created-start-date 2026-05-01 \
  --created-end-date 2026-06-01 \
  --start-date-source created_at \
  --page-size 100 \
  --sleep-ms 50
```

关键参数：

- `--base-url`：API base URL，示例为 `https://host/api/v1`。
- `--token`：Bearer token，需要具备 `qs:evaluation_plan_manager` 或 `qs:admin` 权限。
- `--plan-id`：目标计划 ID。
- `--created-start-date` / `--created-end-date`：受试者创建日期范围，end 是包含边界。
- `--start-date`：统一指定计划开始日期；不传时默认等于 `--created-start-date`。
- `--start-date-source created_at`：每个受试者使用自己的 `created_at` 日期作为计划开始日期。
- `--sleep-ms`：每次 enroll 调用后的暂停时间，用于降低接口压力。
- `--dry-run`：只列出匹配受试者，不调用 enroll。

## Interpretation 报告目录回填

代码先上线 `report_query_catalog` 索引和新报告事务投影，随后手工执行。建议 archive、artifact 分阶段运行；archive 是 120 万历史数据的主要耗时阶段，支持页级并发和 Mongo `BulkWrite`。历史归属以 `archived_reports.domain_id -> assessment.id` 为准，由 MySQL `assessment.testee_id/org_id` 补齐；不信任 legacy 报告中的冗余 `testee_id`：

```bash
# 先用一批 dry-run 验证连接、吞吐与缺失数据；不要把 dry-run 的 checkpoint
# 当成 apply 的续跑点。
go run ./scripts/oneoff/backfill_interpretation_report_catalog \
  --mongo-uri "$MONGO_URI" --mongo-db "$MONGO_DB" \
  --mysql-dsn "$MYSQL_DSN" --source archive \
  --workers 8 --batch-size 1000 --max-docs 100000

# 第一批 apply，从头处理最多 10 万条。
go run ./scripts/oneoff/backfill_interpretation_report_catalog \
  --mongo-uri "$MONGO_URI" --mongo-db "$MONGO_DB" \
  --mysql-dsn "$MYSQL_DSN" --source archive \
  --workers 8 --batch-size 1000 --max-docs 100000 --apply

# 后续批次使用上一批输出的 next_after_id；反复执行直到 complete=true。
go run ./scripts/oneoff/backfill_interpretation_report_catalog \
  --mongo-uri "$MONGO_URI" --mongo-db "$MONGO_DB" \
  --mysql-dsn "$MYSQL_DSN" --source archive \
  --workers 8 --batch-size 1000 --max-docs 100000 \
  --after-id 614332407667044910 --apply

# archive 完成后回填新三对象报告。该阶段仍使用 BulkWrite，但为了保证
# 同一 Assessment 的最新 artifact 确定性胜出，脚本内部串行推进页面。
go run ./scripts/oneoff/backfill_interpretation_report_catalog \
  --mongo-uri "$MONGO_URI" --mongo-db "$MONGO_DB" \
  --source artifact --batch-size 1000 --max-docs 100000 --apply

go run ./scripts/oneoff/backfill_interpretation_report_catalog \
  --mongo-uri "$MONGO_URI" --mongo-db "$MONGO_DB" \
  --verify-only
```

默认 dry-run。每批完成会输出 `complete` 和 `next_after_id`；只有整个并发 wave 成功后 checkpoint 才会前移，失败时可以从输出的 checkpoint 安全重跑。写入是幂等的。

关键参数：

- `--workers`：archive 并发页数，默认 8，建议从 4–8 开始观察 Mongo/MySQL 负载。
- `--batch-size`：单次读取、MySQL `IN` 查询和 Mongo `BulkWrite` 的文档数，默认 1000，最大 10000；旧 `--page-size` 仍作为别名保留。
- `--max-docs`：本次对每个 source 最多处理的文档数，默认 0 表示不限制；用于按 5 万或 10 万条分批执行。
- `--after-id` / `--to-id`：按 `domain_id` 指定可续跑的开闭区间 `(after-id, to-id]`。archive 可将互不重叠的区间交给多个进程；artifact 不应多进程并行。
- `--progress-interval`：进度条刷新间隔，默认 2 秒；`--no-progress` 可关闭。
- `--timeout`：整次命令超时，默认 24 小时；传 `0` 禁用。

进度条显示当前阶段、百分比、处理速率、ETA、最近安全 checkpoint，以及插入、更新、跳过、缺失和失败数。最终摘要会把缺失归属区分为 `missing_assessment`、`missing_testee` 和 `missing_org`。`--verify-only` 也必须传入 MySQL DSN，以分批确认 catalog 的 Assessment 引用仍存在。必须在这些缺失项、`failed`，以及 verify 输出的 `count_mismatch`、`missing_archive`、`wrong_priority`、`dangling_source` 全为 0 后切换目录读取版本。

## Assessment 孤岛 Mongo 文档清理

`cleanup_orphaned_assessment_documents` 使用两条权威关系：

- `archived_reports.domain_id -> assessment.id`；不存在时清理 archive、同 ID legacy report 和 catalog。
- `answersheets.domain_id -> assessment.answer_sheet_id`；不存在时清理答卷和提交幂等记录。

默认 dry-run。答卷可能短暂处于“已保存但 Assessment 尚未创建”状态，因此 answersheets 阶段强制要求 `--answersheet-created-before` 截止线。

```bash
# 先核对报告孤岛数量。
go run ./scripts/oneoff/cleanup_orphaned_assessment_documents \
  --mongo-uri "$MONGO_URI" --mongo-db "$MONGO_DB" --mysql-dsn "$MYSQL_DSN" \
  --source reports --workers 16 --batch-size 1000

# 核对 2026-07-01 之前的孤立答卷。
go run ./scripts/oneoff/cleanup_orphaned_assessment_documents \
  --mongo-uri "$MONGO_URI" --mongo-db "$MONGO_DB" --mysql-dsn "$MYSQL_DSN" \
  --source answersheets --answersheet-created-before 2026-07-01 \
  --workers 16 --batch-size 1000

# 默认 apply 会先备份，再软删除报告/答卷正文；catalog 和幂等记录物理删除。
go run ./scripts/oneoff/cleanup_orphaned_assessment_documents \
  --mongo-uri "$MONGO_URI" --mongo-db "$MONGO_DB" --mysql-dsn "$MYSQL_DSN" \
  --source all --answersheet-created-before 2026-07-01 \
  --workers 16 --batch-size 1000 --apply
```

备份集合使用 `cleanup_bak_orphans_<collection>_<backup-suffix>` 命名。只有确认备份和 dry-run 数量后，才可传 `--hard-delete` 物理删除 report/answersheet 正文；该模式也会扫描此前已经软删除的孤岛，因此应先带 `--hard-delete` 做一次 dry-run，再增加 `--apply`。`--max-docs`、`--after-id` 和 `--to-id` 可用于分批与续跑；续跑 checkpoint 必须取自 apply 输出，不能使用 dry-run checkpoint。

## 验证

Go 脚本的最小编译/测试检查：

```bash
go test ./scripts/oneoff/...
```

Python 脚本的参数帮助：

```bash
python3 scripts/oneoff/enroll_testees_after_date.py --help
```
