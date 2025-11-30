# API 文档

该目录收纳 swagger 工具生成的规范文件，便于外部对接或网关导入。

- `api/apiserver/swagger.(json|yaml)`: 来自 `internal/apiserver/docs`，对应 API Server。
- `api/collection/swagger.(json|yaml)`: 来自 `internal/collection-server/docs`，对应 Collection Server。

更新流程：
1. 按服务更新 swagger 注解。
2. 在仓库根运行：
   - `swag init --parseInternal -g apiserver.go -d cmd/qs-apiserver,internal/apiserver,internal/pkg -o internal/apiserver/docs`
   - `swag init --parseInternal --parseDependency -g main.go -d cmd/collection-server,internal/collection-server,pkg -o internal/collection-server/docs`
3. 将生成的 swagger.json / swagger.yaml 同步到 `api/apiserver` 和 `api/collection`。
