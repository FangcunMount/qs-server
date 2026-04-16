# QS Seed Data Tool

QS 系统测试数据生成工具。

如果你想先快速理解整个工具的职责边界、每个 step 是做什么的、应该怎么选步骤，先看：

- [GUIDE.md](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/GUIDE.md)

## 30 秒结论

- 只想批量生成测评：运行 `--steps assessment`
- 只想批量创建员工账号与临床医师：运行 `--steps staff,clinician`
- 只想把现有 testee 分配给 clinician：运行 `--steps assign_testees`
- 只想把 testee.created_at 按年份权重分布到 `2019-03-25 ~ 2026-04-15`：运行 `--steps testee_fixup_created_at`
- 只想把 actor 历史时间回填成贴近 `testee.created_at` 的拟真时间：运行 `--steps actor_fixup_timestamps`
- 只想给已分配受试者的 clinician 批量创建测评入口：运行 `--steps assessment_entries`
- 只想基于现有 testee 生成入口打开 / intake 行为足迹：运行 `--steps assessment_entry_flow`
- 只想基于入口接入结果继续生成 `answersheet -> assessment_episode -> report`：运行 `--steps assessment_by_entry`
- 想每天模拟一批新用户注册、建档、扫码并填报：运行 `--steps daily_simulation`
- 只想批量创建计划 task：运行 `--steps plan_create_tasks`
- 想在后台长期处理 backlog task：运行 `--steps plan_process_tasks`
- 想把历史时间修正成“按 planned_at 回放”的拟真结果：运行 `--steps plan_fixup_timestamps`
- 只想按最新模型重建统计投影：运行 `--steps statistics_backfill`
- `plan` 只是兼容旧入口，等价于单次执行 `plan_create_tasks`，再执行 one-shot `plan_process_tasks`

## 先选方案

| 目标 | 推荐步骤 | 是否常驻 | 依赖 |
| --- | --- | --- | --- |
| 批量创建员工账号与临床医师 | `staff,clinician` | 否 | apiserver + IAM |
| 批量分配现有 testee 给 clinician | `assign_testees` | 否 | apiserver |
| 按年份权重回填 testee.created_at | `testee_fixup_created_at` | 否 | 本地 MySQL |
| 修正 actor 历史时间 | `actor_fixup_timestamps` | 否 | 本地 MySQL |
| 批量为已分配受试者的 clinician 创建测评入口 | `assessment_entries` | 否 | apiserver + 本地 MySQL |
| 基于现有 testee 生成入口打开 / intake 行为足迹 | `assessment_entry_flow` | 否 | apiserver |
| 基于入口接入结果继续生成真实测评服务过程 | `assessment_by_entry` | 否 | apiserver + collection-server + 本地 MySQL |
| 每天模拟一批新用户注册 / 建档 / 扫码 / 填报 | `daily_simulation` | 否（推荐 cron） | apiserver + collection-server + IAM REST/gRPC |
| 只生成测评数据 | `assessment` | 否 | apiserver + collection-server |
| 只创建 task | `plan_create_tasks` | 否 | 本地 MySQL + MongoDB + Redis |
| 长期后台处理 task | `plan_process_tasks` | 是 | apiserver API |
| 修正 task/assessment/report 时间 | `plan_fixup_timestamps` | 否 | 本地 MySQL + MongoDB |
| 按 `behavior_footprint + assessment_episode` 重建统计投影 | `statistics_backfill` | 否 | apiserver + 本地 MySQL |
| 一次性 create + process | `plan` | 否 | create 走本地依赖，process 走 apiserver API |

## 运行前准备

- `global.orgId` 必须配置
- `api.baseUrl`、`api.token` 或 `iam.*` 必须可用
- `assessment` 需要 `collectionBaseUrl`
- `actor_fixup_timestamps` 需要本地 `local.mysql_dsn`
- `testee_fixup_created_at` 需要本地 `local.mysql_dsn`
- `assessment_entries` 需要本地 `local.mysql_dsn`，因为脚本会把入口时间回填成基于 `testee.created_at` 的结果
- `assessment_entry_flow` 只走真实入口公开 API，不再依赖本地 MySQL
- `assessment_by_entry` 需要本地 `local.mysql_dsn`，用于从现有 creator relation 挑 candidate 并等待 assessment 落库
- `daily_simulation` 需要 `iam.loginUrl` 或 `iam.baseUrl`，以及可达的 `iam.grpc.address`
- `plan_create_tasks` 需要本地 `local.mysql_dsn`、`local.mongo_uri`、`local.mongo_database`、`local.redis_*`
- `plan_fixup_timestamps` 只需要本地 `local.mysql_dsn`、`local.mongo_uri`、`local.mongo_database`
- `plan_process_tasks` 不再初始化本地 runtime，不再要求本地 MySQL / MongoDB / Redis
- `statistics_backfill` 需要本地 `local.mysql_dsn`，因为它会直接重建 `analytics_projection_*`

