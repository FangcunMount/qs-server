# QS-Server Docker 部署文档

## 概述

本目录包含 qs-server 生产环境的 Docker 部署配置文件。

## 文件说明

- `Dockerfile.qs-apiserver` - QS API Server 的 Dockerfile
- `Dockerfile.collection-server` - Collection Server 的 Dockerfile  
- `docker-compose.yml` - Docker Compose 编排文件
- `.dockerignore` - Docker 构建时忽略的文件

## 架构说明

### 服务列表

1. **qs-apiserver** - 问卷量表 API 服务
   - 宿主机映射: 8081 (HTTP) / 8444 (HTTPS) → 容器 8080/8443
   - 健康检查: http://localhost:8081/health

2. **collection-server** - 数据采集服务
   - 宿主机映射: 8082 (HTTP) / 8445 (HTTPS) → 容器 8080/8443
   - 健康检查: http://localhost:8082/health

### 依赖说明

**重要**: 本项目不包含基础设施组件（MySQL、MongoDB、Redis、NSQ）的 Docker 配置。这些组件由独立的 infra 项目管理。

在生产环境部署前，请确保以下基础设施已就绪：
- MySQL 8.0+
- MongoDB 6.0+
- Redis 7.0+
- NSQ

并在配置文件中正确配置连接信息。

## 快速开始

### 1. 环境变量配置

创建 `.env` 文件（可选）：

```bash
# 版本信息
VERSION=1.0.0
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(git rev-parse HEAD)
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

# 运行环境
ENV=prod
```

### 2. 构建镜像

```bash
# 构建所有服务
cd build/docker
docker-compose build

# 或者单独构建某个服务
docker-compose build qs-apiserver
docker-compose build collection-server
```

### 3. 启动服务

```bash
# 启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f
docker-compose logs -f qs-apiserver
docker-compose logs -f collection-server
```

### 4. 停止服务

```bash
# 停止所有服务
docker-compose stop

# 停止并删除容器
docker-compose down

# 停止并删除容器和数据卷
docker-compose down -v
```

## 配置文件

确保以下配置文件存在且配置正确：

```
configs/
├── apiserver.dev.yaml          # QS API Server 开发配置
├── apiserver.prod.yaml         # QS API Server 生产配置
├── collection-server.dev.yaml  # Collection Server 开发配置
├── collection-server.prod.yaml # Collection Server 生产配置
└── cert/                       # SSL 证书目录
```

## 镜像构建特性

### 多阶段构建

使用多阶段构建优化镜像大小：
1. **构建阶段**: 使用 `golang:1.24.0-alpine` 编译应用
2. **运行阶段**: 使用 `alpine:3.21` 最小化镜像

### 版本信息

构建时会自动注入版本信息到二进制文件：
- GitVersion: 版本号
- GitCommit: Git 提交 hash
- GitBranch: Git 分支
- BuildDate: 构建时间

### 安全特性

- 使用非 root 用户运行（appuser）
- 只读配置文件挂载
- 健康检查配置
- 日志轮转限制

## 网络配置

所有服务运行在 `qs-network` 桥接网络中，支持服务间通信。

## 数据持久化

日志文件通过数据卷持久化：
- `qs-apiserver-logs` - API Server 日志
- `qs-collection-logs` - Collection Server 日志

## 健康检查

每个服务都配置了健康检查：
- 检查间隔: 30 秒
- 超时时间: 5 秒
- 重试次数: 3 次
- 启动等待: 10 秒

## 日志管理

Docker 日志配置：
- 驱动: json-file
- 单文件最大: 10MB
- 最大文件数: 3

## 生产环境建议

1. **资源限制**: 在 docker-compose.yml 中添加 resources 限制
2. **环境变量**: 使用 .env 文件管理敏感配置
3. **反向代理**: 建议使用 Nginx/Traefik 作为反向代理
4. **监控**: 集成 Prometheus/Grafana 监控
5. **日志**: 集成 ELK/EFK 日志收集
6. **备份**: 定期备份配置文件和数据

## 故障排查

### 查看容器日志
```bash
docker-compose logs qs-apiserver
docker-compose logs collection-server
```

### 进入容器
```bash
docker-compose exec qs-apiserver sh
docker-compose exec collection-server sh
```

### 检查健康状态
```bash
docker-compose ps
curl http://localhost:8081/health
curl http://localhost:8082/health
```

### 重启服务
```bash
docker-compose restart qs-apiserver
docker-compose restart collection-server
```

## 更新部署

```bash
# 1. 拉取最新代码
git pull

# 2. 重新构建镜像
docker-compose build

# 3. 停止旧容器
docker-compose stop

# 4. 启动新容器
docker-compose up -d

# 5. 清理旧镜像
docker image prune -f
```

## 注意事项

1. 确保配置文件中的数据库连接信息正确
2. 生产环境建议使用具体版本号，避免使用 latest 标签
3. 定期更新基础镜像以获取安全补丁
4. 配置文件中的敏感信息建议使用 Docker secrets 或环境变量
5. 监控容器资源使用情况，及时调整资源限制
