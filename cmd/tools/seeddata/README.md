# QS Seed Data Tool

QS 系统测试数据生成工具。

## 30 秒结论

- 只想批量生成测评：运行 `--steps assessment`
- 只想批量创建计划 task：运行 `--steps plan_create_tasks`
- 想在后台长期处理 backlog task：运行 `--steps plan_process_tasks`
- 想把历史时间修正成“按 planned_at 回放”的拟真结果：运行 `--steps plan_fixup_timestamps`
- `plan` 只是兼容旧入口，等价于单次执行 `plan_create_tasks`，再执行 one-shot `plan_process_tasks`

## 先选方案

| 目标 | 推荐步骤 | 是否常驻 | 依赖 |
| --- | --- | --- | --- |
| 只生成测评数据 | `assessment` | 否 | apiserver + collection-server |
| 只创建 task | `plan_create_tasks` | 否 | 本地 MySQL + MongoDB + Redis |
| 长期后台处理 task | `plan_process_tasks` | 是 | apiserver API |
| 修正 task/assessment/report 时间 | `plan_fixup_timestamps` | 否 | 本地 MySQL + MongoDB |
| 一次性 create + process | `plan` | 否 | create 走本地依赖，process 走 apiserver API |

## 运行前准备

- `global.orgId` 必须配置
- `api.baseUrl`、`api.token` 或 `iam.*` 必须可用
- `assessment` 需要 `collectionBaseUrl`
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

## 按方案运行

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

## 步骤边界

### `assessment`

- 只负责提交答卷并生成测评
- 不碰 plan task
- 通过 collection/apiserver 的真实接口完成

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
```
