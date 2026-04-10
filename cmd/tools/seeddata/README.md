# QS Seed Data Tool

QS 系统测试数据生成工具。

## 功能概述

该工具用于生成 QS 系统的测评测试数据：

1. **测评数据** (assessment) - 通过提交量表答卷触发测评生成
2. **计划任务创建** (plan_create_tasks) - 根据 `assessment_planid` 选择 testee、入组并批量创建/补齐 task
3. **计划任务处理** (plan_process_tasks) - 调度并处理 task，负责 task 的轮转，包括开启、提交答卷、生成测评和 task 过期

其中 `plan` 入口仍然保留，用于兼容旧用法；它的行为等价于顺序执行一次 `plan_create_tasks` 再执行 `plan_process_tasks`。

## 快速开始

### 前置条件

1. `assessment` 步骤需要 apiserver 与 collection-server 已启动并可访问
2. `plan_create_tasks` / `plan_process_tasks` 默认运行在 `local` 模式，需要脚本所在环境可直连 QS 使用的 MySQL / MongoDB / Redis
3. 配置种子数据文件 `configs/seeddata.yaml`

### 基本用法

```bash
# 使用命令行参数(完整示例)
go run ./cmd/tools/seeddata \
  --api-base-url "http://localhost:18082" \
  --collection-base-url "http://localhost:18083" \
  --api-token "..." \
  --config "./configs/seeddata.yaml"

# 仅依赖 seeddata.yaml（推荐）
go run ./cmd/tools/seeddata --config ./configs/seeddata.yaml

# 启用详细日志
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --verbose
```

### 选择性执行

```bash
# 只生成测评数据（仅医学量表）
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "assessment" \
  --assessment-min 3 \
  --assessment-max 10 \
  --testee-offset 0 \
  --testee-limit 1000 \
  --assessment-scale-categories "cognitive,behavior"

# 第一步：先批量创建/补齐计划 task（默认 plan id: 614186929759466030）
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan_create_tasks" \
  --plan-mode local \
  --local-mysql-dsn "user:password@tcp(127.0.0.1:3306)/qs?charset=utf8mb4&parseTime=True&loc=Local" \
  --local-mongo-uri "mongodb://127.0.0.1:27017" \
  --local-mongo-database "qs" \
  --local-redis-addr "127.0.0.1:6379" \
  --local-redis-username "default" \
  --plan-workers 4 \
  --testee-limit 1000 \
  --plan-id 614186929759466030

# 第二步：单独处理 task，默认会持续轮询直到手动停止，适合放到 tmux 后台慢慢跑
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan_process_tasks" \
  --plan-mode local \
  --local-mysql-dsn "user:password@tcp(127.0.0.1:3306)/qs?charset=utf8mb4&parseTime=True&loc=Local" \
  --local-mongo-uri "mongodb://127.0.0.1:27017" \
  --local-mongo-database "qs" \
  --local-redis-addr "127.0.0.1:6379" \
  --local-redis-username "default" \
  --plan-submit-workers 12 \
  --plan-wait-workers 3 \
  --plan-max-inflight-tasks 120 \
  --plan-expire-rate 0.2 \
  --plan-id 614186929759466030

# 只为指定受试者创建/补齐 task
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan_create_tasks" \
  --plan-mode local \
  --plan-workers 4 \
  --plan-id 614186929759466030 \
  --plan-testee-ids "1001,1002,1003"

# 只处理指定受试者已有 task，不再扩大范围
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan_process_tasks" \
  --plan-mode local \
  --plan-id 614186929759466030 \
  --plan-testee-ids "1001,1002,1003" \
  --plan-submit-workers 12 \
  --plan-wait-workers 3 \
  --plan-max-inflight-tasks 120 \
  --plan-expire-rate 0.2

# 用 tmux 在后台长跑 task 处理脚本
tmux new-session -d -s seed-plan-process '
cd /path/to/qs-server && \
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan_process_tasks" \
  --plan-mode local \
  --plan-id 614186929759466030 \
  --plan-submit-workers 12 \
  --plan-wait-workers 3 \
  --plan-max-inflight-tasks 120 \
  --plan-expire-rate 0.2
'

# 兼容旧入口：单次执行 create + process
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan" \
  --plan-mode local \
  --plan-id 614186929759466030

# 如果脚本所在环境不能直连数据库，也可以继续使用远程 HTTP 模式
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan_process_tasks" \
  --plan-mode remote \
  --plan-id 614186929759466030
```

## 推荐执行顺序

推荐按下面方式使用：

