# collection-server

**组件定位**：**前台 BFF** 进程；**不**直连 MySQL/Mongo 主库；对外 **REST**，对内通过 **gRPC** 调用 **apiserver**；本地 **Redis** + **IAM** 支撑排队、会话类辅助与身份。  
限流与排队机制见 [03-缓存与限流](../03-基础设施/03-缓存与限流.md)；REST 契约见 [04-REST](../04-接口与运维/01-REST契约.md)。

---

## 1. 组件定位（在整体中的位置）

| 维度 | 说明 |
| ---- | ---- |
| **角色** | 小程序/收集端入口：**鉴权、限流、排队、监护** 等前置能力 |
| **上游** | 客户端 **REST** |
| **下游（兄弟组件）** | **apiserver gRPC**（强依赖） |
| **下游（数据）** | **Redis**（排队等）；**IAM SDK** |

---

## 2. 内部运行示意图

```mermaid
flowchart LR
    subgraph Col[collection-server]
        MW[全局中间件<br/>并发 / JWT / 限流]
        H[REST Handlers]
        SVC[Application 服务]
        Q[SubmitQueue]
    end

    R[(Redis)]
    API[apiserver gRPC]

    MW --> H --> SVC
    SVC --> Q
    SVC --> API
    Q --> R
    Q --> API
```

**关键点**：**SubmitQueue** 是 **进程内有界队列**（削峰），与 **MQ 跨进程异步** 不同。

---

## 3. 典型请求时序（REST → gRPC）

```mermaid
sequenceDiagram
    participant U as 客户端
    participant G as Gin 中间件链
    participant H as Handler / Service
    participant A as apiserver gRPC

    U->>G: HTTP /api/v1/...
    Note over G: 并发限制 → JWT → 路由限流
    G->>H: 进入业务
    alt 需转调主服务
        H->>A: gRPC
        A-->>H: 响应
    end
    H-->>U: JSON 响应
```

**提交答卷**入口统一走 **`SubmitQueued`**（见 [answersheet_handler.go](../../internal/collection-server/interface/restful/handler/answersheet_handler.go)），内部再根据 **是否启用队列** 分支，见下节。

---

## 4. 答卷提交：直调 gRPC vs 经 SubmitQueue

**配置**：`submit_queue.enabled` 等为 `true` 且参数合法时，[NewSubmissionService](../../internal/collection-server/application/answersheet/submission_service.go) 会创建 **`SubmitQueue`**，并把 **`submitSync`**（内部即 **AnswerSheet gRPC → apiserver**）作为队列 worker 的 **`submit` 回调**；**未启用队列**时 `queue == nil`，`SubmitQueued` **直接调用 `submitSync`**，与「同步直调」等价。

### 4.1 分支示意（flowchart）

```mermaid
flowchart TD
    H[Handler POST /answersheets]
    SQ{submit_queue<br/>已创建?}

    H --> SQ
    SQ -->|否| SYNC[submitSync<br/>直调 gRPC]
    SQ -->|是| ENQ[SubmitQueue.Enqueue]
    ENQ --> W[队列 worker goroutine]
    W --> SYNC
    SYNC --> API[apiserver AnswerSheet gRPC]
```

### 4.2 时序对比（sequenceDiagram）

```mermaid
sequenceDiagram
    participant U as 客户端
    participant H as Handler
    participant S as SubmissionService
    participant Q as SubmitQueue
    participant A as apiserver gRPC

    alt 未启用队列 queue == nil
        U->>H: POST + request_id
        H->>S: SubmitQueued
        Note over S: 内部直接 submitSync
        S->>A: gRPC SubmitAnswerSheet…
        A-->>S: 响应
        S-->>H: 结果
        H-->>U: 200 等
    else 启用队列
        U->>H: POST + request_id
        H->>S: SubmitQueued
        S->>Q: Enqueue（有界 chan + 短等待）
        alt 在 waitTimeout 内完成
            Q->>S: worker 调 submitSync
            S->>A: gRPC
            A-->>S: 响应
            Q-->>S: respCh
            S-->>H: 200 + 结果
        else 超时或已入队处理中
            H-->>U: 202 / 轮询 submit-status 等
        end
    end
```

**关键点**：队列路径下 **真正调 apiserver 的仍是 `submitSync`**，只是可能发生在 **队列 worker** 中；**满队** 返回 **429**、**短等待未结束** 可能 **202 + `request_id` 轮询**，语义以 [submit_queue.go](../../internal/collection-server/application/answersheet/submit_queue.go) 为准。

---

## 5. 核心功能与关键点

| 功能 | 关键点 | 代码锚点 |
| ---- | ------ | -------- |
| **启动** | 可选 **GOMEMLIMIT/GOGC**、**:6060 pprof** | [app.go](../../internal/collection-server/app.go)、[server.go](../../internal/collection-server/server.go) |
| **gRPC 客户端** | 五类 client 注入容器 | [grpc_client_registry.go](../../internal/collection-server/grpc_client_registry.go)、[manager.go](../../internal/collection-server/infra/grpcclient/manager.go) |
| **路由** | 公开路径 vs `/api/v1` 受保护 | [routers.go](../../internal/collection-server/routers.go) |
| **提交排队** | 200/202/429、有界 | [submit_queue.go](../../internal/collection-server/application/answersheet/submit_queue.go) |
| **监护与提交** | JWT 之外的业务校验 | [submission_service.go](../../internal/collection-server/application/answersheet/submission_service.go) |
| **身份中间件** | 与 apiserver **不是**同一套 UserIdentity 实现 | [iam_middleware.go](../../internal/collection-server/interface/restful/middleware/iam_middleware.go) |

---

## 6. 与其它组件的交互

| 对方 | 方式 | 说明 |
| ---- | ---- | ---- |
| **apiserver** | gRPC | 主读写与领域逻辑 |
| **Client** | REST | 唯一对外业务面 |
| **IAM** | SDK | 验签、监护查询等 |
| **Redis** | TCP | 排队与辅助状态 |

---

## 7. 关键代码入口（索引）

| 关注点 | 路径 |
| ------ | ---- |
| 进程入口 | [cmd/collection-server/main.go](../../cmd/collection-server/main.go) |
| 配置 | [options/options.go](../../internal/collection-server/options/options.go) |

---

## 8. 边界与注意事项

- **无** MySQL/Mongo 主库连接；持久化均在 **apiserver**。  
- **匿名只读**仅路由白名单（如部分 scales GET）。  
- **gRPC 不可用**时 REST 可能仍“活着”但业务失败，需看健康检查与下游状态。

---

*说明：写作习惯可对照 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md)；本篇按「运行时组件」体裁组织。*
