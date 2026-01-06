# QS Seed Data Tool

QS 系统测试数据生成工具。

## 功能概述

该工具用于通过 RESTful API 生成 QS 系统的测评测试数据：

1. **测评数据** (assessment) - 通过提交量表答卷触发测评生成

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
```

## 执行顺序

所有步骤按以下顺序执行：

1. **assessment** - 通过 collection-server 提交量表答卷并生成测评

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
