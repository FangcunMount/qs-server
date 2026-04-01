# IAM 认证与身份链路（运行时视角）

**本文回答**：这篇文档解释 IAM 能力在 `qs-server` 运行时如何接入三进程、HTTP JWT 和 gRPC 入站各走什么链路、`collection-server` 的监护关系校验和服务间 token 如何工作，以及排障时该从哪些中间件、拦截器和 SDK 入口入手；本文先给结论和速查，再展开三进程视角与时序图。

## 30 秒结论

如果只看一屏，先看下面这张表：

| 维度 | 结论 |
| ---- | ---- |
| IAM 在系统中的位置 | IAM 不是第四个进程，而是以 SDK / 中间件 / 拦截器形式嵌入 `collection-server` 和 `qs-apiserver` |
| HTTP 认证主路径 | 两个 HTTP 进程都通过 JWT 中间件验用户 token，再把身份写入业务上下文 |
| gRPC 入站主路径 | `qs-apiserver` 的 gRPC 先经过 TLS / mTLS，再进入 Unary 拦截器链，可选读取 IAM JWT metadata |
| `collection -> apiserver` 特点 | 除传输层 mTLS 外，还可能通过 `ServiceAuthHelper` 注入服务间 token；这和前台用户 Bearer 不是一回事 |
| worker 边界 | `qs-worker` 不嵌入 IAM 用户态链路，它只受 `qs-apiserver` gRPC 服务端的认证策略约束 |
| 排障入口 | 先区分 HTTP 还是 gRPC，再看对应中间件 / 拦截器、IAM SDK 调用和配置开关 |

## 重点速查（继续往下读前先记这几条）

1. **IAM 是嵌入式能力，不是独立进程**：它通过 SDK、中间件和拦截器进入两个服务进程。  
2. **用户态 token 和服务态 token 不同**：前台 Bearer JWT、服务间 token、mTLS 客户端证书是三条不同认证材料。  
3. **gRPC 要先分清传输层和应用层**：mTLS 在握手阶段完成，IAMAuthInterceptor 是后续应用层逻辑。  
4. **worker 不走用户态 IAM 链**：排查 worker 回调失败时，不要按前台 JWT 的思路去看。  

**组件定位**：**IAM 不是**本仓第四进程；以 **SDK 模块**形式嵌入 **collection-server** 与 **qs-apiserver**，参与 **HTTP JWT**、**gRPC 可选 JWT**、**监护关系**等。**worker** 不嵌入 IAM 用户态链路。  
配置键、拦截器链、与 worker 的 gRPC 元数据关系见 [03-基础设施/04-IAM与认证.md](../03-基础设施/04-IAM与认证.md)。

---

## IAM 在三进程里分别承担什么

| 进程 | IAM 相关职责（运行时） |
| ---- | ---------------------- |
| **collection-server** | 前台 REST：**用户 JWT** → **UserIdentity**；**Guardianship** 等；调 apiserver gRPC 时若启用 **`iam.service_auth`** 可装配 **服务间 token**（见 **§3.4**） |
| **qs-apiserver** | 后台 REST：同上（**上下文字段更全**）；**gRPC**：拦截器链见 **§3.3**；容器内 **ServiceAuthHelper** 等多用于**向其它服务发请求**，与 **collection → 本进程** 的入站验签是两条线 |
| **qs-worker** | **无** IAM 模块；依赖 **apiserver gRPC** 是否要求 `authorization`（见 03-04） |

---

## 运行时里它们怎样接起来

```mermaid
flowchart LR
    client[Client]

    subgraph col[collection-server]
        JWT1[JWTAuthMiddleware]
        U1[UserIdentityMiddleware]
        G[Guardianship / 应用层]
    end

    subgraph api[qs-apiserver]
        JWT2[JWTAuthMiddleware]
        U2[UserIdentityMiddleware]
        GIAM[gRPC IAMAuthInterceptor]
    end

    subgraph w[worker]
        n[无用户 JWT]
    end

    client -->|REST| JWT1 --> U1 --> G
    client -->|REST| JWT2 --> U2
    w -->|gRPC| GIAM
```

**collection → apiserver（gRPC）**：除 **mTLS 传输**外，若部署启用 **服务间 JWT**，由 **`ServiceAuthHelper`（PerRPC）** 写入 metadata（与前台用户 Bearer **不是**同一条 token）；详见 **§3.4**。

