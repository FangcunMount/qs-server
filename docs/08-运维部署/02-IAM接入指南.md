# QS 系统 IAM 接入指南

## 1. 概述

QS 系统已完成与 IAM 统一认证系统的集成，支持：

- JWT Token 身份验证（本地 JWKS 验签）
- 用户信息查询（gRPC）
- 监护关系验证
- mTLS 安全通信

## 2. 架构

```text
请求 → Gin Router → JWT中间件(JWKS验签) → 业务层 → IAM gRPC客户端 → IAM服务
                           ↓                                    ↑
                    解析用户信息                         mTLS双向认证
```

## 3. 已集成的服务

| 服务 | 端口 | IAM状态 | 说明 |
|------|------|---------|------|
| qs-apiserver | 8081 | ✅ 已集成 | API网关，所有 /api/v1 路由需认证 |
| qs-collection-server | 8082 | ✅ 已集成 | 问卷收集服务，所有 /api/v1 路由需认证 |
| qs-worker | - | ❌ 无需集成 | 事件处理器，内部服务 |

## 4. 配置说明

### 4.1 核心配置

```yaml
# configs/apiserver.prod.yaml
# configs/collection-server.prod.yaml

iam:
  enabled: true  # 启用IAM集成
  
  grpc:
    address: "iam-apiserver:9080"  # IAM gRPC地址
    timeout: 5s
    retry-max: 3
    
    tls:
      enabled: true  # 启用mTLS
      ca-file: "/app/certs/ca/ca-chain.crt"
      cert-file: "/app/certs/clients/qs.crt"
      key-file: "/app/certs/clients/qs.key"
  
  jwt:
    issuer: "http://iam-apiserver:9080"
    audience: ["qs"]
    algorithms: ["RS256", "ES256"]
  
  jwks:
    url: "http://iam-apiserver:9080/.well-known/jwks.json"
    fetch-strategies: ["grpc", "http", "cache"]
    cache-duration: 1h
  
  cache:
    users:
      enabled: true
      ttl: 5m
    guardianship:
      enabled: true
      ttl: 10m
```

### 4.2 证书挂载

**Docker Compose 配置**：

```yaml
services:
  qs-apiserver:
    volumes:
      # IAM mTLS 证书
      - /data/ssl/iam-contracts/grpc/ca:/app/certs/ca:ro
      - /data/ssl/iam-contracts/grpc/clients:/app/certs/clients:ro
```

**宿主机证书要求**：

```bash
/data/ssl/iam-contracts/grpc/
├── ca/ca-chain.crt           # CA证书链（验证IAM服务端）
└── clients/
    ├── qs.crt                # QS客户端证书
    └── qs.key                # QS客户端私钥（权限600）
```

## 5. 代码集成点

### 5.1 主要文件

```text
internal/
├── apiserver/
│   ├── infra/iam/client.go           # IAM SDK客户端封装
│   ├── container/iam_module.go       # IAM模块生命周期
│   ├── server.go                     # 初始化IAM模块
│   └── routers.go                    # 应用JWT中间件
├── collection-server/
│   ├── infra/iam/client.go
│   ├── container/iam_module.go
│   ├── server.go
│   └── routers.go
└── pkg/middleware/jwt_auth.go        # JWT中间件（共享）
```

### 5.2 中间件使用

```go
// routers.go
if container.IAMModule != nil && container.IAMModule.IsEnabled() {
    apiV1.Use(middleware.JWTAuthMiddleware(container.IAMModule.Client().SDK()))
}

// 业务代码获取用户信息
userID := middleware.GetUserID(c)           // 从上下文获取
tenantID := middleware.GetTenantID(c)
roles := middleware.GetRoles(c)
```

### 5.3 监护关系验证

```go
client := container.IAMModule.Client()
resp, err := client.SDK().Guardianship().IsGuardian(ctx, &guardianshippb.IsGuardianRequest{
    ParentUserId: parentID,
    ChildUserId:  childID,
})
```

## 6. 部署步骤

### 6.1 前置条件

```bash
# 1. 检查证书存在
ls -lah /data/ssl/iam-contracts/grpc/ca/ca-chain.crt
ls -lah /data/ssl/iam-contracts/grpc/clients/qs.{crt,key}

# 2. 验证证书有效期
openssl x509 -in /data/ssl/iam-contracts/grpc/clients/qs.crt -noout -dates

# 3. 验证证书链
openssl verify -CAfile /data/ssl/iam-contracts/grpc/ca/ca-chain.crt \
  /data/ssl/iam-contracts/grpc/clients/qs.crt
```

### 6.2 启动服务

```bash
# 使用现有的 docker-compose 启动
cd build/docker
docker-compose -f docker-compose.prod.yml up -d

# 查看日志
docker logs -f qs-apiserver
docker logs -f qs-collection-server
```

