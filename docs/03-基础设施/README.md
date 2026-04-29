# 基础设施

**本文回答**：本组文档解释 `qs-server` 的横切基础设施能力如何落在代码里，包括事件、存储、缓存与限流、Redis、IAM、配置体系等；本文先给结论和阅读导航，再说明它与业务模块、专题分析之间的分工。

## 30 秒结论

如果只看一屏，先看下面这张表：

| 维度 | 结论 |
| ---- | ---- |
| 本组主题 | Runtime Composition、Transport & Contract、事件系统、Data Access、External Integration、存储分布、Resilience Plane、Redis、Security Control Plane、配置体系等横切机制 |
| 真值边界 | 这里讲机制、配置锚点和实现位置；业务接口与聚合边界以 [02-业务模块](../02-业务模块/) 为准 |
| 与专题分工 | [05-专题分析](../05-专题分析/) 侧重“为什么这样设计”；本组侧重“代码里具体怎么挂、怎么配、怎么核对” |
| 与运行时分工 | [01-运行时](../01-运行时/) 讲进程与时序；本组讲事件、存储、IAM、缓存限流等横切能力本身 |
| 阅读顺序 | 先看事件和存储，再看缓存限流、Redis、Security、配置体系 |
| 使用方式 | 需要核对 `events.yaml`、数据库/Redis 分工、安全控制面和 `yaml -> Options` 时，优先回到本组 |

## 重点速查（继续往下读前先记这几条）

1. **不要混层**：本组不重复业务模块里的接口表和聚合规则，只补横切机制的代码锚点与配置事实。
2. **先看横切能力矩阵**：[00-横切能力矩阵.md](./00-横切能力矩阵.md) 说明每个 plane 的观测入口、治理边界和 truth layer。
3. **Runtime Composition 已有深讲入口**：process stage、container、ModuleGraph、ClientBundle 与 Config/Options 统一从 [runtime/README.md](./runtime/README.md) 进入。
4. **Transport & Contract 已有深讲入口**：REST、gRPC、OpenAPI、proto、route matrix 与新增接口 SOP 统一从 [transport/README.md](./transport/README.md) 进入。
5. **Data Access 已有深讲入口**：MySQL、Mongo、migration、read model 和 outbox/idempotency 存储边界统一从 [data-access/README.md](./data-access/README.md) 进入。
6. **External Integration 已有深讲入口**：WeChat、OSS、Notification adapter 统一从 [integrations/README.md](./integrations/README.md) 进入。
7. **事件以主文档、深讲目录和真值文件为准**：事件 topic、delivery、handler 绑定最终以 [`configs/events.yaml`](../../configs/events.yaml)、[01-事件系统](./01-事件系统.md) 与 [event/README.md](./event/README.md) 为准。
8. **Resilience Plane 已有单独入口**：限流、SubmitQueue、背压、Lock lease、幂等、重复抑制和降级统一从 [resilience/README.md](./resilience/README.md) 进入。
9. **Security Control Plane 已有深讲入口**：JWT、IAM、authz snapshot、capability、service auth、mTLS/ACL 统一从 [security/README.md](./security/README.md) 进入。
10. **Redis 文档已经收口成一个中心页 + 深讲目录**：先看 [12-Redis文档中心.md](./12-Redis文档中心.md)，再进入 [redis/README.md](./redis/README.md)；`06/11/13` 保留为摘要与兼容入口，`07-10` 只保留为历史设计稿与阶段记录。

## 为什么这一组要单独存在

如果只看业务模块，很容易把“事件、存储、缓存、IAM、配置”误读成每个模块各自实现的一部分；如果只看专题，又会缺少机制在代码里的真实挂载位置。因此，本组单独承担下面这些问题：

- 事件系统如何把 `events.yaml`、发布端、worker 消费端串起来
- MySQL / MongoDB / Redis 为什么这样分工，以及代码各自从哪里接入
- 限流、SubmitQueue、背压、Lock lease、缓存、Security Control Plane、配置等横切能力究竟挂在什么位置、由哪些配置驱动
- Redis 在缓存、统计、锁、第三方 SDK 和工具链中分别扮演什么角色

本组优先讲**机制、配置锚点和 Verify 方法**，不重复业务模块内的接口表和聚合规则。

## 与其它文档的分工