---

## HTTP、gRPC 和服务间调用分别怎么走

### HTTP：JWT 与身份上下文（两进程共性）

```mermaid
sequenceDiagram
    participant U as Client
    participant Gin as Gin 中间件链
    participant JWT as JWTAuthMiddleware
    participant TV as TokenVerifier SDK
    participant UI as UserIdentityMiddleware

    U->>Gin: Authorization: Bearer …
    Gin->>JWT: 提取 token
    JWT->>TV: Verify（JWKS 本地优先）
    TV-->>JWT: claims
    JWT->>Gin: 注入 UserClaims
    Gin->>UI: 解析 user_id / tenant / roles
    UI->>Gin: 写入业务 context
```

**差异**：collection 与 apiserver 的 **UserIdentityMiddleware 实现不同**，字段集合不一致，**勿混读 context**。

### collection：监护关系（业务层，非纯中间件）

```mermaid
sequenceDiagram
    participant S as Submission 等应用服务
    participant I as Identity/受试者数据
    participant G as GuardianshipService

    S->>I: 查受试者
    alt 需校验监护
        S->>G: IsGuardian
        G-->>S: 是/否
    end
```

**锚点**：[submission_service.go](../../internal/collection-server/application/answersheet/submission_service.go)、[guardianship.go](../../internal/collection-server/infra/iam/guardianship.go)。

### apiserver：gRPC 入站，传输层与 Unary 链顺序（mTLS 先于 IAM JWT）

**易混点**：**mTLS** 先在 **TLS 握手** 完成客户端认证；**IAMAuthInterceptor** 再在 **应用层** 读 metadata 里的 **JWT**（可与 **用户态** 或 **服务态** token 对应）。服务端 **Unary 链**（与代码一致）为：**Recovery → RequestID → Logging →（可选）MTLSInterceptor →（可选）IAMAuth →（可选）授权快照（`Config.ExtraUnaryAfterAuth`，仅 **qs-apiserver** 装配 `AuthzSnapshotLoader` 时）→（可选）ACL →（可选）Audit → Handler**。其中 **TLS/mTLS 在连接建立时完成**；**MTLSInterceptor 在 IAMAuth 之前**；**授权快照在 IAMAuth 之后、ACL 之前**（见 [internal/pkg/grpc/server.go `buildUnaryInterceptors`](../../internal/pkg/grpc/server.go)、[apiserver `grpc_authz_snapshot_interceptor.go`](../../internal/apiserver/grpc_authz_snapshot_interceptor.go)）。

```mermaid
flowchart LR
    subgraph transport[连接]
        TLS[TLS/mTLS 证书校验]
    end

    subgraph chain[Unary 拦截器 由外到内]
        R[Recovery]
        RID[RequestID]
        L[Logging]
        MI[MTLSInterceptor<br/>若 mtls.enabled]
        IA[IAMAuthInterceptor<br/>若 auth.enabled]
        AS[授权快照<br/>ExtraUnaryAfterAuth<br/>若 loader 可用]
        AC[ACL…<br/>若 acl.enabled]
        AU[Audit…<br/>若 audit.enabled]
        H[Handler]
    end

    TLS --> R --> RID --> L --> MI --> IA --> AS --> AC --> AU --> H
```

- **RequireIdentityMatch**：在 **IAMAuth** 内把 **JWT 中的服务身份** 与 **MTLSInterceptor 写入 context 的证书身份** 对齐（见 [interceptor_auth.go `verifyIdentityMatch`](../../internal/pkg/grpc/interceptor_auth.go)）。  
- **授权快照**：与 HTTP [`AuthzSnapshotMiddleware`](../../internal/apiserver/interface/restful/middleware/authz_snapshot_middleware.go) 对齐，写入 **`authz` 快照**与 **`GrantingUserID`**；无 `tenant_id`/`user_id` 的调用不拉快照；**Health / Reflection** 等与 IAM 白名单对齐的路径**跳过**（见 [03-04](../03-基础设施/04-IAM与认证.md)）。  
- **默认跳过 IAM 鉴权**：Health / Reflection（见 [03-04](../03-基础设施/04-IAM与认证.md)）。

### collection → apiserver：服务间 token（ServiceAuthHelper）

