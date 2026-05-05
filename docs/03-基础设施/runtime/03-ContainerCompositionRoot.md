# Container Composition Root

**本文回答**：apiserver Container 作为组合根持有哪些基础设施和业务模块；它如何初始化 EventPublisher、Survey、Scale、Actor、Evaluation、Plan、Statistics、CacheWarmup、Codes、QR/Notification 等能力；Container 与业务层的边界是什么。

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| Container 定位 | apiserver composition root，负责装配依赖，不承载业务规则 |
| 基础设施 | MySQL、Mongo、Redis、CacheSubsystem、Backpressure、MQPublisher、EventPublisher、EventCatalog |
| 业务模块 | Survey、Scale、Actor、Evaluation、Plan、Statistics、IAM、Codes |
| 外部集成 | QRCodeGenerator、SubscribeSender、QRCodeObjectStore、QRCodeService、MiniProgramTaskNotificationService |
| 初始化顺序 | EventPublisher → Survey → Scale → Actor → Evaluation → Plan → Statistics → WarmupCoordinator → PostWire hooks → Codes → QR |
| PostWire 边界 | 少数 late-bound seam；构造函数依赖仍是首选 |
| 不做什么 | 不在 Container 中写业务规则，不让 domain/application 依赖 Container |

一句话概括：

> **Container 是进程内部的“装配台”，不是业务服务定位器。**

---

## 1. Container 持有的依赖

### 1.1 基础设施

| 字段 | 说明 |
| ---- | ---- |
| mysqlDB | MySQL 连接 |
| mongoDB | Mongo database |
| redisCache | Redis client |
| cache | CacheSubsystem |
| backpressure | MySQL/Mongo/IAM limiter |
| mqPublisher | MQ publisher |
| eventPublisher | 统一事件发布器 |
| eventCatalog | 事件契约 |
| publisherMode | mq/logging/nop |
| planEntryURL | 计划任务入口 |
| statisticsRepairWindowDays | 统计修复窗口 |

### 1.2 业务模块

| 字段 | 说明 |
| ---- | ---- |
| SurveyModule | questionnaire / answersheet |
| ScaleModule | scale |
| ActorModule | testee / clinician / operator |
| EvaluationModule | assessment / score / report |
| PlanModule | evaluation plan |
| StatisticsModule | statistics read model / query |
| IAMModule | IAM integration |
| CodesService | code 申请服务 |

### 1.3 外部集成

| 字段 | 说明 |
| ---- | ---- |
| QRCodeGenerator | 微信小程序码生成器 |
| SubscribeSender | 小程序订阅消息发送器 |
| QRCodeObjectStore | OSS public object store |
| QRCodeService | 小程序码生成应用服务 |
| MiniProgramTaskNotificationService | task.opened 小程序通知服务 |

---

## 2. Container 初始化流程

```mermaid
flowchart TD
    start["Container.Initialize"] --> event["initEventPublisher"]
    event --> survey["initSurveyModule"]
    survey --> scale["initScaleModule"]
    scale --> graph["newModuleGraph"]
    graph --> actor["initActorModule"]
    actor --> evaluation["initEvaluationModule"]
    evaluation --> plan["initPlanModule"]
    plan --> statistics["initStatisticsModule"]
    statistics --> warmup["initWarmupCoordinator"]
    warmup --> postCache["postWireCacheGovernanceDependencies"]
    postCache --> postScope["postWireProtectedScopeDependencies"]
    postScope --> codes["initCodesService"]
    codes --> qr["initQRCodeGenerator"]
    qr --> done["initialized=true"]
```

---

## 3. EventPublisher 初始化

Container 初始化时先调用：

```text
initEventPublisher
```

它创建 routing publisher：

- Mode。
- Source=apiserver。
- MQPublisher。
- Catalog。

所有模块共享这个 EventPublisher。

如果未初始化，则 `GetEventPublisher` 返回 NopEventPublisher，避免 nil panic。

---

## 4. 模块初始化顺序

当前顺序反映依赖关系：

| 顺序 | 模块 | 原因 |
| ---- | ---- | ---- |
| 1 | Survey | 问卷/答卷基础能力 |
| 2 | Scale | 量表能力，可能依赖 Survey/Questionnaire |
| 3 | Actor | testee/operator 等行为者 |
| 4 | Evaluation | 评估依赖 survey/scale/actor |
| 5 | Plan | 计划依赖 actor/scale/evaluation |
| 6 | Statistics | 统计依赖多个业务模块 |
| 7 | WarmupCoordinator | 依赖 cache + statistics/scale/query callbacks |
| 8 | Codes | 独立基础应用服务 |
| 9 | QR | 外部集成支持服务 |

---

## 5. ContainerOptions

ContainerOptions 是 Resource Stage 传给 Container 的 runtime 输入：

| 字段 | 用途 |
| ---- | ---- |
| MQPublisher | 构造 EventPublisher |
| PublisherMode | 选择 mq/logging/nop |
| EventCatalog | 事件 topic / 契约 |
| Cache | cache policy/warmup options |
| CacheSubsystem | Redis/cache/governance 组合根 |
| Backpressure | MySQL/Mongo/IAM limiter |
| PlanEntryBaseURL | plan task entry |
| StatisticsRepairWindowDays | statistics repair 默认窗口 |
| Silent | 抑制 bootstrap stdout |

---

## 6. Container 不是服务定位器

不推荐：

```text
application service 运行时去 Container 里拿依赖
domain 依赖 Container
handler 到处访问 Container 内部字段
```

推荐：

```text
Container 构造模块
  -> module 暴露 use case/service
  -> transport deps 显式传入 handler/registry
```

---

## 7. Cleanup

Container 有 `Cleanup` 语义，用于 shutdown 阶段释放容器持有的资源。

具体释放行为应在 `07-Lifecycle与关闭语义.md` 中和 process lifecycle 对齐。

---

## 8. 常见误区

### 8.1 “新功能直接挂 Container 字段就行”

不一定。先判断是业务模块、infra adapter、client bundle 还是 runtime dependency。

### 8.2 “Container 可以承载业务规则”

不应。Container 只做装配。

### 8.3 “post-wire 是正常依赖注入方式”

不应。构造函数依赖优先，post-wire 仅少数 late-bound seam。

### 8.4 “模块初始化顺序可以随便改”

不可以。顺序隐含依赖关系，修改要补测试。

---

## 9. 修改指南

### 9.1 新增业务模块

必须：

1. 定义 assembler module。
2. 明确依赖。
3. 在 Container 增加字段。
4. 在 Initialize 中选择位置。
5. 注册 loaded module。
6. 暴露 REST/GRPC deps，如需要。
7. 补 tests/docs。

### 9.2 新增基础设施服务

必须：

1. 判断是否应在 Resource Stage 创建。
2. 判断是否应进入 ContainerOptions。
3. 判断是否是 Integration Stage 初始化。
4. 明确 Cleanup。
5. 补 lifecycle tests。

---

## 10. Verify

```bash
go test ./internal/apiserver/container
go test ./internal/apiserver/process
```