## 公共变量

```bash
export CFG=./configs/seeddata.yaml
export PLAN_ID=614186929759466030

export MYSQL_DSN='user:password@tcp(127.0.0.1:3306)/qs?charset=utf8mb4&parseTime=True&loc=Local'
export MONGO_URI='mongodb://127.0.0.1:27017'
export MONGO_DB='qs'
export REDIS_ADDR='127.0.0.1:6379'
export REDIS_USERNAME='default'
export REDIS_PASSWORD=''
export REDIS_DB=0
```

## 推荐执行顺序

```bash
staff,clinician \
assign_testees \
testee_fixup_created_at \
actor_fixup_timestamps \
assessment_entries \
assessment_entry_flow \
assessment_by_entry \
assessment \
plan_create_tasks \
plan_process_tasks \
plan_fixup_timestamps \
statistics_backfill
```

统一时间原则：

- 所有与 testee 直接相关的时间都从 `testee.created_at` 推导
- 不使用随机时间
- `actor_fixup_timestamps` 负责回填 actor 侧历史时间
- `assessment_entry_flow` 负责通过真实公开入口 API 生成 `entry_opened` / `intake_confirmed` 等行为足迹
- `assessment_by_entry` 负责通过真实答卷提交链路生成 `answersheet_submitted -> assessment_episode -> report_generated`
- `statistics_backfill` 负责按最新模型从 `behavior_footprint + assessment_episode` 重建 `analytics_projection_*`

已删除的旧步骤：

- `assessment_entry_fixup_timestamps`
- `assessment_fixup_timestamps`

这两个步骤会直接修改历史 `resolve_log / relation / answersheet / assessment / report` 时间，在最新 `behavior_footprint + assessment_episode + analytics_projection_*` 模型下会让统计失真，seeddata 已不再支持。

## 按方案运行

### 方案 0：创建员工账号与临床医师

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "staff,clinician"
```

说明：

- `staff` 会按配置创建或复用员工账号
- `clinician` 会按 `operatorRef` 或 `operatorId` 创建或复用临床医师
- `clinicianGenerators` 可以额外批量生成一组 seed clinician；默认会从好大夫推荐专家页抓取姓名，并配套 staff 账号、手机号和邮箱
- 首次成功抓取后，会把生成名单快照写到仓库根目录 `.seeddata-cache/`；后续重复运行优先使用本地快照，避免外站波动导致同一批 seed clinician 身份漂移
- 可通过 `nameSourceUrlPattern`、`nameSourcePages` 控制抓取来源和页数；默认来源是 `https://www.haodf.com/citiao/jibing-xiaoerduodongzheng/tuijian-doctor.html?p=%d`
- 如果 `clinician.operatorRef` 指向配置里的某个 staff，`clinician` 步骤会自动确保对应员工账号已存在

### 方案 0.5：将现有 testee 分配给 clinician

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "assign_testees" \
  --assignment-workers 8
```

说明：

- `assign_testees` 只操作现有 testee，不创建新 testee
- 支持：
- `explicit`：把指定 testee IDs 分配给某个 clinician
- `round_robin`：把一批现有 testee 按轮询分给一组 clinician
- `random`：把一批现有 testee 按稳定哈希随机分给一组 clinician；映射结果可重复执行，不会每次漂移
- `round_robin` 默认跳过已经存在有效 clinician 关系的 testee，避免重复堆叠
- `--assignment-workers` 控制分配并发；默认 `8`，会自动按实际 job 数量收缩

### 方案 0.75：为已分配受试者的 clinician 批量创建测评入口

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "assessment_entries" \
  --local-mysql-dsn "$MYSQL_DSN"
```