| 维度 | 说明 |
| ---- | ---- |
| **用途** | 标识 **collection-server** 服务身份，向 **apiserver gRPC** 附加 **`authorization`（服务 JWT）**，与 **终端用户 JWT**（REST Bearer）区分 |
| **装配** | [collection `iam_module.go`](../../internal/collection-server/container/iam_module.go) 在 **`iam.service_auth.*`**（含 `ServiceID`、`TargetAudience` 等）合法时创建 **`ServiceAuthHelper`** |
| **实现** | [infra/iam/service_auth.go](../../internal/collection-server/infra/iam/service_auth.go) 实现 **`credentials.PerRPCCredentials`**（`GetRequestMetadata`），并提供 **`DialWithServiceAuth`**（`grpc.WithPerRPCCredentials`） |
| **与默认 gRPC Manager** | [PrepareRun 先 IAM 后 gRPC](../../internal/collection-server/server.go)；若 **`ServiceAuthHelper` 非空**，[`Manager.connect`](../../internal/collection-server/infra/grpcclient/manager.go) 会 **`grpc.WithPerRPCCredentials`**；与 **`DialWithServiceAuth`** 同属 PerRPC 模式（独立拨号场景仍可用后者） |

**与 apiserver 入站配套**（mTLS + 服务 JWT + 可选身份对齐）：见 [03-基础设施/04-IAM与认证.md](../03-基础设施/04-IAM与认证.md) 中 **「用户态与服务态」「gRPC 与可选 mTLS」「internal gRPC」** 等节；**配置键** 见同文 **`iam.service_auth.*`**。

---

## 排障时先看什么

### 核心功能与关键点

| 能力 | 关键点 | 锚点 |
| ---- | ------ | ---- |
| **共享 JWT 中间件** | 多来源取 token | [jwt_auth.go](../../internal/pkg/middleware/jwt_auth.go) |
| **collection IAM 模块** | Verifier + Guardianship 等 | [iam_module.go](../../internal/collection-server/container/iam_module.go) |
| **apiserver IAM 模块** | 另含后台所需服务 | [iam_module.go](../../internal/apiserver/container/iam_module.go) |
| **gRPC 拦截器** | 链顺序见 **§3.3**（含 IAMAuth 后可选授权快照）；Health/Reflection 默认跳过 IAM | [server.go](../../internal/pkg/grpc/server.go)、[interceptor_auth.go](../../internal/pkg/grpc/interceptor_auth.go)、[grpc_authz_snapshot_interceptor.go](../../internal/apiserver/grpc_authz_snapshot_interceptor.go) |
| **服务间 gRPC 认证（collection）** | `ServiceAuthHelper` / `PerRPC` | [iam_module.go](../../internal/collection-server/container/iam_module.go)、[service_auth.go](../../internal/collection-server/infra/iam/service_auth.go) |

---

## 它和其它组件怎么交互

### 与其它组件的交互

| 组件 | 与 IAM 的关系 |
| ---- | ------------- |
| **collection** | REST：**用户 JWT**；gRPC 下游：可选 **服务 JWT**（**§3.4**）+ **mTLS**；应用层 **Guardianship** 等 |
| **apiserver** | REST + **gRPC 入站拦截链（§3.3）** + 各业务 infra 调 Identity/Guardianship |
| **worker** | 间接：仅通过 **apiserver** 是否再调 IAM；通常 **无用户 JWT metadata**（见 [03-04](../03-基础设施/04-IAM与认证.md)） |

---

## 这篇和 03-基础设施/04 怎么分工

| 维度 | 本文（01-运行时/05） | [03-基础设施/04](../03-基础设施/04-IAM与认证.md) |
| ---- | -------------------- | ----------------------------------------------- |
| 侧重 | **哪一进程**承担哪段链路、时序（含 **§3.3 链图**、**§3.4 ServiceAuth**） | **装配、配置键、`grpc.*`、拦截器与 worker 元数据、与 01-运行时/05 对照表** |

---

## 边界与注意事项

- **IAM 不要画成与三进程并列的第四个运行时方块**（除非指外部 IAM 服务拓扑）。  
- **Claims ≠ 领域不变量**，领域侧见 [actor](../02-业务模块/05-actor.md)。  
- **worker** 不表示「每步都验用户 JWT」。

---

*说明：写作习惯可对照 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md)；本篇按「运行时组件」体裁组织。*
