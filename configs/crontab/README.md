# 定时任务配置说明

> **版本**：V1.0  
> **更新日期**：2025-01-XX

## 概述

本目录包含生产环境定时任务的完整配置，采用 **Crontab + Shell 脚本** 方案，实现自动 Token 管理和 API 调用。

**核心特性**：

- ✅ 自动 Token 管理：按需刷新，无需手动维护
- ✅ 统一脚本模板：所有任务使用同一套逻辑
- ✅ 简洁配置：每个任务只需一行
- ✅ 完善日志：统一的日志格式和错误处理

## 文件结构

```text
configs/crontab/
├── qs-scheduler          # Crontab 配置文件（主配置）
├── api-call.sh          # 通用 API 调用脚本
├── refresh-token.sh     # Token 刷新脚本
├── logrotate.conf       # 日志轮转配置
└── README.md           # 本文档
```

## 快速开始

### 方式一：自动部署（推荐）

使用 GitHub Actions 自动部署到生产服务器：

1. **配置 GitHub Secrets**
   - `SVRA_HOST`: 服务器地址
   - `SVRA_USERNAME`: SSH 用户名
   - `SVRA_SSH_KEY`: SSH 私钥
   - `SVRA_SSH_PORT`: SSH 端口（可选，默认 22）
   - `IAM_USERNAME`: IAM 用户名
   - `IAM_PASSWORD`: IAM 密码
   - `IAM_LOGIN_URL`: IAM 登录接口地址（可选）
   - `API_BASE_URL`: API 基础 URL（可选）

2. **触发部署**
   - 推送代码到 `main` 分支（自动触发）
   - 或手动触发：Actions → Deploy Crontab Configuration → Run workflow

3. **验证部署**
   - 查看 GitHub Actions 日志
   - 或 SSH 到服务器验证文件

### 方式二：手动部署

```bash
# 1. 部署脚本
sudo cp configs/crontab/refresh-token.sh /usr/local/bin/qs-refresh-token.sh
sudo cp configs/crontab/api-call.sh /usr/local/bin/qs-api-call.sh
sudo chmod +x /usr/local/bin/qs-refresh-token.sh
sudo chmod +x /usr/local/bin/qs-api-call.sh

# 2. 创建必要目录
sudo mkdir -p /etc/qs-server
sudo mkdir -p /var/log/qs-scheduler
sudo chmod 755 /etc/qs-server
sudo chmod 755 /var/log/qs-scheduler

# 3. 配置 Crontab
sudo cp configs/crontab/qs-scheduler /etc/cron.d/qs-scheduler
sudo vim /etc/cron.d/qs-scheduler  # 修改 IAM_USERNAME, IAM_PASSWORD 等
sudo chmod 644 /etc/cron.d/qs-scheduler

# 4. 配置日志轮转
sudo cp configs/crontab/logrotate.conf /etc/logrotate.d/qs-scheduler
sudo logrotate -d /etc/logrotate.d/qs-scheduler  # 测试配置

# 5. 测试
sudo /usr/local/bin/qs-refresh-token.sh
sudo cat /etc/qs-server/internal-token
sudo /usr/local/bin/qs-api-call.sh /api/v1/statistics/sync/daily
```

## 工作原理

### Token 自动管理

```text
┌─────────────┐
│ Crontab 任务 │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ api-call.sh │
└──────┬──────┘
       │
       ├─→ Token 文件存在？
       │   ├─ 是 → 读取 Token → 执行 API 调用
       │   └─ 否 → 调用 refresh-token.sh
       │              │
       │              ▼
       │       ┌──────────────────┐
       │       │ refresh-token.sh │
       │       └──────┬───────────┘
       │              │
       │              ▼
       │       调用 IAM 登录接口
       │              │
       │              ▼
       │       保存 Token 到文件
       │              │
       └──────────────┴─→ 读取 Token → 执行 API 调用
```

### 工作流程

1. **Crontab 触发任务**：每小时执行一次业务任务
2. **api-call.sh 检查 Token**：检查 Token 文件是否存在
3. **按需刷新 Token**：如果不存在，自动调用 `refresh-token.sh`
4. **执行 API 调用**：使用 Token 调用业务接口
5. **记录日志**：所有操作记录到日志文件

## 配置文件说明

### qs-scheduler

Crontab 主配置文件，包含：

- **环境变量**：IAM 登录信息、API 地址等
- **定时任务**：所有业务任务的调度配置

**关键配置项**：

```bash
# IAM 登录配置
IAM_USERNAME="qs-scheduler@example.com"
IAM_PASSWORD="your-password-here"
IAM_LOGIN_URL="https://iam.example.com/api/v1/auth/login"

# API 配置
API_BASE_URL="http://localhost:8080"
TOKEN_FILE="/etc/qs-server/internal-token"
```

### api-call.sh

通用 API 调用脚本，功能：

- 自动获取 Token（从文件或刷新）
- 执行 API 调用
- 统一错误处理和日志记录
- 401 错误自动刷新 Token 并重试

**使用方式**：

