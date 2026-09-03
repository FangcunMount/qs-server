# internal gRPC 联调 Runbook

## 1. 前置条件

internal gRPC 仍是受保护的进程边界。联调前准备：目标 SHA、目标方法的 proto、调用服务身份、CA/客户端证书、委托主体与测试资源。
方法 ACL 和资源 ownership 的稳定语义见[身份、服务与资源授权](../03-基础设施/security/10-身份、服务与资源授权.md)，证书与网络边界见[敏感信息、网络暴露与验收](../03-基础设施/security/20-敏感信息、网络暴露与验收.md)。

## 2. 无网络契约检查

```bash
bash scripts/proto/generate.sh
git diff -- api/grpc/proto api/grpc/gen
go test -count=1 ./internal/pkg/grpc ./internal/apiserver/transport/grpc/...
go test -count=1 ./internal/collection-server/infra/grpcclient/... ./internal/worker/infra/grpcclient/...
```

预期：proto 重新生成后无非预期 diff；server registry、client contract、mTLS identity 和 default-deny ACL 测试通过。生成器失败或生成物漂移时不要手改 `api/grpc/gen`。

## 3. 环境连通与授权检查

使用受控环境的 `grpcurl`（或等价客户端）并通过文件引用证书，禁止把私钥内容写入命令或工单：

```bash
grpcurl \
  -cacert "$GRPC_CA_FILE" \
  -cert "$GRPC_CLIENT_CERT_FILE" \
  -key "$GRPC_CLIENT_KEY_FILE" \
  "$GRPC_TARGET" grpc.health.v1.Health/Check
```

随后按 proto 构造目标 RPC 的最小测试请求，并执行四类检查：

1. 允许的 service identity + 允许方法 + 合法委托主体成功；
2. 无证书、错误 CA/OU/CN 失败；
3. 已认证但不在方法 ACL 的 workload 失败；
4. 方法 ACL 允许、但最终用户不拥有目标资源时仍失败。

标准 health 成功只证明 gRPC listener/health service 可响应，不证明目标方法及其依赖可用。

## 4. 失败处置与退出条件

- TLS handshake 失败：核对 CA chain、SAN/CN/OU、证书期限和目标名，不关闭 mTLS 绕过；
- `Unauthenticated`：查 service identity 或 delegated subject；
- `PermissionDenied`：区分方法 ACL、capability 与业务资源归属，不能扩大 default policy；
- `Unimplemented`：先确认部署 SHA 与滚动升级兼容窗口；
- `Unavailable`/deadline：保留调用端与服务端 request/trace 标识，检查 listener、依赖和 shutdown 状态。

完成时记录 source/deployed SHA、证书版本（不记录私钥）、方法、正反向结果、限制和失效期；环境/生产证据写入[基础设施生产证据台账](../00-总览/10-基础设施生产证据台账.md)。
