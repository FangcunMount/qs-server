# 端口分布说明

项目按环境拆分端口：开发环境使用 1808* 系列；生产容器内统一 8080/8443/9090/9091，宿主机映射后移一位以避开 IAM（8080/8443/9090/9091）。

## 开发环境（本地/air/make）

| 组件 | HTTP | HTTPS | gRPC | gRPC Health | 配置来源 |
| --- | --- | --- | --- | --- | --- |
| qs-apiserver | 18082 | 18442 | 9090 | 9091 | `configs/apiserver.dev.yaml` |
| collection-server | 18083 | 18443 | - | - | `configs/collection-server.dev.yaml` |

- Makefile 默认 `ENV=dev`，`run-*`/`dev-*` 目标使用上述端口。
- collection-server 的 gRPC 客户端默认指向 apiserver `127.0.0.1:9090`。

## 生产环境（容器内与宿主机映射）

| 组件 | 容器 HTTP | 容器 HTTPS | 容器 gRPC | 容器 gRPC Health | 宿主机映射 HTTP | 宿主机映射 HTTPS | 宿主机映射 gRPC | 宿主机映射 Health | 配置来源 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| qs-apiserver | 8080 | 8443 | 9090 | 9091 | 8081 | 8444 | 9091 | 9092 | `configs/apiserver.prod.yaml`, `build/docker/docker-compose.yml` |
| collection-server | 8080 | 8443 | - | - | 8082 | 8445 | - | - | `configs/collection-server.prod.yaml`, `build/docker/docker-compose.yml` |

- Compose 将宿主机端口后移一位（8081/8444/9091/9092、8082/8445）以避免与已部署的 IAM 系统端口冲突。

## 基础设施（默认值）

来源：`configs/env/.env.dev`、`configs/env/.env.prod`、`scripts/check-infra.sh`

| 组件 | 主机 | 端口 | 说明 |
| --- | --- | --- | --- |
| MySQL | 127.0.0.1 | 3306 | |
| Redis Cache | 127.0.0.1 | 6379 | |
| Redis Store | 127.0.0.1 | 6380 | |
| MongoDB | 127.0.0.1 | 27017 | |
| NSQ lookupd | 127.0.0.1 | 4161 (HTTP) / 4160 (TCP) | |
| NSQ nsqd | 127.0.0.1 | 4151 (HTTP) / 4150 (TCP) | |
| NSQ admin | 127.0.0.1 | 4171 | |

## 其他引用/脚本

- 评估服务（evaluation-server）示例/脚本仍使用 8082（见 `test-message-queue.sh`、`test-evaluation-server.sh`）；正式配置文件未包含在本仓库，可按需要对齐到 8082 或 18084。
- 老文档或示例中的 9080/9081 端口已废弃，当前有效端口以上表和配置文件为准。
