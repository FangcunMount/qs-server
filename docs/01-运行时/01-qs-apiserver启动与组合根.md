# qs-apiserver 启动与组合根

## 1. 结论

`qs-apiserver` 由应用入口创建，容器负责基础设施、业务模块、transport、后台任务和清理顺序。模块装配已收敛到 `internal/apiserver/container/modules/*`。

## 2. 组合顺序

阅读时依次检查：

1. `cmd/qs-apiserver/apiserver.go`；
2. `internal/apiserver` 应用生命周期；
3. `internal/apiserver/container`；
4. `container/modules/registry.go`；
5. 目标模块的 `wire.go`、`install.go`、`bootstrap.go`；
6. REST/gRPC export 与后台 runner。

`registry.go` 中的 legacy sequence 仍记录初始化兼容顺序，但当前业务装配事实应优先看迁移后的模块包。

## 3. 验证

运行模块 registry、wiring、architecture 定向测试；涉及资源生命周期时再覆盖启动和 Cleanup 行为。