| 文档组 | 侧重 |
| ------ | ---- |
| [02-业务模块](../02-业务模块/) | 各 BC 的职责、契约 Verify、模块内锚点 |
| [05-专题分析](../05-专题分析/) | 三界分离、异步链路、保护层与读侧等**设计叙事** |
| **03-基础设施（本文）** | Runtime Composition、Transport & Contract、事件拓扑、Data Access、External Integration、存储分布、Resilience Plane、Redis、Security Control Plane、YAML→Options 等**机制与锚点**；组合图统一从 [runtime/README.md](./runtime/README.md) 进入；接口统一从 [transport/README.md](./transport/README.md) 进入；Data Access 从 [data-access/README.md](./data-access/README.md) 进入；外部集成从 [integrations/README.md](./integrations/README.md) 进入；事件统一从 [01-事件系统](./01-事件系统.md) 和 [event/README.md](./event/README.md) 进入；Redis 统一从 [12-Redis文档中心](./12-Redis文档中心.md) 进入 |

## 建议阅读顺序

1. [00-横切能力矩阵.md](./00-横切能力矩阵.md) — 所有横切 plane 的观测入口、治理边界和 truth layer 索引
2. [runtime/README.md](./runtime/README.md) — Runtime Composition 深讲目录，覆盖 process stage、ModuleGraph、ClientBundle 和 Config/Options contract
3. [transport/README.md](./transport/README.md) — Transport & Contract 深讲目录，覆盖 REST、gRPC、OpenAPI、proto、contract tests 和新增接口 SOP
4. [01-事件系统.md](./01-事件系统.md) — 事件系统兼容入口、真值优先级、事件清单与阅读地图
5. [event/README.md](./event/README.md) — 事件深讲目录，覆盖契约、Publish/Outbox、Worker、SOP、观测和 MQ 选型
6. [02-存储模型.md](./02-存储模型.md) — MySQL / MongoDB / Redis 分工与进程接入
7. [data-access/README.md](./data-access/README.md) — Data Access 深讲目录，覆盖 MySQL、Mongo、migration、read model 和 SOP
8. [integrations/README.md](./integrations/README.md) — External Integration 深讲目录，覆盖 WeChat、OSS、Notification 和 SOP
9. [03-缓存与限流.md](./03-缓存与限流.md) — Resilience Plane 兼容入口，保留保护层摘要与排障入口
10. [resilience/README.md](./resilience/README.md) — Resilience 深讲目录，覆盖限流、队列、背压、锁、幂等、降级和 SOP
11. [12-Redis文档中心.md](./12-Redis文档中心.md) — Redis 当前真值入口与阅读地图
12. [redis/README.md](./redis/README.md) — Redis 深讲目录，覆盖整体架构、runtime、Cache、Lock、Governance、排障和 SOP
13. [06-Redis使用情况.md](./06-Redis使用情况.md) — Redis 在当前代码中的三进程角色、family 边界、治理接口与运维入口摘要
14. [13-Redis缓存业务清单.md](./13-Redis缓存业务清单.md) — 当前 `apiserver` 里各类业务缓存、查询缓存与治理型缓存的清单
15. [security/README.md](./security/README.md) — Security Control Plane 深讲目录，覆盖 Principal、TenantScope、AuthzSnapshot、CapabilityDecision、ServiceIdentity、mTLS/ACL 和新增安全能力 SOP
16. [04-IAM与认证.md](./04-IAM与认证.md) — IAM 与认证兼容入口，与 [01-运行时/05-IAM认证与身份链路.md](../01-运行时/05-IAM认证与身份链路.md) 互补
17. [05-配置体系.md](./05-配置体系.md) — `configs/*.yaml`、`Options`、**`events.yaml` 的特殊性**

如果要回看 Redis 的设计演进或阶段性重构记录，再按需阅读：

- [07-Redis代码总览（源码审计版）.md](../_archive/03-基础设施/07-Redis代码总览（源码审计版）.md)
- [08-Redis分层重构设计.md](../_archive/03-基础设施/08-Redis分层重构设计.md)
- [09-Redis跨仓重构路线.md](../_archive/03-基础设施/09-Redis跨仓重构路线.md)
- [10-apiserver缓存实现层重构.md](../_archive/03-基础设施/10-apiserver缓存实现层重构.md)

同时建议对照：[00-总览](../00-总览/)、[01-运行时](../01-运行时/)、[`configs/events.yaml`](../../configs/events.yaml)。

本组只写**当前实现**，不写历史方案；从 `_archive` 迁入须逐条对源码。
