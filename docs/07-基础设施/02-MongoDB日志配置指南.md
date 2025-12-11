# MongoDB 日志配置指南

## 概述

qs-server 已经集成了 `component-base v0.4.0` 的 MongoDB 日志驱动，可以记录 MongoDB 命令的执行情况，包括：

- ✅ 命令类型（find, insert, update, delete 等）
- ✅ 执行耗时
- ✅ 慢查询警告
- ✅ 连接池事件
- ✅ 错误信息
- ⚠️ **查询语句详情**（需要 component-base v0.5.0+）

## 当前版本功能（component-base v0.4.0）

### 基础日志功能

当前版本的 MongoDB 日志驱动提供以下功能：

#### 1. 命令执行日志

```json
{
  "level": "debug",
  "ts": "2025-12-11T10:30:45.123Z",
  "caller": "logger/mongo.go:280",
  "msg": "MongoDB command succeeded",
  "request_id": 123456,
  "command": "find",
  "connection_id": "192.168.1.100:27017",
  "elapsed_ms": 15.5
}
```

#### 2. 慢查询警告

```json
{
  "level": "warn",
  "ts": "2025-12-11T10:30:45.456Z",
  "caller": "logger/mongo.go:295",
  "msg": "MongoDB slow command",
  "request_id": 123457,
  "command": "find",
  "connection_id": "192.168.1.100:27017",
  "elapsed_ms": 350.2,
  "event": "slow_command",
  "slow_threshold": "200ms"
}
```

#### 3. 错误日志

```json
{
  "level": "error",
  "ts": "2025-12-11T10:30:45.789Z",
  "caller": "logger/mongo.go:310",
  "msg": "MongoDB command failed",
  "request_id": 123458,
  "command": "update",
  "connection_id": "192.168.1.100:27017",
  "elapsed_ms": 5.2,
  "error": "write exception: ....."
}
```

### 当前配置选项

```yaml
mongodb:
  # 基础连接配置
  host: "127.0.0.1:27017"
  username: "app_user"
  password: "your_password"
  database: "qs"
  
  # 日志配置
  enable-logger: true      # 是否启用 MongoDB 日志
  slow-threshold: 200ms    # 慢查询阈值（超过此时间记录 WARN 级别日志）
```

### 当前限制

**❌ 不记录查询语句详情**

出于以下考虑，当前版本默认不记录查询语句的具体内容：

1. **安全性**：避免敏感数据泄露到日志中
2. **性能**：减少日志序列化开销
3. **日志量**：避免日志文件过大

示例 - 当前日志输出：
```json
{
  "msg": "MongoDB command succeeded",
  "command": "find",
  "elapsed_ms": 15.5
}
```

示例 - MySQL 对比（GORM 日志）：
```json
{
  "msg": "SQL executed",
  "sql": "SELECT * FROM users WHERE id = ?",
  "rows": 1,
  "elapsed_ms": 2.3
}
```

## 未来版本功能（component-base v0.5.0+）

### 详细查询日志

未来版本将支持记录查询语句详情，类似 MySQL 的 SQL 日志：

```json
{
  "level": "debug",
  "ts": "2025-12-11T10:30:45.123Z",
  "msg": "MongoDB command succeeded",
  "request_id": 123456,
  "command": "find",
  "collection": "users",
  "connection_id": "192.168.1.100:27017",
  "elapsed_ms": 15.5,
  "command_detail": "{find: \"users\", filter: {age: {$gt: 18}}, limit: 10}"
}
```

### 扩展配置选项

qs-server 已经预留了配置选项，待 component-base 升级后即可启用：

```yaml
mongodb:
  # 基础配置
  enable-logger: true
  slow-threshold: 200ms
  
  # 详细日志配置（需要 component-base v0.5.0+）
  log-command-detail: true   # 记录查询语句详情（类似 MySQL 的 SQL 日志）
  log-reply-detail: false    # 记录响应详情
  log-started: false         # 记录命令开始事件（会增加日志量）
```

### 安全过滤

未来版本的详细日志会自动进行安全处理：

1. **认证命令**：不记录密码等敏感信息
   ```
   command_detail: "[REDACTED: authentication command]"
   ```

2. **写操作**：只记录元数据，不记录具体数据
   ```
   command_detail: "{collection: users, count: 5}"
   ```

3. **内容截断**：限制日志长度
   ```
   command_detail: "{find: \"users\", filter: {...}}... [truncated]"
   ```

## 配置建议

### 开发环境

