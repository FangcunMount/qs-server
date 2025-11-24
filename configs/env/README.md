# QS-Server 环境变量配置说明

## 📁 目录说明

本目录包含 qs-server 项目的环境变量配置文件：

- `.env.dev` - 开发环境配置
- `.env.prod` - 生产环境配置模板
- `.gitignore` - Git 忽略规则（保护敏感信息）

## 🚀 使用方式

### 1. 开发环境

开发环境使用 `.env.dev` 文件，配置与 infra 项目保持一致。

#### 方式一：在 Shell 中加载

```bash
# 加载环境变量
source configs/env/.env.dev

# 启动服务
make run
```

#### 方式二：在 Makefile 中加载

在 Makefile 顶部添加：

```makefile
# 加载开发环境变量
include configs/env/.env.dev
export
```

#### 方式三：在代码中加载

使用 `godotenv` 库：

```go
import "github.com/joho/godotenv"

func init() {
    if err := godotenv.Load("configs/env/.env.dev"); err != nil {
        log.Println("No .env.dev file found")
    }
}
```

### 2. 生产环境

生产环境需要：

1. **复制模板文件**

   ```bash
   cp configs/env/.env.prod configs/env/.env.prod.local
   ```

2. **修改配置**
   编辑 `.env.prod.local`，替换所有 `<YOUR_*>` 占位符为实际值

3. **加载配置**

   ```bash
   source configs/env/.env.prod.local
   ```

4. **启动服务**

   ```bash
   ENV=prod make run
   ```

## 🔒 安全注意事项

### 开发环境

- ✅ `.env.dev` 可以提交到仓库（仅包含开发环境配置）
- ⚠️  密码仅供开发使用，不要用于生产环境
- 📝 与 infra 项目的配置保持同步

### 生产环境

- ❌ **禁止提交** `.env.prod.local` 到代码仓库
- ❌ **禁止提交** 任何包含真实生产密码的文件
- ✅ `.env.prod` 仅作为模板提交
- ✅ 使用占位符 `<YOUR_*>` 标记需要配置的值
- 🔐 建议使用密钥管理服务（Vault、AWS Secrets Manager 等）

### Git 配置

`.gitignore` 已配置忽略：

```gitignore
# 忽略本地环境配置
.env.local
.env.*.local
.env.prod.local

# 忽略包含实际密码的文件
*.secret
*.credentials
```

## 📋 配置项说明

### 基础设施配置

| 组件 | 配置项 | 说明 |
|-----|-------|-----|
| MySQL | `MYSQL_HOST`, `MYSQL_PORT` | 数据库地址和端口 |
|  | `MYSQL_USER`, `MYSQL_PASSWORD` | 应用数据库用户 |
|  | `MYSQL_DATABASE` | 数据库名称 |
| Redis Cache | `REDIS_CACHE_HOST`, `REDIS_CACHE_PORT` | 缓存实例地址（6379） |
|  | `REDIS_CACHE_PASSWORD` | 缓存实例密码 |
| Redis Store | `REDIS_STORE_HOST`, `REDIS_STORE_PORT` | 存储实例地址（6380） |
|  | `REDIS_STORE_PASSWORD` | 存储实例密码 |
| MongoDB | `MONGO_HOST`, `MONGO_PORT` | MongoDB 地址 |
|  | `MONGO_USER`, `MONGO_PASSWORD` | 应用数据库用户 |
|  | `MONGO_URL` | 完整连接字符串 |
| NSQ | `NSQLOOKUPD_TCP_PORT`, `NSQD_TCP_PORT` | NSQ 端口配置 |
|  | `NSQLOOKUPD_HTTP_PORT`, `NSQD_HTTP_PORT` | NSQ HTTP 端口 |

### 应用配置

| 类别 | 配置项 | 说明 |
|-----|-------|-----|
| 服务端口 | `QS_APISERVER_HTTP_PORT` | API Server HTTP 端口 |
|  | `COLLECTION_SERVER_HTTP_PORT` | Collection Server HTTP 端口 |
| JWT | `JWT_SECRET_KEY` | JWT 签名密钥 |
|  | `JWT_TIMEOUT` | Token 有效期 |
| 日志 | `LOG_LEVEL` | 日志级别 |
|  | `LOG_FORMAT` | 日志格式（console/json） |
| 迁移 | `MIGRATION_ENABLED` | 是否启用自动迁移 |
|  | `MIGRATION_AUTOSEED` | 是否自动加载种子数据 |

