# gRPC mTLS 配置指南

> **版本**: V1.0  
> **更新日期**: 2025-12-08  
> **适用范围**: qs-apiserver, collection-server

---

## 概述

本文档说明如何在 QS Server 中配置 gRPC 服务的 mTLS（双向 TLS 认证），实现服务间安全通信。

### 🔒 安全架构

```text
collection-server (前端数据收集)
    ↓ gRPC + mTLS
apiserver (核心业务处理)
    ↓ 
MySQL + MongoDB
```

### 🎯 为什么需要 mTLS？

| 安全维度 | 无 TLS | 单向 TLS | mTLS (已实现) |
| --------- | ------- | --------- | -------------- |
| 传输加密 | ❌ 明文 | ✅ 加密 | ✅ 加密 |
| 服务端身份验证 | ❌ 无 | ✅ 客户端验证服务端 | ✅ 双向验证 |
| 客户端身份验证 | ❌ 无 | ❌ 无 | ✅ 服务端验证客户端 |
| 防止中间人攻击 | ❌ | ✅ | ✅✅ |
| 防止恶意服务接入 | ❌ | ❌ | ✅ |
| 适用场景 | 开发环境 | 公共 API | 内部服务 ✅ |

---

## 证书结构

### 推荐的证书目录结构

> **与 IAM 接入指南保持一致**：使用 infra 项目统一管理的证书结构

```text
/data/infra/ssl/grpc/
├── ca/
│   ├── ca-chain.crt          # CA 证书链（所有服务共享）
│   ├── intermediate-ca.crt   # 中间 CA 证书
│   ├── root-ca.crt           # 根 CA 证书
│   └── ca.key                # CA 私钥（仅用于签发证书）
└── server/
    ├── qs-apiserver.crt      # apiserver 服务端证书
    ├── qs-apiserver.key      # apiserver 服务端私钥
    ├── qs-apiserver-fullchain.crt  # 带链的完整证书（可选）
    ├── qs-collection-server.crt     # collection-server 客户端证书
    ├── qs-collection-server.key     # collection-server 客户端私钥
    ├── qs-collection-server-fullchain.crt  # 带链的完整证书（可选）
    ├── iam-grpc.crt          # IAM gRPC 服务端证书
    └── iam-grpc.key          # IAM gRPC 服务端私钥
```

**说明**：

- 所有证书由 infra 项目统一生成和管理
- 每个服务有独立的证书，共享同一 CA 链
- 开发环境：由 infra 项目的 `scripts/cert/generate-grpc-certs.sh` 生成
- 生产环境：联系运维团队在 infra 项目中生成

### 证书要求

#### CA 证书

- 用于签发所有服务端和客户端证书
- 所有服务必须信任同一个 CA

#### 服务端证书（apiserver）

- **CN (Common Name)**: `qs-apiserver` 或具体域名
- **SAN (Subject Alternative Name)**:
  - `DNS:qs-apiserver`
  - `DNS:apiserver`
  - `DNS:localhost`
  - `IP:127.0.0.1`

#### 客户端证书（collection-server）

- **CN**: `collection-server`
- **OU (Organizational Unit)**: `qs-platform`
- 必须被 apiserver 的白名单允许

---

## 配置说明

### 1. apiserver 配置

#### 开发环境 (`configs/apiserver.dev.yaml`)

```yaml
grpc:
  bind-address: "127.0.0.1"
  bind-port: 9090
  
  # 开发环境使用不安全连接（无需证书）
  insecure: true
  
  # mTLS 配置（开发环境禁用）
  mtls:
    enabled: false
  
  # 功能开关
  enable-reflection: true     # 启用反射，方便 grpcurl 调试
  enable-health-check: true

# IAM 集成配置
iam:
  enabled: false              # 开发环境禁用 IAM（避免证书依赖）
  grpc:
    tls:
      enabled: false          # 开发环境禁用 TLS
```

**说明**：

- 开发环境默认禁用 mTLS，无需配置证书
- IAM 集成在开发环境也默认禁用，避免证书文件依赖
- 如需在开发环境测试 mTLS，设置 `enabled: true` 并配置证书路径

#### 生产环境 (`configs/apiserver.prod.yaml`)

