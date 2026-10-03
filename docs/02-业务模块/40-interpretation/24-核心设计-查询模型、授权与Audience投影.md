# 核心设计：查询模型、授权与 Audience 投影

> 2026-10-02 M6 收口候选补正：Participant 新增只读 `GetAssessmentReportStatus(testee_id, assessment_id)`，Collection 使用独立签名 purpose 和精确 method ACL。归属校验通过后，按原 Outcome 的冻结 ReportType／TemplateVersion 查唯一 Generation，再读其 LatestRunID；最多两轮复核 Outcome 内容身份与 Generation 版本。返回仅含 exists、status、attempt、retry_disposition，无模型输入、失败详情、接单或重试权限。报告缺失时，Collection 据持久状态区分自动恢复、人工处理、终止失败；新授权 Run 可覆盖旧失败提示，succeeded Run 单独不能证明报告可见。此处描述候选源码，不代表已部署或 M6 整项验收；下文旧的“仅包装 GetAssessmentReport”以本补正为准。

> 状态：本文已按当前源码重写。Participant、Administration 与 Operations 查询用例、授权先于正文读取的主链路、Catalog 分页查询和最小 Audience 投影已落地；
> 患者端 gRPC 信任边界、Administration 角色语义、Catalog 关联复验与完整章节级可见性仍有明确缺口。

## 1. 本文回答

本文集中回答报告生成之后的“谁能查、可以查哪些、最终能看到什么”：

1. 为什么报告关联了 OrgID、AssessmentID 和 TesteeID，仍然不能直接返回；
2. 身份认证、资源授权、查询范围和 Audience 投影有什么区别；
3. 患者、家长、医生、运营管理员和内部运维如何映射到当前查询用例；
4. Participant 查自己的报告时，collection-server、IAM ProfileLink 和 apiserver 各负责什么；
5. 旧 Clinician 入口如何退役，后台报告如何按当前 Operator 与门店范围授权；
6. Administration 列表怎样区分全机构范围、指定 Testee 和受限 Testee 集合；
7. Operations 为什么不走业务报告 DTO，而是查 Generation / Run / Artifact 审计证据；
8. `report_query_catalog` 怎样完成筛选、排序、分页和正文定位；
9. Audience 当前真正隐藏了哪些内容，哪些字段其实是被 Transport DTO 丢失的；
10. 当前查询与授权链路还存在哪些可能导致隐私或语义偏移的设计问题。

本文不重复 Artifact、Catalog 和 Archive 的持久化细节，这些见[《报告成品、版本与数据一致性》](./23-核心设计-报告成品、版本与数据一致性.md)。

## 2. 30 秒结论

报告查询不是一次 MongoDB `find`，而是四步安全链路：

```text
Authentication
  谁在调用？
       ↓
Authorization / Scope
  这个行为人能读哪个 Assessment 或哪些 Testee？
       ↓
Report Read Model
  Catalog 当前指向哪份 Artifact / Archive？
       ↓
Audience + Transport Projection
  已授权的报告中，这个读者最终看到哪些内容？
```

这四层不能互相代替：

| 层 | 关键问题 | 当前主要实现 |
| --- | --- | --- |
| 认证 | 调用者是谁 | IAM JWT、HTTP protected scope、gRPC mTLS |
| 授权 | 是否能读目标资源 | ProfileLink、Assessment ownership、active Operator、Testee 门店归属、IAM action scope |
| 范围 | 列表中哪些资源可见 | TesteeID、StoreScopedTesteeIDs 与 OrgID 过滤 |
| 读模型 | 哪份正文是当前报告 | `report_query_catalog -> artifact` |
| Audience 投影 | 授权后还需隐藏什么 | 当前仅对 `ModelExtra` 显式决策 |
| Transport 投影 | REST / gRPC 如何表达内容 | participant gRPC 保留富结构；当前 apiserver REST 仍是较窄的兼容 DTO |

最重要的边界是：

> Association 只说明报告属于谁，不说明当前调用者有权读取。Audience 只决定已授权读者可看哪些内容，不参与资源归属授权。

## 3. 先区分五个容易混淆的概念

### 3.1 Authentication：谁在调用

身份认证建立的是调用者身份：

- collection-system 用户通过 IAM JWT 进入 collection-server；
- apiserver 受保护 REST 路由从 JWT 和机构上下文中得到 `OrgID + OperatorUserID`；
- collection-server 调用 apiserver gRPC 时，当前生产配置使用 mTLS 证明它是 QS 内部服务；
- Operations 内部 REST 同时需要已认证用户和授权快照。

认证成功不表示可以读任意报告。

### 3.2 Authorization：能否读某个资源

授权要回答的是具体关系：

- 当前 IAM User 是否持有 Testee Profile 的 active link；
- 这个 Assessment 是否属于指定 Testee；
- 当前 Operator 是否属于当前 Org 且仍活跃；
- 当前 IAM snapshot 是否允许对应报告 action，Testee 当前门店是否在授权范围内；
- 当前 IAM snapshot 决定 AudienceAdmin 或 AudienceOperator；
- Operations 是否具有 `audit_interpretation` capability。

