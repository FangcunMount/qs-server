# 接口与运维

本组文档介绍 `qs-server` 的对外契约和运维入口，重点回答四个问题：

- 两套 REST API 如何分工，谁面向后台，谁面向前台
- `apiserver` 的 gRPC 契约如何组织，哪些服务给 `collection-server`，哪些只给 `worker`
- 各进程在开发和生产环境分别监听哪些端口，TLS 和 mTLS 放在哪里
- 周期任务如何触发，事件型后台任务如何和运维调度区分

如果你第一次读这组文档，建议顺序是：

1. [01-REST契约.md](./01-REST契约.md)
2. [02-gRPC契约.md](./02-gRPC契约.md)
3. [03-部署与端口.md](./03-部署与端口.md)
4. [04-调度与后台任务.md](./04-调度与后台任务.md)

阅读这组文档时，最好同时结合这些总览与运行时文档：

- [../00-总览/01-系统地图.md](../00-总览/01-系统地图.md)
- [../01-运行时/01-apiserver.md](../01-运行时/01-apiserver.md)
- [../01-运行时/02-collection-server.md](../01-运行时/02-collection-server.md)
- [../01-运行时/03-worker.md](../01-运行时/03-worker.md)
- [../01-运行时/04-进程间通信.md](../01-运行时/04-进程间通信.md)

本组最重要的代码和配置入口是：

- REST 契约导出：
  [../../api/rest/apiserver.yaml](../../api/rest/apiserver.yaml)
  [../../api/rest/collection.yaml](../../api/rest/collection.yaml)
- REST 路由注册：
  [../../internal/apiserver/routers.go](../../internal/apiserver/routers.go)
  [../../internal/collection-server/routers.go](../../internal/collection-server/routers.go)
- gRPC 契约与注册：
  [../../internal/apiserver/interface/grpc/proto](../../internal/apiserver/interface/grpc/proto)
  [../../internal/apiserver/grpc_registry.go](../../internal/apiserver/grpc_registry.go)
  [../../internal/pkg/grpc/server.go](../../internal/pkg/grpc/server.go)
- 端口与部署：
  [../../configs/apiserver.dev.yaml](../../configs/apiserver.dev.yaml)
  [../../configs/apiserver.prod.yaml](../../configs/apiserver.prod.yaml)
  [../../configs/collection-server.dev.yaml](../../configs/collection-server.dev.yaml)
  [../../configs/collection-server.prod.yaml](../../configs/collection-server.prod.yaml)
  [../../configs/worker.dev.yaml](../../configs/worker.dev.yaml)
  [../../configs/worker.prod.yaml](../../configs/worker.prod.yaml)
  [../../build/docker/docker-compose.dev.yml](../../build/docker/docker-compose.dev.yml)
  [../../build/docker/docker-compose.prod.yml](../../build/docker/docker-compose.prod.yml)
- 调度脚本：
  [../../configs/crontab/qs-scheduler](../../configs/crontab/qs-scheduler)
  [../../configs/crontab/api-call.sh](../../configs/crontab/api-call.sh)
  [../../configs/crontab/refresh-token.sh](../../configs/crontab/refresh-token.sh)

这组文档重点说明当前可用的契约文件、端口布局和运维入口，不展开长篇部署手册。
