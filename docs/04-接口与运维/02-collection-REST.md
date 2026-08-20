# collection REST 接入与核验

## 1. 前置条件

- 以 [`api/rest/collection.yaml`](../../api/rest/collection.yaml) 选择 path、method、schema 和 security；
- 明确 User → ProfileLink/Testee → Assessment/Report 的资源归属检查点；
- 写请求准备稳定幂等键，等待类请求准备事实查询回退；
- 使用受控测试身份与数据，token 通过交互式输入或 Secret 注入。

并发与可靠提交边界见 [Concurrency canonical 文档](../03-基础设施/concurrency/README.md)，资源授权见 [Security canonical 文档](../03-基础设施/security/10-身份、服务与资源授权.md)。

## 2. 契约与路由核验

```bash
make docs-api-verify
go test -count=1 ./internal/collection-server/transport/rest/...
git diff --check
git diff -- api/rest/collection.yaml internal/collection-server/docs internal/collection-server/transport/rest
```

预期：生成无非预期 diff，OpenAPI/route/security contract 测试通过。变更提交或报告等待链时，再运行对应 application、gRPC client 和 WebSocket 定向测试。

## 3. 受控环境请求

```bash
export COLLECTION_BASE_URL='https://example.invalid'
export COLLECTION_PATH='/replace-with-openapi-path'
read -r -s COLLECTION_TOKEN
export COLLECTION_TOKEN

curl --fail-with-body --silent --show-error \
  --request GET \
  --header "Authorization: Bearer ${COLLECTION_TOKEN}" \
  --header "X-Request-Id: collection-check-$(git rev-parse --short HEAD)" \
  "${COLLECTION_BASE_URL}${COLLECTION_PATH}"
```

按 OpenAPI 替换请求。答卷提交必须复用同一业务意图的幂等键，分别验证首次受理、同键同内容回读与同键异内容冲突；收到 202 后继续用服务返回的标识查询事实，不把 WebSocket 唤醒当成最终结果。

## 4. 正反向接入验收

1. 正向：授权用户完成一个只读请求；涉及提交时完成“受理 → 状态查询 → 最终事实”链；
2. 反向：缺 token、跨用户 Testee/Assessment、inactive relationship 和无权报告访问均应拒绝；
3. 降级：Signal/WebSocket 丢失时，客户端能回退查询；429/503 按 `Retry-After`/退避处理，不换新幂等键猜测重试；
4. 证据：保存 SHA、OpenAPI hash、request ID、状态码与结果摘要，不保存答案或报告正文。

若跨用户资源可读、同键异内容未冲突或 202 后查不到 durable 事实，停止接入/发布并沿[常见排障](./09-常见排障.md)定位。
