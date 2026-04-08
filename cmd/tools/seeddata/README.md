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
  --plan-id 614186929759466030

# 只回填指定受试者的测评计划
go run ./cmd/tools/seeddata \
  --config ./configs/seeddata.yaml \
  --steps "plan" \
  --plan-id 614186929759466030 \
  --plan-testee-ids "1001,1002,1003"
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
- 计划回填默认会先按受试者 `created_at` 排序，再随机抽样约 `1/5` 的 testee 生成 `start_date`，然后调用 apiserver 的计划入组、调度、任务查询接口。
- 如果显式传入 `--plan-testee-ids`，则只处理这些受试者，跳过随机抽样，也不会再全量扫描 `/api/v1/testees`。
- 计划任务提交时会携带 `task_id`，让 worker 通过既有链路创建测评并完成任务。
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
