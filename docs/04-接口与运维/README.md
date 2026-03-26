# 接口与运维

**本文回答**：本组文档解释 `qs-server` 的机器契约和运维入口分别落在哪里，包括 REST / gRPC 契约、端口与部署映射、周期任务与事件型后台任务的分工；本文先给结论和阅读导航，再说明它与业务模块、基础设施、运行时之间的边界。

本组文档说明 **REST / gRPC 契约、端口与部署映射、周期任务与事件型后台任务的分工**。写作约定见 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md) 的 **Why / What / Where / Verify**；与 [02-业务模块](../02-业务模块/)（领域与 Handler）、[03-基础设施](../03-基础设施/)（IAM、限流、事件配置）、[05-专题分析](../05-专题分析/)（设计叙事）**不重复**：此处固定 **契约入口、运维触发方式、部署事实**。

## 30 秒结论

如果只看一屏，先看下面这张表：

| 维度 | 结论 |
| ---- | ---- |
| 本组主题 | REST 契约、gRPC 契约、端口部署映射、调度与后台任务 |
| 真值边界 | 这里讲机器契约入口和运维事实；业务语义仍回到 [02-业务模块](../02-业务模块/) |
| 契约真值 | REST 以 [api/rest](../../api/rest/) 为准，gRPC 以 [proto](../../internal/apiserver/interface/grpc/proto/) 为准 |
| 运维真值 | 端口、TLS / mTLS、Crontab、ticker 和部署映射从本组文档与 `configs/` 对照核实 |
| 最重要的认识 | “事件型后台任务”和“定时 / 运维触发任务”是两套不同机制，不能混成一条链理解 |
| 阅读方式 | 先读 REST 与 gRPC，再读部署与端口，最后读调度与后台任务 |

## 重点速查（继续往下读前先记这几条）

1. **契约以机器文件为准**：正文只解释入口和 Verify 方式，真正的 REST / gRPC 契约还是以导出 yaml 和 proto 为准。  
2. **不要混文档层**：业务 Handler 的语义、领域动作和模块边界不在本组重复展开。  
3. **后台分两类**：`Crontab / ticker / 运维触发` 与 `MQ 事件驱动` 是两类不同后台机制，本组只负责把它们的运维入口和部署事实讲清楚。  
4. **排障时从这里入手**：遇到“这个接口实际暴露在哪个进程、哪个端口、谁来调、怎么生成契约”这类问题，优先看本组。  

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