授权是业务关系判断，不应由 Catalog 文档里的 TesteeID 自行替代。

### 3.3 Scope：列表中允许出现哪些数据

详情授权可以返回 allow / deny，列表查询则需要把行为人权限翻译为数据过滤器：

```text
Participant       -> TesteeID = actor.TesteeID
Backstage         -> OrgID + explicit store-scoped TesteeIDs
Specified testee  -> verify current store scope, then TesteeID
```

范围计算必须在查询之前完成，不能先取全机构报告，再在内存中删除无权数据。

### 3.4 Audience：已授权读者能看到哪些章节

Audience 是内容可见性角色，当前值为：

- `participant`；
- `clinician`；
- `operator`；
- `admin`。

它不是 IAM JWT 中的 `aud` claim，也不是 Operator role。应用服务在已完成资源授权后，明确传入一个 Audience，Presenter 据此删除不可见章节。

### 3.5 Transport Projection：如何向某类客户端表达

即使 Audience 允许某个字段，REST 或 gRPC DTO 也可能暂时没有表达它。这是协议兼容问题，不是安全策略。

如果不区分这两层，“旧 REST DTO 忘了输出字段”很容易被误认为“业务刻意隐藏字段”，之后一次 DTO 补全反而可能意外暴露内容。

## 4. 当前查询用例与真实业务角色

| 应用用例 | Actor 身份 | 业务使用者 | 主要查询内容 | Audience |
| --- | --- | --- | --- | --- |
| Participant | TesteeID | 患者，或持有受试者 ProfileLink 的家长 | 自己/孩子的当前报告 | participant |
| Administration | OrgID + OperatorUserID + scope | 管理后台及业务工作台 | 按 action 门店范围查询指定受试者或报告列表 | admin / operator，由授权决策决定 |
| Operations | OrgID + OperatorUserID + audit capability | 内部运维和审计人员 | Generation、Run、失败、重试和 Artifact 元数据 | 无，不返回业务正文 |

“Participant”不应被简单翻译为“患者本人”。在儿童测评中，家长可以作为实际小程序用户读取孩子的报告。当前 collection-server 通过 IAM User 到 Testee Profile 的 active link
表达这种关系，报告本身仍属于 Testee，不属于当时的 AnswerSheet Filler。

## 5. 统一报告查询主链路

```mermaid
flowchart TD
    Request["行为人查询请求"]
    Identity["认证并解析 Actor"]
    Access["授权或计算 Scope"]
    Catalog["report_query_catalog<br/>筛选、排序、分页"]
    Bodies["按 SourceKind / SourceID<br/>批量加载 Artifact / Archive"]
    Row["ReportRow<br/>统一兼容读模型"]
    Audience["Audience Projection"]
    Transport["REST / gRPC DTO"]

    Request --> Identity --> Access --> Catalog --> Bodies --> Row --> Audience --> Transport
```

详情查询和列表查询只在授权形式上不同：

- 详情：先对 Assessment 或 Testee + Assessment 做具体授权，再按 AssessmentID 查 Catalog；
- 列表：先将行为人权限转为 TesteeID / TesteeIDs / OrgID，再将过滤器下推到 Catalog。

当前 Participant 和 Administration 应用服务都有“authorize before reader”的单元测试，保证拒绝时不会调用报告 Reader。

## 6. ReportReader 与 Catalog 的查询契约

### 6.1 ReportReader 是 Interpretation 读边界

`interpretationreadmodel.ReportReader` 只提供：

```text
GetReportByAssessmentID(assessmentID)
ListReports(filter, page)
```

它不识别当前行为人，也不应该自己调用 IAM 或 Actor 服务。它接收的必须是应用层已经计算完成的安全过滤器。

这种分层避免了 Mongo 读模型依赖 IAM SDK、MySQL Actor Repository 或 HTTP context。但这也意味着：任何直接调用 ReportReader 的上层都必须自己负责授权。

### 6.2 当前过滤器

| Filter | 用途 |
| --- | --- |
| OrgID | 机构级报告列表 |
| TesteeID | 某一受试者的报告列表 |
| TesteeIDs | 受限工作人员可访问的受试者集合 |
| ModelCode | 按测评模型筛选 |
| RiskLevel | 按精确风险等级筛选 |
| HighRiskOnly | 筛选 `high / severe` |

当前 Participant / Administration 的报告列表公开 DTO 只暴露 TesteeID 与分页，ModelCode 和 RiskLevel 主要供工作台、统计或其他内部读模型使用。

### 6.3 分页与加载

Catalog 先做：

```text
count(filter)
find(filter)
  sort sort_at desc, sort_report_id desc, assessment_id desc
  skip offset
  limit page_size
```

再将当前页按 SourceKind 分为 Artifact IDs 和 Archive IDs，各自一次批量加载正文，最后按 Catalog 顺序组装 ReportRow。

