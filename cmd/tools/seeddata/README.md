# QS Seed Data Tool

QS 系统测试数据生成工具。

## 本文回答

这份 README 重点回答 3 个问题：

- 我现在想造哪类数据，应该跑哪个 `--steps`
- 常见运行方案下，命令应该怎么写
- `assessment`、`plan_create_tasks`、`plan_process_tasks` 的职责边界是什么

## 30 秒结论

- 只想批量生成测评，运行 `--steps assessment`。
- 想批量创建计划 task，运行 `--steps plan_create_tasks`。
- 想慢慢处理已有 task，运行 `--steps plan_process_tasks`；它默认不会自动退出，适合放到 `tmux` 后台长期运行。
- `plan_process_tasks` 本身就是“处理已有 task”的脚本，所以不再需要单独的“恢复模式”概念。
- plan 相关步骤现在只保留本地 runtime 路径：直接读取本地 MySQL / MongoDB / Redis，不再保留 remote 回退模式。
- `plan` 只是兼容旧入口，等价于单次执行一次 `plan_create_tasks`，然后再执行一次 one-shot `plan_process_tasks`。

## 方案速查

| 目标 | 推荐步骤 | 是否常驻 | 适用说明 |
| --- | --- | --- | --- |
| 只生成测评数据 | `assessment` | 否 | 只提交答卷并生成测评，不碰 plan task |
| 只批量创建 task | `plan_create_tasks` | 否 | 先把 task 数据造出来，暂不处理 task |
| 先造 task，再后台慢慢轮转 | `plan_create_tasks` + `plan_process_tasks` | `plan_process_tasks` 常驻 | 最推荐的 plan 造数方式 |
| 只处理已有 task | `plan_process_tasks` | 是 | 不再创建新 task，只消费 backlog |
| 一次性跑完 create + process | `plan` | 否 | 兼容旧入口，适合小批量单次执行 |

## 运行前准备

- `assessment` 依赖 apiserver 与 collection-server 可访问。
- `plan_create_tasks` / `plan_process_tasks` 依赖脚本所在环境可直连 QS 使用的 MySQL / MongoDB / Redis。
- 配置文件在 `configs/seeddata.yaml`，至少要配置好 `global.orgId`、API 地址和鉴权信息。
- `api-token` 为空时，脚本会尝试使用 `iam` 配置登录并自动刷新 token。

## 公共变量

下面的示例默认先准备这些 shell 变量，后面的命令可以直接复制。

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

如果你不想把数据库密码写进仓库里的 `seeddata.yaml`，推荐把 `local.*` 留空，仅在执行时通过命令行覆盖。

## 按方案运行命令

### 方案 1：只生成测评数据

适用场景：
需要批量提交医学量表答卷并生成测评，不需要 plan task。

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

### 方案 2：只批量创建计划 task

适用场景：
先把 `assessment_planid` 对应的 task 数据造出来，后续再单独处理 task。

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

### 方案 3：先创建 task，再后台长期处理 task

适用场景：
你希望先集中造数，再把 task 轮转脚本放到后台慢慢跑几小时或几天。

第一步，创建 task：

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_create_tasks" \
  --plan-id "$PLAN_ID" \
  --plan-workers 4 \
  --testee-limit 1000 \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB" \
  --local-redis-addr "$REDIS_ADDR" \
  --local-redis-username "$REDIS_USERNAME" \
  --local-redis-password "$REDIS_PASSWORD" \
  --local-redis-db "$REDIS_DB"
```

第二步，单独长期运行 task 处理器：

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_process_tasks" \
  --plan-id "$PLAN_ID" \
  --plan-submit-workers 12 \
  --plan-wait-workers 3 \
  --plan-max-inflight-tasks 120 \
  --plan-expire-rate 0.2 \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB" \
  --local-redis-addr "$REDIS_ADDR" \
  --local-redis-username "$REDIS_USERNAME" \
  --local-redis-password "$REDIS_PASSWORD" \
  --local-redis-db "$REDIS_DB"
```