1. **assessment** - 通过 collection-server 提交量表答卷并生成测评
2. **plan_create_tasks** - 先把你想要的 task 数据批量造出来
3. **plan_process_tasks** - 再单独跑 task 处理脚本；它会持续轮询 backlog，适合在后台运行几小时或几天

如果你仍然使用 `plan`，它会在一次执行中串联 `plan_create_tasks -> plan_process_tasks`。

## 幂等性

所有种子操作都是幂等的：

- 已存在的记录会被更新而不是重复创建

多次运行相同配置会更新现有数据而不会创建重复项。

## 测评数据说明

- 仅对 **医学量表** 对应的问卷提交答卷。
- 仅支持 `Section` 与 `Radio` 题型自动填充。
- 使用 apiserver 的测试者列表接口，需要在 `seeddata.yaml` 中配置 `global.orgId`。
- `testee-page-size` 最大为 100（受 apiserver 参数校验限制）。
- token 为空时会使用 `iam` 配置登录并自动刷新 token。

### 计划 task 创建 / 处理说明

- `plan_create_tasks` / `plan_process_tasks` 默认处理计划 `614186929759466030`，可通过 `--plan-id` 覆盖。
- 两个步骤都支持 `--plan-mode local|remote`，默认 `local`。
- `local` 模式所需的 MySQL / MongoDB / Redis 连接信息，既可以写在 `seeddata.yaml` 的 `local.*` 中，也可以通过命令行参数覆盖：
  - `--local-mysql-dsn`
  - `--local-mongo-uri`
  - `--local-mongo-database`
  - `--local-redis-addr`
  - `--local-redis-username`（Redis 6.0+ ACL 场景下通常需要，和 `iam-contracts` 的 `--redis-cache-username` 作用一致）
  - `--local-redis-password`
  - `--local-redis-db`
  - `--local-plan-entry-base-url`
- 如果你不希望把数据库密码写进代码库，推荐把 `seeddata.yaml` 里的 `local.*` 留空，仅在执行脚本时通过命令行传入。
- `local` 模式只把 `plan` 侧收回到 seeddata 进程内：
  - 计划查询
  - 量表/问卷查询
  - testee 查询
  - 入组
  - scoped 调度
  - 查任务 / 任务过期
- `local` 模式下，答卷提交流转仍然是远程真实链路：`seeddata -> apiserver admin-submit -> worker -> assessment -> task.completed`。
- `remote` 模式保留原有 HTTP 实现，适合作为接口链路回归或数据库不可直连时的回退方案。
- 推荐把计划相关能力理解成两个脚本：
  - `plan_create_tasks` 负责选 testee、入组、补齐 task
  - `plan_process_tasks` 负责调度和处理 task 流转
- `plan_create_tasks` 支持 `--plan-workers`，用于控制计划侧工作负载：testee 入组、已有任务扫描、定向调度分批；默认 `1`，建议从 `4` 开始压测。
- `plan_process_tasks` 支持受控的 task 提交流水线：
  - `--plan-submit-workers`：提交答卷的并发 worker 数，默认跟随 `--plan-workers`
  - `--plan-wait-workers`：等待 `task.completed` 的并发 worker 数，默认跟随 `--plan-workers`
  - `--plan-max-inflight-tasks`：已提交但仍在等待 worker/apiserver 消化的最大 task 数；达到上限后，新的提交会阻塞等待，默认根据 submit/wait worker 数自动推导
- 推荐压测起点：
  - `--plan-workers 4`
  - `--plan-submit-workers 12`
  - `--plan-wait-workers 3`
  - `--plan-max-inflight-tasks 120`