这避免了“先加载大量报告正文，再分页”。当页面没有数据时返回空切片，page 默认 1，page size 默认 10，最大 100。

### 6.4 悬空 Source 不静默跳过

如果 Catalog 声明 `artifact/9001`，但正文不存在或已被软删除，ReadModel 会：

1. 记录 AssessmentID、SourceKind 和 SourceID；
2. 返回 `CatalogDanglingSourceError`；
3. 终止整个页面的组装，不会把问题报告从列表中静默删掉。

这是正确的内部一致性语义。当前 Get 应用服务会将 Reader 错误统一包装为“报告不存在”错误码，List 则包装为数据库错误。因此运维定位仍要依赖内部 cause 和日志，对外错误码并不能区分“真的没有报告”和“Catalog 已损坏”。

## 7. Participant：患者或家长查看报告

### 7.1 对外链路

```mermaid
sequenceDiagram
    participant User as collection-system User
    participant BFF as collection-server
    participant IAM as IAM ProfileLink
    participant GRPC as apiserver ParticipantReportService
    participant Eval as Evaluation Testee Service
    participant Report as Interpretation ReportReader

    User->>BFF: GET report(testee_id, assessment_id) + JWT
    BFF->>BFF: parse authenticated UserID
    BFF->>BFF: load Testee -> IAMProfileID
    BFF->>IAM: HasActiveProfileLink(UserID, IAMProfileID)
    IAM-->>BFF: allowed
    BFF->>GRPC: GetAssessmentReport(testee_id, assessment_id)
    GRPC->>Eval: AuthorizeAssessment(TesteeID, AssessmentID)
    Eval-->>GRPC: ownership allowed
    GRPC->>Report: GetReportByAssessmentID
    Report-->>GRPC: ReportRow
    GRPC->>GRPC: AudienceParticipant projection
    GRPC-->>BFF: rich AssessmentReport
```

这条链路包含两个不同的“owner”判断：

1. IAM User 是否可以代表这个 Testee Profile；
2. Assessment 是否真的属于这个 Testee。

只做第 1 个检查，用户可能用自己的 TesteeID 查另一个 Assessment；只做第 2 个检查，调用者可能伪造其他 TesteeID。

### 7.2 apiserver Participant Access 当前保护什么

`participantInterpretationAccess` 的职责是：

- 列表前检查 Testee 存在；
- 详情前先检查 Testee 存在；
- 调用 Evaluation Testee Service，验证 Assessment.TesteeID 等于 Actor.TesteeID；
- 授权通过后才读取报告。

它并不从 gRPC context 中取 IAM UserID，也不调用 ProfileLink 验证“这个调用者是否拥有 Testee”。`ParticipantReportService` 直接将 request 中的 TesteeID 构造为 Participant Actor。

所以当前准确边界是：

> 终端用户到 Testee 的授权依赖 collection-server ProfileLink 中间件；apiserver Participant 用例只验证 Testee 存在和 Assessment 归属。

### 7.3 当前 gRPC 信任边界

仓库的 apiserver 生产配置意图为：

- 启用 mTLS 和客户端证书；
- `allowed-ous: QS`，CN 白名单为空；
- gRPC JWT auth 关闭；
- gRPC ACL 启用、`default_policy: deny`，并从 `configs/grpc-acl.prod.yaml` 加载逐方法白名单；
- `qs-collection-server.svc` 被精确允许调用 `AuthorizeAssessment` 和 `ParticipantReportService/GetAssessmentReport`，但不能调用未列入白名单的患者报告方法。

这把 `ParticipantReportService/GetAssessmentReport` 限定在持有 collection-server 服务身份的调用方，但服务身份仍不能证明 request TesteeID 来自已验证的终端用户 ProfileLink。

因此当前端到端边界是“生产 default-deny 方法 ACL + collection-server ProfileLink 授权 + apiserver Assessment ownership”。
新增内部调用者或 gRPC 方法时，必须同时更新客户端允许方法清单、生产 ACL 和契约测试，不能只依赖 QS OU。

### 7.4 列表与详情的差异

- `GetMyReport` 会验证 Assessment 归属；
- `ListMyReports` 只验证 Testee 存在，之后使用 TesteeID 过滤 Catalog；
- collection-server 当前 ParticipantReportClient 只包装了 GetAssessmentReport，但 proto 已公开 `ListMyReports`。

这使 `ListMyReports` 对可信 gRPC 调用者的要求更高：如果调用者可任意传 TesteeID，它可以直接枚举某个存在 Testee 的当前报告。

## 8. Clinician 入口退役与可见性策略

旧医生报告 REST 入口已退役：

```text
GET /api/v1/clinicians/me/testees/{testee_id}/reports
GET /api/v1/clinicians/me/testees/{testee_id}/reports/{assessment_id}
```

这些请求通过退役路由直接返回 `410 Gone`。独立 clinician 报告应用服务没有生产装配，已于 2026-10-03 删除；
原有授权先于读取和受限投影测试由 Administration 的详情、列表测试承接。

