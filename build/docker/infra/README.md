# 数据库基础设施

## 概述

这是问卷收集&量表测评系统的数据库基础设施配置，包含MySQL、Redis、MongoDB三个数据库服务。

## 目录结构

```text
build/docker/infra/
├── Dockerfile.mysql          # MySQL自定义镜像构建文件
├── Dockerfile.redis          # Redis自定义镜像构建文件  
├── Dockerfile.mongodb        # MongoDB自定义镜像构建文件
├── docker-compose.yml        # 服务编排配置文件
├── deploy.sh                 # 部署管理脚本
└── README.md                 # 说明文档

configs/env/
├── config.env                # 开发环境配置文件
└── config.prod.env           # 生产环境配置模板
```

## 架构特点

### 统一配置管理

- **环境变量配置**: 使用 `config.env` 统一管理所有数据库配置
- **多环境支持**: 提供开发环境和生产环境配置示例
- **配置继承**: 支持环境变量覆盖和默认值

### 自定义镜像

- **配置文件内嵌**: 所有配置文件都构建到镜像中，无需外部挂载
- **初始化脚本集成**: 数据库初始化脚本已集成到镜像
- **生产就绪**: 所有服务都使用生产级配置

### 数据持久化

- **灵活路径配置**: 通过环境变量自定义数据和日志路径
- **数据文件**: 默认挂载到 `/data/{service}/qs/data` 目录
- **日志文件**: 默认挂载到 `/data/logs/qs/{service}` 目录
- **配置隔离**: 配置文件在镜像内部，不依赖外部文件

### 无管理界面

- 专注于数据库服务本身
- 减少资源消耗
- 提高安全性

## 配置管理

### 环境变量配置文件

所有数据库相关配置都统一在环境变量配置文件中管理：

- `configs/env/config.env` - 开发环境配置
- `configs/env/config.prod.env` - 生产环境配置示例

### 配置文件说明

```bash
# 使用开发环境配置（默认）
./deploy.sh deploy

# 使用生产环境配置
cp ../../../configs/env/config.prod.env ../../../configs/env/config.env
# 修改config.env中的密码等敏感信息
./deploy.sh deploy
```

### 主要配置项

| 类别 | 配置项 | 说明 |
|------|--------|------|
| MySQL | MYSQL_HOST, MYSQL_PORT | 数据库连接地址 |
| MySQL | MYSQL_ROOT_PASSWORD | 管理员密码 |
| MySQL | MYSQL_USER, MYSQL_PASSWORD | 应用用户密码 |
| Redis | REDIS_PASSWORD | Redis访问密码 |
| MongoDB | MONGODB_ROOT_PASSWORD | MongoDB管理员密码 |
| 路径 | *_DATA_PATH,*_LOGS_PATH | 数据和日志存储路径 |

## 快速开始

### 一键部署

```bash
# 进入基础设施目录
cd build/docker/infra

# 完整部署所有服务
./deploy.sh deploy
```

### 分步部署

```bash
# 1. 构建自定义镜像
./deploy.sh build

# 2. 启动所有服务
./deploy.sh start

# 3. 查看服务状态
./deploy.sh status
```

## 服务配置

### MySQL 服务

- **镜像**: questionnaire-mysql:latest
- **端口**: 3306
- **数据库**: questionnaire_scale
- **用户**: qs_app_user / qs_app_password_2024
- **管理员**: root / questionnaire_root_2024

### Redis 服务

- **镜像**: questionnaire-redis:latest
- **端口**: 6379
- **密码**: questionnaire_redis_2024
- **持久化**: RDB + AOF
- **内存限制**: 512MB

### MongoDB 服务

- **镜像**: questionnaire-mongodb:latest
- **端口**: 27017
- **数据库**: questionnaire_scale
- **用户**: qs_app_user / qs_app_password_2024
- **管理员**: admin / questionnaire_admin_2024

## 管理命令

### 全局操作

