# Runtime

Runtime 主题只维护三个进程的 composition root、Stage、后台任务 owner、服务启动和关闭语义。配置覆盖、Secret、镜像和网络拓扑已提升为独立的[配置与部署](../config-deployment/README.md)主题。

## Canonical 文档

- [进程生命周期、启动与关闭](./10-进程生命周期、启动与关闭.md)：`runtime/lifecycle` 唯一主文档。

## 快速事实

- qs-apiserver、collection-server、qs-worker 是三个独立进程，各有显式 Stage runner。
- apiserver 组合 9 个 scheduler runner；其中 AI Prompt 评测 prepared lease recovery、Participant AI 解读 lease recovery 与 Mongo
  consistency audit 的版本化生产配置为关闭。能装配不等于生产启用，实际开关以 effective config 和部署证据为准。
- Stage runner 遇错即停但没有通用 rollback；prepare 阶段资源所有权和关闭注册顺序仍是风险点。
- 正常信号关闭、监听器异常退出和 prepare 失败不是同一条路径，必须分别验证。
- 当前 gRPC `GracefulStop()` 没有 deadline，collection pprof 未统一纳入 shutdown，worker 存在双重信号所有权；这些 gap 不能被文档重构抹去。

## 验证入口

```bash
go test -count=1 ./internal/apiserver/process ./internal/collection-server/process ./internal/worker/process ./internal/apiserver/runtime/scheduler
```

真实环境还需验证信号摘流、在途请求、MQ consumer、scheduler lease、超时强退和依赖释放；当前 checkout 没有对应生产关闭演练证据。