`AudienceClinician` 仍是内容可见性策略，和 `AudienceOperator` 一样隐藏 ModelExtra。它不授予资源访问权，
也不代表存在可用的医生专用查询入口。当前后台装配以 IAM 快照和门店范围决定访问，见下一节。

## 9. Administration：机构与受限工作台查询

### 9.1 当前授权与 Audience 决策

`administration.Service` 的 Actor 是 `OrgID + OperatorUserID`。详情经 Evaluation 的
`AuthorizeAssessmentResource` 校验报告 `read` action、当前 Assessment 归属和受试者门店范围；
列表经 Actor 的 `StoreScopeAccess` 校验 `list` action 与当前 Testee 门店归属。

正式 Access 装配根据受信任 IAM 快照选择 Audience：QS 管理员为 `AudienceAdmin`，其他后台 Operator 为
`AudienceOperator`。范围计算和可见性决策分别执行，管理员 Audience 也不能绕过已计算的查询范围。

### 9.2 当前列表范围

| 请求 | Catalog Filter | 含义 |
| --- | --- | --- |
| 指定 TesteeID | 当前 OrgID、指定 TesteeID、显式门店范围 | 查询前验证该 Testee 当前门店归属 |
| 未指定 TesteeID | 当前 OrgID、RestrictToStoreScope=true、StoreScopedTesteeIDs | 仅返回当前门店权限允许的受试者报告 |
| 门店范围为空 | 显式空范围 | 不退化为无过滤的机构查询 |

应用服务仍支持 Access 返回受限 TesteeIDs 的读模型契约，但正式装配使用门店 scope。
详情和列表在投影完成后重新验证访问决策，防止读取期间归属或权限变化。

### 9.3 当前 REST 入口

```text
GET /api/v1/evaluations/assessments/{id}/report
GET /api/v1/evaluations/reports
GET /api/v2/evaluations/assessments/{id}/report
GET /api/v2/evaluations/reports
```

这些路由使用 ReportQuery Journey 将 Evaluation Assessment 查询与 Interpretation 报告查询组合，最终委托 Administration Service 完成授权和报告读取。

应用服务在报告 Reader 前要求 `qs:evaluation:collection:reports` 的 `read` 或 `list` 权限；资源范围由 Evaluation 与 Actor 的当前门店检查决定。

## 10. Operations：查生命周期，不查业务正文

### 10.1 四个内部用例

| 入口 | 回答的问题 |
| --- | --- |
| `/internal/v1/interpretation/reports/{report_id}` | 这份 Artifact 的 Generation、Outcome、Run、Assessment、Type 和 Version 是什么 |
| `/internal/v1/interpretation/outcomes/{outcome_id}/generations` | 一个 Outcome 有哪些报告生成版本和执行历史 |
| `/internal/v1/interpretation/assessments/{assessment_id}/lifecycle` | 一次 Assessment 的 Interpretation 现在停在哪里 |
| `/internal/v1/interpretation/assessments/{assessment_id}/reports` | 该 Assessment 有哪些历史 TemplateVersion Artifact |

Operations 返回的 Report 只有身份和时间元数据，没有 Conclusion、Dimensions、Suggestions 和 ModelExtra。因此它不使用 Audience Presenter。

### 10.2 双层审计权限

Operations 路由组先挂载 `RequireCapability(audit_interpretation)`，Service 内部又检查：

1. Actor.OrgID 与资源 OrgID 一致；
2. context 中存在 authz snapshot；
3. snapshot 具有 `audit_interpretation`，或行为人是 qs:admin。

对应 IAM resource/action 是：

```text
qs:interpretation_reports / audit
```

这是路由防护与应用服务防护的双层约束。即使以后 Operations Service 被新 Transport 重用，也不会只依赖旧路由中间件。

### 10.3 授权需要的最小资源读取

Operations 按 OutcomeID 或 AssessmentID 查询时，先从 Evaluation Outcome 获得 OrgID，完成授权后才查 Generation 和 Run。这是“为授权读取最小资源包络”，不是“先读业务正文”。

但 `FindReportByID` 当前会先用 ReportRepository 恢复完整 InterpretReport，再从 Association.OrgID 做授权，虽然对外只返回元数据，但数据库已经在授权前加载了报告正文。
这不符合最严格的“授权先于正文读取”边界，应使用只返回 OrgID 和成品元数据的轻量 Correlation Reader，或在 Mongo 查询中加入已授权 OrgID。

## 11. Audience 当前真正实现了什么

### 11.1 当前只有一个受控章节

`presentation.Presenter` 当前只识别：

```text
SectionModelExtra = "model_extra"
```

可见性规则是：

| Audience | ModelExtra |
| --- | --- |
| participant | 可见 |
| clinician / operator | 不可见 |
| admin | 可见 |

其他内容——Model、PrimaryScore、Level、Conclusion、Dimensions 和 Suggestions——在应用投影层目前对三类 Audience 都原样保留。

未知 Audience 或未注册 Section 会返回错误，而不是默认全部可见。这个“未知策略 fail closed”的方向是正确的。