```yaml
grpc:
  bind-address: "0.0.0.0"
  bind-port: 9090
  
  # 生产环境必须启用 TLS
  insecure: false
  tls-cert-file: "/etc/qs-server/cert/grpc/server/qs-apiserver.crt"
  tls-key-file: "/etc/qs-server/cert/grpc/server/qs-apiserver.key"
  
  # 消息大小限制
  max-msg-size: 4194304  # 4MB
  
  # 连接管理
  max-connection-age: 120s
  max-connection-age-grace: 20s
  
  # mTLS 双向认证配置
  mtls:
    enabled: true
    ca-file: "/etc/qs-server/cert/grpc/ca/ca-chain.crt"
    require-client-cert: true
    
    # 证书白名单：只允许这些服务访问
    allowed-cns:
      - "collection-server"
      - "evaluation-server"
      - "admin-tool"
    
    # 组织单元白名单
    allowed-ous:
      - "qs-platform"
      - "qs-ops"
    
    # TLS 版本控制
    min-tls-version: "1.2"
  
  # 功能开关
  enable-reflection: false    # 生产环境禁用反射
  enable-health-check: true
```

### 2. collection-server 配置

#### 开发环境 (`configs/collection-server.dev.yaml`)

```yaml
grpc_client:
  endpoint: "127.0.0.1:9090"
  timeout: 30
  insecure: true  # 开发环境使用不安全连接
  
  # mTLS 配置（开发环境注释掉）
  # tls-cert-file: "configs/cert/grpc/client/collection-server.crt"
  # tls-key-file: "configs/cert/grpc/client/collection-server.key"
  # tls-ca-file: "configs/cert/grpc/ca/ca-chain.crt"
  # tls-server-name: "qs-apiserver"
```

#### 生产环境 (`configs/collection-server.prod.yaml`)

```yaml
grpc_client:
  endpoint: "apiserver:9090"
  timeout: 30
  insecure: false  # 生产环境启用 TLS
  
  # mTLS 客户端配置
  tls-cert-file: "/etc/qs-server/cert/grpc/client/collection-server.crt"
  tls-key-file: "/etc/qs-server/cert/grpc/client/collection-server.key"
  tls-ca-file: "/etc/qs-server/cert/grpc/ca/ca-chain.crt"
  tls-server-name: "qs-apiserver"  # 服务端证书的 CN
```

---

## 证书生成

### 使用 infra 项目生成证书（推荐）

> **与 IAM 接入指南保持一致**

#### 开发环境

```bash
# 1. 首次运行：生成 CA 证书（如果已存在则跳过）
cd /path/to/infra
./scripts/cert/generate-grpc-certs.sh generate-ca

# 2. 为 QS apiserver 生成证书
./scripts/cert/generate-grpc-certs.sh generate-server qs-apiserver QS qs-apiserver.internal.example.com

# 3. 为 collection-server 生成证书
./scripts/cert/generate-grpc-certs.sh generate-server qs-collection-server QS qs-collection-server.internal.example.com

# 4. 验证证书
./scripts/cert/generate-grpc-certs.sh verify

# 证书存放位置：
# /data/infra/ssl/grpc/
# ├── ca/
# │   └── ca-chain.crt      # CA 证书链
# └── server/
#     ├── qs-apiserver.crt  # apiserver 证书
#     ├── qs-apiserver.key  # apiserver 私钥
#     ├── qs-collection-server.crt # collection-server 证书
#     └── qs-collection-server.key # collection-server 私钥
```

#### 生产环境

> ⚠️ **重要**: 生产环境的证书通过 CI/CD 管道自动注入到容器中，不使用宿主机挂载。

**证书注入方式**：

1. **infra 项目生成证书**

   ```bash
   cd /path/to/infra
   ./scripts/cert/generate-grpc-certs.sh generate-server qs-apiserver QS qs-apiserver.svc
   ./scripts/cert/generate-grpc-certs.sh generate-server qs-collection-server QS qs-collection-server.svc
   ```

