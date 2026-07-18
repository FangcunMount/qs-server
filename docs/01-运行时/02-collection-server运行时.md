# collection-server 运行时

## 1. 结论

`collection-server` 是前台 BFF 和保护层。它组合身份上下文、租户范围、入口限流、SubmitQueue、SubmitGuard、下游背压、状态查询和 gRPC client，但不拥有 Survey/Evaluation 聚合。

## 2. 请求路径

```text
REST middleware
  -> identity / tenant scope
  -> rate limit / queue / duplicate guard
  -> application journey
  -> apiserver gRPC
  -> response / submit status / report wait
```

具体端点以 `api/rest/collection.yaml` 为准；保护机制以 `internal/collection-server` 的实际装配和配置为准。

## 3. 失败语义

入口过载、排队失败、重复请求、下游背压和业务拒绝必须区分。不得把所有失败都转成成功响应或无限重试。
