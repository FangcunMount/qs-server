# MongoDB Docker部署指南

本文档介绍如何使用Docker部署MongoDB数据库。

## 目录结构

```
build/docker/infra/
├── Dockerfile.mongodb           # MongoDB自定义镜像构建文件
├── docker-compose.mongodb.yml      # Docker Compose配置
└── deploy-mongodb.sh            # MongoDB部署脚本
```

## 配置文件

### MongoDB配置

`configs/mongodb/mongod.conf` 包含MongoDB服务器配置：

```yaml
# 网络配置
net:
  port: 27017
  bindIp: 0.0.0.0

# 存储配置
storage:
  dbPath: /data/db
  journal:
    enabled: true
  wiredTiger:
    engineConfig:
      cacheSizeGB: 1

# 系统日志配置
systemLog:
  destination: file
  path: /var/log/mongodb/mongod.log
  logAppend: true
  logRotate: reopen

# 安全配置
security:
  authorization: enabled

# 副本集配置（可选）
# replication:
#   replSetName: "questionnaire-rs"
```

### 初始化脚本

#### 数据库初始化脚本

`scripts/mongodb/init-mongo.js`:

```javascript
// 创建应用数据库
db = db.getSiblingDB('questionnaire_scale');

// 创建应用用户
db.createUser({
  user: "qs_app_user",
  pwd: "qs_app_password_2024",
  roles: [
    {
      role: "readWrite",
      db: "questionnaire_scale"
    }
  ]
});

// 创建集合（可选，MongoDB会自动创建）
db.createCollection("questionnaires");
db.createCollection("responses");
db.createCollection("users");
db.createCollection("analytics");

print("数据库初始化完成");
```

#### 索引创建脚本

`scripts/mongodb/create-indexes.js`:

```javascript
// 切换到应用数据库
db = db.getSiblingDB('questionnaire_scale');

// 问卷集合索引
db.questionnaires.createIndex({ "title": "text", "description": "text" });
db.questionnaires.createIndex({ "creator_id": 1 });
db.questionnaires.createIndex({ "created_at": -1 });
db.questionnaires.createIndex({ "status": 1 });
db.questionnaires.createIndex({ "category": 1 });

// 回复集合索引
db.responses.createIndex({ "questionnaire_id": 1 });
db.responses.createIndex({ "user_id": 1 });
db.responses.createIndex({ "submitted_at": -1 });
db.responses.createIndex({ "questionnaire_id": 1, "user_id": 1 }, { unique: true });

// 用户集合索引
db.users.createIndex({ "email": 1 }, { unique: true });
db.users.createIndex({ "username": 1 }, { unique: true });
db.users.createIndex({ "created_at": -1 });

// 分析集合索引
db.analytics.createIndex({ "questionnaire_id": 1 });
db.analytics.createIndex({ "event_type": 1 });
db.analytics.createIndex({ "timestamp": -1 });
db.analytics.createIndex({ "questionnaire_id": 1, "event_type": 1 });

print("索引创建完成");
```

## Docker配置

### Dockerfile

`build/docker/infra/Dockerfile.mongodb`:

```dockerfile
FROM mongo:7.0

# 设置维护者信息
LABEL maintainer="questionnaire-scale-team"

# 设置环境变量
ENV MONGO_INITDB_ROOT_USERNAME=admin
ENV MONGO_INITDB_ROOT_PASSWORD=questionnaire_admin_2024
ENV MONGO_INITDB_DATABASE=questionnaire_scale

# 复制配置文件
COPY ../../configs/mongodb/mongod.conf /etc/mongod.conf

# 复制初始化脚本
COPY ../../scripts/mongodb/init-mongo.js /docker-entrypoint-initdb.d/
COPY ../../scripts/mongodb/create-indexes.js /docker-entrypoint-initdb.d/

# 创建数据和日志目录
RUN mkdir -p /data/db /var/log/mongodb && \
    chown -R mongodb:mongodb /data/db /var/log/mongodb

# 设置工作目录
WORKDIR /data/db

# 暴露端口
EXPOSE 27017

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD mongo --eval "db.adminCommand('ping')" || exit 1

# 启动命令
CMD ["mongod", "--config", "/etc/mongod.conf"]
```

### Docker Compose配置

`docker-compose.mongodb.yml`包含：

```yaml
version: '3.8'

services:
  mongodb:
    build:
      context: ../../
      dockerfile: build/docker/infra/Dockerfile.mongodb
    container_name: questionnaire-mongodb
    restart: unless-stopped
    environment:
      MONGO_INITDB_ROOT_USERNAME: admin
      MONGO_INITDB_ROOT_PASSWORD: questionnaire_admin_2024
      MONGO_INITDB_DATABASE: questionnaire_scale
    ports:
      - "27017:27017"
    volumes:
      - /data/mongodb/qs/data:/data/db
      - /data/logs/qs/mongodb:/var/log/mongodb
    networks:
      - questionnaire-network
    healthcheck:
      test: ["CMD", "mongo", "--eval", "db.adminCommand('ping')"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s

networks:
  questionnaire-network:
    driver: bridge
```

