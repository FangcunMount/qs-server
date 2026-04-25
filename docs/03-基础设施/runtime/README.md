# Runtime Composition & Config Plane

**本文回答**：这个目录解释三进程启动时，`process` stage、container、ModuleGraph、ClientBundle、Options / Config 如何协作；它不重复业务模块设计，也不替代 Redis/Event/Resilience/Security 的深讲文档。

---

## 30 秒结论

| 问题 | 当前答案 |
| ---- | -------- |
| container 负责什么 | 作为 composition root 组装模块、runtime clients 和横切依赖，不承载业务规则 |
| apiserver 跨模块依赖在哪里看 | [`internal/apiserver/container/module_graph.go`](../../../internal/apiserver/container/module_graph.go) |
| collection / worker gRPC client 怎么注入 | integration stage 构建 `ClientBundle`，container 一次性安装 |
| 配置链路怎么验证 | `configs/*.yaml -> options.Options -> config.Config -> process deps` 有 contract tests |
| 新增依赖或配置项要做什么 | 更新 ModuleGraph / ClientBundle 或 Options contract，再补文档与 docs hygiene |

---

## 阅读地图

1. [00-CompositionGraph与ConfigOptions.md](./00-CompositionGraph与ConfigOptions.md) — 组合图、PostWire、ClientBundle、配置链路和维护门禁。

相关入口：

- [../05-配置体系.md](../05-配置体系.md)
- [../../01-运行时/01-apiserver.md](../../01-运行时/01-apiserver.md)
- [../../01-运行时/02-collection-server.md](../../01-运行时/02-collection-server.md)
- [../../01-运行时/03-worker.md](../../01-运行时/03-worker.md)