### 11.2 为什么生成一份 Canonical Report

当前系统不为 participant、clinician 和 admin 分别生成三份 Artifact，而是：

```text
one immutable InterpretReport
  -> AudienceParticipant projection
  -> AudienceClinician projection
  -> AudienceAdmin projection
```

这样做的好处是：

- 结果事实和解释文案只固化一次；
- 不会因为某一 Audience 生成失败而出现三份报告不一致；
- 新增读者角色时不需重算 Outcome；
- 可见性策略可以独立测试和演进。

但前提是 Canonical Report 的读取必须先经过授权，而且任何 Transport 都不能绕过 Presenter 直接序列化 ReportRow 或 PO。

### 11.3 当前策略的业务理由尚未固化

从代码可以确认“Clinician 不看 ModelExtra”，但当前仓库没有固化这个策略的业务原因。ModelExtra 中可能包含人格类型、匹配度、特殊触发、稀有度和评论，直觉上并不当然应该对医生隐藏。

因此本文只把它记录为当前实现事实，不将它包装为已被业务论证的最终规则。后续需要产品和医疗业务共同确认。

## 12. Audience 投影与 Transport DTO 的二次收窄

### 12.1 participant gRPC 保留了富报告结构

`interpretation.AssessmentReport` proto 可以表达：

- 完整 ModelIdentity；
- PrimaryScore 和 ResultLevel；
- Dimensions 的 derived scores、level 和 norm reference；
- Suggestions；
- ModelExtra。

Participant 经过 `AudienceParticipant` 投影后，ModelExtra 会进入 gRPC 响应，collection-server 再转换为小程序 BFF DTO。
这是相对于现有 REST DTO 更完整的报告展示契约，但仍不是 Artifact 的无损序列化：当前 proto 没有表达应用 Report Dimension 中的 `Role`、`ParentCode`、`HierarchyLevel` 和 `SortOrder`。

### 12.2 apiserver REST 仍是兼容摘要形状

`response.NewReportResponse` 当前只输出：

- AssessmentID；
- 由 Model.Title / Code 映射的 ScaleName / ScaleCode；
- 由 PrimaryScore.Value 映射的 TotalScore；
- 由 Level.Code 映射的 RiskLevel；
- Conclusion；
- 基础 Dimension 字段；
- Suggestions；
- CreatedAt。

它没有输出：

- Model 的 kind、sub-kind、algorithm、version、product channel 和 algorithm family；
- PrimaryScore 的 kind、label 和 max；
- ResultLevel 的 label 和 severity；
- Dimension derived scores、ResultLevel 和 NormReference；
- ModelExtra。

这些字段的缺失不是 Presenter 做的 Audience 决策，而是 REST response 仍停留在医学量表兼容形状。因此当前“医生看不到 ModelExtra”同时由两层造成：

1. `AudienceClinician` 明确隐藏；
2. REST DTO 无论 Participant/Admin/Clinician 都不表达。

第 2 层不应被当作安全保障，否则以后补全 REST DTO 时就可能绕过真正的 Audience 策略。

## 13. 跨模块组合状态

### 13.1 Assessment `evaluated` 不等于报告已存在

ReportQuery Journey 会将 Evaluation 和 Interpretation 事实组合为前端状态：

```text
Assessment.status != evaluated
  -> 保留 Assessment 状态

Assessment.status == evaluated
  + Catalog 无报告
  -> evaluated

Assessment.status == evaluated
  + Catalog 存在报告
  -> interpreted
  -> interpreted_at = ReportRow.CreatedAt
```

`interpreted` 是 Journey / Read Model 投影，不是 Assessment 聚合状态。这个边界避免 Interpretation 回写 Evaluation 聚合。

### 13.2 投影必须继承已授权 Assessment

Journey 先通过 Evaluation Operator Query 获得已授权 Assessment，然后才用 AssessmentID 检查报告存在。这一步不返回报告正文，只用它的 CreatedAt 建立组合状态。

当前列表投影会对每一个 evaluated Assessment 单独调用一次 `GetReportByAssessmentID`，在大页面上形成 N+1 Catalog 查询。这是读模型性能上的明确改进项。

## 14. 当前安全不变量

### 14.1 已有代码保护

1. Participant 详情在报告 Reader 之前验证 Assessment 属于 Testee。
2. Administration 详情在 Reader 前验证报告 action、Assessment 与受试者当前门店范围。
3. 后台访问要求当前机构的活跃 Operator 与受信任 IAM 授权快照；旧医生入口返回 410。
4. Testee 必须属于 Actor 当前 Org。
5. Administration 受限空集合直接返回空列表，不退化为全库查询。
6. Participant 和 Administration 业务报告都先授权、后加载报告正文。
7. Operations 同时校验当前 Org 与 audit capability。
8. 未知 Audience 和未知 Section 不会默认开放。
9. Catalog 查询只加载当前页正文，悬空 Source 不会被静默忽略。
10. collection-server 的 `reportwait.Service.GetStatus` 与 `Wait` 在读取 status cache、进入 DB fallback 或注册 notifier 前，
    先执行 `User -> active ProfileLink -> Testee -> AuthorizeAssessment`；拒绝时不访问 cache。