## 部署步骤

### 1. 构建镜像

```bash
cd build/docker/infra
docker build -f Dockerfile.mongodb -t questionnaire-mongodb:latest ../../
```

### 2. 启动服务

```bash
docker compose -f docker-compose.mongodb.yml up -d
```

### 3. 验证部署

```bash
# 检查容器状态
docker ps | grep mongodb

# 查看日志
docker compose logs mongodb

# 测试连接
docker exec -it questionnaire-mongodb mongo -u admin -p questionnaire_admin_2024 --authenticationDatabase admin
```

## 连接信息

### 管理员连接

```bash
# 连接字符串
mongodb://admin:questionnaire_admin_2024@localhost:27017/admin

# 命令行连接
mongo -u admin -p questionnaire_admin_2024 --authenticationDatabase admin
```

### 应用连接

```bash
# 连接字符串
mongodb://qs_app_user:qs_app_password_2024@localhost:27017/questionnaire_scale

# 命令行连接
mongo -u qs_app_user -p qs_app_password_2024 --authenticationDatabase questionnaire_scale questionnaire_scale
```

## 监控和维护

### 查看状态

```bash
# 容器状态
docker ps | grep mongodb

# 服务日志
docker compose -f docker-compose.mongodb.yml logs mongodb

# 实时日志
docker compose -f docker-compose.mongodb.yml logs -f mongodb
```

### 性能监控

```bash
# 进入容器
docker exec -it questionnaire-mongodb bash

# MongoDB性能统计
mongo -u admin -p questionnaire_admin_2024 --authenticationDatabase admin --eval "db.stats()"

# 当前操作
mongo -u admin -p questionnaire_admin_2024 --authenticationDatabase admin --eval "db.currentOp()"
```

### 备份和恢复

#### 备份

```bash
# 备份特定数据库
docker exec questionnaire-mongodb mongodump --host localhost --port 27017 \
  --username admin --password questionnaire_admin_2024 --authenticationDatabase admin \
  --db questionnaire_scale --out /tmp/backup

# 复制备份文件到宿主机
docker cp questionnaire-mongodb:/tmp/backup ./mongodb_backup_$(date +%Y%m%d)
```

#### 恢复

```bash
# 复制备份文件到容器
docker cp ./mongodb_backup_20240101 questionnaire-mongodb:/tmp/restore

# 恢复数据库
docker exec questionnaire-mongodb mongorestore --host localhost --port 27017 \
  --username admin --password questionnaire_admin_2024 --authenticationDatabase admin \
  --db questionnaire_scale /tmp/restore/questionnaire_scale
```

## 配置优化

### 内存优化

```yaml
# mongod.conf
storage:
  wiredTiger:
    engineConfig:
      cacheSizeGB: 2  # 根据服务器内存调整
```

### 连接优化

```yaml
# mongod.conf
net:
  maxIncomingConnections: 1000  # 根据需求调整
```

### 日志优化

```yaml
# mongod.conf
systemLog:
  verbosity: 1  # 0=默认, 1-5=更详细
  component:
    query:
      verbosity: 2
```

## 故障排除

### 常见问题

1. **认证失败**
   ```bash
   # 检查用户是否存在
   mongo -u admin -p questionnaire_admin_2024 --authenticationDatabase admin --eval "db.getUsers()"
   ```

2. **连接被拒绝**
   ```bash
   # 检查端口是否开放
   netstat -tlnp | grep 27017
   
   # 检查防火墙设置
   sudo ufw status
   ```

3. **磁盘空间不足**
   ```bash
   # 检查磁盘使用情况
   df -h
   
   # 清理日志
   docker exec questionnaire-mongodb mongo --eval "db.runCommand({logRotate:1})"
   ```

### 诊断命令

```bash
# 检查MongoDB状态
docker exec questionnaire-mongodb mongo -u admin -p questionnaire_admin_2024 --authenticationDatabase admin --eval "db.serverStatus()"

# 检查副本集状态（如果配置了副本集）
docker exec questionnaire-mongodb mongo -u admin -p questionnaire_admin_2024 --authenticationDatabase admin --eval "rs.status()"

# 检查数据库大小
docker exec questionnaire-mongodb mongo -u admin -p questionnaire_admin_2024 --authenticationDatabase admin --eval "db.stats()"
```

## 安全建议

1. **更改默认密码**
   - 修改管理员密码
   - 使用强密码策略

2. **网络安全**
   - 限制访问IP
   - 使用SSL/TLS加密
   - 配置防火墙

3. **权限控制**
   - 为不同用途创建不同用户
   - 遵循最小权限原则
   - 定期审查用户权限

4. **数据加密**
   - 启用静态数据加密
   - 使用传输加密
   - 定期轮换密钥

---

更多MongoDB配置选项，请参考[MongoDB官方文档](https://docs.mongodb.com/) 