- `plan_process_tasks` 和兼容入口 `plan` 支持 `--plan-expire-rate`，用于控制已打开任务中有多少比例会被直接标记为 `expired` 而不是提交答卷；默认 `0.2`，取值范围 `0.0-1.0`。
- `plan_process_tasks` 本身不会创建新 task；它只会处理当前 plan 下已经存在的 task。你可以直接单独运行它，也可以先用 `plan_create_tasks` 造数，再单独运行它。
- 独立运行 `plan_process_tasks` 时，它默认不会自动退出，而是持续执行 `schedule -> 发现 opened task -> submit/wait -> sleep -> 下一轮`，直到收到 `SIGINT` / `SIGTERM`。
- `plan_create_tasks` 支持 `--plan-process-existing-only` 恢复模式：跳过 enroll，只对选中 testee 在该 plan 下已经存在的 task 做状态检查和恢复过滤，适合和兼容入口 `plan` 配合补跑历史遗留数据。
- 恢复模式下，如果没有显式传 `--plan-testee-ids`，脚本会处理 `--testee-limit` 范围内的全部 testee，不再做 `1/5` 随机抽样。
- `plan_create_tasks` 默认会流式扫描受试者列表，并随机抽样约 `1/5` 的 testee；不再先把所有 testee 全量加载到内存后再抽样。抽中的 testee 会按 `created_at` 排序后生成 `start_date`，然后调用 apiserver 的计划入组接口。
- `local` 模式不会通过 HTTP 再请求 plan/testee/scale/questionnaire 接口，因此可以显著减少大批量回填时的 504、超时和重试补偿复杂度。
- `plan_create_tasks` 的 `start_date` 默认取 `testee.created_at`；如果历史脏数据导致 `created_at` 为空，seeddata 会依次回退到 `updated_at`、当前日期，并记录 warning。
- 如果显式传入 `--plan-testee-ids`，两个步骤都只处理这些受试者，跳过随机抽样，也不会再全量扫描 `/api/v1/testees`。
- 显式传入 `--plan-testee-ids` 时，`--testee-limit` 仍然生效；脚本会在去重后只取前 N 个 ID 继续执行。
- 显式模式会更严格：如果 `/api/v1/testees/{id}` 返回的 `created_at` 是零值，脚本会直接报错，不再回退到 `updated_at` 或当前时间。
- 开启 `--plan-process-existing-only` 时，脚本启动会先统计这批 testee 在目标 plan 下已有多少 `pending/opened/completed/expired/canceled` task，并打印到日志；如果一条现有 task 都没有，会直接退出，不会创建新 task。
- 恢复模式会在最开始过滤掉“最后一个 `seq` 对应 task 已经 `completed/expired`”的 testee，避免这些已完成 plan 的 testee 继续进入后续的调度和 task 扫描。
- 恢复模式里，`ExpireTask` 会按幂等方式补偿：如果第一次过期请求超时，但回读任务状态发现它已经进入 `expired/completed/canceled`，脚本会继续往下跑，不会因为重复过期返回 `400 Invalid argument` 而中断整轮恢复。
- `plan_process_tasks` 的 task 执行阶段已经改成 submit/wait 双阶段流水线：提交 worker 会把答卷快速发出去，等待 worker 负责轮询任务完成，中间通过有界 inflight 池削峰填谷；调度接口仍按批次串行调用，不会整机构一次性放量。
- `plan_process_tasks` 在没有传 `--plan-testee-ids` 时，会按 plan 维度分页扫描 `opened` task；传了 `--plan-testee-ids` 时，则只分页扫描该范围内的 task，不再先把整批 task 全量加载到内存。
- `--testee-limit` 只影响 `assessment` 和 `plan_create_tasks`；独立运行 `plan_process_tasks` 时不会再据此截断处理范围。
- 计划任务提交时会携带 `task_id`，让 worker 通过既有链路创建测评并完成任务。
- `plan_process_tasks` 会以 `planned_at` 作为业务时间基准：`open_at` 对齐 `planned_at`，`expire_at` 基于该时间继续推导，`completed_at` 默认使用 `planned_at + 2h`。
- 为避免 seeddata 把整个计划自动收尾为 `finished`，脚本会故意保留 1 个 `opened` task 不处理，让计划保持 `active`。
- 被抽中过期的 `opened` task 会走 apiserver 内部 `ExpireTask` 真实命令，不会提交答卷，因此最终会形成 `completed` 和 `expired` 混合任务。
- `plan_process_tasks` 不会真实发送 `task.opened` 小程序消息；它只会生成对应的任务开放数据，并通过 `source=seeddata` 让 worker 跳过对外通知。
- `plan_create_tasks` 默认按 `created_at` 升序处理所有受试者后再抽样；若要限制范围，可继续使用 `--testee-offset` 和 `--testee-limit`。
- 如果你直接修改了 MongoDB 中的 `scale.questionnaire_version`，而脚本仍提示 `questionnaire version mismatch`，优先排查 apiserver Redis 里的量表详情缓存；通常需要删除 `scale:<scale_code小写>`，或带命名空间的 `<cache.namespace>:scale:<scale_code小写>` 后再重试。

## 配置文件示例

详见 `configs/seeddata.yaml`，包含完整的测试数据配置示例

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

plan:
  mode: "local" # local / remote

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

testees: []
questionnaires: []
scales: []
```

可以通过 `--steps` 参数指定要执行的步骤：