```yaml
mongodb:
  enable-logger: true
  slow-threshold: 200ms
  # 开发环境可以启用详细日志用于调试（需要 component-base v0.5.0+）
  log-command-detail: true   # 启用查询详情
  log-reply-detail: false    # 响应详情通常不需要
  log-started: false         # 减少日志量
```

### 生产环境

```yaml
mongodb:
  enable-logger: true
  slow-threshold: 500ms      # 生产环境可以调高阈值
  # 生产环境强烈建议关闭详细日志
  log-command-detail: false  # 避免性能影响和敏感信息泄露
  log-reply-detail: false
  log-started: false
```

## 使用示例

### 查看 MongoDB 日志

假设日志输出到文件 `/data/logs/qs/qs-apiserver.log`：

```bash
# 查看所有 MongoDB 日志
grep "MongoDB" /data/logs/qs/qs-apiserver.log

# 查看慢查询
grep "slow_command" /data/logs/qs/qs-apiserver.log

# 查看错误
grep "MongoDB.*error" /data/logs/qs/qs-apiserver.log

# 分析特定命令
grep "\"command\":\"find\"" /data/logs/qs/qs-apiserver.log
```

### 日志级别

MongoDB 日志使用以下级别：

- `DEBUG`：正常命令执行（需要日志级别设置为 debug）
- `WARN`：慢查询
- `ERROR`：命令执行失败

### 性能监控

通过日志可以监控 MongoDB 性能：

```bash
# 统计慢查询
grep "slow_command" /data/logs/qs/qs-apiserver.log | wc -l

# 分析慢查询的命令类型
grep "slow_command" /data/logs/qs/qs-apiserver.log | \
  grep -o '"command":"[^"]*"' | sort | uniq -c

# 查看最慢的查询（需要 jq 工具）
grep "slow_command" /data/logs/qs/qs-apiserver.log | \
  jq -r '[.elapsed_ms, .command, .request_id] | @tsv' | \
  sort -rn | head -10
```

## 升级路径

当 component-base 升级到 v0.5.0 后，按以下步骤启用详细日志：

### 1. 升级依赖

```bash
go get github.com/FangcunMount/component-base@v0.5.0
go mod tidy
```

### 2. 更新配置

在配置文件中启用详细日志选项：

```yaml
mongodb:
  enable-logger: true
  slow-threshold: 200ms
  log-command-detail: true  # 启用查询详情
```

### 3. 重新部署

重启服务使配置生效：

```bash
# 开发环境
make restart-apiserver

# 生产环境
kubectl rollout restart deployment/qs-apiserver
```

### 4. 验证

检查日志是否包含查询详情：

```bash
tail -f /data/logs/qs/qs-apiserver.log | grep "command_detail"
```

## 常见问题

### Q1: 为什么看不到查询语句？

**A**: 当前版本（component-base v0.4.0）默认不记录查询语句详情。需要等待 component-base 升级到 v0.5.0+ 并启用 `log-command-detail: true`。

### Q2: 日志级别如何设置？

**A**: MongoDB 命令日志使用 `DEBUG` 级别，需要在日志配置中设置：

```yaml
log:
  level: debug  # 或通过命令行 --log.level=debug
```

### Q3: 如何只记录慢查询？

**A**: 慢查询使用 `WARN` 级别，可以设置日志级别为 `warn`：

```yaml
log:
  level: warn
```

这样只会记录慢查询和错误，不会记录普通的命令执行。

### Q4: 生产环境是否应该启用 MongoDB 日志？

**A**: 建议启用基础日志（`enable-logger: true`），但：
- 设置合理的 `slow-threshold`（如 500ms）
- **不要**启用 `log-command-detail`（避免性能影响和敏感信息泄露）
- 设置合适的日志级别（如 `info` 或 `warn`）

### Q5: MongoDB 日志会影响性能吗？

**A**: 基础日志（不包含详情）的性能影响很小（< 1%）。但启用详细日志（`log-command-detail: true`）会有以下影响：
- 日志序列化开销
- 磁盘 I/O 增加
- 日志文件快速增长

因此生产环境不建议启用详细日志。

## 相关文档

- [component-base MongoDB Logger 文档](https://github.com/FangcunMount/component-base/blob/main/pkg/logger/README.md)
- [MongoDB Command Monitoring](https://www.mongodb.com/docs/drivers/go/current/fundamentals/monitoring/)
- [日志配置总览](./01-高并发与事件驱动架构设计.md)

## 更新日志

- **2025-12-11**: 初始版本，集成 component-base v0.4.0 基础日志功能
- **待定**: component-base v0.5.0 升级，支持查询详情日志
