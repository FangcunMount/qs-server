# Migration 集成使用指南

## ✅ 已完成的集成

Migration package 已成功集成到 `qs-apiserver` 服务中，会在服务启动时自动执行数据库迁移。

## 📋 配置说明

### 1. 配置文件（`configs/apiserver.dev.yaml`）

```yaml
mysql:
  host: "127.0.0.1:3306"
  username: "qs_app_user"
  password: "qs_app_password_2024"
  database: "questionnaire_scale"
  # ... 其他配置 ...
  
  # Migration 配置
  enable-migration: true   # 是否启用自动迁移（默认: true）
  auto-seed: false         # 是否自动加载种子数据（默认: false）
```

### 2. 命令行参数

也可以通过命令行参数覆盖配置：

```bash
./qs-apiserver \
  --mysql.enable-migration=true \
  --mysql.auto-seed=false
```

## 🚀 工作流程

### 服务启动时的迁移流程

1. **服务启动** → `internal/apiserver/server.go:PrepareRun()`
2. **数据库初始化** → `DatabaseManager.Initialize()`
3. **执行迁移** → `DatabaseManager.runMigrations()`
4. **迁移器执行** → `migration.Migrator.Run()`
5. **应用 SQL 文件** → 从 `internal/pkg/migration/migrations/*.sql` 读取
6. **记录版本** → 在 `schema_migrations` 表中记录

### 迁移行为

- ✅ **首次启动**: 执行所有迁移文件，创建表结构
- ✅ **后续启动**: 检查版本，跳过已执行的迁移
- ✅ **新版本发布**: 仅执行新增的迁移文件
- ✅ **不会覆盖数据**: 使用版本控制，不会重复执行

## 📁 迁移文件位置

```text
internal/pkg/migration/migrations/
├── 000001_init_actor_schema.up.sql      # ✅ 已创建（Actor 模块表结构）
├── 000001_init_actor_schema.down.sql    # ✅ 已创建（回滚脚本）
└── 000002_xxx.up.sql                    # 未来的迁移文件
```

## 📝 当前已包含的迁移

### v1: Actor 模块初始化（`000001_init_actor_schema.up.sql`）

- ✅ `testee` 表：受试者信息
  - 支持 IAM 用户和儿童绑定
  - 包含标签、重点关注标记
  - 测评统计字段（总次数、最后测评时间、风险等级）
  - 软删除、乐观锁支持

- ✅ `staff` 表：员工信息
  - 支持 IAM 用户绑定
  - 角色列表（JSON 数组）
  - 联系方式、激活状态
  - 软删除、乐观锁支持

## 🔧 如何添加新的迁移

### 1. 创建迁移文件

按照版本号递增命名：

```bash
# 升级脚本
000002_add_new_feature.up.sql

# 降级脚本
000002_add_new_feature.down.sql
```

### 2. 编写 SQL

**升级脚本示例** (`000002_add_new_feature.up.sql`):

```sql
-- 添加新字段
ALTER TABLE testee ADD COLUMN `nickname` varchar(50) DEFAULT NULL COMMENT '昵称';

-- 创建新表
CREATE TABLE IF NOT EXISTS `new_table` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  ...
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**降级脚本示例** (`000002_add_new_feature.down.sql`):

```sql
-- 回滚新表
DROP TABLE IF EXISTS `new_table`;

-- 回滚字段
ALTER TABLE testee DROP COLUMN `nickname`;
```

### 3. 重启服务

迁移文件已嵌入到二进制文件中（通过 `//go:embed`），重新编译后：

```bash
go build -o tmp/apiserver cmd/qs-apiserver/apiserver.go
./tmp/apiserver --config=configs/apiserver.dev.yaml
```

服务启动时会自动检测并执行新的迁移。

## 🔍 迁移状态检查

### 查看当前版本

```sql
SELECT * FROM schema_migrations;
```

输出示例：

```text
+----------+-------+
| version  | dirty |
+----------+-------+
|        1 | false |
+----------+-------+
```

- `version`: 当前数据库版本（对应迁移文件的编号）
- `dirty`: 是否处于脏状态（迁移失败时为 true）

## 🛡️ 生产环境建议

### 推荐配置

```yaml
mysql:
  enable-migration: true   # ✅ 启用自动迁移
  auto-seed: false         # ⚠️ 生产环境禁用种子数据
```

### 安全实践

1. **测试环境先验证**: 新迁移先在测试环境执行
2. **备份数据库**: 重要更新前备份数据库
3. **监控日志**: 关注服务启动日志中的迁移信息
4. **禁用种子数据**: 生产环境设置 `auto-seed: false`

## 📊 日志示例

### 成功迁移

```text
[INFO] Starting database migration...
✅ Database migration completed successfully! Current version: 1
```

### 已是最新版本

```text
[INFO] Starting database migration...
✅ Database is already up to date! Current version: 1
```

### 迁移失败

```text
[ERROR] migration failed: syntax error at line 10
```

## 🚫 禁用迁移

如果需要禁用自动迁移（例如使用外部迁移工具）：

```yaml
mysql:
  enable-migration: false  # 禁用自动迁移
```

或命令行：

```bash
./qs-apiserver --mysql.enable-migration=false
```

## 🔄 手动回滚（开发环境）

目前需要手动执行 `.down.sql` 文件来回滚：

```bash
mysql -u qs_app_user -p questionnaire_scale < internal/pkg/migration/migrations/000001_init_actor_schema.down.sql
```

## 📚 相关文档

- Migration Package 详细文档: `internal/pkg/migration/README.md`
- Actor 模块设计文档: `docs/collection-server/01-用户模块设计.md`
- 数据库配置说明: `configs/mysql/README.md`
