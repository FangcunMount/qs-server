# Transport & Contract Plane 深讲阅读地图

**本文回答**：Transport & Contract Plane 的文档应该从哪里读、各篇分别维护什么事实，以及新增 REST/gRPC 接口时应该回到哪些 contract test。本文是稳定入口，细节不要散落到运行时或业务模块文档里重复维护。

## 30 秒结论

| 问题 | 结论 |
| ---- | ---- |
| 本层解决什么 | 把 REST、gRPC、OpenAPI、proto、路由注册、handler/service adapter 的契约边界收口 |
| 当前真值 | 运行时 router/registry + `api/rest/*.yaml` + `.proto` + contract tests |
| 当前边界 | 不改业务语义；只保证 wire contract、auth surface、adapter ownership 可核对 |
| 迁移状态 | REST handler/DTO 与 gRPC service adapter 已归属 `transport/*`；generated proto 继续保留历史 `interface/grpc/proto` 路径 |
| 入口文档 | 本目录讲架构和 SOP；[04-接口与运维](../../04-接口与运维/) 保留契约运维索引 |

## 阅读顺序

1. [00-整体架构.md](./00-整体架构.md) - 五层模型、三进程职责、当前包边界。
2. [01-REST路由与契约.md](./01-REST路由与契约.md) - route matrix、OpenAPI、auth surface、collection/apiserver 分工。
3. [02-gRPC契约与服务适配.md](./02-gRPC契约与服务适配.md) - proto、registry、service adapter、worker/collection client。
4. [03-OpenAPI与Proto生成边界.md](./03-OpenAPI与Proto生成边界.md) - generated artifacts、漂移检查、当前不重生成 proto 的边界。
5. [04-新增接口SOP.md](./04-新增接口SOP.md) - 新增或修改接口时的决策和验收清单。

## Verify

```bash
go test ./internal/apiserver/transport/rest ./internal/apiserver/transport/grpc
go test ./internal/collection-server/transport/rest ./internal/pkg/httpauth
python scripts/check_docs_hygiene.py
```
