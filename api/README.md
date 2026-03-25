# API 文档

本目录保存当前对外导出的 REST 契约文件。

当前主入口是：

- `api/rest/apiserver.yaml`
- `api/rest/collection.yaml`

这两份文件是基于服务内 `swagger.json` 生成的 OAS 3.1 摘要，适合：

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

## 注意事项

- `api/rest` 是导出的契约目录，不是实际业务接口前缀；真实业务接口仍然挂在 `/api/v1`。
- 实际是否已经挂载某条路由，最终以对应服务的 `routers.go` 为准。
- 服务内原始 swagger 文件仍然保留在：
  - `internal/apiserver/docs/swagger.json`
  - `internal/collection-server/docs/swagger.json`
