# QS Seed Data Tool

QS 系统测试数据生成工具。

## 30 秒结论

- 只想批量生成测评：运行 `--steps assessment`
- 只想批量创建员工账号与临床医师：运行 `--steps staff,clinician`
- 只想把现有 testee 分配给 clinician：运行 `--steps assign_testees`
- 只想把 actor 历史时间回填成贴近 `testee.created_at` 的拟真时间：运行 `--steps actor_fixup_timestamps`
- 只想给已分配受试者的 clinician 批量创建测评入口：运行 `--steps assessment_entries`
- 只想把入口推进到 `resolve + intake`：运行 `--steps assessment_entry_flow`
- 只想基于入口 intake 结果继续生成真实测评：运行 `--steps assessment_by_entry`
- 只想批量创建计划 task：运行 `--steps plan_create_tasks`
- 想在后台长期处理 backlog task：运行 `--steps plan_process_tasks`
- 想把历史时间修正成“按 planned_at 回放”的拟真结果：运行 `--steps plan_fixup_timestamps`
- 只想主动刷新统计读模型：运行 `--steps statistics_backfill`
- `plan` 只是兼容旧入口，等价于单次执行 `plan_create_tasks`，再执行 one-shot `plan_process_tasks`

## 先选方案

| 目标 | 推荐步骤 | 是否常驻 | 依赖 |
| --- | --- | --- | --- |
| 批量创建员工账号与临床医师 | `staff,clinician` | 否 | apiserver + IAM |
| 批量分配现有 testee 给 clinician | `assign_testees` | 否 | apiserver |
| 修正 actor 历史时间 | `actor_fixup_timestamps` | 否 | 本地 MySQL |
| 批量为已分配受试者的 clinician 创建测评入口 | `assessment_entries` | 否 | apiserver + 本地 MySQL |
| 批量推进入口 resolve + intake | `assessment_entry_flow` | 否 | apiserver + 本地 MySQL |
| 基于入口 intake 结果继续生成真实测评 | `assessment_by_entry` | 否 | apiserver + 本地 MySQL + MongoDB |
| 只生成测评数据 | `assessment` | 否 | apiserver + collection-server |
| 只创建 task | `plan_create_tasks` | 否 | 本地 MySQL + MongoDB + Redis |
| 长期后台处理 task | `plan_process_tasks` | 是 | apiserver API |
| 修正 task/assessment/report 时间 | `plan_fixup_timestamps` | 否 | 本地 MySQL + MongoDB |
| 主动刷新统计读模型 | `statistics_backfill` | 否 | apiserver internal API |
| 一次性 create + process | `plan` | 否 | create 走本地依赖，process 走 apiserver API |

## 运行前准备

- `global.orgId` 必须配置
- `api.baseUrl`、`api.token` 或 `iam.*` 必须可用
- `assessment` 需要 `collectionBaseUrl`
- `actor_fixup_timestamps` 需要本地 `local.mysql_dsn`
- `assessment_entries` 需要本地 `local.mysql_dsn`，因为脚本会把入口时间回填成基于 `testee.created_at` 的结果
- `assessment_entry_flow` 需要本地 `local.mysql_dsn`，因为脚本会回填 `resolve_log` 和入口来源关系时间
- `assessment_by_entry` 需要本地 `local.mysql_dsn`、`local.mongo_uri`、`local.mongo_database`
- `plan_create_tasks` 需要本地 `local.mysql_dsn`、`local.mongo_uri`、`local.mongo_database`、`local.redis_*`
- `plan_fixup_timestamps` 只需要本地 `local.mysql_dsn`、`local.mongo_uri`、`local.mongo_database`
- `plan_process_tasks` 不再初始化本地 runtime，不再要求本地 MySQL / MongoDB / Redis

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
- `assessment_entry_flow` 负责回填 `resolve / intake / assessment_entry` 来源关系时间
- `assessment_by_entry` 负责回填 answersheet / assessment / report 时间

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
- 如果 `clinician.operatorRef` 指向配置里的某个 staff，`clinician` 步骤会自动确保对应员工账号已存在

### 方案 0.5：将现有 testee 分配给 clinician

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "assign_testees"
```

说明：

- `assign_testees` 只操作现有 testee，不创建新 testee
- 支持：
  - `explicit`：把指定 testee IDs 分配给某个 clinician
  - `round_robin`：把一批现有 testee 按轮询分给一组 clinician
- `round_robin` 默认跳过已经存在有效 clinician 关系的 testee，避免重复堆叠

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
  --steps "assessment_entry_flow" \
  --local-mysql-dsn "$MYSQL_DSN"
```

说明：

- 默认扫描当前机构下 active clinician 的 active entry
- 每个 entry 默认取最早的 5 个已分配 testee 做 `resolve + intake`
- 若 testee 没有 `profile_id`，默认跳过；只有 `allowTemporaryTestee=true` 才允许临时建档
- 成功后会回填 `assessment_entry_resolve_log` 和入口来源 relation 时间

### 方案 0.95：从入口 intake 结果继续生成真实测评

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "assessment_by_entry" \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB"
```

说明：

- 只处理 `source_type=assessment_entry` 的 active creator relation
- `scale` target 会正常推进
- `questionnaire` target 只有在类型为 `MedicalScale` 时才会推进
- 纯 survey 问卷会被记录为 skip
- answersheet / assessment / report 时间会回填到入口链路时间轴

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

### 方案 6：主动刷新统计读模型

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "statistics_backfill"
```

说明：

- 固定顺序是 `daily -> accumulated -> plan -> validate`
- 完成后会主动预热 overview / clinicians / entries / periodic / plan 统计接口

## 步骤边界

### `assessment`

- 只负责提交答卷并生成测评
- 不碰 plan task
- 通过 collection/apiserver 的真实接口完成

### `staff`

- 创建或复用员工账号
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

### `clinician`

- 创建或复用临床医师
- 支持三种绑定方式：
  - `operatorRef`：引用 `staffs[].key`
  - `operatorId`
  - 仅 `employeeCode`（不绑定员工，仅用于幂等匹配）
- 幂等匹配顺序：
  - `operatorId`
  - `employeeCode`

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
  - 默认跳过已有关联的 testee；若要强制纳入，用 `includeAlreadyAssigned: true`

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
- 会回填 `resolve_log` 和入口来源 relation 时间

### `assessment_by_entry`

- 只处理 `assessment_entry` 来源的 active creator relation
- 通过真实 answersheet 提交链路触发 assessment
- 会等待 assessment 落库，再回填 answersheet / assessment / report 时间

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

assessmentStatusProfile:
  pending: 0.10
  submitted: 0.15
  interpreted: 0.70
  failed: 0.05
```

说明：

- `assessmentEntryFlow`、`assessmentByEntry` 默认按“当前机构全部 clinician / 全部 entry”工作；只有填了筛选条件时才缩小范围
- `assessmentStatusProfile` 预留给第二阶段 `assessment_fixup_statuses`，本轮步骤不会消费它
