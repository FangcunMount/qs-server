# 接口与运维

本组文档说明 **REST / gRPC 契约、端口与部署映射、周期任务与事件型后台任务的分工**。写作约定见 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md) 的 **Why / What / Where / Verify**；与 [02-业务模块](../02-业务模块/)（领域与 Handler）、[03-基础设施](../03-基础设施/)（IAM、限流、事件配置）、[05-专题分析](../05-专题分析/)（设计叙事）**不重复**：此处固定 **契约入口、运维触发方式、部署事实**。

## 与其它文档的分工

| 文档组 | 侧重 |
| ------ | ---- |
| [02-业务模块](../02-业务模块/) | 各模块 REST/gRPC **业务语义**、用例与锚点 |
| [03-基础设施](../03-基础设施/) | JWT、**gRPC 拦截器链**、限流键、[`configs/events.yaml`](../../configs/events.yaml) |
| [01-运行时](../01-运行时/) | 各进程职责与启动顺序 |
| **04-接口与运维（本文）** | **OpenAPI 导出**、**proto 与注册**、端口表、Crontab 与进程内 ticker |

## 建议阅读顺序

1. [01-REST契约.md](./01-REST契约.md) — 双 REST 面、契约生成与 Verify  
2. [02-gRPC契约.md](./02-gRPC契约.md) — 服务矩阵、`GRPCRegistry`、Internal 与 worker  
3. [03-部署与端口.md](./03-部署与端口.md) — 监听地址、Compose 映射、TLS / mTLS  
4. [04-调度与后台任务.md](./04-调度与后台任务.md) — Crontab、统计同步 ticker、与 [01-事件系统](../03-基础设施/01-事件系统.md) 的异步链

**机器可读契约**：[api/rest/apiserver.yaml](../../api/rest/apiserver.yaml)、[api/rest/collection.yaml](../../api/rest/collection.yaml)、[internal/apiserver/interface/grpc/proto](../../internal/apiserver/interface/grpc/proto/)；各进程配置见 [configs/](../../configs/)。