11. `AuthorizeAssessment` 已进入 proto、collection client 允许方法清单和生产 default-deny ACL；
    `report_status_assessment_ownership_total{result}`、对应 duration 指标和 WebSocket 固定低基数拒绝指标提供观测入口。

### 14.2 应补强的不变量

1. 任何 Participant gRPC 调用必须携带可验证的终端用户—Testee 授权证据，或严格限定为已履行该校验的 BFF。
2. 新增或移动 gRPC 方法时，必须保持 method-level ACL、客户端允许方法清单和 proto 契约同步；不能因 mTLS 已通过而放宽 default-deny。
3. 一个 Actor 被投影为哪个 Audience，必须来自同一份权限决策，不能由不同路由手工指定后产生偏差。
4. Catalog 的 AssessmentID / OrgID / TesteeID 必须与它指向的正文关联完全一致。
5. 详情查询返回的 ReportRow.AssessmentID 必须等于请求并已授权的 AssessmentID。
6. Transport 只能序列化已完成 Audience 投影的 DTO，不能直接序列化 PO 或 ReportRow。
7. 安全可见性必须由 Presenter 明确决定，不能依赖某个旧 Transport 暂时没有输出字段。
8. Operations 在授权前只允许读取最小资源包络，不加载报告正文。

## 15. 当前设计问题与风险

### 15.1 Participant gRPC 没有端到端绑定 IAM User 与 Testee

当前安全性建立在“collection-server 必然先跑 ProfileLink middleware”的信任上，但 Participant gRPC 本身没有验证证据。生产配置的 mTLS OU 范围又大于单一 collection-server。

建议的改造方向可以是下列之一，但需要在安全架构中统一选型：

- 启用真正可加载规则的 gRPC method ACL，只允许 collection-server 调用 ParticipantReportService；
- 传递可验证的终端 Principal / delegated subject，由 apiserver 再次校验 ProfileLink；
- 将“已验证 Testee subject”建模为签名的内部授权上下文，而不是普通 request field。

只启用 gRPC JWT 但仍不将 UserID 绑定 TesteeID，不能单独解决这个问题。

### 15.2 Audience 决策与实际入口

Administration 已使用 Access 返回的 Audience，不再按包名无条件指定 `AudienceAdmin`。
当前装配区分 admin/operator；独立 clinician 查询实现已退出。

受限 clinician/operator 与 admin 的 ModelExtra 可见性由实际 Administration 详情、列表的表驱动测试保护。
新增敏感章节或新接入方仍需扩展可见性矩阵，不能把当前有限章节测试当成完整业务验收。

### 15.3 Audience 策略过于粗粒度，且业务根据不清晰

当前只有 ModelExtra 一个 Section，无法表达：

- 医生可见、患者需简化的专业解读；
- 患者可见但运营工作台不必展示的个性化展示素材；
- 仅审计可见的模板、Builder 和来源证据；
- 建议、严重风险提示和临床备注的不同可见性；
- 家长与年长患者本人的差异。

在没有真实产品需求前不需要预先构造复杂 RBAC，但至少应将每个已存在 Section 的业务理由、默认可见性和测试矩阵固化。

### 15.4 Catalog 没有在读取时复验 Source 关联

Catalog 行中有 AssessmentID、OrgID 和 TesteeID，Artifact 正文也有同样的冻结关联。但当前 `loadCatalogRows` 只验证 SourceID 是否能加载，没有验证：

```text
catalog.assessment_id == body.assessment_id
catalog.org_id        == body.org_id
catalog.testee_id     == body.testee_id
```

这在正常成功事务中不会出问题，因为 Catalog 从同一 Artifact 投影并与它一起提交。但历史回填、人工修复或数据损坏如果把一个已授权 Assessment 的 Catalog 指向了其他人的正文，应用层可能在完成正确授权后返回错误的报告内容。

这是需要优先补强的安全一致性边界。读模型应在组装 ReportRow 前对比 Catalog 与 Source envelope，不一致时返回专用错误并告警。

### 15.5 REST DTO 兼容映射遮蔽了真实内容契约

人格、行为常模和认知任务报告需要 ModelIdentity、派生分、NormReference 和 ModelExtra。但当前 generic REST 仍用 ScaleName、TotalScore 和 RiskLevel 表达。

这不仅是“少几个字段”，而是对多测评模型统一报告契约的偏移。后续需要一个真正的 Report Response V2，且 V2 必须从 Audience-projected Report 映射，不能从 PO 重建另一套规则。

### 15.6 Operations FindReport 在授权前加载了完整 Artifact

当前对外没有返回正文，但严格的“authorize before content”原则仍未满足。这个问题可以通过轻量 ArtifactMetadataReader 解决，也可以将 OrgID 作为已授权查询条件。

### 15.7 ReportRow 缺少安全与追溯包络

