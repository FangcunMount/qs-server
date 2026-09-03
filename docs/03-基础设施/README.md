# 基础设施

本层按“系统要解决的问题”组织，不按 Redis、NSQ、MySQL 等组件平均分栏。一个组件可以参与多个机制，但业务事实、容量预算、恢复动作和安全边界必须有唯一 owner。

## 1. 版本与证据基线

当前仓库源码复核基线、逐篇状态和基础设施签署矩阵以 [`document-closure.json`](../document-closure.json) 为准；
带日期、部署 SHA、effective config 与失效期的环境/生产结果进入[基础设施生产证据台账](../00-总览/10-基础设施生产证据台账.md)。稳定设计页不保存上一轮主机数、容器数、数据行数或探针结果。基础设施事实源变化后必须重新复核，不能只修改日期或沿用历史结论。

这里的“当前”分三层：

| 层次 | 本轮能确认什么 | 不能据此确认什么 |
| --- | --- | --- |
| `已实现` | 当前仓库中存在实现、装配、配置合同与自动化测试入口 | 真实生产实例一定启用了同一配置 |
| `生产配置意图` | `configs/*.prod.yaml` 中版本化的阈值、开关与依赖拓扑 | 部署时的 flags、env、secret mounts 与实际容量 |
| `待补证据` | 文档保留明确的环境验收项 | Mongo transaction、MQ delivery、Redis 故障、多实例并发和 SIGTERM drain 已在真实环境通过 |

因此，本目录是当前仓库的基础设施事实层，不是生产运行报告。阅读时遇到数字，先判断它是代码默认值、生产配置意图，还是压测/运行观测值。

## 2. 第一遍整体学习路线

第一遍不建议从某个 Redis package 开始。按下面四轮建立全景，细节暂时只追到“最终事实在哪里、失败后怎样收敛”：

1. [基础设施总览](./00-基础设施总览.md)：建立三进程、四类事实和失败边界。
2. [统一模型与推理方法](./04-统一模型与推理方法.md)：先分清请求、意图、协调、持久事实与派生证据，再学习如何从失败窗口推导机制。
3. [基础设施能力地图](./01-基础设施能力地图.md)、[基础设施设计原则](./02-基础设施设计原则.md)：把系统问题映射到 owner、scope、正确性机制与降级语义。
4. [核心链路全景](./03-核心链路全景.md)：把统一模型代入可靠提交、异步处理、缓存读取、报告等待和运行时治理。
5. 依次阅读 [Concurrency / Resilience](./concurrency/README.md)、[Event](./event/README.md)、[Cache](./cache/README.md)，
   最后用 Data Access、Migration & Recovery、Security、Observability、Runtime、Config & Deployment 补全事实、权限、证据、交付与生命周期边界。

第一遍读完后，至少应能回答三个问题：Redis 全部不可用时哪些正确性仍成立、一次 `202 Accepted` 精确承诺了什么、为什么限流/锁/Signal/指标都不能替代持久化事实。

## 3. 专题深入顺序

完成第一遍后，再按专题 README 中的编号顺序阅读全文。推荐先后顺序不是组件重要性排序，而是认知依赖：

1. Concurrency / Resilience：从请求与业务意图出发，理解入口准入、容量与持久化裁决为什么不能混用。
2. Event：沿已提交事实向外传播，理解本地事务、at-least-once、结算与恢复状态机。
3. Cache：沿持久事实构造派生读取，理解新鲜度、回源放大、失效和最终收敛。
4. 六项支撑能力：用 Data Access、Migration & Recovery、Security、Observability、Runtime、Config & Deployment 交叉验证前三条主线。

## 4. 一条因果主线、三个专题切面

三个专题不是并列组件目录，而是同一条因果链的不同切面：

```text
请求与业务意图
  → 入口准入与并发容量（Concurrency）
  → 持久事实与出站意图（Data Access + Event）
  → 投递、结算与恢复（Event）
  → 投影、缓存与新鲜度（Cache）
```

| 系统问题 | 入口 | 主要机制 | 最终事实 |
| --- | --- | --- | --- |
| 高频读与缓存一致性 | [Cache](./cache/README.md) | capability registry、L1/L2、singleflight、失效、预热 | DB/业务 read model |
| 异步执行与可靠协作 | [Event](./event/README.md) | Event contract、Transactional Outbox、MQ、Signal、dead letter/replay | 本地事务 + Outbox + 业务状态 |
| 突发流量与故障隔离 | [Concurrency / Resilience](./concurrency/README.md) | Gate、RateLimit、Backpressure、SubmitCoalescer、LockLease、治理 | DB 唯一约束、claim/CAS、事务 |