2. **CI/CD 管道将证书注入容器**

   ```yaml
   # .github/workflows/deploy.yml 或 GitLab CI
   steps:
     - name: Inject certificates
       run: |
         # 从 Secrets 读取证书
         echo "$GRPC_CA_CERT" | base64 -d > /tmp/ca-chain.crt
         echo "$GRPC_SERVER_CERT" | base64 -d > /tmp/qs-apiserver.crt
         echo "$GRPC_SERVER_KEY" | base64 -d > /tmp/qs-apiserver.key
         
         # 构建镜像时注入证书
         docker build \
           --secret id=grpc_ca,src=/tmp/ca-chain.crt \
           --secret id=grpc_cert,src=/tmp/qs-apiserver.crt \
           --secret id=grpc_key,src=/tmp/qs-apiserver.key \
           -t qs-apiserver:latest .
   ```

3. **Dockerfile 接收证书**

   ```dockerfile
   # Dockerfile
   FROM golang:1.24 AS builder
   WORKDIR /app
   COPY . .
   RUN go build -o qs-apiserver ./cmd/qs-apiserver
   
   FROM alpine:3.18
   
   # 创建证书目录
   RUN mkdir -p /data/infra/ssl/grpc/{ca,server}
   
   # 从构建密钥注入证书（CI/CD 时）
   RUN --mount=type=secret,id=grpc_ca \
       --mount=type=secret,id=grpc_cert \
       --mount=type=secret,id=grpc_key \
       cat /run/secrets/grpc_ca > /data/infra/ssl/grpc/ca/ca-chain.crt && \
       cat /run/secrets/grpc_cert > /data/infra/ssl/grpc/server/qs-apiserver.crt && \
       cat /run/secrets/grpc_key > /data/infra/ssl/grpc/server/qs-apiserver.key && \
       chmod 600 /data/infra/ssl/grpc/server/*.key
   
   COPY --from=builder /app/qs-apiserver /usr/local/bin/
   
   ENTRYPOINT ["qs-apiserver"]
   ```

4. **Kubernetes Secrets 方式（可选）**

   ```yaml
   # k8s-secrets.yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: qs-grpc-certs
   type: Opaque
   data:
     ca-chain.crt: <base64-encoded-ca-cert>
     qs-apiserver.crt: <base64-encoded-server-cert>
     qs-apiserver.key: <base64-encoded-server-key>
   ---
   # deployment.yaml
   apiVersion: apps/v1
   kind: Deployment
   spec:
     template:
       spec:
         containers:
         - name: qs-apiserver
           volumeMounts:
           - name: grpc-certs
             mountPath: /data/infra/ssl/grpc
             readOnly: true
         volumes:
         - name: grpc-certs
           secret:
             secretName: qs-grpc-certs
             items:
             - key: ca-chain.crt
               path: ca/ca-chain.crt
             - key: qs-apiserver.crt
               path: server/qs-apiserver.crt
             - key: qs-apiserver.key
               path: server/qs-apiserver.key
               mode: 0600
   ```

**配置路径说明**：

```yaml
# 所有环境使用统一的容器内路径
/data/infra/ssl/grpc/ca/ca-chain.crt           # CA 证书链
/data/infra/ssl/grpc/server/qs-apiserver.crt   # 服务器证书
/data/infra/ssl/grpc/server/qs-apiserver.key   # 服务器私钥
```

**证书来源**：

- **开发环境**: 直接使用宿主机 `/data/infra/ssl/` 目录
- **生产环境**: CI/CD 管道将证书写入容器的 `/data/infra/ssl/` 目录

**安全要点**：

- ✅ 证书以 Secrets 形式存储在 CI/CD 平台
- ✅ 构建时动态注入，不暴露在镜像层中
- ✅ 容器内只读挂载，权限 600
- ✅ 定期通过 CI/CD 轮换证书

### 手动使用 OpenSSL 生成（不推荐）

如果需要手动生成证书（用于测试或理解），可以参考以下步骤：

#### 1. 生成 CA 证书

```bash
# 创建目录
mkdir -p /data/infra/ssl/grpc/{ca,server}

# 生成 CA 私钥
openssl genrsa -out /data/infra/ssl/grpc/ca/ca.key 4096

# 生成 CA 证书
openssl req -new -x509 -days 3650 -key /data/infra/ssl/grpc/ca/ca.key \
  -out /data/infra/ssl/grpc/ca/ca-chain.crt \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=QS Platform/OU=Platform/CN=QS Root CA"
```

#### 2. 生成服务端证书（apiserver）

