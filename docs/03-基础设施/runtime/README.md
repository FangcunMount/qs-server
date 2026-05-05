# Runtime Composition Plane 阅读地图

**本文回答**：`03-基础设施/runtime/` 这一组文档应该如何阅读；它和 `01-运行时/` 的边界是什么；qs-server 进程内部如何从配置、资源、container、integration、transport、background runtime 到 shutdown 形成一条可测试的启动组合链路。

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 模块定位 | `runtime/` 负责**基础设施视角的启动组合**，不是业务三进程协作说明 |
| 核心主轴 | Config/Options → Process Stage → ResourceBootstrap → Container → Integrations → Transports → Background Runtimes → Shutdown |
| 当前主实现 | apiserver 的 `PrepareRun` stage pipeline：resources、container、integrations、transports、background runtimes、shutdown callback |
| Container 定位 | composition root，持有 DB/Redis/cache/MQ/event/backpressure/module/external adapter 等依赖 |
| ResourceBootstrap 定位 | 初始化 MySQL/Mongo、Redis runtime、CacheSubsystem、MQ publisher、EventCatalog、Backpressure，然后生成 ContainerOptions |
| ModuleGraph 定位 | late-bound seam，只处理构造期依赖无法优雅表达的少数 post-wire；构造函数依赖仍是首选 |
| Transport 定位 | 根据 Container 构建 REST deps 与 gRPC deps，注册 HTTP routes 和 gRPC services |
| Lifecycle 定位 | 统一注册 shutdown callback，按 runtime hooks、container cleanup、authz sync、DB、HTTP、gRPC 的顺序关闭 |
| 不负责 | 不重复 `01-运行时/` 的三进程业务协作；不深讲 Redis/Event/Resilience/Security 子系统内部设计 |

一句话概括：

> **Runtime Composition Plane 解释“一个进程如何被装配起来”，而不是“业务请求如何在三进程之间流动”。**

---

## 1. 本目录文档地图

```text
runtime/
├── README.md
├── 00-整体架构.md
├── 01-ProcessStage与启动流水线.md
├── 02-ResourceBootstrap资源装配.md
├── 03-ContainerCompositionRoot.md
├── 04-ConfigOptions配置链路.md
├── 05-ClientBundle与外部客户端注入.md
├── 06-ModuleGraph与PostWire边界.md
├── 07-Lifecycle与关闭语义.md
└── 08-新增Runtime依赖SOP.md
```

| 顺序 | 文档 | 先回答什么 |
| ---- | ---- | ---------- |
| 1 | [00-整体架构.md](./00-整体架构.md) | Runtime composition 总图、阶段划分、与其它基础设施模块边界 |
| 2 | [01-ProcessStage与启动流水线.md](./01-ProcessStage与启动流水线.md) | `PrepareRun` stage pipeline、失败阶段定位、preparedServer |
| 3 | [02-ResourceBootstrap资源装配.md](./02-ResourceBootstrap资源装配.md) | DB/Redis/MQ/EventCatalog/Backpressure/ContainerOptions 如何准备 |
| 4 | [03-ContainerCompositionRoot.md](./03-ContainerCompositionRoot.md) | Container 持有哪些依赖、模块初始化顺序、composition root 边界 |
| 5 | [04-ConfigOptions配置链路.md](./04-ConfigOptions配置链路.md) | YAML/options/config 如何转换成 runtime deps 和 ContainerOptions |
| 6 | [05-ClientBundle与外部客户端注入.md](./05-ClientBundle与外部客户端注入.md) | IAM/WeChat/OSS/gRPC/service auth 等客户端如何注入 |
| 7 | [06-ModuleGraph与PostWire边界.md](./06-ModuleGraph与PostWire边界.md) | post-wire 的真实用途、当前 hook 状态、禁止滥用边界 |
| 8 | [07-Lifecycle与关闭语义.md](./07-Lifecycle与关闭语义.md) | runtime hooks、container cleanup、subscriber、DB、HTTP、gRPC 的关闭顺序 |
| 9 | [08-新增Runtime依赖SOP.md](./08-新增Runtime依赖SOP.md) | 新增 runtime 依赖、配置、客户端、后台任务、关闭钩子的流程 |

---

## 2. 与 `01-运行时/` 的区别

| 目录 | 关注点 |
| ---- | ------ |
| `01-运行时/` | 三进程业务运行时：apiserver、collection-server、worker 如何协作、gRPC 如何调用、事件如何流动 |
| `03-基础设施/runtime/` | 单进程内部装配：资源、容器、模块、外部客户端、transport、后台任务、生命周期如何组合 |

举例：

- `01-运行时/04-进程间调用与gRPC.md` 讲 collection-server 如何调用 apiserver。
- `runtime/05-ClientBundle与外部客户端注入.md` 讲 gRPC client / IAM client 作为 runtime dependency 如何被创建、注入和关闭。

---

## 3. 推荐阅读路径

### 3.1 第一次理解启动组合

```text
00-整体架构
  -> 01-ProcessStage与启动流水线
  -> 02-ResourceBootstrap资源装配
  -> 03-ContainerCompositionRoot
```

### 3.2 要新增配置项

```text
04-ConfigOptions配置链路
  -> 08-新增Runtime依赖SOP
```

### 3.3 要新增客户端或外部集成

```text
05-ClientBundle与外部客户端注入
  -> integrations/00-整体架构
  -> 08-新增Runtime依赖SOP
```

### 3.4 要新增后台任务或关闭逻辑

```text
01-ProcessStage与启动流水线
  -> 07-Lifecycle与关闭语义
  -> 08-新增Runtime依赖SOP
```

---

## 4. 核心不变量

1. Runtime 层只做装配，不承载业务规则。
2. 构造函数依赖优先，post-wire 只是少数 late-bound seam。
3. ResourceBootstrap 负责资源准备，Container 负责模块组装。
4. Transport stage 不应创建业务模块，只消费 Container 输出。
5. Background runtime 必须注册 shutdown hook。
6. 新配置项必须能从 config/options 追踪到使用点。
7. 启动失败应能定位 failed stage。
8. 文档必须说明降级启动和 fallback 行为。

---

## 5. Verify

```bash
go test ./internal/apiserver/process
go test ./internal/apiserver/container
go test ./internal/apiserver/transport/rest
go test ./internal/apiserver/transport/grpc
```

如果修改文档：

```bash
make docs-hygiene
git diff --check
```
