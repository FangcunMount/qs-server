# 定时任务调度文档

> **版本**：V3.0  
> **更新日期**：2025-01-XX

## 文档索引

### 核心文档

1. **[定时任务调度机制](./01-定时任务调度机制.md)**
   - 系统架构和设计理念
   - 任务执行流程
   - 定时任务列表
   - 添加新任务指南

1. **[架构决策：独立Sync服务vsCrontab](./02-架构决策：独立Sync服务vsCrontab.md)**
   - 架构方案对比
   - 决策理由
   - 适用场景分析

### 生产环境配置

1. **[configs/crontab/README.md](../../../configs/crontab/README.md)**（推荐）
   - 生产环境完整配置指南
   - 脚本使用说明
   - 部署步骤
   - 故障排查

## 快速开始

### 方式一：GitHub Actions 自动部署（推荐）

1. 配置 GitHub Secrets
2. 推送代码到 `main` 分支
3. 自动部署完成

**详细说明**：参考 [GitHub Actions自动部署](./08-GitHub Actions自动部署.md)

### 方式二：手动部署

1. 部署脚本（`api-call.sh`、`refresh-token.sh`）
2. 配置 Crontab（`qs-scheduler`）
3. 配置日志轮转（`logrotate.conf`）

**详细说明**：参考 [configs/crontab/README.md](../../../configs/crontab/README.md)

## 核心特性

- ✅ **自动 Token 管理**：使用 `refresh-token.sh` 和 `api-call.sh` 脚本自动获取和刷新 Token
- ✅ **统一脚本模板**：所有任务使用 `api-call.sh` 统一处理，配置简洁
- ✅ **GitHub Actions 自动部署**：通过 CI/CD 自动部署配置到生产服务器
- ✅ **完善的日志管理**：统一的日志格式和轮转配置

## 定时任务列表

### Statistics 模块

- 同步每日统计（每小时第 0 分）
- 同步累计统计（每小时第 5 分）
- 同步计划统计（每小时第 10 分）
- 校验数据一致性（每小时第 15 分）

### Plan 模块

- 调度待推送任务（每小时第 20 分）

## 相关资源

- **配置文件**：`configs/crontab/`
- **GitHub Actions**：`.github/workflows/deploy-crontab.yml`
- **业务接口**：`internal/apiserver/interface/restful/handler/`

## 更新日志

### V3.0 (2025-01-XX)

- ✅ 实现自动 Token 刷新机制
- ✅ 添加 GitHub Actions 自动部署
- ✅ 完善日志管理和轮转配置
- ✅ 优化脚本和配置文档

### V2.0 (2025-01-XX)

- ✅ 采用 Crontab + HTTP 接口方案
- ✅ 移除独立 sync 服务

### V1.0 (2025-01-XX)

- ✅ 初始版本
