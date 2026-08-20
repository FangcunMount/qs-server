# apiserver REST 接入与核验

## 1. 前置条件

- 以 [`api/rest/apiserver.yaml`](../../api/rest/apiserver.yaml) 选择 path、method、request/response schema 和 security；
- 明确该操作的调用主体、QS 组织范围、IAM capability 与资源 ownership；
- 准备受控环境地址和测试身份，不在命令行或文档中粘贴 token。

授权边界以[身份、服务与资源授权](../03-基础设施/security/10-身份、服务与资源授权.md)为准；本页不复制路由或 DTO 清单。

## 2. 契约与路由核验

```bash
make docs-api-verify
go test -count=1 ./internal/apiserver/transport/rest/...
git diff --check
git diff -- api/rest/apiserver.yaml internal/apiserver/docs internal/apiserver/transport/rest
```

预期：生成与比较退出码为 0，受影响路由测试通过，OpenAPI 与 router/handler 的公开性和安全声明一致。若只改了 YAML 或生成文件而生成源未变，停止并修正源头。

## 3. 受控环境请求

```bash
export APISERVER_BASE_URL='https://example.invalid'
export APISERVER_PATH='/replace-with-openapi-path'
read -r -s APISERVER_TOKEN
export APISERVER_TOKEN

curl --fail-with-body --silent --show-error \
  --request GET \
  --header "Authorization: Bearer ${APISERVER_TOKEN}" \
  --header "X-Request-Id: rest-check-$(git rev-parse --short HEAD)" \
  "${APISERVER_BASE_URL}${APISERVER_PATH}"
```

把 method、path、header 和 body 按当前 OpenAPI 替换。写接口必须使用专用测试数据和稳定幂等键；禁止在生产用任意 POST/PUT/DELETE 做“连通性探测”。

## 4. 反向检查

对受保护操作至少验证：缺 token、无 capability、错误组织范围或不属于该主体的资源返回拒绝；对公开操作则验证其业务 token/签名/资源约束，而不是只验证无需 Bearer。跨用户或跨组织读取一旦成功，立即停止发布并按 Security gap 处理。

## 5. 完成与失败处置

- 成功：记录 SHA、OpenAPI hash、测试命令、成功/拒绝响应状态与 request ID；
- 401：先查凭证、issuer/audience 与时间，再查 principal 投影；
- 403：查 capability、组织范围和 resource ownership，不通过扩大 ACL 绕过；
- 404/405：回查 OpenAPI 完整 path 与 method，再核对 router 装配；
- 5xx：用 request ID 关联日志，避免把内部 error、token 或响应正文写入证据。

生产结果写入[基础设施生产证据台账](../00-总览/10-基础设施生产证据台账.md)。
