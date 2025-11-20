# 数据库迁移

本目录包含数据库迁移文件和迁移工具。

## ⚠️ 重要说明：迁移不会覆盖数据

**常见误解：每次启动都执行迁移，会不会覆盖线上数据？**

**答案：不会！** 迁移使用 **版本控制机制**：

```text
┌─────────────────────────────────────────┐
│  schema_migrations 表（自动创建）        │
├─────────────────────────────────────────┤
│  version  │  dirty                      │
│  1        │  false   ← 已执行版本       │
└─────────────────────────────────────────┘

第 1 次启动 → 执行 v1 迁移 → 记录 version=1 ✅
第 2 次启动 → 检查 version=1 → 跳过（0 SQL 执行）✅
第 3 次启动 → 检查 version=1 → 跳过（0 SQL 执行）✅
新版本发布 → 检查 version=1 → 仅执行 v2 → 记录 version=2 ✅
```

**关键点**：

- ✅ 迁移是**增量的**，不是全量的
- ✅ 每个版本只执行**一次**
- ✅ 后续启动会**跳过**已执行的版本
- ✅ 不会删除或覆盖现有数据

## 📁 目录结构

```text
migration/
├── migrate.go              # 迁移工具实现
├── migrations/             # 迁移 SQL 文件（嵌入到二进制）
│   ├── 000001_init_schema.up.sql      # 初始化表结构
│   ├── 000001_init_schema.down.sql    # 回滚表结构
│   ├── 000002_seed_data.up.sql        # 种子数据
│   ├── 000002_seed_data.down.sql      # 回滚种子数据
│   └── ...
└── README.md               # 本文件
```

## 🚀 快速开始

### 1. 安装依赖

```bash
# 添加 golang-migrate 依赖
go get -u github.com/golang-migrate/migrate/v4
go get -u github.com/golang-migrate/migrate/v4/database/mysql
go get -u github.com/golang-migrate/migrate/v4/source/iofs

# （可选）安装 CLI 工具，用于创建迁移文件
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

### 2. 在应用中使用

```go
package main

import (
    "database/sql"
    "fmt"
    
    "github.com/FangcunMount/iam-contracts/internal/pkg/migration"
    _ "github.com/go-sql-driver/mysql"
)

func main() {
    // 1. 连接数据库
    db, err := sql.Open("mysql", "user:pass@tcp(localhost:3306)/iam_contracts")
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // 2. 配置迁移器
    cfg := &migration.Config{
        Enabled:  true,              // 启用自动迁移
        AutoSeed: false,             // 生产环境设为 false
        Database: "iam_contracts",   // 数据库名称
    }

    // 3. 创建迁移器并执行
    migrator := migration.NewMigrator(db, cfg)
    if version, applied, err := migrator.Run(); err != nil {
        panic(err)
    } else if applied {
        fmt.Printf("migrated to version %d\n", version)
    } else {
        fmt.Printf("database already up to date (version %d)\n", version)
    }

    // 4. 启动应用...
}
```

### 3. 创建新的迁移

```bash
# 使用 migrate CLI 创建迁移文件
migrate create -ext sql -dir internal/pkg/migration/migrations -seq add_new_feature

# 生成的文件:
# - 000003_add_new_feature.up.sql   (升级脚本)
# - 000003_add_new_feature.down.sql (回滚脚本)
```

编辑生成的文件：

**000003_add_new_feature.up.sql**:

```sql
-- 添加新功能
ALTER TABLE iam_users ADD COLUMN nickname VARCHAR(64) COMMENT '昵称';
```

**000003_add_new_feature.down.sql**:

```sql
-- 回滚新功能
ALTER TABLE iam_users DROP COLUMN nickname;
```

## 🔧 高级用法

### 手动控制迁移

```go
// 获取当前版本
version, dirty, err := migrator.Version()

// 回滚最近的一次迁移
err = migrator.Rollback()
```

### 环境变量配置

```yaml
# configs/apiserver.yaml
mysql:
  host: ${MYSQL_HOST:127.0.0.1}
  port: ${MYSQL_PORT:3306}
  database: ${MYSQL_DATABASE:iam_contracts}
  username: ${MYSQL_USER:root}
  password: ${MYSQL_PASSWORD:}

migration:
  enabled: ${MIGRATION_ENABLED:true}
  auto-seed: ${MIGRATION_AUTO_SEED:false}
```

### Docker 部署

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .

# 构建（迁移文件会被嵌入）
RUN go build -o /apiserver ./cmd/apiserver

# 运行阶段
FROM alpine:latest
COPY --from=builder /apiserver .
CMD ["./apiserver"]
```

容器启动时会自动执行迁移，无需手动操作。

## 📊 迁移表

`golang-migrate` 会自动创建 `schema_migrations` 表来追踪版本：

```sql
mysql> SELECT * FROM schema_migrations;
+---------+-------+
| version | dirty |
+---------+-------+
|       2 |     0 |
+---------+-------+
```

- `version`: 当前数据库版本号
- `dirty`: 是否处于中间状态（0=正常，1=异常需手动修复）

## 🔐 安全注意事项

### 生产环境

1. **备份优先**

   ```bash
   # 迁移前自动备份
   mysqldump iam_contracts > backup_$(date +%Y%m%d_%H%M%S).sql
   ```

2. **权限分离**
   - 应用账号：只需 SELECT, INSERT, UPDATE, DELETE
   - 迁移账号：需要 CREATE, DROP, ALTER 等 DDL 权限

3. **测试迁移**
   - 在测试环境先验证
   - 确保 down 脚本能正确回滚

4. **金丝雀发布**
   - 先在一个实例上执行
   - 验证成功后再推广

### 开发环境

```bash
# 重置数据库到初始状态
cd scripts/sql
./reset-db.sh

# 应用会在启动时自动执行迁移
go run cmd/apiserver/apiserver.go
```

## 📚 参考文档

- [golang-migrate 官方文档](https://github.com/golang-migrate/migrate)
- [数据库迁移指南](../../../docs/DATABASE_MIGRATION_GUIDE.md)
- [Schema 定义](../../../configs/mysql/schema.sql)

## ❓ 常见问题

### Q: 为什么要使用迁移工具？

A: 在容器化环境中，应用只打包二进制文件。使用 `embed.FS` 可以将 SQL 文件嵌入到二进制中，启动时自动执行，无需挂载外部文件。

### Q: 如何处理 dirty 状态？

A: Dirty 状态表示迁移中途失败。需要：

1. 检查日志确定失败原因
2. 手动修复数据库到一致状态
3. 更新 `schema_migrations` 表的 dirty 字段为 0

```sql
UPDATE schema_migrations SET dirty = 0 WHERE version = X;
```

### Q: 生产环境如何禁用自动迁移？

A: 设置环境变量：

```bash
export MIGRATION_ENABLED=false
```

### Q: 如何在 Kubernetes 中使用？

A: 使用 Init Container：

```yaml
initContainers:
- name: migrate
  image: your-app:latest
  command: ["/app/migrate-only"]  # 特殊命令只执行迁移
```

或者让应用启动时自动执行（推荐）。
