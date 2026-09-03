# 配置与部署

本目录是配置、Secret、镜像、CD、网络拓扑和部署验收的事实入口。它回答“进程最终读取了什么”“哪些文件必须随包交付”“端口如何暴露”“一次部署怎样才算完成”，不记录某台主机在某个时点运行了多少副本。

## 阅读顺序

1. [配置、Secret 与外部文件](./10-配置、Secret与外部文件.md)
2. [镜像、CD 与网络拓扑](./20-镜像、CD与网络拓扑.md)
3. [部署验收与回滚](./30-部署验收与回滚.md)

## Canonical 边界

| 问题 | 唯一主文档 | 其它文档只允许做什么 |
| --- | --- | --- |
| 配置、Secret、外部文件 | [10](./10-配置、Secret与外部文件.md) | 进程文档可引用其有效配置，sidecar 可说明本包需要的单个文件 |
| 镜像、CD、网络 | [20](./20-镜像、CD与网络拓扑.md) | [接口与运维](../../04-接口与运维/README.md)只保留执行命令和故障处置 |
| 部署验收、回滚 | [30](./30-部署验收与回滚.md) | 生产结果只写入[基础设施生产证据台账](../../00-总览/10-基础设施生产证据台账.md) |

源码支持一种能力，不等于生产已启用；仓库配置表达部署意图，不等于实际容器正在使用；某次部署通过，也不等于之后持续健康。三类事实必须分开。

## 事实源与验证

- 配置：`configs/*.yaml`、三个进程的 `options` 包、`internal/pkg/configcontract`。
- 打包：`build/docker/Dockerfile.*`、`scripts/cd/prepare-package.sh`。
- 部署：`build/docker/docker-compose.prod.yml`、`.github/workflows/cd.yml`、`scripts/cd/remote-deploy.sh`。
- 静态回归：
  `go test -count=1 ./internal/pkg/configcontract ./internal/pkg/options ./internal/pkg/grpc ./internal/apiserver/options ./internal/collection-server/options ./internal/worker/options`
  。
- CD 脚本回归：
  `bash scripts/cd/test-prepare-package.sh`、`bash scripts/cd/test-plan-services.sh`、
  `bash scripts/cd/test-production-compose-network-exposure.sh`。

真实 Secret 挂载、目标网络连通、有效配置摘要、精确镜像 SHA 和部署后探针只能由目标环境验证；本目录不会把本地静态测试提升为这些证据。