ReportRow 当前有 AssessmentID，却没有 OrgID、TesteeID、SourceKind、SourceID、ReportID、ReportType 和 TemplateVersion。这导致：

- 应用层无法对 Catalog 与正文关联做完整复验；
- 上层无法在审计日志中记录实际返回的 ReportID 和版本；
- 无法告诉客户端返回的是 Artifact 还是 Archive 兼容结果；
- 不利于对历史推导数据做风险标记。

下一版读模型应包含最小 SourceEnvelope 与 ArtifactProvenance，业务 DTO 再根据 Audience 决定是否对外展示。

### 15.8 深分页、大范围 `$in` 和 count/page 不是强一致快照

当前分页使用 skip/limit，受限工作人员使用 TesteeIDs `$in`，count 和 page 是两次独立查询。因此：

- 页码很深时 skip 成本会增长；
- 可访问 Testee 数量很大时 `$in` 条件和序列化开销增长；
- count 与 page 之间如果新报告插入，total 和当前页可能不属于同一时刻快照；
- 新报告插入到首页后，后续 offset 页可能重复或跳过数据。

对普通 10–100 条页面，这可以是可接受的最终一致读取；对大型机构工作台，后续应评估基于 `(sort_at, sort_report_id, assessment_id)` 的 cursor pagination，以及将访问范围转为可查读模型，而不是一次传入大量 ID。

### 15.9 Assessment 组合状态存在 N+1 查询

Assessment 列表先从 Evaluation 取一页，再对每个 evaluated Assessment 单独查 Catalog。这会将一次列表变成 1+N 次存储访问。

比较合适的方向是 ReportReader 增加批量 existence / metadata 查询，或建立独立 Journey 状态投影，而不是让 Evaluation 聚合新增 `interpreted` 状态。

## 16. 建议的目标授权模型

当前最值得收敛的不是“再增加一个中间件”，而是建立一个可解释的读取决策：

```text
ReportAccessDecision
  ActorKind
  ActorIdentity
  ResourceScope
  Audience
  AllowedSections
  DecisionSource
  AuditContext
```

建议流程：

```mermaid
flowchart LR
    Principal["Authenticated Principal"]
    Resource["Requested Assessment/Testee"]
    Policy["Report Access Policy"]
    Decision["AccessDecision<br/>scope + audience + sections"]
    Reader["Scoped ReportReader"]
    Presenter["Presenter"]
    DTO["Transport DTO"]

    Principal --> Policy
    Resource --> Policy
    Policy --> Decision
    Decision --> Reader
    Decision --> Presenter
    Presenter --> DTO
```

这样可以避免当前“一个 Access 返回 scope，另一行代码手工选 Audience”的偏移。但这不意味着必须马上引入通用 ABAC 引擎；当前可以先用显式值对象和策略测试收敛。

## 17. 一个具体例子：家长与后台 Operator 查看同一份报告

假设：

- 儿童 TesteeID = 3001；
- 家长 IAM UserID = U-88，对 Testee Profile 有 active link；
- AssessmentID = 5001，属于 Testee 3001；
- 后台 OperatorUserID = 7001，在 Org 9 处于 active 状态；
- Testee 3001 当前门店属于该 Operator 的报告 read 范围；
- Catalog 将 Assessment 5001 指向 Artifact 9001。

家长链路：

```text
U-88 JWT
  -> active ProfileLink to Testee 3001
  -> Assessment 5001 belongs to Testee 3001
  -> Catalog 5001 -> Artifact 9001
  -> AudienceParticipant
  -> gRPC rich report, ModelExtra visible
```

后台链路：

```text
Org 9 + Operator 7001
  -> active Operator and trusted IAM snapshot
  -> reports read action and current Testee store scope
  -> Assessment 5001 belongs to Testee 3001
  -> Catalog 5001 -> Artifact 9001
  -> AudienceOperator
  -> ModelExtra removed
  -> current REST compatibility DTO removes additional rich fields
```

两个读者读的是同一份不可变 Artifact，差异来自已授权后的投影，而不是两次重新计分或两份互相独立的报告。

## 18. 面试追问

### 18.1 报告中已经有 TesteeID，为什么查询前还要读 Assessment 或 Actor 关系？

TesteeID 是资源关联，不是当前调用者的授权证据。授权需要验证 IAM User/ProfileLink、Assessment ownership、当前活跃 Operator、报告 action 与 Testee 门店范围。
如果仅比对客户端传入 TesteeID 和报告 TesteeID，客户端可以同时伪造两者。

### 18.2 为什么不把授权逻辑写进 Mongo ReportReader？

ReportReader 是持久化无关的读边界，它不应该理解 HTTP context、IAM SDK 或 Clinician relation repository。应用层先将行为人权限翻译为资源授权或安全过滤器，Reader 只负责高效执行。

### 18.3 Audience 是 RBAC 吗？

不是完整 RBAC。当前 Audience 只是报告内容投影的读者类别。资源授权仍由 ProfileLink、Assessment ownership、当前 Operator 门店范围和 IAM action scope 决定。