用 `tmux` 在后台长跑：

```bash
tmux new-session -d -s seed-plan-process "
cd /path/to/qs-server && \
go run ./cmd/tools/seeddata \
  --config '$CFG' \
  --steps 'plan_process_tasks' \
  --plan-id '$PLAN_ID' \
  --plan-submit-workers 12 \
  --plan-wait-workers 3 \
  --plan-max-inflight-tasks 120 \
  --plan-expire-rate 0.2 \
  --local-mysql-dsn '$MYSQL_DSN' \
  --local-mongo-uri '$MONGO_URI' \
  --local-mongo-database '$MONGO_DB' \
  --local-redis-addr '$REDIS_ADDR' \
  --local-redis-username '$REDIS_USERNAME' \
  --local-redis-password '$REDIS_PASSWORD' \
  --local-redis-db '$REDIS_DB'
"
```

### 方案 4：只处理已有 task

适用场景：
task 已经存在，不想再 enroll，只想处理 backlog。

处理整个 plan 下已有 task：

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_process_tasks" \
  --plan-id "$PLAN_ID" \
  --plan-submit-workers 12 \
  --plan-wait-workers 3 \
  --plan-max-inflight-tasks 120 \
  --plan-expire-rate 0.2 \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB" \
  --local-redis-addr "$REDIS_ADDR" \
  --local-redis-username "$REDIS_USERNAME" \
  --local-redis-password "$REDIS_PASSWORD" \
  --local-redis-db "$REDIS_DB"
```

只处理指定 testee 的已有 task：

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_process_tasks" \
  --plan-id "$PLAN_ID" \
  --plan-testee-ids "1001,1002,1003" \
  --plan-submit-workers 12 \
  --plan-wait-workers 3 \
  --plan-max-inflight-tasks 120 \
  --plan-expire-rate 0.2 \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB" \
  --local-redis-addr "$REDIS_ADDR" \
  --local-redis-username "$REDIS_USERNAME" \
  --local-redis-password "$REDIS_PASSWORD" \
  --local-redis-db "$REDIS_DB"
```

### 方案 5：兼容旧入口，一次跑完 create + process

适用场景：
批量不大，希望一次命令直接跑完。

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan" \
  --plan-id "$PLAN_ID" \
  --plan-workers 4 \
  --plan-submit-workers 12 \
  --plan-wait-workers 3 \
  --plan-max-inflight-tasks 120 \
  --plan-expire-rate 0.2 \
  --testee-limit 1000 \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB" \
  --local-redis-addr "$REDIS_ADDR" \
  --local-redis-username "$REDIS_USERNAME" \
  --local-redis-password "$REDIS_PASSWORD" \
  --local-redis-db "$REDIS_DB"