说明：

- `assessment_entries` 会扫描当前机构下 `is_active=true` 且 `assigned_testee_count>0` 的 clinician
- 对每个 clinician 按 `assessmentEntryTargets` 配置批量补齐共享入口
- 若已存在同 `targetType + targetCode + targetVersion` 的入口，则直接跳过
- 入口 `created_at / updated_at` 会回填到该 clinician 名下已分配 testee 的最早 `created_at`
- 如果配置 `expiresAfter`，会基于这个入口时间锚点计算 `expires_at`

### 方案 0.8：回填 actor 历史时间

如果你需要先把 testee 的创建时间按年份权重铺到一个更长的历史区间，再让后续 actor / entry / plan 时间都跟着变得更拟真，先运行：

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "testee_fixup_created_at" \
  --local-mysql-dsn "$MYSQL_DSN"
```

说明：

- 只更新当前机构、未删除 testee 的 `created_at`
- 会按当前 `created_at / id` 顺序稳定排序，然后分布到 `2019-03-25 00:00:00 ~ 2026-04-15 23:59:59`
- 分布按年份权重归一化：
  - `2019: 5`
  - `2020: 6`
  - `2021: 11`
  - `2022: 18`
  - `2023: 22`
  - `2024: 25`
  - `2025: 13`
  - `2026: 2`
- 每个年份内部会先按天做稳定加权分布，再在每天内部做稳定随机分散，不再是完全等间距
- 工作日的日分布权重大约是周末的 2 倍，同时工作日之间会保留稳定波动，不会形成“每天都差不多”的平铺效果
- `2019` 从 `2019-03-25` 开始，`2026` 到 `2026-04-15` 结束
- 如果某条记录的 `updated_at < created_at`，会自动把 `updated_at` 追平到新的 `created_at`
- 这一步只改 `testee` 表；如果你要让 actor / entry / plan 相关时间也与新 `created_at` 对齐，后续继续跑 `actor_fixup_timestamps` 以及相关 fixup step

### 方案 0.85：回填 actor 历史时间

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "actor_fixup_timestamps" \
  --local-mysql-dsn "$MYSQL_DSN"
```

说明：

- 只修正非 `source_type=assessment_entry` 的 active relation
- `primary / attending / collaborator` 会分别落到 `testee.created_at + 2h / 4h / 6h`
- clinician 时间取其最早非入口关系时间前 7 天
- staff 时间取 clinician 时间前 1 天

### 方案 0.9：把入口推进到 `resolve + intake`

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "assessment_entry_flow"
```

说明：

- 默认扫描当前机构下 active clinician 的 active entry
- 每个 entry 默认取最早的 5 个已分配 testee 做 `resolve + intake`
- 若 testee 没有 `profile_id`，默认跳过；只有 `allowTemporaryTestee=true` 才允许临时建档
- 只走真实公开入口 API：
  - `GET /api/v1/public/assessment-entries/{token}`
  - `POST /api/v1/public/assessment-entries/{token}/intake`
- 不再回填 `assessment_entry_resolve_log`
- 最新模型下，这一步的产出是异步的：
  - `footprint.entry_opened`
  - `footprint.intake_confirmed`
  - 视业务事实还会产出 `footprint.testee_profile_created` / `footprint.care_relationship_established`

### 方案 0.95：从入口 intake 结果继续生成真实测评

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "assessment_by_entry" \
  --local-mysql-dsn "$MYSQL_DSN"
```

说明：

- 只处理 `source_type=assessment_entry` 的 active creator relation
- `scale` target 会正常推进
- `questionnaire` target 只有在类型为 `MedicalScale` 时才会推进
- 纯 survey 问卷会被记录为 skip
- 通过真实管理员提交答卷链路触发：
  - `footprint.answersheet_submitted`
  - `footprint.assessment_created`
  - `footprint.report_generated`
- 不再本地补建 assessment，也不再手工回写 answersheet / assessment / report 时间
- 对最新模型来说，这一步会自然形成：
  - `assessment_episode`
  - `analytics_projection_*`

### 方案 1：只生成测评

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "assessment" \
  --assessment-min 3 \
  --assessment-max 10 \
  --assessment-workers 10 \
  --assessment-submit-workers 10 \
  --testee-page-size 100 \
  --testee-offset 0 \
  --testee-limit 1000 \
  --assessment-scale-categories "cognitive,behavior"
