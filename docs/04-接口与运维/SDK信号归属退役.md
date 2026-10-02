# SDK 信号归属退役

M6-04 将通用最佳努力 Redis 发布订阅接口和适配器从 component-base 迁入 reliable-messaging。报告业务信号、缓存治理、授权等待、权威读模型与轮询兜底仍由 QS 负责。原频道／前缀、JSON、连接借用、取消与坏消息错误处理语义不变。不修改应用配置、任务授权或消息重放；不新增部署服务、数据库或业务流程。

原消息类不再由 component-base 维护；固定 SDK 标签通过后才升级组件固定标签。旧镜像／原配置用于明确回退窗口，不引入运行时兼容转发。

迁移调用点：

- `internal/collection-server/cache/subsystem.go`
- `internal/collection-server/container/application_runtime.go`
- `internal/collection-server/application/reportnotify/watcher.go`
- `internal/collection-server/application/reportwait/service.go`
- `internal/apiserver/cache/governance/signal_watcher.go`
- `internal/apiserver/cache/subsystem/signal_runtime.go`
- `internal/pkg/reportstatus/signaling.go`
- `internal/pkg/reportstatus/reporter.go`
- `internal/pkg/architecture/event_cache_signal_boundary_test.go`