```

## 关键行为说明

### `assessment`

- 仅对医学量表对应的问卷提交答卷。
- 当前自动填充支持 `Radio`、`Checkbox`、`Text`、`Textarea`、`Number`；`Section` 题不会生成答案。
- 使用 apiserver 的 testee 列表接口，因此需要在 `seeddata.yaml` 中配置 `global.orgId`。
- `--testee-page-size` 最大建议不超过 `100`，受 apiserver 参数校验限制。

### `plan_create_tasks`

- 默认计划 ID 是 `614186929759466030`，可通过 `--plan-id` 覆盖。
- 默认会流式扫描 testee，并按约 `1/5` 抽样；不会先把全量 testee 全部留在内存里再抽样。
- 抽样后的 testee 会再按优先级排序：
  `没有 task` 优先，`没有当前 plan 的 task` 次之，然后是 `task 更少的` 优先。
- 传入 `--plan-testee-ids` 后，会跳过随机抽样和全量 testee 扫描，只处理这些 testee。
- 显式传入 `--plan-testee-ids` 时，`--testee-limit` 仍然生效；去重后只取前 N 个。
- `start_date` 默认取 `testee.created_at`；如果普通模式下历史脏数据导致它为空，会依次回退到 `updated_at`、当前日期并打 warning。
- 显式 `--plan-testee-ids` 模式更严格：如果某个 testee 的 `created_at` 是零值，脚本会直接报错。
- `plan_create_tasks` 的职责只有一件事：选 testee、入组、补齐 task；它不负责处理历史 task backlog。

### `plan_process_tasks`

- 不会创建新 task；它只负责调度并处理当前 plan 下已存在的 task。
- 独立运行时默认不会退出，而是持续执行：
  `schedule -> 发现 opened task -> submit/wait -> sleep -> 下一轮`，直到收到 `SIGINT` / `SIGTERM`。
- 因为 `plan_process_tasks` 已经可以单独处理已有 task，所以不再需要单独的“恢复模式”。
- 支持 submit/wait 双阶段流水线：
  `--plan-submit-workers` 控制提交答卷并发，
  `--plan-wait-workers` 控制等待任务完成并发，
  `--plan-max-inflight-tasks` 控制已提交未完成 task 的上限。
- 推荐压测起点：
  `--plan-workers 4`
  `--plan-submit-workers 12`
  `--plan-wait-workers 3`
  `--plan-max-inflight-tasks 120`
- `--plan-expire-rate` 用于控制 `opened` task 中有多少比例会被直接标记为 `expired`，默认 `0.2`。
- 不传 `--plan-testee-ids` 时，会按 plan 维度分页扫描 `opened` task。
- 传了 `--plan-testee-ids` 时，只扫描该范围内的 task。
- 独立运行 `plan_process_tasks` 时，`--testee-limit` 不再影响处理范围。
- 为避免把整个 plan 自动收尾为 `finished`，脚本会故意保留 1 个 `opened` task 不处理，让 plan 维持 `active`。
- 被抽中过期的 `opened` task 会走真实 `ExpireTask` 命令，不会提交答卷，因此最后会形成 `completed` 与 `expired` 混合结果。

### plan 本地 runtime

- plan 相关步骤现在只保留本地 runtime，不再保留 remote 模式。
- 本地连接信息可以写在 `seeddata.yaml` 的 `local.*` 中，也可以通过命令行覆盖：
  `--local-mysql-dsn`
  `--local-mongo-uri`
  `--local-mongo-database`
  `--local-redis-addr`
  `--local-redis-username`
  `--local-redis-password`
  `--local-redis-db`
  `--local-plan-entry-base-url`
- plan 本地 runtime 会把计划查询、量表/问卷查询、testee 查询、入组、定向调度、任务查询、任务过期收回到 seeddata 进程内。
- 但答卷提交流转仍然走真实链路：
  `seeddata -> apiserver admin-submit -> worker -> assessment -> task.completed`。

### `plan`

- `plan` 只是兼容旧入口。
- 它的行为等价于：
  先执行一次 `plan_create_tasks`，
  再对刚刚选中的 testee 执行一次 one-shot `plan_process_tasks`。
- 如果你要长时间慢慢处理 backlog，优先直接使用独立的 `plan_process_tasks`。

### 幂等性

- 所有种子操作都按幂等方式设计。
- 已存在的数据会被更新或跳过，不会因为重复执行而无限创建重复记录。

## 常见问题

### `questionnaire version mismatch`

如果你直接修改了 MongoDB 里的 `scale.questionnaire_version`，而脚本仍然报 `questionnaire version mismatch`，优先排查 apiserver Redis 缓存。

通常需要删除以下 key 后再重试：

- `scale:<scale_code小写>`
- 或带命名空间的 `<cache.namespace>:scale:<scale_code小写>`

## 配置文件示例

详见 `configs/seeddata.yaml`。下面是常见字段示例：

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

testees: []
questionnaires: []
scales: []
```
