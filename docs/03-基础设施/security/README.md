# Security：身份、权限、组织与资源归属

Security 层不是一个 JWT middleware。一次请求只有依次通过“凭证可信、主体可识别、组织范围合法、动作被允许、资源确实属于该主体/组织”之后，才具备完整授权。任何一层都不能替代其它层。

## 1. 先看结论

- AuthN 只回答 token/证书是谁签发、是否有效；AuthZ 才回答该主体能否执行某个动作。
- IAM `tenant_domain` 是授权域，QS `org_id` 是业务组织范围，两者语义不同，不能互相填充。
- apiserver 不信任 JWT 中的 `org_id` 作为 QS 组织事实，而是从本地 active operator membership 解析；客户端只可用 `X-Org-Id`/`org_id` 指明候选范围。
- capability 决策基于 IAM authorization snapshot 的 `(resource, action)`，不以 JWT roles 直接替代；缺少快照时 capability middleware 拒绝。
- collection-server 不在通用 HTTP middleware 中解析 QS org，而是在报告/测评查询前证明 IAM User 与 Testee 的 active ProfileLink。
- gRPC 的 mTLS、IAM bearer 和方法级授权是三层不同控制。当前生产配置主要依赖 mTLS；这能认证工作负载，不自动等于每个 RPC 都有业务 capability 检查。
- 当前 HTTP 路由装配在 IAM 被关闭或 verifier 缺失时会跳过整组认证并继续注册路由。这是已实现现状，也是明确的生产风险；不能把文档写成“配置错误时默认拒绝”。
- 身份、授权快照或 ProfileLink 查询在已启用安全链中失败时，一般返回 401/403/503，不应使用旧缓存或本地角色扩大权限。

## 2. 安全链分层

```text
transport protection
  TLS / mTLS / trusted network
          ↓
credential verification
  JWT signature, issuer, audience, expiry
          ↓
principal projection
  user / service + session/token metadata
          ↓
scope resolution
  IAM tenant domain + QS org or Testee subject
          ↓
capability decision
  IAM snapshot resource/action
          ↓
resource authorization
  operator / clinician / participant / ProfileLink ownership
          ↓
business command/query
```

| 层 | 核心问题 | 当前事实源 |
| --- | --- | --- |
| Transport | 通道是否加密、对端证书是否可信 | HTTPS、gRPC TLS/mTLS、网络边界 |
| Authentication | bearer 是否有效，主体是谁 | IAM TokenVerifier、JWKS/remote verify |
| Principal | user/service 的稳定上下文是什么 | `internal/pkg/securityplane` 只读投影 |
| Organization | 请求落在哪个 QS org | active operator membership / Testee link |
| Capability | 能否 read/manage/publish/audit | IAM authorization snapshot |
| Resource access | 能否访问这一条 Testee/Assessment/Report | application access service 和 repository 条件 |
| Audit | 谁在何时执行了什么高风险动作 | request/event ID、system governance action run、业务审计字段 |

`securityplane`/`securityprojection` 是统一观察和传递身份语义的模型，不是一个会自动执行授权的“安全控制器”。真正的 deny 必须发生在 middleware、interceptor 或 application access service。

## 3. HTTP 用户认证

### 3.1 TokenVerifier

apiserver 与 collection-server 都通过 IAM SDK 构造 TokenVerifier：

1. 按配置校验 issuer、audience、algorithm、clock skew 和 required claims。
2. 启用 JWKS 时优先本地验签，并维护定时刷新/缓存。
3. JWKS 获取错误可使用 SDK 缓存；verifier 也具备 IAM gRPC 远程验证能力。
4. `force_remote_verification=true` 时每次 token 校验走 IAM 权威在线验证。

本地 JWKS 验签减少 IAM 同步依赖，但它验证的是签名和 claims，不等价于实时会话撤销检查。需要即时撤销语义的入口应使用 remote verify 或引入明确的撤销证据，不能把 24 小时 JWKS cache TTL 当成会话有效期。

### 3.2 HTTP middleware