## 5. 支撑能力

| 能力 | 当前实现入口 | 设计论证 | 核心问题 |
| --- | --- | --- | --- |
| 数据访问 | [Data Access](./data-access/README.md) | [设计决策](./data-access/15-核心设计决策与替代方案.md) | 谁拥有事实、事务边界在哪里、跨存储如何收敛 |
| Migration 与恢复 | [Migration & Recovery](./migration-recovery/README.md) | [migration 边界](./migration-recovery/10-Migration、dirty与回退边界.md) | schema、dirty、one-off、恢复与生产证据如何受控 |
| 安全 | [Security](./security/README.md) | [身份与资源授权](./security/10-身份、服务与资源授权.md) | 凭证、主体、组织、capability 与资源归属如何逐层证明 |
| 可观测性 | [Observability](./observability/README.md) | [设计决策](./observability/15-核心设计决策与替代方案.md) | 日志、指标、状态和持久审计分别能证明什么 |
| 运行时 | [Runtime](./runtime/README.md) | [生命周期](./runtime/10-进程生命周期、启动与关闭.md) | 依赖如何装配、后台任务和资源如何启停 |
| 配置与部署 | [Config & Deployment](./config-deployment/README.md) | [部署验收](./config-deployment/30-部署验收与回滚.md) | 配置、Secret、镜像、网络与 exact-SHA 部署如何闭环 |

## 6. 贯穿全层的边界

- Outbox 保证已提交业务事件的可靠出站；MQ 提供有界 at-least-once；Redis Signal 只做可丢失唤醒。
- Cache miss、失效或 Redis 故障不能改变业务事实，只影响性能、新鲜度和部分运行时协调。
- Gate、RateLimit、Backpressure 控制不同的量；SubmitCoalescer/LockLease 降低竞态代价，但不能替代数据库约束。
- AuthN、OrgScope、capability 和资源归属是连续的授权链，任意一层成功都不能替代其它层。
- `healthy`、`ready`、指标为零和 durable state 已完成不是同一结论。
- 每个 client、subscriber、scheduler、goroutine 都必须有构造、启动、停止和关闭 owner。
- 治理操作必须带组织范围、确认、幂等 request ID、审计和状态冲突检查，不能绕过业务边界直接改主事实。

## 7. 如何判断文档与代码冲突

事实优先级：

1. 当前 source、composition root 与运行时 handler。
2. `configs/events.yaml`、`configs/signals.yaml`、migration、API/proto 等机器契约。
3. 当前 `configs/*.prod.yaml` 和部署注入的配置。
4. 本目录 active docs。
5. 历史文档、重构计划和 Git 记录。

文档中的状态词：

- `已实现`：当前代码与装配可追踪。
- `待补证据`：代码或配置已存在，但仍缺真实依赖、多实例、压测或运行窗口证据。
- `当前风险/限制`：代码真实存在但边界不完整。
- `规划改造`：只描述目标，不得作为现行能力承诺。

## 8. 验证入口

```bash
make docs-check
go test -count=1 ./internal/pkg/architecture ./internal/pkg/configcontract
```

专题文档末尾列出了更窄的测试入口。unit/contract test 不能替代真实 Mongo transaction、MySQL migration、MQ delivery、Redis 故障、跨实例并发和 SIGTERM drain 演练。

## 9. 文档如何继续维护

统一词汇、四类事实和跨模块因果链由本目录根级文档维护；Cache、Event、Concurrency 只展开自己的失败窗口与实现；Data Access、Migration &
Recovery、Security、Observability、Runtime、Config & Deployment 分别补充事实、恢复、权限、证据、生命周期和交付。禁止再增加一份平行的“全量架构说明”。

行为或架构变化时，专题文档不能只更新配置表，还要重新检查：业务损失和不变量是否变化、替代方案为何仍不选、降级/恢复是否改变、原有验证证据是否仍有效、是否触发了重决策条件。完整要求见 [文档写作约定](../CONTRIBUTING-DOCS.md#51-基础设施专题的知识结构)。
