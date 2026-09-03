# gRPC 契约变更 Runbook

## 1. 前置条件

以 [`api/grpc/proto`](../../api/grpc/proto/) 为机器事实源，生成文件位于 `api/grpc/gen`。变更前记录 checkout SHA，
并确认 `protoc`、`protoc-gen-go`、`protoc-gen-go-grpc` 可用：

```bash
git rev-parse HEAD
git status --short
command -v protoc
command -v protoc-gen-go
command -v protoc-gen-go-grpc
```

## 2. 修改与生成

1. 只改 proto 源，不手改 `api/grpc/gen`；
2. 兼容变更新增字段号/方法；删除字段时保留 `reserved`，不得复用已发布字段号；
3. 同步 server registry、service adapter、所有 client、生产 ACL 与兼容测试；
4. 执行生成并检查 diff：

```bash
bash scripts/proto/generate.sh
git diff --check
git diff -- api/grpc/proto api/grpc/gen
make docs-check
```

`docs-check` 会从 proto 重新派生文件、service 和 RPC inventory；不要在本文手工维护枚举。生成出现非预期删除、package 路径变化或字段号复用时立即停止。

## 3. 非缓存定向测试

```bash
go test -count=1 ./internal/apiserver/transport/grpc/...
go test -count=1 ./internal/collection-server/infra/grpcclient/... ./internal/collection-server/port/grpcbridge/... ./internal/collection-server/port/aiexplanation/...
go test -count=1 ./internal/worker/infra/grpcclient/...
go test -count=1 ./internal/pkg/grpc ./internal/pkg/configcontract
```

以下高风险契约发生相关变更时必须保留对应行为测试：

| 变更面 | 必查结果 |
| --- | --- |
| AnswerSheet durable result lookup | hit/miss/conflict/read error/cancel/deadline，以及旧服务 `Unimplemented` 的滚动升级回退 |
| Assessment ownership authorization | owner 成功、owner 不匹配 `PermissionDenied`、collection client/bridge 转发 |
| Participant AI explanation | capability/request/get 三个 delegated purpose、功能关闭的 `Unimplemented -> feature_disabled`、完整结构化内容投影 |
| Prompt evaluation step | event ID metadata 与 payload 一致、请求审计匹配、active lease 映射 `Aborted`、重复/取消目标 ACK 且不调用 Provider |
| 新增 internal 方法 | 调用 workload 身份、default-deny ACL、委托主体和最终资源 ownership |
| 启用 gRPC JWT auth | `TokenVerifier` 存在时正常启动；缺失时 server 构建失败，不允许跳过认证 |

方法 ACL 只限制哪个服务能调用 RPC，不能替代 User/Testee/Assessment 等业务资源归属；稳定规则见 [Security canonical 文档](../03-基础设施/security/10-身份、服务与资源授权.md)。

## 4. 环境兼容验收

按[internal gRPC 联调 Runbook](./04-internal-gRPC.md)执行：

1. 新 client → 新 server 正向请求；
2. 旧 client → 新 server 兼容请求；
3. 滚动升级需要时，新 client → 旧 server 的预期降级/回退；
4. 错误证书、未授权 workload、未授权方法和跨资源请求均失败；
5. 保存 deployed SHA、proto hash、证书版本、请求/响应 status 和观察窗口。

任一旧调用方无法解析、ACL 未同步或资源授权被绕过时停止发布；不能通过 default allow、关闭 mTLS 或吞掉 `PermissionDenied` 完成联调。

生产结果写入[基础设施生产证据台账](../00-总览/10-基础设施生产证据台账.md)。