`internal/pkg/middleware.JWTAuthMiddlewareWithOptions` 当前支持从以下位置提取 token：

1. `Authorization: Bearer <token>`，也兼容 header 中直接放 token。
2. `access_token` query。
3. `access_token` cookie。

生产客户端应使用 Bearer header。query token 会进入 URL、代理和访问日志，cookie 还涉及 CSRF/SameSite 策略；“代码支持”不代表这些方式具有相同安全性。

验证成功后，middleware 投影：

- `user_id`、`account_id`。
- `tenant_domain`、原始 JWT `org_id`。
- `session_id`、`token_id`。
- roles、AMR 和 verify metadata。

随后 `httpauth.UserIdentityMiddleware` 生成 user principal。注意：此时 JWT `org_id` 不会直接写成已解析的 QS OrgScope。

## 4. apiserver 受保护请求链

`/api/v1`、`/api/v2`、`/internal/v1`、`/internal/v2` 共用以下组级链路：

```text
JWT verify
  → UserIdentity
  → Require tenant_domain
  → Resolve active operator org
  → Require QS org scope
  → Load IAM authz snapshot
  → route-level capability
  → application resource authorization
```

### 4.1 组织解析

- `tenant_domain` 来自 IAM JWT，表示 Casbin/IAM 授权域。
- `org_id` 从 QS active operator membership 解析。
- 请求可通过 `X-Org-Id`，兼容 `org_id` query，选择一个候选组织。
- 用户没有 active membership、候选组织不匹配或多组织但未选择时，分别拒绝为 401/403/400 语义。
- 只有找不到 ActiveOperatorChecker 的兼容装配才使用固定 org `1` resolver；这不是多租户生产设计。

因此不能把“JWT 中有 org_id”当作本地资源查询可直接信任的条件。

### 4.2 IAM authorization snapshot

`internal/pkg/iamauth.SnapshotLoader` 读取 IAM `GetAuthorizationSnapshot`：

- key 为 `domain + user + app`。
- 进程内默认 TTL 为 30 秒，并用 singleflight 合并并发 miss。
- `iam.authz.version` 消息推进 tenant authz version 水位并剔除旧快照。
- 拉取失败时 Authz middleware 返回 503，不继续扩大权限。

snapshot 包含 roles、permissions、authz version、Casbin domain 和 app。apiserver capability 映射把业务能力翻译为稳定的 IAM resource/action，例如 questionnaire read/manage、assessment model publish、plan manage、evaluation retry 和 interpretation audit。

`RequireCapabilityMiddleware` 只看 snapshot；snapshot 缺失、能力未知或 permission 不满足都 deny。旧 `RequireRoleMiddleware` 仍存在兼容路径，但新增动作级授权应使用 capability，不应信任 JWT role 代替 IAM 权限快照。

### 4.3 资源级授权

即使 capability 允许，也只能说明“原则上可以做此类动作”。application 仍需验证：

- resource `org_id` 与当前 OrgScope 一致。
- clinician 与 testee 的关系仍有效。
- participant 只能访问自己的 Assessment/Report。
- operator active 状态、报告 audience、业务状态和命令前置条件成立。

查询时必须把 org/actor 条件带进 repository/read model；“先按 ID 查出来，再在 handler 里忘记检查”会形成 IDOR。

## 5. collection-server 请求链

collection-server 在 `/api/v1` 使用：

```text
JWT verify
  → UserIdentity
  → Require tenant_domain
  → IAM authz snapshot（权限视图）
  → route-specific User → Testee access
```

它对以下只读 catalog path 显式跳过认证：

- `GET /api/v1/assessment-models`
- `GET /api/v1/assessment-models/hot`
- `GET /api/v1/assessment-models/options`
- `GET /api/v1/typology-models`
- `GET /api/v1/typology-models/categories`

这是精确 method/path 白名单，不是整个 catalog prefix 都公开；例如动态 `/:code` 是否公开要以当前 path 白名单为准。

collection 不把 JWT org 投影为 QS OrgScope。报告等受保护查询通过 `TesteeAccessMiddleware`：

