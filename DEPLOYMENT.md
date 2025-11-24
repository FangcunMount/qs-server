# QS-Server CI/CD 部署指南

## 🚀 快速开始

本项目已配置完整的 GitHub Actions CI/CD 流程，支持自动化测试、构建和部署。

## 📋 前提条件

### 1. 配置 GitHub Secrets

在开始之前，需要在 GitHub 仓库中配置以下 Secrets：

#### Organization Secrets（组织级别，8个）

```text
✅ SVRA_HOST              - 生产服务器地址
✅ SVRA_USERNAME          - SSH 用户名
✅ SVRA_SSH_KEY           - SSH 私钥
✅ SVRA_SSH_PORT          - SSH 端口（默认 22）
✅ MONGODB_HOST           - MongoDB 地址
✅ MONGODB_PORT           - MongoDB 端口（默认 27017）
✅ REDIS_HOST             - Redis 地址
✅ REDIS_PORT             - Redis 端口（默认 6379）
✅ DOCKERHUB_USERNAME     - Docker Hub 用户名
✅ DOCKERHUB_TOKEN        - Docker Hub 令牌
```

#### Repository Secrets（仓库级别，6个）

```text
✅ MONGODB_USERNAME       - MongoDB 用户名
✅ MONGODB_PASSWORD       - MongoDB 密码
✅ MONGODB_DBNAME         - 数据库名称
✅ REDIS_PASSWORD         - Redis 密码（可选）
✅ JWT_SECRET             - JWT 密钥
✅ WWW_UID / WWW_GID      - 应用用户（可选，默认 1000）
```

### 2. 验证配置

配置完成后，运行验证工作流：

```bash
Actions → Test SSH Connection → Run workflow
```

## 🔄 部署流程

### 自动部署（推荐）

**开发流程**:

1. 在 `develop` 分支开发功能
2. 提交代码并创建 PR 到 `develop`
3. GitHub Actions 自动运行测试和代码检查
4. 合并 PR 后，合并 `develop` 到 `main`
5. 推送到 `main` 分支自动触发完整部署

```bash
# 示例
git checkout develop
git pull origin develop

# 开发完成后
git checkout main
git merge develop
git push origin main

# 🎉 自动触发：Test → Lint → Build → Docker → Deploy
```

### 手动部署

如需手动触发部署：

```bash
Actions → CI/CD Pipeline → Run workflow
  → Branch: main
  → Service: all (或选择 apiserver / collection)
  → Run workflow
```

## 🏗️ 服务架构

### 服务组件

| 服务 | 容器名 | HTTP 端口 | HTTPS 端口 | 配置文件 |
|------|--------|----------|-----------|---------|
| API Server | qs-apiserver | 8081 | 9445 | apiserver.prod.yaml |
| Collection Server | qs-collection-server | 8082 | 9446 | collection-server.prod.yaml |

### 部署目录结构

```text
/opt/qs-server/
├── qs-apiserver/
│   └── configs/
│       ├── apiserver.prod.yaml
│       └── env/
│           └── config.prod.env
└── qs-collection-server/
    └── configs/
        ├── collection-server.prod.yaml
        └── env/
            └── config.prod.env

/data/logs/qs-server/
├── qs-apiserver/
└── qs-collection-server/

/opt/backups/qs-server/
├── qs-apiserver/
├── qs-collection-server/
└── mongodb/
```

## 🔍 监控和维护

### 自动监控

系统会自动执行以下检查：

- ⏰ **每天 01:00** - MongoDB 自动备份
- ⏰ **每 30 分钟** - 服务器健康检查（自动重启 unhealthy 容器）
- ⏰ **每 6 小时** - 快速连通性检查

### 手动检查

```bash
# 快速健康检查
Actions → Ping Runner → Run workflow

# 完整健康检查
Actions → Server Health Check → Run workflow

# 查看数据库状态
Actions → Database Operations → Run workflow → status → mongodb
```

## 💾 数据库管理

### 自动备份

- **时间**: 每天凌晨 01:00（北京时间）
- **保留**: 最近 5 次备份
- **位置**: `/opt/backups/qs-server/mongodb/`

### 手动备份

```bash
Actions → Database Operations → Run workflow
  → Operation: backup
  → Database: mongodb
  → Run workflow
```

### 恢复备份

```bash
# 1. 查看可用备份
Actions → Database Operations → status → mongodb

# 2. 恢复指定备份
Actions → Database Operations → restore → mongodb
  → Backup name: qs_mongodb_backup_20250124_010000.tar.gz
  → Run workflow
```

## 🛠️ 本地开发

### 环境准备

```bash
# 安装依赖
make deps-download

# 安装开发工具
make install-tools

# 检查基础设施
make check-infra
```

### 构建和运行

```bash
# 构建所有服务
make build-all

# 运行所有服务（开发环境）
make run-all

# 查看服务状态
make status-all

# 查看日志
make logs-all

# 停止所有服务
make stop-all
```

### 测试

```bash
# 运行所有测试
make test

# 运行代码检查
make lint

# 运行测试并生成覆盖率报告
make test-coverage
```

### 使用 Air 热重载

```bash
# API Server 热重载
make dev-apiserver

# Collection Server 热重载
make dev-collection

# 查看热重载状态
make dev-status

# 停止热重载
make dev-stop
```

## 📚 更多文档

- **完整 CI/CD 文档**: [.github/workflows/README.md](.github/workflows/README.md)
- **架构设计**: [docs/项目文档/01-软件架构设计总览.md](docs/项目文档/01-软件架构设计总览.md)
- **API 文档**: [docs/apiserver/README.md](docs/apiserver/README.md)
- **Collection Server**: [docs/collection-server/README.md](docs/collection-server/README.md)

## 🚨 故障排查

### 部署失败

```bash
# 1. 查看 GitHub Actions 日志
Actions → CI/CD Pipeline → 查看失败的 job

# 2. SSH 登录服务器查看
ssh user@server
sudo docker ps -a
sudo docker logs --tail 100 qs-apiserver
sudo docker logs --tail 100 qs-collection-server

# 3. 检查健康状态
curl http://localhost:8081/healthz
curl http://localhost:8082/health
```

### 容器无法启动

```bash
# 查看容器状态
sudo docker ps -a --filter "name=qs-"

# 查看容器日志
sudo docker logs --tail 200 qs-apiserver

# 检查配置文件
sudo cat /opt/qs-server/qs-apiserver/configs/env/config.prod.env

# 手动重启
sudo docker restart qs-apiserver
```

### 数据库连接问题

```bash
# 测试 MongoDB 连接
mongosh --host $MONGODB_HOST --port $MONGODB_PORT \
  --username $MONGODB_USERNAME --password $MONGODB_PASSWORD

# 检查容器网络
sudo docker network inspect qs-network

# 查看数据库日志
sudo docker logs qs-apiserver | grep -i mongodb
```

## 📞 获取帮助

### 问题排查顺序

1. **查看 GitHub Actions 日志** - 最详细的错误信息
2. **运行健康检查** - `Actions → Server Health Check`
3. **查看服务器日志** - `sudo docker logs <container>`
4. **验证 Secrets 配置** - 确保所有必需的 Secrets 已配置
5. **查看文档** - `.github/workflows/README.md`

### 支持渠道

- **GitHub Issues**: 提交问题和功能请求
- **Pull Requests**: 提交改进和修复
- **文档**: 查阅项目文档目录

---

**维护**: FangcunMount Team  
**最后更新**: 2025年11月24日
