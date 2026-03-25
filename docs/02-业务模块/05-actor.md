# actor

本文档按 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md) 中的**业务模块推荐结构**撰写；写作时需覆盖的动机、命名、实现位置与可核对性，见该文「讲解维度」一节，本文正文不重复贴标签。

---

## 30 秒了解系统

### 概览

`actor` 是 `qs-apiserver` 里的**主体（Actor）模块**，在问卷/测评限界上下文内回答两件事：**谁是被测的人**（`Testee`）、**谁是机构侧操作者**（`Staff`）。它把 **IAM** 的用户/儿童档案与业务侧 **`testeeID` / `staffID`** 对齐，并承载标签、重点关注、机构内角色等**本 BC 状态**；**不是**统一登录与全局权限中心。

代码主路径：`internal/apiserver/domain/actor`（`testee`、`staff`、引用值对象）、`internal/apiserver/application/actor`；持久化当前主要在 **MySQL**（见「核心存储」）。受试者读路径可经 **Redis** 装饰仓储（见 [assembler/actor.go](../../internal/apiserver/container/assembler/actor.go)）。与 IAM 的交互经 [infra/iam](../../internal/apiserver/infra/iam)（`GuardianshipService`、`IdentityService` 等）。

### 模块边界

| | 内容 |
| -- | ---- |
| **负责（摘要）** | 受试者/员工档案与查询；与 IAM 的绑定与校验；`testeeID`/`staffID` 及 `TesteeRef`/`StaffRef`；后台 REST + C 端 gRPC（testee）；internal gRPC `TagTestee` |
| **不负责（摘要）** | 登录与系统级授权（IAM）；问卷与答卷（[survey](./01-survey.md)）；测评与报告（[evaluation](./03-evaluation.md)）；机构聚合全生命周期；`collection-server` BFF 策略 |
| **关联专题** | 三界与主体引用 [05-专题/01](../05-专题分析/01-测评业务模型：survey、scale、evaluation%20为什么分离.md)；异步链路 [05-专题/02](../05-专题分析/02-异步评估链路：从答卷提交到报告生成.md)；读侧 [05-专题/03](../05-专题分析/03-保护层与读侧架构：限流、背压、缓存、统计预聚合.md) |

#### 负责什么（细项）

维护文档时**以本清单为模块职责真值**之一，与代码不一致时应改代码或改文。

