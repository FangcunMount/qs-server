# QS Seed Data Tool

QS 系统测试数据生成工具。

## 功能概述

该工具用于通过 RESTful API 生成 QS 系统的测评测试数据：

1. **测评数据** (assessment) - 通过提交量表答卷触发测评生成
2. **计划回填** (plan) - 按受试者创建时间回填指定测评计划，生成 task.opened 数据并完成对应任务

## 快速开始

### 前置条件

1. apiserver 与 collection-server 已启动并可访问
2. 配置种子数据文件 `configs/seeddata.yaml`（包含 API/IAM 信息）

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

# 只回填测评计划（默认 plan id: 614186929759466030）
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan" \
  --plan-workers 4 \
  --plan-expire-rate 0.2 \
  --plan-id 614186929759466030

# 只回填指定受试者的测评计划
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan" \
  --plan-workers 4 \
  --plan-expire-rate 0.2 \
  --plan-id 614186929759466030 \
  --plan-testee-ids "1001,1002,1003"

# 恢复模式：只处理已有 task，不再新入组
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan" \
  --plan-id 614186929759466030 \
  --plan-testee-ids "1001,1002,1003" \
  --plan-process-existing-only
```

## 执行顺序

所有步骤按以下顺序执行：

1. **assessment** - 通过 collection-server 提交量表答卷并生成测评
2. **plan** - 读取 apiserver 的计划/任务接口，按 testee.created_at 回填计划、调度任务并提交答卷，并生成任务开放数据

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

### 计划回填说明

- `plan` 步骤默认回填计划 `614186929759466030`，可通过 `--plan-id` 覆盖。
- `plan` 步骤支持 `--plan-workers`，用于控制计划入组和任务执行的并发 worker 数；默认 `1`，建议从 `4` 开始压测。
- `plan` 步骤支持 `--plan-expire-rate`，用于控制已打开任务中有多少比例会被直接标记为 `expired` 而不是提交答卷；默认 `0.2`，取值范围 `0.0-1.0`。
- `plan` 步骤支持 `--plan-process-existing-only` 恢复模式：跳过 enroll，只对选中 testee 在该 plan 下已经存在的 task 做状态检查、定向调度和后续处理，适合补跑历史遗留的 `pending/opened` task。
- 计划回填默认会流式扫描受试者列表，并随机抽样约 `1/5` 的 testee；不再先把所有 testee 全量加载到内存后再抽样。抽中的 testee 会按 `created_at` 排序后生成 `start_date`，然后调用 apiserver 的计划入组、调度、任务查询接口。
- `start_date` 默认取 `testee.created_at`；如果历史脏数据导致 `created_at` 为空，seeddata 会依次回退到 `updated_at`、当前日期，并记录 warning。
- 如果显式传入 `--plan-testee-ids`，则只处理这些受试者，跳过随机抽样，也不会再全量扫描 `/api/v1/testees`。
- 显式传入 `--plan-testee-ids` 时，`--testee-limit` 仍然生效；脚本会在去重后只取前 N 个 ID 继续执行。
- 显式模式会更严格：如果 `/api/v1/testees/{id}` 返回的 `created_at` 是零值，脚本会直接报错，不再回退到 `updated_at` 或当前时间。
- 开启 `--plan-process-existing-only` 时，脚本启动会先统计这批 testee 在目标 plan 下已有多少 `pending/opened/completed/expired/canceled` task，并打印到日志；如果一条现有 task 都没有，会直接退出，不会创建新 task。
- 并发只发生在两段：testee 入组、以及按 testee 维度提交/等待任务完成；两段都使用固定数量的 worker 和有缓冲 channel，其余 testee 会在队列中等待；调度接口仍只会串行调用一次。
- 计划任务提交时会携带 `task_id`，让 worker 通过既有链路创建测评并完成任务。
- `plan` 回填会以 `planned_at` 作为业务时间基准：`open_at` 对齐 `planned_at`，`expire_at` 基于该时间继续推导，`completed_at` 默认使用 `planned_at + 2h`。
- 为避免 seeddata 把整个计划自动收尾为 `finished`，脚本会故意保留 1 个 `opened` task 不处理，让计划保持 `active`。
- 被抽中过期的 `opened` task 会走 apiserver 内部 `ExpireTask` 真实命令，不会提交答卷，因此最终会形成 `completed` 和 `expired` 混合任务。
- 计划回填不会真实发送 `task.opened` 小程序消息；它只会生成对应的任务开放数据，并通过 `source=seeddata` 让 worker 跳过对外通知。
- 计划回填默认按 `created_at` 升序处理所有受试者后再抽样；若要限制范围，可继续使用 `--testee-offset` 和 `--testee-limit`。
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

global:
  orgId: 0
  defaultTag: ""

testees: []
questionnaires: []
scales: []
```

可以通过 `--steps` 参数指定要执行的步骤：