```bash
qs-api-call.sh <endpoint> [log_file]

# 示例
qs-api-call.sh /api/v1/statistics/sync/daily
qs-api-call.sh /api/v1/statistics/sync/daily /var/log/qs-scheduler/sync-daily.log
```

**环境变量**：

- `TOKEN_FILE`：Token 文件路径（默认：`/etc/qs-server/internal-token`）
- `API_BASE_URL`：API 基础 URL（默认：`http://localhost:8080`）
- `TIMEOUT`：请求超时时间（秒，默认：300）
- `REFRESH_TOKEN_SCRIPT`：Token 刷新脚本路径（默认：`/usr/local/bin/qs-refresh-token.sh`）

### refresh-token.sh

Token 刷新脚本，功能：

- 调用 IAM 登录接口
- 解析响应获取 Token
- 保存 Token 到文件（权限 600）

**环境变量**：

- `IAM_USERNAME`：IAM 用户名（必需）
- `IAM_PASSWORD`：IAM 密码（必需）
- `IAM_LOGIN_URL`：IAM 登录接口地址（默认：`https://iam.example.com/api/v1/auth/login`）
- `TOKEN_FILE`：Token 文件路径（默认：`/etc/qs-server/internal-token`）
- `LOG_FILE`：日志文件路径（默认：`/var/log/qs-scheduler/refresh-token.log`）

## 定时任务列表

| 任务 | 执行时间 | 说明 | 日志文件 |
|------|---------|------|---------|
| `sync-daily` | 每小时第 0 分 | 同步每日统计数据 | `sync-daily.log` |
| `sync-accumulated` | 每小时第 5 分 | 同步累计统计数据 | `sync-accumulated.log` |
| `sync-plan` | 每小时第 10 分 | 同步计划统计数据 | `sync-plan.log` |
| `validate` | 每小时第 15 分 | 校验数据一致性 | `validate.log` |
| `schedule-tasks` | 每小时第 20 分 | 调度待推送任务 | `schedule-tasks.log` |

**注意**：Token 刷新由 `api-call.sh` 按需执行，无需单独的 crontab 任务。

## 监控和维护

### 查看任务状态

```bash
# 查看所有日志
ls -lh /var/log/qs-scheduler/

# 查看最近的任务执行日志
tail -f /var/log/qs-scheduler/sync-daily.log

# 查看错误日志
grep -i error /var/log/qs-scheduler/*.log

# 检查任务执行频率
grep -c "$(date +%Y-%m-%d)" /var/log/qs-scheduler/sync-daily.log
```

### 手动执行任务

```bash
# 使用脚本（推荐）
sudo /usr/local/bin/qs-api-call.sh /api/v1/statistics/sync/daily

# 直接使用 curl（需要先获取 Token）
TOKEN=$(cat /etc/qs-server/internal-token)
curl -X POST \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  http://localhost:8080/api/v1/statistics/sync/daily
```

### 验证配置

```bash
# 查看 crontab 配置
cat /etc/cron.d/qs-scheduler

# 查看 cron 服务状态
sudo systemctl status cron
# 或
sudo systemctl status crond

# 查看系统日志
sudo grep CRON /var/log/syslog
```

## 故障排查

### Token 刷新失败

**症状**：日志中出现 "Failed to refresh token" 或 "IAM_USERNAME is not set"

**排查步骤**：

1. **检查环境变量**

   ```bash
   # 查看 crontab 配置中的环境变量
   grep -E "IAM_USERNAME|IAM_PASSWORD" /etc/cron.d/qs-scheduler
   ```

2. **手动测试 Token 刷新**

   ```bash
   # 设置环境变量
   export IAM_USERNAME="qs-scheduler@example.com"
   export IAM_PASSWORD="your-password"
   
   # 执行刷新脚本
   sudo /usr/local/bin/qs-refresh-token.sh
   ```

3. **检查 IAM 登录接口**

   ```bash
   curl -X POST \
     -H "Content-Type: application/json" \
     -d '{"username":"qs-scheduler@example.com","password":"your-password"}' \
     https://iam.example.com/api/v1/auth/login
   ```

### API 调用失败

**症状**：日志中出现 "API call failed with HTTP code: XXX"

**排查步骤**：

1. **检查 Token 文件**

   ```bash
   # 查看 Token 文件是否存在
   ls -l /etc/qs-server/internal-token
   
   # 查看 Token 内容（前 20 个字符）
   sudo head -c 20 /etc/qs-server/internal-token
   ```

2. **测试 API 地址**

   ```bash
   # 检查 API 服务是否可访问
   curl -v http://localhost:8080/health
   ```

3. **查看详细错误日志**

   ```bash
   tail -f /var/log/qs-scheduler/sync-daily.log
   ```

### 任务未执行

**症状**：日志文件没有更新

**排查步骤**：

1. **检查 cron 服务**

   ```bash
   sudo systemctl status cron
   sudo systemctl start cron  # 如果未运行
   ```

2. **检查配置文件格式**

   ```bash
   # 验证 crontab 格式
   cat /etc/cron.d/qs-scheduler
   ```