- **受试者 `Testee`**：注册/幂等确保存在、基本信息与档案绑定、标签与重点关注、测评相关统计字段（见 [testee.go](../../internal/apiserver/domain/actor/testee/testee.go)）。
- **员工 `Staff`**：机构内注册、与 IAM 用户绑定、角色分配、激活/停用（见 [staff.go](../../internal/apiserver/domain/actor/staff/staff.go)）。
- **跨模块引用**：`TesteeRef` / `StaffRef`（[ref.go](../../internal/apiserver/domain/actor/ref.go)）；`FillerRef` 建模「谁填写」与「谁被测」分离（[filler_ref.go](../../internal/apiserver/domain/actor/filler_ref.go)，接入程度见「核心模式」）。
- **对外协议**：后台 [REST](#核心契约restgrpcinternal-grpc-与领域事件)；C 端 [ActorService gRPC](#核心契约restgrpcinternal-grpc-与领域事件)；worker 等经 **internal** `TagTestee` 打标。

#### 不负责什么（细项）

- **JWT / 会话**：由中间件与 Handler 上下文提供 `user_id` / `org_id` 等，**不**作为 `actor` 聚合内持久化状态（见「核心模式」）。
- **领域事件总线**：当前 **无** 与 `survey`/`plan` 同级的稳定 `actor.*` MQ 事件落地（代码中多为注释占位，见「核心契约」）。

### 契约入口

- **REST**：`/api/v1/testees`、`/api/v1/staff` 等以 [api/rest/apiserver.yaml](../../api/rest/apiserver.yaml) 为准；Handler [actor.go](../../internal/apiserver/interface/restful/handler/actor.go)、路由 [routers.go](../../internal/apiserver/routers.go)。
- **C 端 / BFF gRPC**：`CreateTestee`、`GetTestee`、`ListTesteesByOrg` 等见 [actor.proto](../../internal/apiserver/interface/grpc/proto/actor/actor.proto)、[actor_service.go](../../internal/apiserver/interface/grpc/service/actor_service.go)。
- **internal gRPC**：`TagTestee` 见 [internal.proto](../../internal/apiserver/interface/grpc/proto/internalapi/internal.proto)、[internal.go](../../internal/apiserver/interface/grpc/service/internal.go)。
- **领域事件**：**N/A（当前无纳入 `configs/events.yaml` 的 actor 领域事件）**；与代码注释占位对照见「核心契约」。

### 运行时示意图

```mermaid
flowchart LR
    admin[后台管理端]
    collection[collection-server]
    iam[IAM]
    worker[qs-worker]

    subgraph apiserver[qs-apiserver]
        actor[actor]
        survey[survey]
        evaluation[evaluation]
        plan[plan]
    end

    admin -->|REST testee+staff| actor
    collection -->|gRPC ActorService| actor
    actor -->|Guardianship / Identity| iam
    survey -->|testeeID 引用| actor
    evaluation -->|testeeID / staff 查询| actor
    plan -->|testeeID| actor
    worker -->|internal TagTestee| actor
```

#### 运行时图说明

后台 REST 管理受试者与员工；小程序经 `collection-server` 只打 **testee** gRPC；`plan`/`survey`/`evaluation` 引用 `testeeID`；异步链路完成后 **worker** 可经 internal gRPC 调 `TagTestee` 更新标签与重点关注。

### 主要代码入口（索引）

| 关注点 | 路径 |
| ------ | ---- |
| 装配 | [internal/apiserver/container/assembler/actor.go](../../internal/apiserver/container/assembler/actor.go) |
| 领域 | [internal/apiserver/domain/actor/](../../internal/apiserver/domain/actor/) |
| 应用服务 | [internal/apiserver/application/actor/](../../internal/apiserver/application/actor/) |
| 持久化 | [internal/apiserver/infra/mysql/actor/](../../internal/apiserver/infra/mysql/actor/) |
| IAM 适配 | [internal/apiserver/infra/iam/](../../internal/apiserver/infra/iam/) |

---

## 模型与服务

与 [survey](./01-survey.md)、[plan](./04-plan.md) 一致，本节用 **ER 图**表达租户与 IAM 外键语义，再用 **概念 flowchart** 与 **分层图**对齐 interface → application → domain。

### 模型 ER 图

描述 `actor` 子域内实体与 **租户边界**、**IAM 外部系统** 的关系（**非**与 MySQL 表字段 1:1）。本模块**不**实现 `Organization` 聚合；`org_id` 仅作多租户外键。`Testee` 与 `Staff` **无**彼此直接外键，只可能 **同属** `org_id`。

```mermaid
erDiagram
    ORG_SCOPE {
        int64 org_id PK
    }

    TESTEE {
        uint64 id PK
        int64 org_id FK
        uint64 profile_id "可选"
    }

    STAFF {
        uint64 id PK
        int64 org_id FK
        int64 iam_user_id
    }

    IAM_CHILD {
        uint64 id PK
    }

    IAM_USER {
        int64 id PK
    }

    ORG_SCOPE ||--o{ TESTEE : org_id
    ORG_SCOPE ||--o{ STAFF : org_id
    TESTEE }o--o| IAM_CHILD : profile_id
    STAFF }o--|| IAM_USER : user_id
```

- **ORG_SCOPE**：仅表达「机构 ID」这一租户维度；权威机构模型不在 `actor`。
- **IAM_***：落在 IAM 限界上下文；图中为 **逻辑外键**，非 `actor` 表内子表。
- **Testee.profile_id**：可选，当前语义对应 IAM 儿童档案（见 [testee.go](../../internal/apiserver/domain/actor/testee/testee.go)）；**Staff.user_id** 绑定 IAM 用户（见 [staff.go](../../internal/apiserver/domain/actor/staff/staff.go)）。

### 模型关系（概念）

`actor` **不是**单一聚合模块，而是 **两条子线**（`testee` / `staff`）加 **引用值对象**。

```mermaid
flowchart TB
    subgraph TE[Testee 线]
        T[Testee]
        TID[ID / OrgID / ProfileID]
        TBIZ[Tags / IsKeyFocus / AssessmentStats]
    end

    subgraph ST[Staff 线]
        S[Staff]
        SID[StaffID / OrgID / UserID]
        SROLE[Roles / IsActive]
    end

    subgraph REF[引用]
        TR[TesteeRef]
        SR[StaffRef]
        FR[FillerRef]
    end

    T --> TID
    T --> TBIZ
    S --> SID
    S --> SROLE
    TR -.-> T
    SR -.-> S
    FR -.->|填写人 vs 被测人| T
```

- 与上图对照：`flowchart` 强调 **子域内职责块**；**租户与 IAM** 以 **ER 图**为准。

### 领域模型与领域服务

#### 限界上下文

- **解决**：本 BC 内「被测主体」与「机构侧操作者」的持久化视图、与 IAM 的绑定、testee 侧标签/重点关注。
- **不解决**：统一身份认证与全局 RBAC；问卷内容与测评流水线。

#### 核心概念

| 概念 | 职责 | 与相邻概念的关系 |
| ---- | ---- | ---------------- |
| `Testee` | 聚合根：机构内受试者，可绑定 IAM `profileID`（儿童档案） | 被 `plan`、`AnswerSheet` 等引用 `testeeID` |
| `Staff` | 聚合根：机构内员工，绑定 IAM `userID`，含角色与激活状态 | 后台操作与授权查询的主体之一 |
| `TesteeRef` / `StaffRef` | 值对象：跨聚合引用 ID（及可选展示字段） | [ref.go](../../internal/apiserver/domain/actor/ref.go) |
| `FillerRef` | 值对象：区分「谁填写」与「谁被测」 | 设计已完成，**全面接入 AnswerSheet 仍待演进**（见文件头注释） |

#### 主要领域服务（按包）

| 子域 | 类型 | 职责摘要 | 锚点 |
| ---- | ---- | -------- | ---- |
| testee | `Factory` / `Editor` / `Binder` / `Validator` / `Tagger` | 创建、编辑、档案绑定、校验、打标 | [testee/](../../internal/apiserver/domain/actor/testee/) |
| staff | `Factory` / `Editor` / `Binder` / `Validator` / `RoleAllocator` / `Lifecycler` | 注册、绑定用户、角色、生命周期 | [staff/](../../internal/apiserver/domain/actor/staff/) |

### 应用服务、领域服务与领域模型

| 应用服务 | 行为者 / 用途 | 目录锚点 |
| -------- | ------------- | -------- |
| `TesteeRegistrationService` | C 端：注册、Ensure、与 IAM 儿童档案校验 | `application/actor/testee/registration_service.go` |
| `TesteeManagementService` | 后台：信息维护、绑定、标签与重点关注 | `application/actor/testee/management_service.go` |
| `TesteeQueryService` | 通用查询 | `application/actor/testee/query_service.go` |
| `TesteeBackendQueryService` | 后台：本地 Testee + IAM 监护人等补充 | `application/actor/testee/backend_query_service.go` |
| `TesteeTaggingService` | 系统：`TagTestee` 内部实现 | `application/actor/testee/tagging_service.go` |
| `StaffLifecycleService` | 员工注册与绑定、同步联系方式 | `application/actor/staff/lifecycle_service.go` |
| `StaffAuthorizationService` | 角色分配、激活/停用 | `application/actor/staff/authorization_service.go` |
| `StaffQueryService` | 员工查询 | `application/actor/staff/query_service.go` |

```mermaid
flowchart TB
    subgraph IF[interface]
        REST[REST ActorHandler]
        GRPC[ActorService gRPC]
        INT[InternalService.TagTestee]
    end

    subgraph APP[application · testee]
        REG[RegistrationService]
        MGT[ManagementService]
        QRY[QueryService]
        BQ[BackendQueryService]
        TAG[TaggingService]
    end

    subgraph APS[application · staff]
        LSV[StaffLifecycleService]
        AUTHZ[StaffAuthorizationService]
        SQ[StaffQueryService]
    end

    subgraph DOM[domain]
        T[Testee + domain services]
        ST[Staff + domain services]
    end

    subgraph IAM[infra · iam]
        GS[GuardianshipService]
        ID[IdentityService]
    end

    REST --> REG
    REST --> MGT
    REST --> QRY
    REST --> BQ
    REST --> LSV
    REST --> AUTHZ
    REST --> SQ
    GRPC --> REG
    GRPC --> QRY
    INT --> TAG

    REG --> T
    REG --> GS
    MGT --> T
    BQ --> QRY
    BQ --> GS
    BQ --> ID
    TAG --> MGT
    TAG --> T

    LSV --> ST
    LSV --> ID
    AUTHZ --> ST
    SQ --> ST
```

#### 分层图说明

- **装配注入**：`ActorModule.Initialize` 接收 `*gorm.DB`、可选 `GuardianshipService`、`IdentityService`、可选 **Redis**（装饰 `TesteeRepo`），见 [assembler/actor.go](../../internal/apiserver/container/assembler/actor.go)。
- **评估跨域查询**：`ActorHandler` 可通过 `SetEvaluationServices` 延迟注入测评查询（用于如量表分析类 REST），见同文件。

---

## 核心设计

### 核心契约：REST、gRPC、internal gRPC 与领域事件

#### 输入

- **后台 REST**：`/api/v1/testees`、`/api/v1/testees/by-profile-id`、`/api/v1/testees/{id}`、`/api/v1/testees/{id}/scale-analysis` 等；`/api/v1/staff` 等——以 [apiserver.yaml](../../api/rest/apiserver.yaml) 为准。
- **C 端 gRPC（仅 testee）**：`CreateTestee`、`GetTestee`、`UpdateTestee`、`TesteeExists`、`ListTesteesByOrg`、`ListTesteesByUser` 等 — [actor.proto](../../internal/apiserver/interface/grpc/proto/actor/actor.proto)。
- **internal gRPC**：`TagTestee(TagTesteeRequest{ testee_id, risk_level, scale_code, mark_key_focus, high_risk_factors })` — [internal.proto](../../internal/apiserver/interface/grpc/proto/internalapi/internal.proto)。

**与 OpenAPI / 路由对读（Verify）**：

以下为 **机器可读契约**（以 [api/rest/apiserver.yaml](../../api/rest/apiserver.yaml) 为准）；路由挂载以 [routers.go](../../internal/apiserver/routers.go)（`registerActorProtectedRoutes` 等）为准，二者不一致时应以代码为准并同步 yaml。

| HTTP | 路径（前缀 `/api/v1`） | 摘要 | 备注 |
| ---- | ---------------------- | ---- | ---- |
| `GET` | `/testees` | 受试者列表 | Actor |
| `GET` | `/testees/by-profile-id` | 按 profile 查受试者 | Actor |
| `GET` | `/testees/{id}` | 受试者详情 | Actor |
| `PUT` | `/testees/{id}` | 更新受试者 | Actor |
| `GET` | `/testees/{id}/scale-analysis` | 量表分析（聚合 evaluation 读模型） | 见「核心模式」§8 |
| `GET` | `/testees/{id}/periodic-stats` | 周期统计 | yaml / Handler 有；**当前 `routers.go` 未见挂载**——以实际路由为准，未注册则接口不可用 |
| `GET` | `/testees/{id}/plans` | 受试者参与计划列表 | OpenAPI 标签 Plan-Query，与 [plan](./04-plan.md) 查询衔接 |
| `GET` | `/testees/{id}/plans/{plan_id}/tasks` | 计划下任务列表 | 同上 |
| `GET` | `/testees/{id}/tasks` | 受试者任务列表 | 同上 |
| `POST` | `/staff` | 创建员工 | Actor |
| `GET` | `/staff` | 员工列表 | Actor |
| `GET` | `/staff/{id}` | 员工详情 | Actor |
| `DELETE` | `/staff/{id}` | 删除员工 | Actor |

**gRPC（Verify）**：

| 服务 | RPC | 用途 |
| ---- | ----- | ---- |
| `actor.ActorService` | `CreateTestee`、`GetTestee`、`UpdateTestee`、`TesteeExists`、`ListTesteesByOrg`、`ListTesteesByUser` | C 端 / BFF，见 [actor.proto](../../internal/apiserver/interface/grpc/proto/actor/actor.proto) |
| `internalapi.InternalService` | `TagTestee` | worker 等内部打标，见 [internal.proto](../../internal/apiserver/interface/grpc/proto/internalapi/internal.proto) |

#### 输出（领域事件）

**当前状态**：[`configs/events.yaml`](../../configs/events.yaml) **无** `actor.*` / `testee.*` / `staff.*` 条目；领域代码中 **打标、员工激活** 等处存在 **注释掉的 `events.Publish(...)` 占位**（如 [testee/counter.go](../../internal/apiserver/domain/actor/testee/counter.go)、[staff/lifecycler.go](../../internal/apiserver/domain/actor/staff/lifecycler.go)）。

**Verify**：若未来引入 MQ 事件，须新增 yaml、领域事件类型与发布点，并更新本文。

**当前真实输出形态**：REST/gRPC 直接返回实体或 DTO；`TagTestee` 返回操作结果；其他模块持有 **`testeeID` / `staffID` 引用**。

### 核心链路：受试者注册、后台查询与打标

#### C 端注册与幂等（gRPC）

```mermaid
sequenceDiagram
    participant Client as 小程序
    participant Col as collection-server
    participant GRPC as ActorService
    participant Reg as RegistrationService
    participant IAM as GuardianshipService
    participant Repo as Testee Repo

    Client->>Col: 创建/查询受试者
    Col->>GRPC: gRPC
    GRPC->>Reg: Register / Ensure
    Reg->>IAM: ValidateChildExists 等
    Reg->>Repo: FindByProfile / Save
    GRPC-->>Col: 响应
```

BFF 侧编排示例：[collection-server/application/testee/service.go](../../internal/collection-server/application/testee/service.go)。

#### 后台受试者详情（含监护人）

`TesteeBackendQueryService` 在 [backend_query_service.go](../../internal/apiserver/application/actor/testee/backend_query_service.go) 中组合 **本地 Testee** 与 IAM **监护人/身份** 信息；IAM 失败时策略见「边界与注意事项」。

#### 测评后打标（internal）

`InternalService.TagTestee` → `TesteeTaggingService`：按风险等级等更新标签、可选重点关注（实现见 [tagging_service.go](../../internal/apiserver/application/actor/testee/tagging_service.go)、[internal.go](../../internal/apiserver/interface/grpc/service/internal.go)）。

### 核心横切：actor、IAM、collection-server 与业务模块

| 侧 | 职责 | 与 `actor` 的衔接 |
| ---- | ---- | ---------------- |
| **IAM** | 用户/儿童档案、监护关系 | 注册时校验档案存在；后台查询补监护人 |
| **collection-server** | BFF、鉴权 | 仅转发 **testee** gRPC，不暴露 `staff` gRPC |
| **survey / plan / evaluation** | 业务事实与编排 | 引用 `testeeID`（及 staff 查询场景） |

**结论**：`actor` 是 **BC 内主体投影**；**认证授权**仍以 IAM + 中间件为准。

### 核心集成：actor 与 IAM（分场景）

仓库内 IAM 通过 **适配层** [infra/iam](../../internal/apiserver/infra/iam) 封装 IAM SDK（`GuardianshipService`、`IdentityService` 等），**不是**在领域模型里直接依赖 gRPC 协议细节。

| 场景 | actor 入口 | IAM 侧做什么 | 典型行为（与代码一致） |
| ---- | ---------- | ------------- | ------------------------ |
| **受试者注册 / 绑定档案** | [RegistrationService](../../internal/apiserver/application/actor/testee/registration_service.go) | `GuardianshipService.ValidateChildExists` | 当存在 `profileID`（儿童档案）且监护服务 **已启用** 时，先校验 IAM 中儿童存在；未启用则跳过该校验路径 |
| **后台受试者详情（监护人等）** | [BackendQueryService](../../internal/apiserver/application/actor/testee/backend_query_service.go) | `GuardianshipService.ListGuardians` + `IdentityService` 补全用户资料 | 本地 `Testee` 为主；有 `profileID` 且服务启用时拉监护人与展示信息；IAM 不可用或关闭时仍可返回本地数据，监护人字段可能为空（见「边界与注意事项」） |
| **员工注册与绑定** | [StaffLifecycleService](../../internal/apiserver/application/actor/staff/lifecycle_service.go) | `IdentityService.SearchUsers` / `CreateUser` 等 | 在 IAM 侧检索或创建用户，再与 `Staff` 绑定；服务未启用时走代码内分支（见该文件） |

**Staff ↔ IAM 方法锚点（补充）**（均在同文件 [lifecycle_service.go](../../internal/apiserver/application/actor/staff/lifecycle_service.go) 的 `resolveOrCreateUser` 内）：

- **`IdentityService.SearchUsers`**：`SearchUsers(ctx, &identityv1.SearchUsersRequest{ Phones: []string{dto.Phone} })`，用于注册员工时按**手机号**在 IAM 查是否已有用户（约 186–199 行）。
- **`IdentityService.CreateUser`**：`CreateUser(ctx, dto.Name, dto.Email, dto.Phone)`，搜索未命中时**创建** IAM 用户并返回 `userID`（约 201–203 行）。

**装配与注入**：`ActorModule.Initialize` 将 `GuardianshipService`、`IdentityService` 从容器注入（参数顺序见 [assembler/actor.go](../../internal/apiserver/container/assembler/actor.go)）；未注入或 `IsEnabled()==false` 时，行为以各应用服务内的 **nil / 降级分支** 为准。

**Verify**：改 IAM 契约或 SDK 时，须同步 `infra/iam`、上述应用服务与本文表。

### 核心模式与实现要点

#### 1. Testee 是业务主体，不是 IAM.Child 的别名

本地持久化 `Testee`，可绑定 `profileID`；标签、重点关注、统计字段属于 **本模块**。其他聚合引用 **`testeeID`**，降低对 IAM 主键的直接耦合。

#### 2. Staff 是 IAM.User 在机构内的业务投影

同一 IAM 用户可在不同 `orgID` 下对应不同 `Staff` 与角色（见 [staff/types.go](../../internal/apiserver/domain/actor/staff/types.go)）；认证信息不在此重复造轮子。

#### 3. gRPC 只暴露 testee，是有意收敛

`ActorService` **无** staff 方法；员工管理走 **后台 REST**。避免把 BFF 扩成「全用户中心」。

#### 4. 请求用户身份在接口层，不进 actor 聚合

JWT / claims 由 [iam_middleware](../../internal/apiserver/interface/restful/middleware/iam_middleware.go)、[handler/base](../../internal/apiserver/interface/restful/handler/base.go) 等解析；领域对象不持久化「当前会话」。

#### 5. Testee 创建入口统一，接入场景不同

- **前台路径**：`collection-server` → gRPC → `RegistrationService`。
- **计划路径**：[plan](./04-plan.md) 入组引用已有 `testeeID`，**不**由 plan 代建 Testee（需前置存在）。
- **筛查等**：以代码与产品为准；勿假设未在仓库落地的模块会自动建 Testee。

#### 6. TesteeRef / StaffRef / FillerRef

引用值对象减轻跨聚合依赖；`FillerRef` 表达「填写人 ≠ 被测人」，**主链路全面接入仍以代码为准**。

#### 7. IAM 集成：绑定 + 补充

详见上文 **「核心集成：actor 与 IAM（分场景）」**；适配类见 [guardianship.go](../../internal/apiserver/infra/iam/guardianship.go)、[identity.go](../../internal/apiserver/infra/iam/identity.go)。原则：**本地 `testeeID`/`staffID` 仍为业务主键**，IAM 用于档案存在性、监护关系与用户资料补全。

#### 8. REST 上的「量表分析」类接口

如 `GetScaleAnalysis` 在 Handler 层聚合 **evaluation** 读数据，语义是「以 testee 为维度的查询」，**不是** actor 域原生生成的核心业务对象（见 [actor.go](../../internal/apiserver/interface/restful/handler/actor.go) 与装配注入）。

### 核心存储：MySQL 与可选 Redis

| 数据 | 存储 | 实现锚点 |
| ---- | ---- | -------- |
| `Testee`、`Staff` | MySQL | [infra/mysql/actor](../../internal/apiserver/infra/mysql/actor) |
| Testee 读路径缓存（可选） | Redis 装饰仓储 | [testee_cache.go](../../internal/apiserver/infra/cache/testee_cache.go)（`NewCachedTesteeRepository`，装配见 [assembler/actor.go](../../internal/apiserver/container/assembler/actor.go)） |

### 核心代码锚点索引

| 关注点 | 路径 |
| ------ | ---- |
| 装配 | [internal/apiserver/container/assembler/actor.go](../../internal/apiserver/container/assembler/actor.go) |
| 应用服务 | [internal/apiserver/application/actor/](../../internal/apiserver/application/actor/) |
| 领域 | [internal/apiserver/domain/actor/](../../internal/apiserver/domain/actor/) |
| REST | [internal/apiserver/interface/restful/handler/actor.go](../../internal/apiserver/interface/restful/handler/actor.go) |
| gRPC | [internal/apiserver/interface/grpc/service/actor_service.go](../../internal/apiserver/interface/grpc/service/actor_service.go) |
| internal gRPC | [internal/apiserver/interface/grpc/service/internal.go](../../internal/apiserver/interface/grpc/service/internal.go)（`TagTestee`） |
| MySQL | [internal/apiserver/infra/mysql/actor/](../../internal/apiserver/infra/mysql/actor/) |

---

## 边界与注意事项

### 常见误解

- **`actor` ≠ IAM**：登录、全局权限仍以 IAM 为准；本模块是 **业务主体与机构内角色**。
- **无独立 organization 聚合**：`orgID` 为租户/查询边界。
- **`Testee.AssessmentStats` 等**：多为读模型或快照字段，**不是**每次业务写路径都强一致重算（以代码为准）。
- **领域事件**：当前 **无** 稳定 MQ 事件表；勿把注释占位当运行时行为。
- **BackendQueryService**：IAM 不可用或数据不全时，可能仅返回本地 Testee，监护人字段为空。
- **GetScaleAnalysis**：跨 [evaluation](./03-evaluation.md) 聚合查询，勿归类为纯 actor 领域数据。

### 维护时核对

- 变更 REST：同步 [api/rest/apiserver.yaml](../../api/rest/apiserver.yaml) 与 Handler。
- 变更 C 端 gRPC：同步 [actor.proto](../../internal/apiserver/interface/grpc/proto/actor/actor.proto) 与 `collection-server`。
- 变更 `TagTestee`：同步 [internal.proto](../../internal/apiserver/interface/grpc/proto/internalapi/internal.proto)、[internal.go](../../internal/apiserver/interface/grpc/service/internal.go) 与 worker 调用方。
- 若引入 actor 领域事件：新增 [configs/events.yaml](../../configs/events.yaml)、领域事件定义、发布点，并废除本文「N/A」表述。

---

*写作约定见 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md)。三界与主体引用见 [05-专题分析/01](../05-专题分析/01-测评业务模型：survey、scale、evaluation%20为什么分离.md)。*
