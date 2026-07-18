# runtime

runtime 模块是 qs-server 的运行治理支撑层，用于描述配置加载、模块装配、服务启动、优雅关闭、worker 生命周期和资源约束。

## 1. 这个模块解决什么问题

它解决服务如何启动、依赖如何装配、配置如何进入模块、后台 worker 如何运行、停机时如何释放资源的问题。

## 2. 它在 qs-server 中处于什么位置

runtime 横跨 qs-apiserver、collection-server 和 qs-worker 三个进程，是业务模块和基础设施能力被装配到进程中的入口。

## 3. 整体架构是什么

配置加载形成 options；ResourceBootstrap 初始化 DB / Redis / MQ 等资源；container 装配模块；transport 和 worker runtime 启动；lifecycle 管理 shutdown。

## 4. 关键链路有哪些

| 链路 | 文档 |
| --- | --- |
| 整体架构 | [01-运行时整体架构.md](01-运行时整体架构.md) |
| 配置管理 | [02-配置管理.md](02-配置管理.md) |
| 服务启动与模块装配 | [03-服务启动与模块装配.md](03-服务启动与模块装配.md) |
| 优雅关闭 | [04-优雅关闭.md](04-优雅关闭.md) |
| Worker 生命周期 | [05-Worker生命周期.md](05-Worker生命周期.md) |
| 部署与资源约束 | [06-部署与资源约束.md](06-部署与资源约束.md) |

## 5. 为什么选择当前方案

三进程职责不同，不能把运行时装配写成单一组件。通过 container 和 process bootstrap 分别约束 apiserver、collection-server、worker 的依赖边界。

## 6. 代码事实源

- [../../../internal/apiserver/container](../../../internal/apiserver/container)
- [../../../internal/collection-server/container](../../../internal/collection-server/container)
- [../../../internal/worker/process](../../../internal/worker/process)
- [../../../internal/worker/container](../../../internal/worker/container)
- [../../../internal/pkg/configcontract](../../../internal/pkg/configcontract)