1. 从 apiserver 取得 Testee 及其 IAM Profile ID。
2. 向 IAM 检查当前 User 与 Profile 的 active link。
3. not found、无 profile 或 link false 返回 403。
4. apiserver/IAM 不可用或依赖未配置返回 503。

这条链是资源归属证明，authorization snapshot 不能替代它。

## 6. gRPC 与服务身份

### 6.1 Server transport

apiserver gRPC server 可组合：

- TLS 或 mTLS server credentials。
- mTLS identity interceptor。
- IAM JWT interceptor。
- OrgScope 与 AuthzSnapshot interceptor。
- ACL 和 audit interceptor。

健康检查和 reflection 跳过 IAM JWT。生产配置当前：

- `insecure=false`。
- mTLS enabled，要求 client cert，并以 CA/OU 等约束客户端。
- gRPC IAM `auth.enabled=false`。
- ACL 与 gRPC audit disabled。
- reflection disabled。

所以当前生产 gRPC 主保证是“持有受信证书的工作负载可以建立连接”。它不自动提供 user JWT capability，也不等于所有 service 都可执行全部业务动作；敏感 RPC 仍需 application guard、delegated subject 或后续方法级 ACL。

### 6.2 ServiceAuth bearer

apiserver/collection-server 的 IAM `ServiceAuthHelper` 能申请并刷新 service token，通过 `authorization: Bearer ...` 附加到 gRPC PerRPC metadata。它表达 service ID 和 target audience，不是用户委托。

`serviceauth.RequireTransportSecurity()` 当前为了兼容返回 `false`；因此 PerRPC credential 本身不会强制 gRPC channel 使用 TLS。真正的传输安全必须由 dialer 的 TLS/mTLS 配置保证，review 时必须同时检查 client credentials，不能只看到 Bearer metadata 就判定安全。

### 6.3 mTLS 与委托主体

mTLS 表达调用方 workload；当 collection-server 代表某个 Testee 查询报告时，还需要 delegated subject 绑定 purpose、testee/assessment 和有效期。`ParticipantReportService` 会校验允许的 workload、delegated token 签名/过期和业务资源归属。

服务身份、用户身份与被委托的 Testee 是三个不同主体，不能把其中一个 ID 填进另一个字段绕过授权。

## 7. Public、Protected 与 Internal 不是同义词

apiserver 显式不走 protected middleware 的入口包括：

- `/health`、`/readyz`、`/ping`。
- `/governance/redis`。
- `/api/v1/public/*`。
- QR code 与 assessment asset 读取。

`/internal/*` 只是路径和受众命名；当前它与普通 protected API 一样走 JWT、OrgScope 和 route-level capability，并不是“只要内网就可访问”。

generic server 还可能按配置开放 `/metrics`、`/debug/pprof` 等运维端点。生产配置当前启用了 metrics 和 profiling，因此必须通过监听地址、反向代理、防火墙或网络策略限制暴露；不要假设业务 JWT middleware 会自动包住这些端点。

公开 asset handler 的对象可见性由 HTTP 协议控制。OSS bucket ACL 不是业务授权替代品；公开 path 只能存放设计为公开的对象。

## 8. 失败与降级语义

| 场景 | 当前行为 | 安全含义 |
| --- | --- | --- |
| bearer 缺失/无效 | 401 | fail closed |
| TokenVerifier 已挂载但 verify 出错 | 401 | 不把 IAM/JWKS 故障当匿名用户 |
| tenant domain 缺失 | 401 | 不猜授权域 |
| OrgScope 无法解析 | 401/403/400/500 | 不信任 JWT org 或任意 header |
| Authz snapshot loader 调用失败 | 503 | 权限不可确认，不执行 capability route |
| capability snapshot 缺失/denied | 403 | fail closed |
| collection ProfileLink 查询失败 | 503 | 不把依赖故障当允许 |
| ProfileLink 不存在 | 403 | fail closed |
| IAM 整体 disabled | **受保护 group 不安装认证 middleware** | **fail open 的配置风险** |
| IAM enabled 但 TokenVerifier nil | **受保护 group 不安装认证 middleware** | **fail open 的装配风险** |
| AuthzSnapshotLoader 未装配 | group 保留 JWT/Org，capability route 因无 snapshot deny | 非 capability route 仍需 application 授权审计 |
| gRPC auth enabled 但 verifier nil | interceptor 被跳过并记录 warning | fail open 的装配风险 |

