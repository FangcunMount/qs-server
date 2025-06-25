# 数据库 Docker 部署指南

## 概述

本文档介绍如何使用Docker部署问卷收集&量表测评系统的完整数据库环境，包括MySQL、Redis、MongoDB三种数据库服务。

## 快速开始

### 一键部署所有服务

```bash
# 部署所有数据库服务
./scripts/deploy-all.sh deploy
```

### 分步部署

```bash
# 1. 启动所有服务
docker compose up -d

# 2. 查看状态
./scripts/deploy-all.sh status

# 3. 查看访问信息
./scripts/deploy-all.sh info
```

## 服务管理

### 统一管理命令

```bash
./scripts/deploy-all.sh start          # 启动所有服务
./scripts/deploy-all.sh stop           # 停止所有服务  
./scripts/deploy-all.sh restart        # 重启所有服务
./scripts/deploy-all.sh status         # 查看服务状态
./scripts/deploy-all.sh logs           # 查看所有日志
./scripts/deploy-all.sh backup         # 备份所有数据库
./scripts/deploy-all.sh info           # 显示访问信息
```

### 单服务管理

```bash
./scripts/deploy-all.sh start mysql    # 启动MySQL
./scripts/deploy-all.sh logs redis     # 查看Redis日志
./scripts/deploy-all.sh connect mongodb # 连接MongoDB
```

## 访问信息

### 数据库连接

| 数据库 | 连接字符串 |
|-------|----------|
| MySQL | `mysql://qs_app_user:qs_app_password_2024@localhost:3306/questionnaire_scale` |
| Redis | `redis://localhost:6379` (密码: questionnaire_redis_2024) |
| MongoDB | `mongodb://qs_app_user:qs_app_password_2024@localhost:27017/questionnaire_scale` |

### Web 管理界面

| 服务 | URL | 用户名/密码 |
|------|-----|------------|
| phpMyAdmin (MySQL) | http://localhost:8082 | root / questionnaire_root_2024 |
| Redis Commander | http://localhost:8083 | admin / admin123 |
| Mongo Express | http://localhost:8081 | admin / admin123 |

## 配置文件

### 文件结构

```
questionnaire-scale/
├── docker-compose.yml              # 完整服务配置
├── docker-compose.mysql.yml        # MySQL独立配置
├── docker-compose.redis.yml        # Redis独立配置
├── scripts/deploy-all.sh           # 统一部署脚本
├── configs/
│   ├── mysql/my.cnf                # MySQL配置
│   ├── redis/redis.conf            # Redis配置
│   └── mongodb/mongod.conf         # MongoDB配置
├── data/                           # 数据持久化目录
│   ├── mysql/
│   ├── redis/
│   └── mongodb/
└── logs/                           # 日志目录
```

### 数据库配置参数

| 数据库 | 版本 | 端口 | 用户名 | 密码 |
|-------|------|------|-------|------|
| MySQL | 8.0 | 3306 | qs_app_user | qs_app_password_2024 |
| Redis | 7.2 | 6379 | - | questionnaire_redis_2024 |
| MongoDB | 7.0 | 27017 | qs_app_user | qs_app_password_2024 |

## 独立部署

### MySQL 独立部署

```bash
docker compose -f docker-compose.mysql.yml up -d
```

### Redis 独立部署

```bash
docker compose -f docker-compose.redis.yml up -d
```

### MongoDB 独立部署

```bash
./scripts/deploy-mongodb.sh deploy
```

## 数据备份与恢复

### 备份所有数据库

```bash
# 自动备份
./scripts/deploy-all.sh backup

# 备份文件位置
backups/20240101_120000/
├── mysql_backup.sql
├── redis_backup.rdb
└── mongodb_backup/
```

### 手动备份

```bash
# MySQL备份
docker exec questionnaire-mysql mysqldump -u root -pquestionnaire_root_2024 --all-databases > mysql_backup.sql

# Redis备份
docker exec questionnaire-redis redis-cli -a questionnaire_redis_2024 --rdb redis_backup.rdb

# MongoDB备份
docker exec questionnaire-mongodb mongodump --out mongodb_backup
```

## 故障排除

### 常见问题

1. **端口冲突**
   ```bash
   # 检查端口占用
   lsof -i :3306
   lsof -i :6379
   lsof -i :27017
   ```

2. **权限问题**
   ```bash
   # 修复数据目录权限
   sudo chown -R 999:999 data/
   ```

3. **容器启动失败**
   ```bash
   # 查看详细日志
   docker compose logs mysql
   docker compose logs redis
   docker compose logs mongodb
   ```

### 调试命令

```bash
# 查看服务状态
docker compose ps

# 查看容器日志
docker compose logs -f

# 进入容器
docker exec -it questionnaire-mysql bash
docker exec -it questionnaire-redis sh
docker exec -it questionnaire-mongodb bash
```

## 生产环境建议

### 安全配置

1. 修改默认密码
2. 配置防火墙规则
3. 启用SSL/TLS连接
4. 定期更新镜像

### 性能优化

1. 调整内存分配
2. 配置SSD存储
3. 优化网络设置
4. 监控资源使用

### 高可用配置

1. MySQL主从复制
2. Redis集群模式
3. MongoDB复制集
4. 负载均衡配置

## 维护操作

```bash
# 清理数据（危险操作）
./scripts/deploy-all.sh clean

# 查看资源使用
docker stats

# 清理未使用的镜像
docker image prune

# 更新服务
docker compose pull
docker compose up -d
```

---

**注意**: 生产环境请务必修改默认密码并配置适当的安全策略。 