```bash
./deploy.sh deploy     # 完整部署
./deploy.sh build      # 构建镜像
./deploy.sh start      # 启动所有服务
./deploy.sh stop       # 停止所有服务
./deploy.sh restart    # 重启所有服务
./deploy.sh status     # 查看服务状态
./deploy.sh logs       # 查看所有日志
./deploy.sh backup     # 备份所有数据库
./deploy.sh clean      # 清理所有数据
./deploy.sh info       # 显示访问信息
```

### 单服务操作

```bash
./deploy.sh start mysql    # 启动MySQL
./deploy.sh stop redis     # 停止Redis
./deploy.sh logs mongodb   # 查看MongoDB日志
./deploy.sh shell mysql    # 进入MySQL容器
./deploy.sh connect redis  # 连接Redis
```

## 数据持久化配置

### 目录映射

| 服务 | 容器路径 | 宿主机路径 |
|------|----------|-----------|
| MySQL数据 | /var/lib/mysql | ../../../data/mysql |
| MySQL日志 | /var/log/mysql | ../../../logs/mysql |
| Redis数据 | /data | ../../../data/redis |
| Redis日志 | /var/log/redis | ../../../logs/redis |
| MongoDB数据 | /data/db | ../../../data/mongodb |
| MongoDB配置 | /data/configdb | ../../../data/mongodb-config |
| MongoDB日志 | /var/log/mongodb | ../../../logs/mongodb |

### 备份策略

```bash
# 自动备份所有数据库
./deploy.sh backup

# 备份文件保存在
../../../backups/YYYYMMDD_HHMMSS/
├── mysql_backup.sql
├── redis_backup.rdb
└── mongodb_backup/
```

## 连接信息

### 应用连接字符串

```bash
# MySQL
mysql://qs_app_user:qs_app_password_2024@localhost:3306/questionnaire_scale

# Redis  
redis://localhost:6379
# 密码: questionnaire_redis_2024

# MongoDB
mongodb://qs_app_user:qs_app_password_2024@localhost:27017/questionnaire_scale
```

### 管理员连接

```bash
# MySQL管理员
mysql -h localhost -P 3306 -u root -pquestionnaire_root_2024

# Redis连接
redis-cli -h localhost -p 6379 -a questionnaire_redis_2024

# MongoDB管理员
mongo mongodb://admin:questionnaire_admin_2024@localhost:27017/admin
```

## 健康检查

所有服务都配置了自动健康检查：

- **MySQL**: 每30秒检查数据库连接
- **Redis**: 每30秒执行ping命令
- **MongoDB**: 每30秒检查服务状态

## 故障排除

### 常见问题

1. **容器启动失败**

   ```bash
   ./deploy.sh logs <service>  # 查看详细日志
   ```

2. **端口冲突**

   ```bash
   lsof -i :3306   # 检查MySQL端口
   lsof -i :6379   # 检查Redis端口
   lsof -i :27017  # 检查MongoDB端口
   ```

3. **权限问题**

   ```bash
   sudo chown -R $USER:$USER ../../../data/
   sudo chown -R $USER:$USER ../../../logs/
   ```

### 调试命令

```bash
# 查看容器状态
docker ps

# 查看镜像
docker images | grep questionnaire

# 查看网络
docker network ls

# 清理资源
docker system prune
```

## 安全注意事项

1. **生产环境**: 请修改默认密码
2. **网络隔离**: 考虑使用内部网络
3. **防火墙**: 配置适当的端口访问规则
4. **备份加密**: 生产环境备份文件应加密存储

## 资源要求

### 最小配置

- CPU: 4核
- 内存: 6GB
- 磁盘: 20GB

### 推荐配置

- CPU: 8核
- 内存: 12GB  
- 磁盘: 100GB SSD

## 版本信息

- MySQL: 8.0
- Redis: 7.2
- MongoDB: 7.0
- Docker Compose: 3.8
