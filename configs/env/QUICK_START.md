# 环境配置快速参考

## 🚀 快速开始

### 开发环境

```bash
# 1. 加载环境变量
source configs/env/.env.dev

# 2. 检查基础设施（确保 infra 项目已启动）
make check-infra

# 3. 启动服务
make run
```

### 一键启动（推荐）

```bash
# Makefile 已配置自动加载环境变量和检查基础设施
make run-all
```

## 📝 配置文件说明

| 文件 | 用途 | 是否提交 |
| ----- | ------ | --------- |
| `.env.dev` | 开发环境配置 | ✅ 可以提交 |
| `.env.prod` | 生产环境模板 | ✅ 可以提交 |
| `.env.prod.local` | 生产环境实际配置 | ❌ 不要提交 |
| `.env.*.local` | 任何本地配置 | ❌ 不要提交 |

## 🔧 使用方式

### 方式一：Shell 加载

```bash
# 加载环境变量
source configs/env/.env.dev

# 环境变量会应用到当前 Shell 会话
echo $MYSQL_HOST
# 输出: 127.0.0.1

# 启动服务
./tmp/apiserver
./tmp/collection-server
```

### 方式二：Makefile 集成

在 Makefile 中添加：

```makefile
# 在文件顶部
-include configs/env/.env.dev
export

# 使用配置
run-apiserver:
 @echo "MySQL Host: $(MYSQL_HOST)"
 @./tmp/apiserver
```

### 方式三：代码加载

**安装依赖：**

```bash
go get github.com/joho/godotenv
```

**Go 代码：**

```go
import (
    "log"
    "os"
    "github.com/joho/godotenv"
)

func init() {
    // 加载环境变量文件
    if err := godotenv.Load("configs/env/.env.dev"); err != nil {
        log.Println("Warning: No .env.dev file found")
    }
}

func main() {
    // 读取配置
    mysqlHost := os.Getenv("MYSQL_HOST")
    mysqlPort := os.Getenv("MYSQL_PORT")
    
    log.Printf("MySQL: %s:%s", mysqlHost, mysqlPort)
}
```

### 方式四：Docker 部署

**docker-compose.yml:**

```yaml
services:
  qs-apiserver:
    env_file:
      - configs/env/.env.prod.local
    environment:
      - ENV=prod
```

## 🔍 环境检查

### 检查所有组件

```bash
# 方式一：使用 Makefile
make check-infra

# 方式二：直接使用脚本
bash scripts/check-infra.sh all

# 方式三：加载环境变量后检查
source configs/env/.env.dev
bash scripts/check-infra.sh all
```

### 检查单个组件

```bash
# 检查 MySQL
make check-mysql
# 或
bash scripts/check-infra.sh mysql

# 检查 Redis
make check-redis
# 或
bash scripts/check-infra.sh redis

# 检查 MongoDB
make check-mongodb
# 或
bash scripts/check-infra.sh mongodb

# 检查 NSQ
make check-nsq
# 或
bash scripts/check-infra.sh nsq
```

### 自定义配置检查

```bash
# 使用自定义 MySQL 配置
MYSQL_HOST=192.168.1.100 \
MYSQL_PORT=3307 \
MYSQL_USER=custom_user \
MYSQL_PASSWORD=custom_pass \
bash scripts/check-infra.sh mysql

# 使用自定义超时时间
CHECK_TIMEOUT=10 bash scripts/check-infra.sh all
```

## 🔐 密码管理

### 开发环境

开发环境密码与 infra 项目保持一致：

```bash
# MySQL
MYSQL_ROOT_PASSWORD=dev_root_123
MYSQL_PASSWORD=qs_app_password_2024

# Redis
REDIS_PASSWORD=dev_admin_123

# MongoDB
MONGO_INITDB_ROOT_PASSWORD=dev_mongo_123
MONGO_PASSWORD=qs_app_password_2024

# JWT
JWT_SECRET_KEY=questionnaire-scale-jwt-secret-key-2024-dev
```

### 生产环境

生成强密码：

```bash
# 生成 32 位随机密码
openssl rand -base64 32

# 生成 64 位 JWT 密钥
openssl rand -base64 64

# 创建生产配置
cp configs/env/.env.prod configs/env/.env.prod.local

# 使用生成的密码替换占位符
vim configs/env/.env.prod.local
```

## 📊 配置优先级

配置项的优先级（从高到低）：

1. **系统环境变量** - 临时覆盖
2. **`.env.*.local` 文件** - 本地配置
3. **`.env.dev` / `.env.prod` 文件** - 标准配置
4. **YAML 配置文件** - 应用配置
5. **代码默认值** - 兜底配置

示例：

```bash
# 1. 加载 .env.dev（优先级 3）
source configs/env/.env.dev

# 2. 创建本地覆盖（优先级 2）
echo "MYSQL_HOST=192.168.1.100" > configs/env/.env.local
source configs/env/.env.local

# 3. 临时覆盖（优先级 1）
export LOG_LEVEL=debug

# 最终生效：
# - MYSQL_HOST=192.168.1.100 (来自 .env.local)
# - LOG_LEVEL=debug (来自临时环境变量)
# - 其他配置来自 .env.dev
```

## 🆘 故障排查

### 问题：环境变量未生效

```bash
# 1. 确认环境变量已加载
source configs/env/.env.dev

# 2. 验证环境变量
echo $MYSQL_HOST
env | grep MYSQL

# 3. 检查配置文件语法
cat configs/env/.env.dev | grep -v "^#" | grep "="
```

### 问题：基础设施检查失败

```bash
# 1. 确认 infra 项目已启动
cd /path/to/infra
docker-compose ps

# 2. 检查端口占用
netstat -an | grep 3306  # MySQL
netstat -an | grep 6379  # Redis
netstat -an | grep 27017 # MongoDB
netstat -an | grep 4151  # NSQ

# 3. 查看详细错误
bash scripts/check-infra.sh mysql 2>&1

# 4. 使用自定义超时
CHECK_TIMEOUT=30 bash scripts/check-infra.sh all
```

### 问题：密码错误

```bash
# 1. 确认密码与 infra 项目一致
cd /path/to/infra
cat .env.dev | grep PASSWORD

# 2. 更新 qs-server 配置
vim configs/env/.env.dev

# 3. 重新加载
source configs/env/.env.dev

# 4. 测试连接
make check-mysql
```

## 📚 相关命令

```bash
# 环境管理
source configs/env/.env.dev              # 加载开发环境
source configs/env/.env.prod.local       # 加载生产环境
env | grep "MYSQL\|REDIS\|MONGO\|NSQ"    # 查看相关环境变量
unset $(env | grep "^MYSQL" | cut -d= -f1) # 清除 MySQL 相关变量

# 基础设施检查
make check-infra                         # 检查所有组件
make check-mysql                         # 检查 MySQL
make check-redis                         # 检查 Redis  
make check-mongodb                       # 检查 MongoDB
make check-nsq                           # 检查 NSQ

# 服务管理
make run-all                             # 启动所有服务（自动检查）
make run-apiserver                       # 启动 API Server
make run-collection                      # 启动 Collection Server
make stop-all                            # 停止所有服务
make status-all                          # 查看服务状态

# 开发工具
make dev                                 # 启动开发环境（热更新）
make logs                                # 查看日志
make health                              # 健康检查
```

## 🔗 相关文档

- [完整 README](README.md) - 详细使用说明
- [基础设施检查脚本](../../scripts/check-infra.sh) - 检查脚本源码
- [Makefile](../../Makefile) - 构建和运行配置