```

### 方案 2：只创建计划 task

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_create_tasks" \
  --plan-id "$PLAN_ID" \
  --plan-workers 4 \
  --testee-page-size 100 \
  --testee-offset 0 \
  --testee-limit 1000 \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB" \
  --local-redis-addr "$REDIS_ADDR" \
  --local-redis-username "$REDIS_USERNAME" \
  --local-redis-password "$REDIS_PASSWORD" \
  --local-redis-db "$REDIS_DB"
```

### 方案 3：后台长期处理 backlog task

`plan_process_tasks` 现在是 API-only 轻量进程：

- 不再启动本地 apiserver container
- 不再直连本地 MySQL / MongoDB / Redis
- 适合放在 `tmux` 后台常驻

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_process_tasks" \
  --plan-id "$PLAN_ID" \
  --plan-submit-workers 8 \
  --plan-wait-workers 4 \
  --plan-max-inflight-tasks 64 \
  --plan-submit-queue-size 32 \
  --plan-submit-qps 4 \
  --plan-submit-burst 8 \
  --plan-expire-rate 0.2
```

`tmux` 后台示例：

```bash
tmux new-session -d -s seed-plan-process "
cd /path/to/qs-server && \
go run ./cmd/tools/seeddata \
  --config '$CFG' \
  --steps 'plan_process_tasks' \
  --plan-id '$PLAN_ID' \
  --plan-submit-workers 8 \
  --plan-wait-workers 4 \
  --plan-max-inflight-tasks 64 \
  --plan-submit-queue-size 32 \
  --plan-submit-qps 4 \
  --plan-submit-burst 8 \
  --plan-expire-rate 0.2
"
```

只处理指定 testee 的 backlog：

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_process_tasks" \
  --plan-id "$PLAN_ID" \
  --plan-testee-ids "1001,1002,1003" \
  --plan-submit-workers 8 \
  --plan-wait-workers 4 \
  --plan-max-inflight-tasks 64 \
  --plan-submit-queue-size 32 \
  --plan-submit-qps 4 \
  --plan-submit-burst 8 \
  --plan-expire-rate 0.2
```

### 方案 4：处理完成后再修正历史时间

`plan_process_tasks` 现在只走真实业务语义。  
如果你要把结果修正成“按 `planned_at` 回放历史”的拟真时间，再单独执行 `plan_fixup_timestamps`。

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_fixup_timestamps" \
  --plan-id "$PLAN_ID" \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB"
```

只修正指定 testee：

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_fixup_timestamps" \
  --plan-id "$PLAN_ID" \
  --plan-testee-ids "1001,1002,1003" \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB"
```

### 方案 5：兼容旧入口，一次跑完 create + process

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan" \
  --plan-id "$PLAN_ID" \
  --plan-workers 4 \
  --plan-submit-workers 8 \
  --plan-wait-workers 4 \
  --plan-max-inflight-tasks 64 \
  --testee-limit 1000 \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB" \
  --local-redis-addr "$REDIS_ADDR" \
  --local-redis-username "$REDIS_USERNAME" \
  --local-redis-password "$REDIS_PASSWORD" \
  --local-redis-db "$REDIS_DB"
```

注意：

- `plan` 不会自动串联 `plan_fixup_timestamps`
- 如果你需要历史时间拟真，要在 `plan` 之后手动再跑一次 `plan_fixup_timestamps`

### 方案 6：按最新模型重建统计投影

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --local-mysql-dsn "$MYSQL_DSN" \
  --steps "statistics_backfill"
```

说明：

- 不再调用旧的 internal sync API
- 会先等待 analytics projector 基本空闲，再直接按最新模型重建：
  - `behavior_footprint`
  - `assessment_episode`
  - `analytics_projection_org_daily`
  - `analytics_projection_clinician_daily`
  - `analytics_projection_entry_daily`
- 固定行为是：
  - 检查 `analytics_projector_checkpoint(status=processing)`
  - 读取 `analytics_pending_event`
  - 清空并重建 `analytics_projection_*`
  - 预热 overview / clinicians / entries / periodic / plan 统计接口

## 步骤边界

### `assessment`

