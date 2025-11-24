# GitHub Actions Workflows

本项目使用 GitHub Actions 实现自动化 CI/CD 流程，采用 Docker 容器化部署架构。

## 📋 目录

- [工作流概览](#工作流概览)
- [环境配置](#环境配置)
- [Secrets 配置](#secrets-配置)
- [使用指南](#使用指南)
- [故障排查](#故障排查)

---

## 工作流概览

### 1. **cicd.yml** - 主 CI/CD 流程

- **触发方式**:
  - Push 到 main/develop 分支
  - Pull Request 到 main 分支
  - 手动触发 (workflow_dispatch)
- **运行时间**: ~15-20 分钟
- **执行流程**:

```text
Validate Secrets (验证配置)
  ↓
Test (单元测试) ━━━┓
                   ┣━━→ Parallel
Lint (代码检查) ━━━┛
  ↓
Build (并行构建 apiserver + collection)
  ↓
Docker (并行构建镜像) ← 仅 main 分支
  ↓
Deploy (并行部署服务) ← 仅 main 分支
  - deploy-apiserver
  - deploy-collection
  ↓
Health Check (健康验证)
```

**服务组件**:

- **qs-apiserver**: 问卷量表 API 服务器 (端口 8081/9445)
- **qs-collection-server**: 问卷收集服务器 (端口 8082/9446)

**部署特性**:

- 支持选择性部署（全部、API、Collection）
- 自动备份配置文件
- 滚动更新零停机
- 健康检查自动验证
- 失败自动回滚

---

### 2. **test-ssh.yml** - SSH 连接测试

- **触发方式**: 手动触发
- **运行时间**: ~1 分钟
- **用途**: 验证 SSH 配置和服务器状态

**检查内容**:

- GitHub Runner 信息
- SSH 连接测试
- 时区信息验证
- 系统信息
- Docker 状态
- QS-Server 服务状态
- 资源使用情况

---

### 3. **server-check.yml** - 服务器健康检查

- **触发方式**:
  - 自动触发: 每 30 分钟执行一次
  - 手动触发
- **运行时间**: ~3-5 分钟
- **检查内容**:

**系统健康**:

- CPU、内存、磁盘使用率
- 系统负载
- Top 进程

**Docker 服务**:

- Docker daemon 状态
- QS-Server 容器运行状态
- 容器健康检查状态
- **自动恢复**: unhealthy 容器自动重启

**API 健康**:

- API Server HTTP/HTTPS 端点
- Collection Server HTTP/HTTPS 端点

**数据库与缓存**:

- MongoDB 连接日志检查
- Redis 连接日志检查

---

### 4. **ping-runner.yml** - 快速连通性检查

- **触发方式**:
  - 自动触发: 每 6 小时执行一次
  - 手动触发
- **运行时间**: ~1-2 分钟
- **检查内容**:

**生产服务器**:

- 系统状态
- 资源概览
- Docker 服务状态
- QS-Server 容器状态

**API 健康**:

- API Server 快速检查
- Collection Server 快速检查

---

### 5. **db-ops.yml** - 数据库操作

- **触发方式**:
  - **自动触发**: 每天北京时间凌晨 01:00 自动备份
  - **手动触发**: 支持 backup/restore/status 操作
- **运行时间**: 2-10 分钟
- **支持操作**:
  - `backup`: 备份 MongoDB（保留最近 5 次备份）
  - `restore`: 从指定备份恢复
  - `status`: 查看数据库状态和可用备份

**自动备份策略**:

```yaml
时间: 每天北京时间 01:00
保留: 最近 5 次备份
位置: /opt/backups/qs-server/mongodb/
格式: qs_mongodb_backup_YYYYMMDD_HHMMSS.tar.gz
```

---

## 工作流时间表

| 工作流 | 触发方式 | 频率 | 用途 |
|--------|---------|------|------|
| **cicd.yml** | push/PR/手动 | 按需 | 持续集成和部署 |
| **db-ops.yml** | **自动**/手动 | **每天 01:00** | 数据库备份和操作 |
| **server-check.yml** | 自动/手动 | 每 30 分钟 | 深度健康检查 |
| **ping-runner.yml** | 自动/手动 | 每 6 小时 | 快速连通性检查 |
| **test-ssh.yml** | 仅手动 | - | SSH 和环境验证 |

---

## 环境配置

### 当前架构

```text
开发环境 (MacBook)
    ↓ git push
GitHub (CI/CD)
    ↓ Docker deploy
生产环境 (SVRA)
  ├─ Docker: qs-apiserver (8081/9445)
  ├─ Docker: qs-collection-server (8082/9446)
  ├─ MongoDB: RDS
  ├─ Redis: Container
  └─ NSQ: Optional
```

### 技术栈

**开发与构建**:

- **Go**: 1.24
- **框架**: Gin
- **构建**: Docker multi-stage build
- **镜像仓库**: GitHub Container Registry (ghcr.io) + Docker Hub

**部署架构**:

- **容器化**: Docker
- **服务器**: 单台生产服务器 (SVRA)
- **网络**: Docker network (qs-network)
- **端口映射**:
  - API Server: 8081→9080(HTTP), 9445→9444(HTTPS)
  - Collection: 8082→9080(HTTP), 9446→9444(HTTPS)

**数据存储**:

- **MongoDB**: RDS 托管服务
- **Redis**: Docker 容器
- **NSQ**: 可选消息队列

---

## Secrets 配置

### 配置位置

`Settings` → `Secrets and variables` → `Actions`

### 必需的 Secrets

#### Organization Secrets（组织级别，共享配置）

**服务器连接**:

| Secret 名称 | 说明 | 示例值 |
|------------|------|--------|
| `SVRA_HOST` | 生产服务器 IP/域名 | `192.168.1.100` |
| `SVRA_USERNAME` | SSH 登录用户名 | `deploy` |
| `SVRA_SSH_KEY` | SSH 私钥（完整） | 见 SSH 配置 |
| `SVRA_SSH_PORT` | SSH 端口 | `22` |
| `SVRA_SUDO_PASSWORD` | sudo 密码（可选） | - |

**基础设施连接**:

| Secret 名称 | 说明 | 示例值 |
|------------|------|--------|
| `MONGODB_HOST` | MongoDB 服务器地址 | `mongodb.example.com` |
| `MONGODB_PORT` | MongoDB 端口 | `27017` |
| `REDIS_HOST` | Redis 服务器地址 | `localhost` |
| `REDIS_PORT` | Redis 端口 | `6379` |
| `NSQ_NSQD_HOST` | NSQ NSQD 地址（可选） | `localhost` |
| `NSQ_NSQD_PORT` | NSQ NSQD 端口（可选） | `4150` |

**Docker Hub**:

| Secret 名称 | 说明 |
|------------|------|
| `DOCKERHUB_USERNAME` | Docker Hub 用户名 |
| `DOCKERHUB_TOKEN` | Docker Hub 访问令牌 |

#### Repository Secrets（仓库级别，敏感信息）

**数据库凭证**:

| Secret 名称 | 说明 | 示例值 |
|------------|------|--------|
| `MONGODB_USERNAME` | MongoDB 用户名 | `qs_user` |
| `MONGODB_PASSWORD` | MongoDB 密码 | `***` |
| `MONGODB_DBNAME` | 数据库名称 | `qs_db` |

**其他凭证**:

| Secret 名称 | 说明 |
|------------|------|
| `REDIS_PASSWORD` | Redis 密码（可选） |
| `JWT_SECRET` | JWT 密钥 |

**部署配置**（可选）:

| Secret 名称 | 说明 | 默认值 |
|------------|------|-------|
| `WWW_UID` | 应用用户 UID | `1000` |
| `WWW_GID` | 应用用户 GID | `1000` |

---

## 使用指南

### 日常开发流程

#### 1. 功能开发（develop 分支）

```bash
# 创建功能分支
git checkout -b feature/new-feature develop

# 开发并本地测试
make test
make lint
make build-all

# 提交代码
git add .
git commit -m "feat: add new feature"
git push origin feature/new-feature

# 创建 PR 到 develop 分支
# GitHub 自动运行: test + lint
```

#### 2. 发布到生产（main 分支）

```bash
# 合并 develop 到 main
git checkout main
git merge develop
git push origin main

# 自动触发完整 CI/CD 流程:
# 1. Validate Secrets
# 2. Test + Lint (并行)
# 3. Build (并行构建两个服务)
# 4. Docker Build & Push (并行)
# 5. Deploy (并行部署两个服务)
# 6. Health Check
```

#### 3. 选择性部署

```bash
# 仅部署 API Server
Actions → CI/CD Pipeline → Run workflow
  → Service: apiserver
  → Run

# 仅部署 Collection Server
Actions → CI/CD Pipeline → Run workflow
  → Service: collection
  → Run

# 部署所有服务
Actions → CI/CD Pipeline → Run workflow
  → Service: all
  → Run
```

### 数据库管理

#### 自动备份

- **时间**: 每天北京时间凌晨 01:00
- **保留**: 最近 5 次备份
- **位置**: `/opt/backups/qs-server/mongodb/`
- **无需手动干预**

#### 手动备份

```bash
Actions → Database Operations → Run workflow
  → Operation: backup
  → Database: mongodb
  → Run
```

#### 恢复数据库

```bash
# 1. 查看可用备份
Actions → Database Operations → Run workflow
  → Operation: status
  → Database: mongodb

# 2. 记录要恢复的备份文件名
# 例如: qs_mongodb_backup_20250124_010000.tar.gz

# 3. 执行恢复
Actions → Database Operations → Run workflow
  → Operation: restore
  → Database: mongodb
  → Backup name: qs_mongodb_backup_20250124_010000.tar.gz
  → Run

# ⚠️ 注意: 5 秒延迟给你反悔的机会
```

### 监控和告警

#### 查看工作流状态

访问: `https://github.com/FangcunMount/qs-server/actions`

**自动监控时间表**:

- ⏰ **01:00** (北京时间) - 数据库自动备份
- ⏰ **每 30 分钟** - 服务器健康检查
- ⏰ **每 6 小时** - 快速连通性检查

---

## 故障排查

### 常见问题

#### 1. SSH 连接失败

**排查步骤**:

```bash
# 1. 验证 SSH 配置
Actions → Test SSH Connection → Run workflow

# 2. 检查 Secrets
Settings → Secrets → 确认 SVRA_* 存在

# 3. 测试本地连接
ssh -i ~/.ssh/your_key user@server-host

# 4. 检查服务器日志
ssh user@server "sudo journalctl -u ssh -n 50"
```

#### 2. 部署失败 - 健康检查超时

**排查步骤**:

```bash
# 1. 检查容器状态
Actions → Ping Runner → Run workflow

# 2. SSH 登录查看日志
ssh user@server
sudo docker logs --tail 100 qs-apiserver
sudo docker logs --tail 100 qs-collection-server

# 3. 检查端口绑定
sudo docker ps
sudo netstat -tlnp | grep -E "8081|8082|9445|9446"

# 4. 手动测试 API
curl http://localhost:8081/healthz
curl http://localhost:8082/health
```

#### 3. 数据库连接失败

**排查步骤**:

```bash
# 1. 验证配置
Actions → Database Operations → status

# 2. 测试连接
ssh user@server
mongosh --host $MONGODB_HOST --port $MONGODB_PORT \
  --username $MONGODB_USERNAME --password $MONGODB_PASSWORD

# 3. 检查容器日志
sudo docker logs qs-apiserver | grep -i mongodb
```

#### 4. 容器 unhealthy

**自动恢复**:

- `server-check.yml` 会自动检测并重启 unhealthy 容器

**手动排查**:

```bash
# 查看健康检查日志
sudo docker inspect --format='{{json .State.Health}}' qs-apiserver | jq

# 手动执行健康检查
sudo docker exec qs-apiserver curl -f http://localhost:9080/healthz

# 查看应用日志
sudo docker logs --tail 100 qs-apiserver
```

---

## 快速参考

### 常用操作

```bash
# 部署到生产
git push origin main

# 手动备份数据库
Actions → Database Operations → backup → mongodb

# 查看数据库状态
Actions → Database Operations → status → mongodb

# 健康检查
Actions → Server Health Check → Run workflow

# SSH 连接测试
Actions → Test SSH Connection → Run workflow

# 查看容器日志
ssh user@server "sudo docker logs --tail 100 qs-apiserver"
```

### Secrets 清单

**Organization Secrets (8个)**:

```text
SVRA_HOST, SVRA_USERNAME, SVRA_SSH_KEY, SVRA_SSH_PORT
MONGODB_HOST, MONGODB_PORT, REDIS_HOST, REDIS_PORT
DOCKERHUB_USERNAME, DOCKERHUB_TOKEN
```

**Repository Secrets (6个)**:

```text
MONGODB_USERNAME, MONGODB_PASSWORD, MONGODB_DBNAME
REDIS_PASSWORD, JWT_SECRET
WWW_UID (可选), WWW_GID (可选)
```

### 时区转换参考

GitHub Actions cron 使用 **UTC 时间**：

| 北京时间 | UTC 时间 | Cron 表达式 |
|---------|---------|------------|
| 01:00 | 17:00 (前一天) | `0 17 * * *` |
| 02:00 | 18:00 (前一天) | `0 18 * * *` |
| 10:00 | 02:00 | `0 2 * * *` |

---

**最后更新**: 2025年11月24日

**维护**: FangcunMount Team

**支持**: 通过 GitHub Issues 提交问题或建议
