# 新增安全能力 SOP

**本文回答**：新增或修改 JWT、IAM、authz snapshot、capability、service auth、mTLS、ACL、operator projection 时，应该先补什么模型和测试，避免安全事实散落。

## 30 秒结论

新增安全能力默认流程是：

```mermaid
flowchart LR
    Model["模型/边界"]
    Contract["contract tests"]
    Runtime["runtime adapter"]
    Docs["security docs"]
    Hygiene["docs hygiene + go test"]

    Model --> Contract --> Runtime --> Docs --> Hygiene
```

如果不能说明它属于 `Principal`、`TenantScope`、`AuthzSnapshot`、`CapabilityDecision`、`ServiceIdentity` 中哪一类，先不要直接写中间件或 handler。

## 标准步骤

| 步骤 | 必做内容 |
| ---- | -------- |
| 1. 定模型 | 明确新增能力属于身份、租户范围、授权快照、能力判断、服务身份、传输安全还是 projection |
| 2. 锁行为 | 补 contract tests，覆盖 status、context key、metadata、error code、fallback |
| 3. 选位置 | transport 只做 adapter，application/authz 做 capability，IAM SDK 留在 infra/container |
| 4. 补文档 | 更新本目录对应深讲页和锚点 |
| 5. 验证 | 跑目标测试、docs hygiene、`git diff --check` |

## 常见变更决策表

| 变更 | 应放在哪里 | 不应做什么 |
| ---- | ---------- | ---------- |
| 新 JWT claim | `middleware` 验证投影 + `securityplane.Principal` 文档 | 不在 handler 直接读 raw claim |
| 新 tenant/org 规则 | identity/scope middleware + `TenantScope` tests | 不在业务服务里解析 `tenant_id` 字符串 |
| 新 REST 权限 | `application/authz` capability + route middleware | 不用 JWT roles 判断 capability |
| 新 service-to-service 调用 | service auth wrapper / gRPC client wiring | 不手写 authorization metadata 字符串 |
| 新 mTLS/ACL 规则 | gRPC config / interceptor contract | 不把 ACL 文件加载写成已实现，除非有测试 |
| operator roles 变化 | role projection service / IAM assignment flow | 不把本地 projection 当权限真值 |

## 必补测试

| 能力 | 测试要求 |
| ---- | -------- |
| JWT / Principal | claims 字段、metadata、fallback ID、HTTP/gRPC 对齐 |
| TenantScope | numeric org、非数字 tenant、空 tenant、scope 错误码 |
| AuthzSnapshot | snapshot load failure、context injection、version invalidation |
| CapabilityDecision | allowed/denied/missing snapshot/unknown capability |
| ServiceIdentity | metadata header、token error、transport security flag |
| mTLS / ACL | interceptor order、identity match、default policy、skip method |
| Projection | unchanged no-op、changed persist、persist failure best-effort |

## 否定边界

- 不新增绕过 authz snapshot 的 role-based handler 判断。
- 不把 JWT roles 写成业务权限真值。
- 不把 ACL file loader 写成已实现，除非同时补 parser 和 contract tests。
- 不在 docs 中把 service auth 的 `RequireTransportSecurity=false` 写成长期安全目标。
- 不让业务模块直接依赖 IAM SDK 或 component-base mTLS/ACL primitive。

## Verify

```bash
GOTOOLCHAIN=local /Users/yangshujie/.gvm/gos/go1.25.9/bin/go test ./internal/pkg/securityplane ./internal/pkg/middleware ./internal/pkg/httpauth ./internal/pkg/grpc ./internal/pkg/iamauth
GOTOOLCHAIN=local /Users/yangshujie/.gvm/gos/go1.25.9/bin/go test ./internal/apiserver/transport/rest/middleware ./internal/apiserver/transport/grpc ./internal/apiserver/infra/iam ./internal/collection-server/infra/iam
python scripts/check_docs_hygiene.py
git diff --check
```
