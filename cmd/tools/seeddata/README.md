# QS Seed Data Tool

QS 系统测试数据生成工具。

## 功能概述

该工具用于快速生成 QS 系统的测试数据，包括:

1. **受试者数据** (testee) - 创建受试者档案及相关信息
2. **问卷数据** (questionnaire) - 创建问卷、问题和选项
3. **量表数据** (scale) - 创建医学量表及因子配置

注：答卷和测评数据生成功能待实现

## 快速开始

### 前置条件

1. MySQL 数据库已创建并完成迁移
2. MongoDB 服务已启动
3. 配置种子数据文件 `configs/seeddata.yaml`

### 基本用法

```bash
# 使用命令行参数(完整示例)
go run ./cmd/tools/seeddata \
  --mysql-dsn "root:password@tcp(localhost:3306)/qs_apiserver?parseTime=true&loc=Local" \
  --mongo-uri "mongodb://localhost:27017" \
  --mongo-database "qs_apiserver" \
  --config "./configs/seeddata.yaml"

# 使用环境变量
export MYSQL_DSN="root:password@tcp(localhost:3306)/qs_apiserver?parseTime=true&loc=Local"
export MONGO_URI="mongodb://localhost:27017"
export MONGO_DATABASE="qs_apiserver"
go run ./cmd/tools/seeddata --config ./configs/seeddata.yaml

# 启用详细日志
go run ./cmd/tools/seeddata \
  --mysql-dsn "..." \
  --mongo-uri "..." \
  --mongo-database "..." \
  --config ./configs/seeddata.yaml \
  --verbose
```

### 选择性执行

```bash
# 只生成受试者和问卷数据
go run ./cmd/tools/seeddata \
  --mysql-dsn "..." \
  --mongo-uri "..." \
  --mongo-database "..." \
  --config ./configs/seeddata.yaml \
  --steps "testee,questionnaire"

# 只生成量表数据
go run ./cmd/tools/seeddata \
  --mysql-dsn "..." \
  --mongo-uri "..." \
  --mongo-database "..." \
  --config ./configs/seeddata.yaml \
  --steps "scale"
```

## 执行顺序

所有步骤按以下顺序执行：

1. **testee** - 在 MySQL 中创建受试者记录
2. **questionnaire** - 在 MongoDB 中创建问卷文档
3. **scale** - 在 MongoDB 中创建量表文档

## 幂等性

所有种子操作都是幂等的：

- 已存在的记录会被更新而不是重复创建
- 受试者通过 `name + orgID` 识别
- 问卷通过 `code` 识别
- 量表通过 `code` 识别

多次运行相同配置会更新现有数据而不会创建重复项。

## 配置文件示例

详见 `configs/seeddata.yaml`，包含完整的测试数据配置示例

可以通过 `--steps` 参数指定要执行的步骤：