3. **检查文件权限**

   ```bash
   # 确保脚本可执行
   ls -l /usr/local/bin/qs-*.sh
   
   # 确保日志目录可写
   ls -ld /var/log/qs-scheduler
   ```

4. **查看系统日志**

   ```bash
   sudo grep CRON /var/log/syslog | tail -20
   sudo journalctl -u cron | tail -20
   ```

## 安全注意事项

### IAM 凭证安全

- ✅ 使用专用服务账号，不要使用管理员账号
- ✅ 限制账号权限，仅授予必要的接口访问权限
- ✅ 不要将密码提交到代码仓库
- ✅ 使用密钥管理服务存储密码（如 HashiCorp Vault、AWS Secrets Manager）
- ✅ 定期轮换密码

### Token 文件安全

- ✅ Token 文件权限设置为 600（仅 root 可读）
- ✅ Token 文件存储在 `/etc/qs-server/` 目录（受保护）
- ✅ 不要将 Token 内容记录到日志中
- ✅ 监控 Token 使用情况

### 文件权限

```bash
# Crontab 配置文件
sudo chmod 644 /etc/cron.d/qs-scheduler
sudo chown root:root /etc/cron.d/qs-scheduler

# 脚本文件
sudo chmod 755 /usr/local/bin/qs-*.sh
sudo chown root:root /usr/local/bin/qs-*.sh

# Token 文件（由脚本自动设置）
# 权限：600，所有者：root:root

# 日志目录
sudo chmod 755 /var/log/qs-scheduler
sudo chown root:root /var/log/qs-scheduler
```

## 日志管理

### 日志文件位置

所有日志文件位于 `/var/log/qs-scheduler/` 目录：

- `sync-daily.log` - 每日统计同步日志
- `sync-accumulated.log` - 累计统计同步日志
- `sync-plan.log` - 计划统计同步日志
- `validate.log` - 数据校验日志
- `schedule-tasks.log` - 任务调度日志

### 日志轮转

已配置日志轮转（`logrotate.conf`）：

- **轮转频率**：每天一次
- **保留天数**：30 天
- **自动压缩**：是
- **延迟压缩**：是（下次轮转时压缩）

### 手动清理日志

```bash
# 清理 30 天前的压缩日志
find /var/log/qs-scheduler/ -name "*.log.*" -mtime +30 -delete

# 手动执行日志轮转
sudo logrotate -f /etc/logrotate.d/qs-scheduler
```

## 常见问题

### Q1: Token 多久刷新一次？

**A**: Token 按需刷新。当 Token 文件不存在或为空时，`api-call.sh` 会自动调用 `refresh-token.sh` 获取新 Token。如果 Token 过期导致 API 返回 401，脚本会自动刷新 Token 并重试。

### Q2: 如何添加新的定时任务？

**A**: 在 `qs-scheduler` 文件中添加新的 crontab 行：

```bash
# 每小时执行新任务（每小时的第 25 分）
25 * * * * root /usr/local/bin/qs-api-call.sh /api/v1/your/new/endpoint /var/log/qs-scheduler/your-task.log
```

### Q3: 如何修改任务执行时间？

**A**: 修改 `qs-scheduler` 文件中对应任务的 crontab 表达式。例如：

```bash
# 改为每 30 分钟执行一次
*/30 * * * * root /usr/local/bin/qs-api-call.sh /api/v1/statistics/sync/daily ...
```

### Q4: 如何查看任务执行历史？

**A**: 查看日志文件：

```bash
# 查看最近 100 行日志
tail -n 100 /var/log/qs-scheduler/sync-daily.log

# 查看今天的执行记录
grep "$(date +%Y-%m-%d)" /var/log/qs-scheduler/sync-daily.log
```

### Q5: 如何临时禁用某个任务？

**A**: 在 `qs-scheduler` 文件中注释掉对应的行：

```bash
# 0 * * * * root /usr/local/bin/qs-api-call.sh /api/v1/statistics/sync/daily ...
```

## GitHub Actions 自动部署

系统支持通过 GitHub Actions 自动部署 crontab 配置到生产服务器。

**详细说明**：参考 [GitHub Actions 自动部署指南](../../../docs/10-scheduler/08-GitHub Actions自动部署.md)

**快速使用**：

1. 配置 GitHub Secrets（SVRA_HOST, SVRA_USERNAME, SVRA_SSH_KEY, IAM_USERNAME, IAM_PASSWORD 等）
2. 推送代码到 `main` 分支（自动触发）
3. 或手动触发：Actions → Deploy Crontab Configuration → Run workflow

## 相关文档

- [定时任务调度机制](../../../docs/10-scheduler/01-定时任务调度机制.md)
- [Crontab 配置示例](../../../docs/10-scheduler/06-Crontab配置示例.md)
- [内部调用 Token 生成指南](../../../docs/10-scheduler/07-内部调用Token生成指南.md)
- [GitHub Actions 自动部署指南](../../../docs/10-scheduler/08-GitHub Actions自动部署.md)
