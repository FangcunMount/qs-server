# 接口与运维

本层提供机器契约入口、执行型接入指南和运行操作，不复制维护完整 API 字段表。

## 1. 契约入口

| 契约 | 文档 | 机器事实源 |
| --- | --- | --- |
| 总览 | [接口契约总览](./00-接口契约总览.md) | `api/README.md` |
| apiserver REST | [apiserver REST](./01-apiserver-REST.md) | `api/rest/apiserver.yaml` |
| collection REST | [collection REST](./02-collection-REST.md) | `api/rest/collection.yaml` |
| gRPC | [gRPC 契约](./03-gRPC契约.md) | `api/grpc/proto` |
| internal gRPC | [internal gRPC](./04-internal-gRPC.md) | proto + service/client 实现 |

## 2. 运维入口

- [配置与环境变量](./05-配置与环境变量.md)
- [部署与端口](./06-部署与端口.md)
- [调度任务](./07-调度任务.md)
- [健康检查与观测](./08-健康检查与观测.md)
- [常见排障](./09-常见排障.md)

## 3. 保留的执行指南

- [容量档位建议](./10-QPS容量档位与资源配置建议.md)
- [300QPS 混合场景压测 SOP](./11-300QPS混合场景压测SOP.md)
- [小程序报告等待接入](./12-小程序报告等待接入指南.md)
- [小程序接入文档](./15-小程序接入文档.md)
- [测评后台接入文档](./16-测评后台接入文档.md)

这些指南仍在现行层，但逐端点复核状态见 [重建状态](../MIGRATION-STATUS.md)。发生冲突时以机器契约和当前前端调用为准。