- 只负责提交答卷并生成测评
- 不碰 plan task
- 通过 collection/apiserver 的真实接口完成
- 现在按 `testee` 做确定性补齐，不再每次随机追加
- 同一个 testee 每次都会推导出同一批目标量表，并且只提交缺失的 `questionnaire_code`

### `staff`

- 创建、复用或更新员工账号
- 新建账号模式要求：
  - `name`
  - `phone`
  - `password`
  - `roles`
- 复用已有 IAM 用户模式要求：
  - `userId`
  - `name`
  - `roles`
- 幂等匹配顺序：
  - `userId`
  - `phone`
  - `email`
- 命中已有 staff 且配置发生漂移时，会同步更新：
  - `name`
  - `email`
  - `phone`
  - `roles`
  - `isActive`

### `clinician`

- 创建、复用或更新临床医师
- 支持三种绑定方式：
  - `operatorRef`：引用 `staffs[].key`
  - `operatorId`
  - 仅 `employeeCode`（不绑定员工，仅用于幂等匹配）
- 也支持通过 `clinicianGenerators` 批量生成 clinician，并默认从好大夫推荐专家页抓取姓名，再配套 staff 账号、手机号和邮箱
- 生成 staff 邮箱默认按“姓名全拼 + 固定序号@域名”构造，例如 `zhangyiwen001@fangcunmount.com`，避免和现网真实账号撞邮箱
- 首次成功抓取后，会把生成名单快照写到仓库根目录 `.seeddata-cache/`；后续重复运行优先使用本地快照，避免外站波动导致同一批 seed clinician 身份漂移
- 幂等匹配顺序：
  - `operatorId`
  - `employeeCode`
- 命中已有 clinician 且配置发生漂移时，会同步更新：
  - `name`
  - `department`
  - `title`
  - `clinicianType`
  - `employeeCode`
  - `isActive`
  - `operator` 绑定（仅当配置显式提供 `operatorRef` / `operatorId`）

### `assign_testees`

- 只通过管理员关系接口建立 clinician-testee 关系
- 支持关系类型：
  - `primary`
  - `attending`
  - `collaborator`
- `strategy=explicit`
  - 需要 `clinicianRef` 或 `clinicianId`
  - 需要 `testeeIds`
- `strategy=round_robin`
  - 需要 `clinicianRefs` / `clinicianIds`
  - 可选 `testeeOffset`、`testeeLimit`、`testeePageSize`
  - 默认跳过已存在 active `primary / attending / collaborator` 关系的 testee；若要强制纳入，用 `includeAlreadyAssigned: true`
- `strategy=random`
  - 需要 `clinicianRefs` / `clinicianIds`
  - 可选 `testeeOffset`、`testeeLimit`、`testeePageSize`
  - 按 `assignment.key + testee_id` 做稳定随机分配，兼容重复运行

### `assessment_entries`

- 扫描当前机构下 `is_active=true` 且 `assigned_testee_count>0` 的 clinician
- 只创建 clinician 共享入口，不按 testee 单独建入口
- 目标配置来自 `assessmentEntryTargets`
- 幂等键：
  - `clinician_id`
  - `targetType`
  - `targetCode`
  - `targetVersion`
- 已存在的入口无论 active 还是 inactive，都视为“已存在”并跳过
- 时间规则：
  - 入口 `created_at / updated_at` 会回填到该 clinician 名下 active relation 对应 testee 的最早 `created_at`
  - 同一 clinician 下多个 target 会按配置顺序做固定分钟偏移，避免完全相同时间
  - `expiresAfter` 会基于这个时间锚点计算
  - `expiresAt` 仍可显式给绝对时间；若早于推导出的 `created_at`，脚本会直接报错

### `actor_fixup_timestamps`

- 只依赖本地 MySQL
- 只修正非 `assessment_entry` 来源的 active clinician relation
- 会继续回填 clinician / staff 的 `created_at / updated_at`

### `assessment_entry_flow`

- 复用公开接口：
  - `GET /api/v1/public/assessment-entries/{token}`
  - `POST /api/v1/public/assessment-entries/{token}/intake`
- 只从已有 access relation 中挑 testee
- 若 `entry + testee` 已存在 active creator relation，则直接跳过
- 不回填任何旧日志或 relation 时间
- 负责生成真实的行为足迹入口：
  - `footprint.entry_opened`
  - `footprint.intake_confirmed`
  - 可选的 `footprint.testee_profile_created`
  - 可选的 `footprint.care_relationship_established`

