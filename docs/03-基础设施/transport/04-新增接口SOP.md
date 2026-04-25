# 新增接口 SOP

**本文回答**：新增或修改 REST/gRPC 接口时，工程师必须按什么顺序改代码、补测试、更新契约和文档。

## 30 秒结论

新增接口不要先写 handler。先确定调用方、surface、auth、wire contract，再补 contract test，最后实现 adapter。

```mermaid
flowchart LR
    Intent["确定调用方与 surface"]
    Contract["定义 REST/OpenAPI 或 proto"]
    Test["补 contract test"]
    Adapter["实现 handler/service adapter"]
    App["调用 application service"]
    Docs["更新接口文档"]
    Verify["go test + docs hygiene"]

    Intent --> Contract --> Test --> Adapter --> App --> Docs --> Verify
```

## REST 新增流程

1. 判断接口属于 collection BFF、apiserver business，还是 apiserver internal。
2. 在对应 `transport/rest/routes_*.go` 或 router 中注册 route，明确 middleware surface。
3. 更新 `api/rest/*.yaml` 或对应生成链。
4. 补 OpenAPI contract test 或 route matrix test。
5. handler 只做 bind/validate/call/write，不写领域规则。
6. 更新 [REST 契约](../../04-接口与运维/01-REST契约.md) 或业务模块深讲。

## gRPC 新增流程

1. 判断是否真的需要 gRPC；前台/后台普通查询优先 REST。
2. 修改 `.proto`，保持 package 和 go_package 策略一致。
3. 本轮若必须重生成 proto，必须单独声明 codegen 变更，不混在 transport cleanup 中。
4. 在 `transport/grpc/registry.go` 增加注册路径。
5. service adapter 放在 `transport/grpc/service` 主路径；不要新增 `interface/*` implementation 包。
6. 补 proto/registry contract test。

## 不可变边界

- 不在 handler/service adapter 中写业务状态机。
- 不新增未记录的 internal governance route。
- 不新增未声明的 proto go_package 漂移。
- collection transport 不得 import apiserver interface/transport。

## Verify

```bash
go test ./internal/apiserver/transport/rest ./internal/apiserver/transport/grpc
go test ./internal/collection-server/transport/rest
python scripts/check_docs_hygiene.py
git diff --check
```