### 6.3 验证启动

**成功日志**：

```text
INFO: Initializing IAM SDK client...
INFO: Loading TLS certificates...
INFO: mTLS handshake successful
INFO: IAM SDK client initialized successfully
INFO: 🔐 JWT authentication middleware enabled for /api/v1
```

## 7. 测试验证

### 7.1 健康检查

```bash
# 不需要认证的端点
curl http://localhost:8081/healthz
curl http://localhost:8082/healthz
```

### 7.2 认证测试

```bash
# 1. 无Token（应返回401）
curl http://localhost:8081/api/v1/questionnaires

# 2. 有效Token（应成功）
curl -H "Authorization: Bearer <valid-token>" \
     http://localhost:8081/api/v1/questionnaires

# 3. 查看Token解析信息
curl -H "Authorization: Bearer <token>" \
     http://localhost:8081/api/v1/me
```

### 7.3 容器内验证

```bash
# 检查证书挂载
docker exec qs-apiserver ls -lah /app/certs/ca/
docker exec qs-apiserver ls -lah /app/certs/clients/

# 检查证书可读
docker exec qs-apiserver cat /app/certs/ca/ca-chain.crt > /dev/null && echo "✅ CA可读"
docker exec qs-apiserver cat /app/certs/clients/qs.crt > /dev/null && echo "✅ 证书可读"
docker exec qs-apiserver cat /app/certs/clients/qs.key > /dev/null && echo "✅ 私钥可读"
```

## 8. 故障排查

### 8.1 常见错误

| 错误信息 | 原因 | 解决方案 |
|---------|------|---------|
| `no such file or directory` | 证书未挂载 | 检查 docker-compose.yml 挂载配置 |
| `certificate verify failed` | CA证书不匹配 | 确保使用正确的 ca-chain.crt |
| `permission denied` | 证书权限不足 | 调整宿主机证书权限（ca/crt: 644, key: 600） |
| `connection refused :9080` | IAM服务未启动 | 检查 iam-apiserver 容器状态 |
| `context deadline exceeded` | gRPC超时 | 检查网络连通性，增加 timeout 配置 |
| `token invalid` | JWT验签失败 | 检查 JWKS URL 和 issuer 配置 |

### 8.2 调试命令

```bash
# 检查IAM连通性
docker exec qs-apiserver ping -c 3 iam-apiserver
docker exec qs-apiserver nc -zv iam-apiserver 9080

# 查看IAM服务状态
docker ps | grep iam-apiserver
docker logs iam-apiserver | tail -n 50

# 检查证书链
docker exec qs-apiserver sh -c '
  openssl verify -CAfile /app/certs/ca/ca-chain.crt /app/certs/clients/qs.crt
'

# 查看配置
docker exec qs-apiserver cat /app/configs/apiserver.prod.yaml | grep -A 30 "iam:"
```

## 9. 运维操作

### 9.1 临时禁用IAM

```yaml
# 修改配置文件
iam:
  enabled: false  # 改为 false

# 重启服务
docker restart qs-apiserver qs-collection-server
```

### 9.2 证书更新

```bash
# 1. 备份旧证书
cp -r /data/ssl/iam-contracts/grpc/clients/qs.{crt,key} /backup/

# 2. 替换新证书
cp new-qs.crt /data/ssl/iam-contracts/grpc/clients/qs.crt
cp new-qs.key /data/ssl/iam-contracts/grpc/clients/qs.key

# 3. 重启服务
docker restart qs-apiserver qs-collection-server

# 4. 验证
docker logs -f qs-apiserver | grep "mTLS handshake successful"
```

### 9.3 监控指标

关注以下日志：

- IAM gRPC 调用延迟
- JWKS 缓存命中率
- 用户信息缓存命中率
- Token 验证失败次数
- 证书过期告警（提前30天）

## 10. 安全建议

- ✅ 私钥文件权限设置为 600
- ✅ 使用只读挂载 `:ro`
- ✅ 定期轮换证书（建议90天）
- ✅ 启用 mTLS（即使同一网络）
- ✅ 监控证书过期时间
- ✅ 审计 IAM 调用日志
- ✅ 限制 gRPC 超时和重试次数

## 11. 相关文件

- `configs/apiserver.prod.yaml` - API服务器配置
- `configs/collection-server.prod.yaml` - Collection服务器配置
- `build/docker/docker-compose.prod.yml` - 生产环境部署
- `build/docker/docker-compose.dev.yml` - 开发环境部署
- `internal/pkg/middleware/jwt_auth.go` - JWT中间件实现
- `internal/apiserver/infra/iam/client.go` - IAM客户端封装