后四项不能用“开发环境方便”掩盖。生产运行应通过配置门禁/启动验证保证 IAM、verifier、snapshot、mTLS 与必要 ACL 均已装配；当前代码仍需继续收敛为 production fail-fast/fail-closed。

## 9. 敏感信息与日志

禁止输出：

- bearer/service token、cookie、session token 和 delegated subject 原文。
- JWT 完整 claims 或 `Extra` 值。
- MySQL/Mongo/Redis 密码、OSS secret、WeChat AppSecret、TLS private key。
- AnswerSheet 答案、报告正文和对象签名 URL。

当前 JWT middleware 的 Debug 日志会记录 verifier result、raw claims 和 mapped claims；生产即使通常使用较高日志级别，这仍是需要明确治理的敏感日志风险。安全做法是只记录 request ID、user/token ID 的不可逆或最小必要标识、issuer/audience 判定结果、错误分类和 Extra key names。

配置结构对部分 secret 使用 `json:"-"`，但这不等于日志、Viper dump、panic 或命令行参数天然脱敏。密钥应由 secret manager/只读挂载/环境注入，并限制文件权限与轮换范围。

启动流程中的 Viper/Options 输出已使用 `configmask`，但 `cliflag.PrintFlags` 会在 Debug 级别输出原始 flag value。worker 当前 production YAML 又配置为 debug，因此不得通过 CLI flag 传递数据库密码、token 或 shared secret；代码还应把 flag 输出纳入同一脱敏策略。

## 10. 常见误判

| 看到 | 不能直接推出 |
| --- | --- |
| JWT signature valid | 用户仍有当前权限或会话未撤销 |
| JWT role=admin | 具备 QS capability |
| JWT org_id=7 | 可以访问 QS org 7 |
| mTLS handshake success | 可以调用任意 RPC |
| IAM snapshot cache hit | 权限永远实时 |
| path 叫 `/internal` | 网络层和业务层都已隔离 |
| OSS object 可读 | 当前用户拥有其业务资源 |
| middleware 已注册 | application 不再需要资源归属检查 |

## 11. 扩展与验收清单

新增 HTTP/gRPC 接口时：

1. 明确 public、authenticated user、operator、participant 还是 service workload。
2. 声明 tenant domain、QS org、Testee/Assessment 等 scope 从哪里解析。
3. 选择 capability，并映射到 IAM resource/action；不要新增硬编码 role 真值。
4. 在 application 做资源归属检查，repository 查询带 scope。
5. 明确 IAM/JWKS/ProfileLink/资源查询故障时的 401/403/503。
6. 为 IDOR、跨组织、失效 membership、缺失 snapshot 和依赖故障写负向测试。
7. 若是 gRPC，检查 TLS/mTLS、JWT/service token、delegated subject 与 ACL，而不是只看其中一层。
8. 更新 OpenAPI/proto contract 和公开路由白名单测试。

验证入口：

```bash
go test ./internal/pkg/middleware \
  ./internal/pkg/httpauth \
  ./internal/pkg/iamauth \
  ./internal/pkg/serviceauth \
  ./internal/pkg/securityplane \
  ./internal/pkg/securityprojection \
  ./internal/pkg/orgscope \
  ./internal/pkg/grpc

go test ./internal/apiserver/application/authz \
  ./internal/apiserver/transport/rest/middleware \
  ./internal/apiserver/transport/grpc/... \
  ./internal/collection-server/application/testeeaccess \
  ./internal/collection-server/transport/rest/middleware
```

代码测试只能证明已覆盖的策略分支；生产验收还必须验证证书链/OU、反向代理暴露面、IAM issuer/audience、JWKS 轮换、真实 ProfileLink 和 disabled/misconfigured 启动门禁。
