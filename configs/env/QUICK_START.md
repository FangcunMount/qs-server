# 本地配置快速开始

> 本页只保留当前可执行的最短路径。完整配置边界见 [环境配置目录](README.md)。

## 1. 准备基础设施

MySQL、MongoDB、Redis 与 NSQ 由基础设施环境提供。先验证连通性：

```bash
make check-infra
```

需要临时指定检查目标时，直接给检查命令传入无前缀的基础设施变量：

```bash
MYSQL_HOST=127.0.0.1 MYSQL_PORT=3306 make check-mysql
```

## 2. 启动三个进程

开发环境完整启动：

```bash
make quick-start
```

该命令使用：

- `configs/apiserver.dev.yaml`
- `configs/collection-server.dev.yaml`
- `configs/worker.dev.yaml`

启动后检查：

```bash
make health
```

## 3. 按需覆盖应用配置

应用不会自动加载 `configs/env/.env.dev`。覆盖 YAML 字段时必须使用应用前缀：

```bash
QS_APISERVER_MYSQL_HOST=127.0.0.1 make run-apiserver
COLLECTION_SERVER_GRPC_CLIENT_ENDPOINT=127.0.0.1:9090 make run-collection
QS_WORKER_MESSAGING_NSQ_ADDR=127.0.0.1:4150 make run-worker
```

前缀分别为：

- `QS_APISERVER_`
- `COLLECTION_SERVER_`
- `QS_WORKER_`

YAML 路径中的点号和连字符转换为下划线。

## 4. 选择生产 YAML

```bash
ENV=prod make build-all
```

这只切换 Makefile 使用的配置文件，不会自动准备生产密钥、证书、数据库或 systemd 环境。生产发布必须继续执行部署预检和运行态验收。

## 常见误区

- `source configs/env/.env.dev` 不等于应用配置已覆盖。
- `MYSQL_HOST` 与 `QS_APISERVER_MYSQL_HOST` 不是同一个配置层。
- `make check-infra` 通过只证明依赖可连接，不证明三个进程已健康。
- `.env.prod` 中的占位符不是线上运行事实。

## 继续阅读

- [环境配置目录](README.md)
- [本地开发与配置约定](../../docs/00-总览/06-本地开发与配置约定.md)
- [配置与环境变量](../../docs/04-接口与运维/05-配置与环境变量.md)
- [部署与端口](../../docs/04-接口与运维/06-部署与端口.md)
