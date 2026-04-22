# 接口与运维

**本文回答**：本组文档解释 `qs-server` 的机器契约和运维入口分别落在哪里，包括 REST / gRPC 契约、端口与部署映射、周期任务与事件型后台任务的分工；本文先给结论和阅读导航，再说明它与业务模块、基础设施、运行时之间的边界。

## 30 秒结论

如果只看一屏，先看下面这张表：

| 维度 | 结论 |
| ---- | ---- |
| 本组主题 | REST 契约、gRPC 契约、端口部署映射、调度与后台任务，以及典型生产事故复盘 |
| 真值边界 | 这里讲机器契约入口和运维事实；业务语义仍回到 [02-业务模块](../02-业务模块/) |
| 契约真值 | REST 以 [api/rest](../../api/rest/) 为准，gRPC 以 [proto](../../internal/apiserver/interface/grpc/proto/) 为准 |
| 运维真值 | 端口、TLS / mTLS、Crontab、ticker、部署映射和事故处置证据都从本组文档与 `configs/` / 日志 / 代码对照核实 |
| 最重要的认识 | “事件型后台任务”和“定时 / 运维触发任务”是两套不同机制，不能混成一条链理解 |
| 阅读方式 | 先读 REST 与 gRPC，再读部署与端口，最后读调度与后台任务 |

## 重点速查（继续往下读前先记这几条）

1. **契约以机器文件为准**：正文只解释入口和 Verify 方式，真正的 REST / gRPC 契约还是以导出 yaml 和 proto 为准。  
2. **不要混文档层**：业务 Handler 的语义、领域动作和模块边界不在本组重复展开。  
3. **后台分两类**：`Crontab / ticker / 运维触发` 与 `MQ 事件驱动` 是两类不同后台机制，本组只负责把它们的运维入口和部署事实讲清楚。  
4. **排障时从这里入手**：遇到“这个接口实际暴露在哪个进程、哪个端口、谁来调、怎么生成契约”这类问题，优先看本组。  

## 为什么这一组要单独存在

契约和运维事实如果散落在模块文、运行时文和基础设施文里，会出现两个问题：一是读者不知道“接口真值文件到底在哪”，二是部署、端口、调度与事件后台任务容易混成一条链。因此本组单独承担：

- REST / gRPC 机器契约入口在哪里、如何和代码注册点互相验证
- HTTP / gRPC 监听地址、TLS / mTLS、Compose 映射与对外暴露面如何核对
- `Crontab / ticker / 运维触发` 与 `MQ 事件驱动` 这两类后台机制在运维视角下如何区分

本组优先讲**契约入口、运维触发方式、部署事实**，不重复模块文里的业务语义。

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
5. [05-事故复盘：2026-04-16 qs.evaluation.lifecycle 积压与 30 秒尾延迟.md](./05-事故复盘：2026-04-16%20qs.evaluation.lifecycle%20积压与%2030%20秒尾延迟.md) — 一次真实生产排障如何从“数据库疑似瓶颈”收敛到“应用层同步缓存失效”
6. [06-operating 缓存治理页接入.md](./06-operating%20缓存治理页接入.md) — 自研 operating 后台如何复用 internal 缓存治理接口与 Grafana 深链接
7. [07-代码质量基线报告：2026-04-22.md](./07-代码质量基线报告：2026-04-22.md) — 当前质量门禁、覆盖率、安全扫描和下一阶段治理优先级
8. [08-架构边界审计与Tier1测试策略：2026-04-22.md](./08-架构边界审计与Tier1测试策略：2026-04-22.md) — 分层边界现状、允许例外、Tier 1 测试策略与第一阶段深度治理落点
9. [09-govulncheck剩余advisory分组处置：2026-04-22.md](./09-govulncheck剩余advisory分组处置：2026-04-22.md) — 当前 active finding、已吸收的历史 advisory，以及 watchlist 的分组处置方式

**机器可读契约**：[api/rest/apiserver.yaml](../../api/rest/apiserver.yaml)、[api/rest/collection.yaml](../../api/rest/collection.yaml)、[internal/apiserver/interface/grpc/proto](../../internal/apiserver/interface/grpc/proto/)；各进程配置见 [configs/](../../configs/)。