### 18.4 为什么只保存一份 Canonical Report？

因为患者和医生看到的业务核心应该来自同一 Outcome 与同一生成成品。不同读者的差异是展示和可见性问题，不应导致同一测评产生三套可能漂移的结果。

### 18.5 当前后台 Operator 和管理员真的看到不同报告吗？

应用层上，AudienceOperator 隐藏 ModelExtra，AudienceAdmin 保留；AudienceClinician 的保留策略同样隐藏。但当前 apiserver REST DTO 本身不输出 ModelExtra，因此当前 REST 响应上这个差异看不出来。
患者 gRPC / collection BFF 契约会输出 ModelExtra。

### 18.6 为什么 Operations 不直接返回报告正文？

Operations 的任务是回答生命周期、失败原因、重试决策和版本来源，而不是成为另一个患者报告出口。最小化返回内容符合审计和隐私保护原则。

## 19. 代码导航

| 主题 | 事实源 |
| --- | --- |
| Participant 查询用例 | `internal/apiserver/application/interpretation/participant/service.go` |
| 受限后台投影验证 | `internal/apiserver/application/interpretation/administration/service_test.go` |
| Administration 查询用例 | `internal/apiserver/application/interpretation/administration/service.go` |
| Operations 审计用例 | `internal/apiserver/application/interpretation/operations/service.go` |
| Audience 枚举 | `internal/apiserver/domain/interpretation/policy/policy.go` |
| Section 可见性 | `internal/apiserver/domain/interpretation/presentation/presenter.go` |
| 统一应用 Report 投影 | `internal/apiserver/application/interpretation/reportprojection/mapper.go` |
| ReportRow 契约 | `internal/apiserver/port/interpretationreadmodel/readmodel.go` |
| Catalog 查询和正文加载 | `internal/apiserver/infra/mongo/interpretation/artifact_read_model.go` |
| Participant / Administration Access 装配 | `internal/apiserver/container/module_init.go` |
| Operations capability 适配 | `internal/apiserver/container/modules/interpretation/assemble.go` |
| 当前门店范围访问规则 | `internal/apiserver/application/actor/access/store_scope.go` |
| 授权关系类型 | `internal/apiserver/domain/actor/relation/types.go` |
| Assessment ownership | `internal/apiserver/application/evaluation/testee/service.go` |
| Administration scope 计算 | `internal/apiserver/application/evaluation/operator/service.go` |
| Evaluation + Interpretation 组合状态 | `internal/apiserver/application/journey/reportquery/service.go` |
| Participant gRPC | `internal/apiserver/transport/grpc/service/participant_report.go` |
| Operations REST | `internal/apiserver/transport/rest/handler/interpretation_actor.go` |
| REST 报告 DTO | `internal/apiserver/transport/rest/response/evaluation.go` |
| collection-server ProfileLink 校验 | `internal/collection-server/transport/rest/middleware/iam_middleware.go` |
| collection-server gRPC Client | `internal/collection-server/infra/grpcclient/evaluation_client.go` |
| apiserver 生产 gRPC 信任配置 | `configs/apiserver.prod.yaml` |
| 生产逐方法 ACL | `configs/grpc-acl.prod.yaml` |
| report-status ownership 指标 | `internal/pkg/reportstatus/metrics.go` |

## 20. 验证建议

本篇所述语义需要至少由以下测试保护：

```bash
go test ./internal/apiserver/application/interpretation/participant
go test ./internal/apiserver/application/interpretation/administration
go test ./internal/apiserver/application/interpretation/operations
go test ./internal/apiserver/domain/interpretation/presentation
go test ./internal/apiserver/infra/mongo/interpretation
go test ./internal/apiserver/application/actor/access
go test ./internal/apiserver/application/journey/reportquery
go test ./internal/apiserver/transport/grpc/service
go test ./internal/apiserver/transport/rest
go test ./internal/collection-server/transport/rest/middleware
go test ./internal/collection-server/application/reportwait
go test ./internal/pkg/configcontract -run GRPCACL
```

重点场景包括：

- 授权失败时 ReportReader 一次都不能被调用；
- Testee 和 Assessment 交叉伪造时必须拒绝；
- 后台 Operator 越过报告 action、门店范围或跨 Org 时必须拒绝；
- creator relation 不能授予报告访问权；
- 受限空 Testee 集合不得触发无范围 Catalog 查询；
- Operations 跨 Org 或缺少 audit capability 时必须拒绝；
- Participant/Admin/Clinician 的 Section 可见性必须使用表驱动矩阵测试；
- 未知 Audience / Section 必须 fail closed；
- Catalog 指向正文缺失或关联不一致时必须返回一致性错误；
- generic Administration 路由不能为 Clinician 产生 Admin-only 内容；
- collection-server 未绑定 ProfileLink 时必须在 gRPC 前拒绝；
- ReportResponse V2 不能丢失已允许的富报告字段；
- Assessment 列表的 interpreted 组合状态不能回写 Evaluation 聚合。