### `assessment_by_entry`

- 只处理 `assessment_entry` 来源的 active creator relation
- 通过真实 answersheet 提交链路触发 assessment
- 会等待 assessment 落库
- 不再本地补建 assessment，也不再手工回写 answersheet / assessment / report 时间
- 负责生成真实的测评服务过程：
  - `footprint.answersheet_submitted`
  - `assessment_episode`
  - `footprint.assessment_created`
  - `footprint.report_generated`

### `assessment_entry_fixup_timestamps`

- 已删除
- 原因：直接改 `assessment_entry_resolve_log / relation / assessment / report` 时间会污染最新的 `behavior_footprint + assessment_episode + analytics_projection_*`

### `assessment_fixup_timestamps`

- 已删除
- 原因：直接改历史 assessment/report 时间不会同步修复行为足迹和测评服务过程，会让统计与业务真相分叉

### `plan_create_tasks`

- 只负责选 testee、入组、补齐 task
- 使用本地 runtime
- 普通模式会流式扫描 testee，不会先把全量 testee 常驻内存
- 抽样后优先级是：
  - 没有 task 的 testee 优先
  - 没有当前 plan task 的 testee 次之
  - 总 task 更少的 testee 更靠前
- `start_date` 现在只认 `testee.created_at`
- 普通模式遇到 `created_at=0` 的 testee：跳过并打 warning
- 显式 `--plan-testee-ids` 模式遇到 `created_at=0`：直接报错

### `plan_process_tasks`

- 只负责调度和处理已有 task
- 使用 apiserver API，不使用本地 MySQL / MongoDB / Redis
- 默认常驻，不自动退出
- 处理顺序是：
  - 先消费已有 `opened` backlog
  - `opened` 不足时，再按窗口调度 `due pending`
  - 再进入 submit / wait 双阶段 pipeline
- 提交侧自带缓冲队列和限速：
  - `--plan-submit-queue-size`
  - `--plan-submit-qps`
  - `--plan-submit-burst`
- `--testee-limit` 不影响 `plan_process_tasks`
- `--plan-testee-ids` 只用于限制处理范围

### `plan_fixup_timestamps`

- 只做离线时间修正
- 不调用新的业务命令
- 直接定向更新 MySQL / MongoDB
- 目标范围：
  - `assessment_task`
  - `assessment`
  - `answersheet`
  - `interpret_report`
- 默认规则：
  - `task.open_at = planned_at`
  - `task.expire_at = planned_at + ttl`
  - `completed_at = planned_at + 5m`
  - `interpret_at = completed_at + 30s`

## 常见问题

### 为什么 `plan_process_tasks` 不再接收本地连接参数

因为它已经切成轻量进程路径，只通过 apiserver API 驱动 task 流转。  
本地 MySQL / MongoDB / Redis 现在只属于：

- `plan_create_tasks`
- `plan_fixup_timestamps`

### 为什么 `plan_process_tasks` 不再做历史时间拟真

因为 `seeddata` 的一次性造数语义不应该污染 qs-server 主业务。  
现在主链路只做真实业务处理；如果需要历史拟真，再单独跑 `plan_fixup_timestamps`。

### `questionnaire version mismatch`

如果你直接改了 MongoDB 里的 `scale.questionnaire_version`，但脚本仍然报错，优先排查 apiserver Redis 缓存。

通常需要删除：

- `scale:<scale_code小写>`
- 或 `<cache.namespace>:scale:<scale_code小写>`

## 配置文件示例

