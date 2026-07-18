# Runtime Infrastructure

## 1. 结论

运行时基础设施负责配置装载、依赖装配、后台 runner、health/readiness 和资源释放；业务模块只声明依赖，不自行创建全局客户端。

## 2. 事实源

- 配置：`configs/*.yaml` 与环境变量说明；
- 进程生命周期：`internal/apiserver`、`internal/collection-server`、`internal/worker`；
- apiserver 组合根：`internal/apiserver/container`；
- 业务模块：`container/modules/*/wire.go`、`install.go`、`bootstrap.go`；
- platform 能力：`container/modules/platform`。

## 3. 验证

检查配置解析、缺失依赖、启动失败回滚、health/readiness、runner cancel 和 Cleanup 顺序。端口与资源配额以部署配置为准，不在本文复制。