```bash
# 生成私钥
openssl genrsa -out /data/infra/ssl/grpc/server/qs-apiserver.key 2048

# 生成证书签名请求（CSR）
openssl req -new -key /data/infra/ssl/grpc/server/qs-apiserver.key \
  -out /data/infra/ssl/grpc/server/qs-apiserver.csr \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=QS Platform/OU=qs-platform/CN=qs-apiserver"

# 创建扩展配置（SAN）
cat > /tmp/server-ext.cnf << EOF
subjectAltName = DNS:qs-apiserver,DNS:apiserver,DNS:localhost,IP:127.0.0.1
extendedKeyUsage = serverAuth
EOF

# 使用 CA 签发证书
openssl x509 -req -in /data/infra/ssl/grpc/server/qs-apiserver.csr \
  -CA /data/infra/ssl/grpc/ca/ca-chain.crt \
  -CAkey /data/infra/ssl/grpc/ca/ca.key \
  -CAcreateserial -out /data/infra/ssl/grpc/server/qs-apiserver.crt \
  -days 365 -extfile /tmp/server-ext.cnf
```

#### 3. 生成客户端证书（collection-server）

```bash
# 生成私钥
openssl genrsa -out /data/infra/ssl/grpc/server/qs-collection-server.key 2048

# 生成 CSR
openssl req -new -key /data/infra/ssl/grpc/server/qs-collection-server.key \
  -out /data/infra/ssl/grpc/server/qs-collection-server.csr \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=QS Platform/OU=qs-platform/CN=qs-collection-server"

# 创建扩展配置
cat > /tmp/client-ext.cnf << EOF
extendedKeyUsage = clientAuth
EOF

# 使用 CA 签发证书
openssl x509 -req -in /data/infra/ssl/grpc/server/qs-collection-server.csr \
  -CA /data/infra/ssl/grpc/ca/ca-chain.crt \
  -CAkey /data/infra/ssl/grpc/ca/ca.key \
  -CAcreateserial -out /data/infra/ssl/grpc/server/qs-collection-server.crt \
  -days 365 -extfile /tmp/client-ext.cnf
```

### 验证证书

```bash
# 验证服务端证书
openssl verify -CAfile /data/infra/ssl/grpc/ca/ca-chain.crt \
  /data/infra/ssl/grpc/server/qs-apiserver.crt

# 验证客户端证书
openssl verify -CAfile /data/infra/ssl/grpc/ca/ca-chain.crt \
  /data/infra/ssl/grpc/server/qs-collection-server.crt

# 查看证书详情
openssl x509 -in /data/infra/ssl/grpc/server/qs-apiserver.crt -text -noout

# 查看证书 CN 和 OU（用于白名单配置）
openssl x509 -in /data/infra/ssl/grpc/server/qs-collection-server.crt -noout -subject
```

---

## 部署流程

#### docker-compose.yml

```yaml
services:
  apiserver:
    image: qs-apiserver:latest
    volumes:
      # 挂载 infra 项目统一管理的证书
      - /data/infra/ssl/grpc:/data/infra/ssl/grpc:ro
      - ./configs/apiserver.prod.yaml:/app/configs/apiserver.yaml:ro
    environment:
      - CONFIG_FILE=/app/configs/apiserver.yaml
    ports:
      - "9090:9090"
    
  collection-server:
    image: collection-server:latest
    volumes:
      # 挂载 infra 项目统一管理的证书
      - /data/infra/ssl/grpc:/data/infra/ssl/grpc:ro
      - ./configs/collection-server.prod.yaml:/app/configs/collection-server.yaml:ro
    environment:
      - CONFIG_FILE=/app/configs/collection-server.yaml
    depends_on:
      - apiserver
```   - CONFIG_FILE=/app/configs/collection-server.yaml
    depends_on:
      - apiserver
```

#### 创建 Secret

```bash
kubectl create secret generic grpc-certs \
  --from-file=ca.crt=/data/infra/ssl/grpc/ca/ca-chain.crt \
  --from-file=apiserver.crt=/data/infra/ssl/grpc/server/qs-apiserver.crt \
  --from-file=apiserver.key=/data/infra/ssl/grpc/server/qs-apiserver.key \
  --from-file=collection.crt=/data/infra/ssl/grpc/server/qs-collection-server.crt \
  --from-file=collection.key=/data/infra/ssl/grpc/server/qs-collection-server.key