```yaml
api:
  baseUrl: "http://localhost:18082"
  collectionBaseUrl: "http://localhost:18083"
  token: ""
  retry:
    maxRetries: 3
    minDelay: "200ms"
    maxDelay: "5s"

iam:
  loginUrl: "https://iam.example.com/api/v1/authn/login"
  username: "your-username"
  password: "your-password"

local:
  mysql_dsn: "user:password@tcp(127.0.0.1:3306)/qs?charset=utf8mb4&parseTime=True&loc=Local"
  mongo_uri: "mongodb://127.0.0.1:27017"
  mongo_database: "qs"
  redis_addr: "127.0.0.1:6379"
  redis_username: ""
  redis_password: ""
  redis_db: 0
  plan_entry_base_url: "https://collect.example.com/entry"

global:
  orgId: 0
  defaultTag: ""

staffs:
  - key: "doctor_shi"
    name: "史老师"
    phone: "+8618000000001"
    email: "shi@example.com"
    password: "Doctor@123"
    roles: ["qs:staff", "qs:evaluator"]
    isActive: true

clinicians:
  - key: "clinician_shi"
    operatorRef: "doctor_shi"
    name: "史老师"
    department: "儿童心理健康科"
    title: "主任医师"
    clinicianType: "doctor"
    employeeCode: "CLN001"
    isActive: true

testeeAssignments:
  - key: "primary_for_shi"
    strategy: "explicit"
    relationType: "primary"
    clinicianRef: "clinician_shi"
    testeeIds: ["10001", "10002"]

  - key: "attending_pool"
    strategy: "round_robin"
    relationType: "attending"
    clinicianRefs: ["clinician_shi"]
    testeeOffset: 0
    testeeLimit: 100
    testeePageSize: 100
    includeAlreadyAssigned: false

assessmentEntryTargets:
  - key: "sdq"
    targetType: "questionnaire"
    targetCode: "sdq"
    targetVersion: "v1"
    expiresAfter: "180d"

  - key: "mchat"
    targetType: "scale"
    targetCode: "mchat"
    targetVersion: ""

assessmentEntryFlow:
  clinicianRefs: []
  clinicianIds: []
  entryIDs: []
  maxIntakesPerEntry: 5
  allowTemporaryTestee: false

assessmentByEntry:
  clinicianRefs: []
  clinicianIds: []
  entryIDs: []
  maxAssessmentsPerEntry: 5

dailySimulation:
  countPerRun: 20
  workers: 4
  runDate: ""
  clinicianRef: "clinician_shi"
  targetType: "scale"
  targetCode: "3adyDE"
  targetVersion: ""
  userPassword: "DailySim@123"
  userPhonePrefix: "+86199"
  userEmailDomain: "fangcunmount.com"
  guardianRelation: "guardian"
  testeeSource: "daily_simulation"
  testeeTags: ["seed", "daily-simulation"]
  isKeyFocus: false

assessmentStatusProfile:
  pending: 0.10
  submitted: 0.15
  interpreted: 0.70
  failed: 0.05
```

说明：

- `assessmentEntryFlow`、`assessmentByEntry` 默认按“当前机构全部 clinician / 全部 entry”工作；只有填了筛选条件时才缩小范围
- `dailySimulation` 默认使用当天日期做稳定用户生成；同一天重复运行会复用同一批 guardian / child / testee，不会无限膨胀
- `dailySimulation` 如果未指定 `entryId`，会按 `clinicianRef/clinicianId + targetType/targetCode/targetVersion` 自动确保一个 active entry
- `dailySimulation` 依赖 IAM gRPC；`iam.grpc.address` 必须指向可达的 IAM gRPC 端点
- `assessmentStatusProfile` 预留给第二阶段 `assessment_fixup_statuses`，本轮步骤不会消费它

## 方案 0.97：每天模拟一批新用户注册、建档、扫码和填报

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "daily_simulation,statistics_backfill"
```

说明：

- 每个模拟用户都会走一条完整链路：注册 guardian user -> 创建 child -> 创建 testee -> 扫码指定 clinician entry -> intake -> 填写目标问卷/量表
- `countPerRun` 控制每天新增多少用户
- `runDate` 为空时默认取当天；同一天重复执行会复用同一批稳定账号和 testee
- `statistics_backfill` 建议和 `daily_simulation` 一起跑，用当前 `behavior_footprint + assessment_episode` 重建统计投影

### 后台执行脚本

仓库已提供：

```bash
./scripts/run_daily_simulation.sh
```

默认行为：

- 运行 `daily_simulation,statistics_backfill`
- 默认配置文件：`./configs/seeddata.yaml`
- 默认日志文件：`./logs/seeddata-daily-simulation.log`

可用环境变量：

- `SEEDDATA_CONFIG`
- `SEEDDATA_STEPS`
- `SEEDDATA_GO`
- `SEEDDATA_LOG_FILE`

cron 示例：

```cron
0 2 * * * cd /path/to/qs-server && ./scripts/run_daily_simulation.sh
```
