# 03-基础设施

**本文回答**：qs-server 的基础设施层应该按什么主线阅读；缓存、事件、高并发保护分别解决什么系统问题；现有 `redis/`、`resilience/` 细文档如何归入新的主线。

---

## 30 秒结论

qs-server 的基础设施不是按 Redis、MQ、Outbox、限流这些技术点平均展开，而是围绕**高并发测评链路**做三层治理：

1. **缓存模块**：治理读侧高频查询，降低问卷目录、测评模型、报告状态等读流量对 MongoDB / MySQL 的压力。
2. **事件模块**：治理异步测评链路，把答卷提交、测评执行、报告生成、状态通知拆成可追踪、可重试、可补偿的异步流程。
3. **高并发保护模块**：治理突发入口、重复提交、下游处理能力有限和报告查询风暴。

`data-access / security / integrations / runtime / observability` 是支撑层：它们提供持久化、身份权限、外部适配、运行时装配和观测能力，但不是本目录的叙事主角。

---

## 主线入口

| 主线 | 解决的问题 | 入口 |
| ---- | ---------- | ---- |
| 基础设施总览 | 三条主线如何共同保护高并发测评链路 | [00-基础设施总览.md](./00-基础设施总览.md) |
| 能力矩阵 | 每个能力域解决什么问题、用什么机制、支撑哪条业务链路 | [01-能力矩阵.md](./01-能力矩阵.md) |
| 缓存模块 | 高频读、目录查询、模型查询、报告状态查询带来的 DB 压力 | [cache/README.md](./cache/README.md) |
| 事件模块 | 异步测评、可靠出站、worker 消费、一次性唤醒 | [event/README.md](./event/README.md) |
| 高并发保护模块 | 入口限流、提交削峰、重复提交、下游背压、report 查询治理 | [concurrency/README.md](./concurrency/README.md) |

---

## 当前目录结构

```text
03-基础设施/
├── README.md
├── 00-基础设施总览.md
├── 01-能力矩阵.md
├── cache/                 # 新主线入口：读侧治理
├── event/                 # 新主线入口：事件驱动治理
├── concurrency/           # 新主线入口：高并发保护
├── data-access/           # 支撑层：持久化、仓储、UoW、Outbox store
├── security/              # 支撑层：Principal、AuthzSnapshot、ServiceIdentity
├── integrations/          # 支撑层：WeChat、OSS、Notification
├── runtime/               # 支撑层：启动装配、资源注入、生命周期
└── observability/         # 支撑层：metrics、healthz、logging、governance endpoint
```

旧 `redis/` / `resilience/` 的总入口、总览和旧矩阵已归档到 `docs/_archive/`。仍有证据价值的细节文档会在后续逐篇迁移到 `cache/` 和 `concurrency/`，迁移前只作为实现细节参考，不作为新的阅读入口。

---

## 阅读顺序

第一次理解基础设施，按下面顺序读：

```text
README.md
00-基础设施总览.md
01-能力矩阵.md
cache/README.md
event/README.md
concurrency/README.md
```

然后按问题进入支撑层：

| 你要解决的问题 | 继续读 |
| -------------- | ------ |
| 仓储、事务、migration、read model | [data-access/README.md](./data-access/README.md) |
| IAM、权限快照、内部服务身份 | [security/README.md](./security/README.md) |
| 微信、对象存储、通知适配 | [integrations/README.md](./integrations/README.md) |
| 启动流水线、资源装配、container wiring | [runtime/README.md](./runtime/README.md) |
| 指标、健康检查、日志、治理端点 | [observability/README.md](./observability/README.md) |

---

## 边界原则

| 能力 | 必须讲清的边界 |
| ---- | -------------- |
| 缓存 | 缓存是读侧治理层，不是业务事实源；Redis 异常时可以回源，但必须有并发保护 |
| 事件 | Outbox 解决可靠出站，MQ 解决异步解耦，Redis signaling 只做临时唤醒 |
| 高并发保护 | 限流、队列、背压、重复抑制只保护系统边界，业务正确性仍由状态机、幂等键、DB 约束和 Outbox 兜底 |
| 数据访问 | Repository / UnitOfWork 保存事实，不反向定义领域模型 |
| 安全 | IAM / AuthzSnapshot / CapabilityDecision 是权限事实来源，JWT claim 不是业务权限真值 |
| 外部集成 | 第三方 SDK 通过 adapter/port 接入，不泄露到领域模型 |
| Runtime | Runtime 负责装配和生命周期，不承载业务规则 |

---

## 事实来源

维护本目录时，优先核对下面事实源：

| 事实类型 | 主要来源 |
| -------- | -------- |
| 事件契约 | `configs/events.yaml`、`configs/signals.yaml` |
| 缓存与信令 | `internal/pkg/cacheplane`、`internal/apiserver/infra/cache`、`internal/pkg/cachesignal`、`internal/collection-server/application/catalogl1` |
| Outbox | `internal/apiserver/outboxcore`、`internal/apiserver/infra/mongo/eventoutbox`、`internal/apiserver/infra/mysql/eventoutbox` |
| Worker 消费 | `internal/worker/handlers`、`internal/pkg/eventcatalog` |
| 高并发保护 | `internal/pkg/resilience`、`internal/collection-server/application/answersheet/submit_queue.go`、`internal/collection-server/infra/redisops`、`internal/pkg/resilience/locklease` |
| report 查询 | `api/rest/collection.yaml`、`docs/04-接口与运维/12-小程序报告等待接入指南.md` |
| 压测验收 | `docs/04-接口与运维/11-300QPS混合场景压测SOP.md`、`Makefile` |

如果 prose 文档与源码、配置或机器契约冲突，以源码、配置和机器契约为准。