```

#### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: qs-apiserver
spec:
  template:
    spec:
      volumes:
        - name: grpc-certs
          secret:
            secretName: grpc-certs
      containers:
        - name: apiserver
          volumeMounts:
            - name: grpc-certs
              mountPath: /data/infra/ssl/grpc
              readOnly: true
```           mountPath: /etc/qs-server/cert/grpc
              readOnly: true
```

---

## 故障排查

### 常见错误

#### 1. `certificate signed by unknown authority`

**解决**:

```bash
# 检查 CA 文件是否正确
ls -l /data/infra/ssl/grpc/ca/ca-chain.crt

# 验证证书链
openssl verify -CAfile /data/infra/ssl/grpc/ca/ca-chain.crt \
  /data/infra/ssl/grpc/server/qs-apiserver.crt
```nssl verify -CAfile /etc/qs-server/cert/grpc/ca/ca-chain.crt \
  /etc/qs-server/cert/grpc/server/qs-apiserver.crt
```

#### 2. `tls: bad certificate`

**原因**: 服务端拒绝客户端证书

**解决**:

```bash
# 检查客户端证书的 CN 是否在白名单中
openssl x509 -in /data/infra/ssl/grpc/server/qs-collection-server.crt -noout -subject

# 检查 apiserver 配置
cat configs/apiserver.prod.yaml | grep -A 10 "allowed-cns"
```

#### 3. `x509: certificate has expired`

**原因**: 证书已过期

**解决**:

```bash
# 检查证书有效期
openssl x509 -in /data/infra/ssl/grpc/server/qs-apiserver.crt -noout -dates

# 使用 infra 项目重新生成证书
cd /path/to/infra
./scripts/cert/generate-grpc-certs.sh generate-server qs-apiserver QS qs-apiserver.internal.example.com
```

#### 测试 gRPC 连接

```bash
# 使用 grpcurl 测试（需要证书）
grpcurl \
  -cacert /data/infra/ssl/grpc/ca/ca-chain.crt \
  -cert /data/infra/ssl/grpc/server/qs-collection-server.crt \
  -key /data/infra/ssl/grpc/server/qs-collection-server.key \
  apiserver:9090 list

# 测试健康检查
grpcurl \
  -cacert /data/infra/ssl/grpc/ca/ca-chain.crt \
  -cert /data/infra/ssl/grpc/server/qs-collection-server.crt \
  -key /data/infra/ssl/grpc/server/qs-collection-server.key \
  apiserver:9090 grpc.health.v1.Health/Check
```key /etc/qs-server/cert/grpc/client/collection-server.key \
  apiserver:9090 grpc.health.v1.Health/Check
```

#### 查看日志

```bash
# apiserver 日志
docker logs -f qs-apiserver | grep -i "grpc\|tls\|mtls"

# collection-server 日志
docker logs -f qs-collection-server | grep -i "grpc\|tls\|mtls"
```

---

## 性能优化

### 连接池配置

```go
// collection-server/infra/grpcclient/manager.go
PoolSize: 5  // 增加连接池大小（高并发场景）
```

### Keep-Alive 配置

```yaml
# apiserver 配置
grpc:
  max-connection-age: 120s        # 连接最大存活时间
  max-connection-age-grace: 20s   # 关闭宽限期
```

---

## 安全最佳实践

1. ✅ **生产环境必须启用 mTLS**
2. ✅ **定期轮换证书**（建议每 90 天）
3. ✅ **使用强加密算法**（TLS 1.2+）
4. ✅ **限制证书白名单**（只允许必要的服务）
5. ✅ **保护私钥文件**（权限 600，只读挂载）
6. ✅ **监控证书过期时间**
7. ❌ **不要在代码中硬编码证书**
8. ❌ **不要将私钥提交到 Git**

---

## 参考资料

- [gRPC Authentication Guide](https://grpc.io/docs/guides/auth/)
- [OpenSSL Certificate Management](https://www.openssl.org/docs/)
- [component-base/pkg/grpc/mtls](https://github.com/FangcunMount/component-base)
- [内部文档: 端口配置](./01-端口配置.md)
- [内部文档: IAM 接入指南](./02-IAM接入指南.md)
