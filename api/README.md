# API 文档

本目录保存当前对外导出的 REST 契约文件。

当前主入口是：

- `api/rest/apiserver.yaml`
- `api/rest/collection.yaml`

这两份文件是基于服务内 `swagger.json` 生成的 OAS 3.1 契约，适合：

- 外部对接
- 网关导入
- `/api/rest` 静态发布
- Swagger UI 浏览

## 当前生成链路

1. 先生成服务内 swagger 文件：
   - `make docs-swagger`
   - 产物位于 `internal/apiserver/docs` 和 `internal/collection-server/docs`
2. 再生成对外 REST 契约：
   - `make docs-rest`
   - 产物位于 `api/rest/apiserver.yaml` 和 `api/rest/collection.yaml`
3. 如需检查漂移：
   - `make docs-verify`

对应命令定义见 [../Makefile](../Makefile)。

## 运行时发布方式

`qs-apiserver` 和 `collection-server` 都会把 `./api/rest` 静态挂载到：

- `/api/rest`

同时把 Swagger UI 挂载到：

- `/swagger-ui/`
- `/swagger`

## 路径与安全语义

- `servers.url` 只表示协议、主机和端口；每个 `paths` key 都是可直接请求的完整运行时路径。客户端必须拼接 `server + path`，不要自行追加或剥离 `/api/v1`。
- apiserver 同时包含 `/api/v1`、`/api/v2`、`/internal/v1` 和根路径健康检查；collection-server 包含 `/api/v1` 与根路径健康检查/治理接口。
- 两份规范均声明 root `BearerAuth`。匿名健康、公开目录、二维码等 operation 以 `security: []` 显式覆盖；带 operation-level security 的接口会原样保留。
- 每个 operation 都有唯一 `operationId`，模板路径参数与 `parameters[in=path]` 一致，并提供标准 500 错误响应；受保护 operation 还提供 401/403。
- 实际是否已经挂载某条路由，最终以对应服务的 `router.go`、`registrars.go` 和 `routes_*.go` 为准。
- 服务内原始 swagger 文件仍然保留在：
  - `internal/apiserver/docs/swagger.json`
  - `internal/collection-server/docs/swagger.json`

`make docs-verify` 会同时检查 Swagger/OAS 路径方法覆盖、operationId 唯一性、路径参数、description、安全声明和标准错误响应。