## 🔍 环境检查

使用脚本检查基础设施是否就绪：

```bash
# 检查所有组件
make check-infra

# 或使用脚本（支持环境变量）
MYSQL_HOST=192.168.1.100 bash scripts/check-infra.sh mysql
```

检查脚本会读取以下环境变量：

- `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`
- `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`
- `MONGODB_HOST`, `MONGODB_PORT`, `MONGODB_USER`, `MONGODB_PASSWORD`
- `NSQ_LOOKUP_HOST`, `NSQ_LOOKUP_PORT`, `NSQ_D_HOST`, `NSQ_D_PORT`
- `CHECK_TIMEOUT` (默认 5 秒)

## 📝 最佳实践

### 1. 配置管理

```bash
# 开发环境：直接使用 .env.dev
source configs/env/.env.dev

# 测试环境：创建 .env.test.local
cp configs/env/.env.dev configs/env/.env.test.local
# 修改为测试环境配置
source configs/env/.env.test.local

# 生产环境：创建 .env.prod.local
cp configs/env/.env.prod configs/env/.env.prod.local
# 修改为生产环境配置
source configs/env/.env.prod.local
```

### 2. 密码管理

**开发环境：**

```bash
# 密码与 infra 项目保持一致
MYSQL_PASSWORD=dev_root_123
REDIS_PASSWORD=dev_admin_123
MONGO_PASSWORD=dev_mongo_123
```

**生产环境：**

```bash
# 使用强密码
MYSQL_PASSWORD=$(openssl rand -base64 32)
REDIS_PASSWORD=$(openssl rand -base64 32)
MONGO_PASSWORD=$(openssl rand -base64 32)
JWT_SECRET_KEY=$(openssl rand -base64 64)
```

### 3. 配置验证

启动服务前验证配置：

```bash
# 1. 加载环境变量
source configs/env/.env.dev

# 2. 检查基础设施
make check-infra

# 3. 验证配置文件
make config-check  # 如果有这个命令

# 4. 启动服务
make run
```

### 4. Docker 部署

在 docker-compose.yml 中引用：

```yaml
services:
  qs-apiserver:
    env_file:
      - ../../configs/env/.env.prod.local
    environment:
      - ENV=prod
```

## 🆘 常见问题

### Q1: 如何在代码中读取环境变量？

**Go 代码：**

```go
import "os"

mysqlHost := os.Getenv("MYSQL_HOST")
mysqlPort := os.Getenv("MYSQL_PORT")
```

### Q2: 环境变量优先级？

优先级：`系统环境变量 > .env 文件 > 配置文件默认值`

### Q3: 如何覆盖单个配置项？

```bash
# 加载 .env.dev
source configs/env/.env.dev

# 覆盖单个配置
export MYSQL_HOST=192.168.1.100
export LOG_LEVEL=debug

# 启动服务
make run
```

### Q4: 生产环境如何管理密码？

推荐使用密钥管理服务：

- **Docker Secrets** (Docker Swarm)
- **Kubernetes Secrets** (K8s)
- **AWS Secrets Manager** (AWS)
- **HashiCorp Vault** (通用)

### Q5: 如何在 CI/CD 中使用？

```yaml
# .github/workflows/deploy.yml
- name: Setup environment
  run: |
    echo "MYSQL_HOST=${{ secrets.MYSQL_HOST }}" >> configs/env/.env.prod.local
    echo "MYSQL_PASSWORD=${{ secrets.MYSQL_PASSWORD }}" >> configs/env/.env.prod.local
    source configs/env/.env.prod.local
```

## 📚 相关文档

- [infra 项目环境配置](../../docs/独立启动服务指南.md)
- [基础设施检查指南](../../docs/基础设施检查指南.md)
- [Docker 部署文档](../../build/docker/README.md)
