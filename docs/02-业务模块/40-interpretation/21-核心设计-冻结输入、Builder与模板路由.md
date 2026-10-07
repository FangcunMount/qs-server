# 核心设计：冻结输入、Builder 与模板路由

> 状态：本文已按当前源码重写。它描述的是报告生成的输入与扩展机制，不展开 Generation、Run、重试和可靠提交；这些生命周期问题由下一篇文档说明。

## 1. 本文回答

本文集中回答五个问题：

1. 为什么报告生成必须使用 Evaluation 提交时冻结的事实，而不能重新查询当前 ModelCatalog；
2. Outcome 结果事实、ReportInput 报告素材和 InterpretationInput 分别是什么；
3. Interpretation 怎样验证 current-only Outcome/ReportInput 契约，又不让 Builder 感知持久化格式；
4. Registry 怎样根据运行机制、报告类型和模板版本选择 Builder；
5. `TemplateVersion`、`TemplateID`、`AdapterKey`、`BuilderIdentity` 和 `ContentSchemaVersion` 为什么不能混为一个“模板字段”。

它最终要保护的是一个很具体的历史语义：

> 同一份已经可靠提交的 Outcome，无论立即生成报告、失败后重试，还是在模型发布新版本后重新生成，都只能依据当时成立的结果事实和当时冻结的报告素材；它不能因为当前配置发生变化而得到另一种解释。

## 2. 30 秒结论

Interpretation 的输入不是 AnswerSheet，也不是可变 AssessmentModel，而是 Evaluation 已经可靠提交的 `Outcome Record`：

```text
Outcome Record
├── Model / Runtime Identity       当次执行使用的模型和机制身份
├── Payload                        已成立的执行结果事实
└── ReportInput                    当次报告需要的冻结模型素材
        │
        ▼
FromOutcomeRecord                 防腐与严格契约适配边界
        │
        ▼
InterpretationInput               Interpretation 自己拥有的只读输入
├── Association / Model / Runtime
├── Result
├── ReportSpec
└── mechanism-specific facts
        │
        ▼
Rendering Registry
DecisionKind + ReportType + TemplateVersion
+ optional Algorithm / ReportProfile
（AlgorithmFamily 仅由 DecisionKind 派生）
        │
        ▼
Builder                           确定性地组装 Draft
        │
        ▼
Report Draft
```

这套设计将变化拆成三层：

| 变化 | 由谁吸收 | 主链路是否改变 |
| --- | --- | --- |
| Outcome/ReportInput schema 演进 | codec 与显式迁移契约；当前仅接受既定 schema | Builder 接口保持稳定，旧格式不能静默兼容 |
| 已有机制下增加一个模型 | 冻结输入和现有 Builder | 否 |
| 同一机制增加具体呈现模板 | Template / Adapter | 通常不变 |
| 出现新的报告输入形态 | 新 Builder 或新的机制键 | Registry 注册变化 |
| 报告生成语义发生不兼容变化 | 新 TemplateVersion | 新 Generation，不覆盖旧报告 |

Builder 是“报告内容构建器”，不是模板文件，也不是报告生成用例。它只接受完整的 `InterpretationInput`，返回内存中的 Draft，不访问仓储，不创建 Generation，不推进 Run，也不提交 Report。

## 3. 为什么必须冻结报告输入

### 3.1 报告生成与模型发布不在同一个时间点

一次测评可能经历：

```text
T1  运营发布模型 v12
T2  患者作答，Evaluation 使用 v12 产生 Outcome
T3  报告第一次生成失败
T4  运营发布模型 v13，修改解释文案或类型详情
T5  系统或管理员重试 T2 的报告
```

如果 T5 根据 model code 查询“当前已发布模型”，T2 的历史结果就会被 v13 的素材解释。即使分数没有变化，报告中的类型名称、因子标题、解释区间、建议、图片或来源声明也可能变化。

这会破坏三个业务承诺：

- 历史报告必须保留测评发生时的语义；
- 同一个 Outcome 的自动重试不能改变结果呈现；
- 新模型发布不能反向改写已经完成的历史测评。

因此，报告所需的模型素材必须在 Evaluation 可靠提交 Outcome 时一起冻结，而不是到 Interpretation 执行时临时查找。

### 3.2 只冻结 Outcome 分数仍然不够

Outcome 可以保存总分、因子分、等级、类型编码和常模派生分，但这些事实不一定包含完整报告素材。例如：

- 因子 `attention` 的标题、最大分和解释规则；
- 某人格类型编码对应的名称、一句话描述、优势、弱点和建议；
- 报告使用的图片、来源署名和许可声明；
- 常模表版本、适用年龄和性别范围；
- 人格模型应使用哪个 Adapter 与 TemplateID。

如果把所有展示素材直接塞进 Outcome Payload，会让 Evaluation 再次拥有报告结构；如果完全不保存，又只能读取当前模型。当前实现采用更清晰的双事实设计：

| Outcome Record 部分 | 回答的问题 | 内容性质 |
| --- | --- | --- |
| `Payload` | “这次测评算出了什么？” | 分数、等级、维度、类型编码、能力结果等执行事实 |
| `ReportInput` | “解释这些结果需要哪些当时的模型素材？” | 发布模型中供报告使用的冻结快照 |
| `Model` / `Runtime` | “它由哪个模型和哪种机制产生？” | 模型 code/version 与执行路由身份 |

`Payload` 与 `ReportInput` 职责不同，但共同构成历史报告可重放的输入。

### 3.3 冻结不等于复制整个业务世界

冻结输入应遵循“报告重放所需的最小充分事实”，而不是把所有上游聚合序列化一遍：

- 不冻结可变 Assessment 聚合；
- 不冻结 AnswerSheet 作为 Builder 输入；
- 不把 ModelCatalog 仓储接口交给 Builder；
- 不复制与报告无关的运营编辑状态；
- 只保存计算事实、模型身份、运行机制和报告所需发布素材。

这也是为什么生产适配器直接从 Outcome Record 构建 InterpretationInput，而不是恢复一个 Assessment 后再次调用 Evaluation 或 ModelCatalog。

### 3.4 MBTI 四轴结构化事实

`MBTI_OEJTS / v64-report-202608-v1` 的新 Outcome 在提交时从同一不可变 DefinitionV2 提取 `mbti-pole-catalog/v1`，作为 ReportInput schema 3 的可选扩展。冻结内容包括 EI、SN、TF、JP 的组合顺序、名称、两极、原始分范围及阈值；不保存答题或执行载荷，不在报告恢复时查询最新模型。

Interpretation 将该目录与 Outcome 中的原始分、偏好方向及精确强度合并，写入报告维度的 `mbti-pole-facts/v1`。强度是偏好强度，不是置信度；展示文案的取整不影响结构化值。缺失值与合法零值分别处理，四轴缺失、重复、模型身份或类型方向冲突均拒绝构建；持久化扩展损坏也拒绝回读。Mongo 使用可选 `pole_facts` 字段，无需集合结构迁移。

历史 ReportInput 或报告没有该扩展时保留原读取行为，不从描述反解析、不从最新模型补造，也不自动回填旧数据。`report-content/v1` 的原字段和量表路径保持原语义；新事实块以自身 schema 版本识别。

AI bridge 对精确模型及完整四轴事实投影 `qs-report-snapshot/v2`：显式白名单保留来源、模型、类型、四轴与建议，不输出图片或稀有度。缺少事实的历史 MBTI 报告返回 `not_applicable`，不能退回量表快照或从文案补算；损坏事实明确失败。旧量表 `qs-report-snapshot/v1` 字节不变。

本批属于 P1 可信事实与兼容读取；先部署 qs-ai 兼容版本，再发布 QS。qs-ai 当前生产准入仍拒绝人格 v2，直到 P2 的冻结执行、恢复和资产闭环通过。没有自动批准、发布、模型调用或历史数据补写。Go 投影测试导出的合成快照由 qs-ai CI 直接解码和组装，不能把该契约验证当作真实人格生成验收。

事实入口与验证：

- [`report_input_freeze.go`](../../../internal/apiserver/application/evaluation/outcome/commit/report_input_freeze.go)：提交层组合原模型元数据与计算规则，冻结为中立输入契约。
- [`mbti_poles.go`](../../../internal/apiserver/application/interpretation/automation/input/mbti_poles.go)：合并已提交的精确结果事实。
- [`pole_facts.go`](../../../internal/apiserver/domain/interpretation/report/pole_facts.go)：报告不可变事实及一致性约束。
- [`mbti_facts_test.go`](../../../internal/apiserver/infra/mongo/interpretation/mbti_facts_test.go)：脱敏基线从冻结输入、Builder 到 BSON 回读的回归；不代表生产验收。

## 4. 三层输入模型

### 4.1 第一层：Outcome Record

Evaluation 提交的只读 Record 是跨模块事实边界。与本文相关的字段包括：

| 字段 | 含义 | Interpretation 的用法 |
| --- | --- | --- |
| `ID` | Outcome 唯一身份 | 形成 Generation 来源与追踪关系 |
| `OrgID` | 组织身份 | 形成报告关联事实，不直接授予访问权限 |
| `AssessmentID` | 测评身份 | 关联报告与查询索引 |
| `TesteeID` | 受试者身份 | 形成参与者查询范围 |
| `Model` | 模型 kind、algorithm、code、version、title | 构建报告模型身份 |
| `Runtime` | canonical DecisionKind | 校验机制并解析 Builder |
| `SchemaVersion` | Outcome 事实 schema | 选择版本化解码方式 |
| `Payload` | 结果事实 JSON | 解码为版本中立 Execution |
| `ReportInput` | 冻结报告素材 JSON | 解码为 InputSnapshot |
| `EvaluatedAt` | 结果成立时间 | 历史审计依据 |

Record 仓储只暴露查询，不提供修改方法。Interpretation 只能消费这一事实，不能反向修改 EvaluationOutcome。

### 4.2 第二层：版本中立的 Execution 与 InputSnapshot

`evaluationfact/codec` 负责把持久化格式转换为稳定的运行时结构：

```text
record.Payload
  -> DecodeExecution(record)
  -> Execution
     ├── Primary
     ├── Level
     ├── Profile
     ├── Dimensions
     ├── Validity
     └── typed Detail

record.ReportInput
  -> DecodeReportInput(record)
  -> InputSnapshot
     ├── ModelSnapshot
     └── model-specific frozen payload
```

这一层仍由 Evaluation fact port 定义，因为它解释的是“Evaluation 究竟提交了什么”，不是最终报告该怎样展示。

### 4.3 第三层：InterpretationInput

`FromOutcomeRecord` 再将上述两种数据转换为 Interpretation 自己的只读输入：

| 组成 | 主要字段 | 用途 |
| --- | --- | --- |
| `OutcomeID` | Outcome ID | 追踪事实来源 |
| `Association` | OrgID、AssessmentID、TesteeID | 报告关联与后续查询 |
| `Model` | kind、algorithm、code、version、title | 报告展示和追踪 |
| `Runtime` | canonical DecisionKind | Builder 机制路由，Family 为进程内派生值 |
| `Result` | Primary、Level | 已成立的总结果事实 |
| `Report` | type、version、algorithm、profile、adapter、template | 报告路由与模板选择 |
| `FactorScoring` | 因子模型与因子结果 | 因子计分、常模与任务类报告 |
| `PersonalityType` | 类型详情 | 人格类型类报告 |
| `TraitProfile` | 特质详情 | 连续特质画像类报告 |

Builder 只依赖这一层，不解码持久化 JSON。当前 codec 仅接受 Outcome schema 2 和 ReportInput schema 3；版本不符在 Builder 运行前拒绝。

## 5. FromOutcomeRecord 是防腐边界

生产报告链路的关键适配器是 `application/interpretation/automation/input.FromOutcomeRecord`。它承担四类工作。

### 5.1 解码并验证跨模块事实

适配器先调用：

```text
DecodeExecution(record)
DecodeReportInput(record)
```

任一步解码失败，报告生成不会带着半完整数据继续执行。由于输入映射发生在 Starter 创建 Generation / Run 之前，这类错误当前不会形成 InterpretationRun；Automation 通过独立 AdmissionFailure 记录稳定分类和治理证据；它不伪造已创建的 Run。

### 5.2 严格消费冻结身份

当前适配器只读取 Outcome.Runtime.DecisionKind；缺失或无法映射到 AlgorithmFamily 时返回 catalog_incompatible，不根据 kind、algorithm 或默认值恢复身份。AlgorithmFamily 是从已验证 DecisionKind 派生的进程内属性，ReportProfile 也由 DecisionKind 派生；它们不是额外持久化事实。

模板路由来自同一 Outcome.ReportInput 的冻结 InterpretationAssets。所有 section 的 TemplateID/TemplateVersion 必须完整一致，typology routing 还必须与其一致；缺失或冲突进入 artifact_contract_invalid，不查询当前 ModelCatalog 或补默认模板。

事实入口：[FromOutcomeRecord](../../../internal/apiserver/application/interpretation/automation/input/outcome_record_adapter.go)、[冻结模板解析](../../../internal/apiserver/domain/interpretation/reporttemplate/resolve.go)。

### 5.3 将通用结果映射为报告事实

Execution 中的通用结果会映射为：

- `raw_total` -> 报告主分数；
- `match_percent` -> 匹配百分比；
- risk level code -> 报告风险等级；
- 非风险型 level -> 保留 code、label、severity；
- dimensions -> 因子、特质、任务维度和派生分数；
- norm reference -> 报告所需常模引用。

这一步只变换表达形式，不应重新计算 Outcome。

### 5.4 构造机制专用事实

适配器按 AlgorithmFamily 建立互斥或专用输入：

| AlgorithmFamily | 主要输入 | 冻结素材来源 |
| --- | --- | --- |
| `factor_scoring` | `FactorScoringFacts` | scale payload |
| `factor_norm` | `FactorScoringFacts` | behavioral rating snapshot、norming |
| `task_performance` | `FactorScoringFacts` | cognitive snapshot |
| `factor_classification` | `PersonalityTypeFacts` 或 `TraitProfileFacts` | typology payload |

这种设计让统一执行主链路只处理 InterpretationInput，而把不同模型的事实形状保留在明确的扩展槽位中。

## 6. current-only schema 与结果所有权

### 6.1 四种版本边界

| 版本概念 | 当前契约 | 保护对象 |
| --- | --- | --- |
| Outcome SchemaVersion | 2 | Evaluation Payload 结果事实 |
| ReportInput schema | 3 | 冻结模型/解释素材与可选扩展 |
| TemplateVersion | legacy-v1、2026-08-v1 等精确已发布版本 | 完整报告生成语义 |
| ContentSchemaVersion | report-content/v1 等 | Artifact Content 结构 |

Outcome schema 0/1/未知值和非当前 ReportInput 都拒绝；legacy-v1 是精确保留的模板 release 名称，不表示旧 Outcome schema decoder 仍可使用。PayloadFormat 不属于当前冻结运行身份。源码见[Outcome codec](../../../internal/apiserver/port/evaluationfact/codec/codec.go)与[输入契约](../../../internal/apiserver/port/evaluationinput/)。

### 6.2 typology 分类事实与冻结解释资产

schema 2 保存 TypeCode、Pattern、MatchPercent/Similarity、IsSpecial、SpecialTrigger 等分类事实；Interpretation 在同一 Outcome 的冻结 typology ReportInput 内查找名称、画像、建议、图片和来源。TypeCode 不存在、素材损坏或模板身份冲突时直接拒绝，不回读当前 ModelCatalog。

### 6.3 因子模型与常模素材

因子报告从冻结素材恢复 code、title、max score、total 标识及解释资产，再结合 Outcome dimensions 的既有分数。解释只按冻结 OutcomeCode 选文案，不重新匹配分数产生结果。

常模维度具有 T-score 时必须已有 Outcome Level.Code；缺失即失败。冻结常模匹配用于验证已有 code 并恢复 conclusion/suggestion，code 不一致也失败；只有展示 label 为空时可用冻结 conclusion 补展示文字。这不会创建或改变 Outcome-owned Level.Code。源码见[fact_mapping.applyFrozenNormInterpretation](../../../internal/apiserver/application/interpretation/automation/input/fact_mapping.go)。

## 7. 报告路由身份与派生机制

Registry 的公开 Key 由三个必需字段和两个可选细分字段组成：

```text
Key
├── DecisionKind      必需：canonical 结果机制
├── ReportType        必需：报告业务类型
├── TemplateVersion   必需：精确报告生成语义版本
├── Algorithm         可选：算法细分
└── ReportProfile     可选：呈现细分
```

内部 builderIndexKey 从 DecisionKind 派生 AlgorithmFamily；调用方不能提供另一个 Family。ProductChannel 已不在公开路由键或 InterpretationInput 中。

### 7.1 AlgorithmFamily

AlgorithmFamily 描述“执行结果是怎样的一类机制”，例如：

- `factor_scoring`；
- `factor_norm`；
- `factor_classification`；
- `task_performance`。

它是最稳定的粗粒度扩展轴。具体 model code 不应直接成为 Registry 主键，否则每新增一个量表都要注册新 Builder。

### 7.2 DecisionKind

DecisionKind 描述“结果如何从分数或维度中被判定”，例如：

- `score_range`；
- `norm_lookup`；
- `pole_composition`；
- `trait_profile`；
- `nearest_pattern`；
- `dominant_factor`；
- `ability_level`。

相同 AlgorithmFamily 下可以存在多个 DecisionKind。例如 typology 的极点组合、最近模式和连续特质画像，共享分类机制族，但报告内容形态并不完全相同。

### 7.3 ReportType

ReportType 表示业务上要生成哪一类报告。当前仅实现 `standard`，但它已经进入 Builder 接口、Registry Key、Generation 幂等键和 Report 身份。

将来如果增加 clinician detail、participant summary 等独立报告成品，应先决定它们是不同 ReportType，还是同一 canonical Report 的 Audience 投影，不能仅靠新增模板字符串绕过报告身份设计。

### 7.4 TemplateVersion

TemplateVersion 标识一代不可变的报告生成语义。当前定义希望它同时覆盖：

- Builder 行为；
- 解释规则；
- 内容模板；
- Content schema。

它参与 Generation 幂等键：同一 Outcome、同一 ReportType、同一 TemplateVersion 只对应一个生成意图；新版本应产生新 Generation 和新 Report，而不是覆盖旧成品。

当前发布目录同时保留 `legacy-v1` 与 `2026-08-v1`。ModelCatalog active snapshot 显式冻结 TemplateID/TemplateVersion，Outcome 继续冻结同一组路由身份；
运行时只解析已发布 release，缺失或未知版本会 fail-closed。

### 7.5 Algorithm 与 ReportProfile

Algorithm 支持同一机制内的算法差异，ReportProfile 支持 scale/norm/task/personality_type/trait_profile 等呈现细分。默认 Builder 主要注册 canonical DecisionKind + ReportType + TemplateVersion，ReportProfile 由 DecisionKind 派生。

Registry 允许依次放宽可选 Algorithm/Profile 匹配，但始终保留相同 DecisionKind、ReportType 和 TemplateVersion；这不是缺失身份或跨模板版本 fallback。源码见[Registry](../../../internal/apiserver/domain/interpretation/rendering/registry.go)。

## 8. 五个容易混淆的“模板身份”

下面五个字段处在不同层次：

| 概念 | 示例 | 回答的问题 | 当前落点 |
| --- | --- | --- | --- |
| `TemplateVersion` | `legacy-v1`、`2026-08-v1` | 使用哪一代不可变报告生成语义？ | ModelCatalog snapshot、Outcome Input、Registry、Generation、Report、release manifest |
| `TemplateID` | `mbti`、`sbti`、`bigfive` | typology Builder 内选择哪个具体内容模板？ | 冻结 ReportInput -> Input.Report |
| `AdapterKey` | `personality_type`、`trait_profile` | 将 typology facts 交给哪种内置适配方式？ | 冻结 ReportInput -> Input.Report |
| `BuilderIdentity` | `factor-scoring`、`typology` | 实际是哪一个 Builder 实现生成的？ | Builder、Artifact、生成事件、日志 |
| `ContentSchemaVersion` | `report-content/v1` | Draft/Report Content 使用什么结构？ | Builder、Artifact、生成事件 |

### 8.1 TemplateVersion 不是 TemplateID

`legacy-v1` 表示整套报告生成语义的兼容发布；`mbti` 表示 TypologyBuilder 内的一个具体内容模板。一个 TemplateVersion 可以包含多个 TemplateID。

如果 MBTI 文案或结构发生不兼容变化，不能只在同一个 TemplateID 后面偷偷更换代码；应判断是否需要新 TemplateVersion，保证历史 Outcome 重放仍然可以定位旧语义。

### 8.2 AdapterKey 不是 Algorithm

Algorithm 描述模型如何计算，AdapterKey 描述 Interpretation 使用哪类内置报告适配器。当前支持 personality_type 与 trait_profile；DefinitionV2 的显式 AdapterKey 优先，否则按已验证 ReportKind 派生，template kind 缺 AdapterKey 拒绝。它不会从 mbti/sbti/bigfive 算法字符串猜测 Adapter。

ModelCatalog 发布时校验 adapter、algorithm、DecisionKind、TemplateID 的兼容性，冻结 TypologyRouting；生产 Builder 读取冻结 AdapterKey 与 TemplateID，未知或不兼容选择失败。事实入口：[ResolvedAdapterKey](../../../internal/apiserver/port/modelcatalog/payload/typology/spec.go)、[报告路由](../../../internal/apiserver/port/modelcatalog/payload/typology/report_routing.go)、[模板注册表](../../../internal/apiserver/domain/interpretation/typology/patterns/template_registry.go)。

### 8.3 BuilderIdentity 不是 Builder 路由键

Registry 根据机制 Key 选择 Builder，而不是根据 `BuilderIdentity` 反查。BuilderIdentity 是执行证据，用来回答“最终是哪段实现生成了内容”。

当前它已经进入 InterpretReport artifact、Mongo 持久化、`interpretation.report.generated` 事件和执行日志。Committer 会校验 Artifact 与本次 Builder 声明一致；
历史缺失字段恢复为显式 unknown，而不是伪装成当前 Builder。

### 8.4 ContentSchemaVersion 不是 TemplateVersion

同一 Content schema 可以被多个 Builder 或 TemplateVersion 复用；反过来，升级 Content schema 也通常需要评估是否发布新 TemplateVersion。

当前所有默认 Builder 都返回 `report-content/v1`。这个值同时保存在 Report 成品与生成事件中，并在成功提交时做一致性校验。

## 9. Registry 注册契约

Builder 接口要求：

```go
type Builder interface {
    ReportType() policy.ReportType
    TemplateVersion() policy.TemplateVersion
    BuilderIdentity() string
    ContentSchemaVersion() string
    Build(context.Context, InterpretationInput) (*report.Draft, error)
}
```

要进入 Registry，Builder 还必须实现 `KeyedBuilder`；一个 Builder 支持多个机制键时，可以实现 `MultiKeyedBuilder`。

注册阶段拒绝以下错误：

- nil Builder；
- 没有暴露 MechanismKey；
- ReportType 为空；
- TemplateVersion 为空；
- BuilderIdentity 为空；
- ContentSchemaVersion 为空；
- MechanismKey 的 TemplateVersion 与 Builder 声明不一致；
- 两个 Builder 注册完全相同的 Key。

这些检查把路由冲突提前到进程装配阶段，而不是等到某个用户提交测评后才暴露。

当前 Builder 由组合根显式构造并注册，没有使用动态插件发现：

```text
DefaultBuilders
├── FactorScoringBuilder
├── TypologyBuilder
├── NormProfileBuilder
└── TaskPerformanceBuilder
```

对当前模块规模而言，手工注册清晰、可追踪，也能在启动时验证冲突。只有当 Builder 数量、独立团队或可插拔部署需求显著增加时，才有必要引入更复杂的自动注册机制。

## 10. Registry 解析与回落

### 10.1 核心路由身份必须显式存在

从 InterpretationInput 构造 RoutingContext 时，当前实现会：

- ReportType 为空 -> 拒绝；
- TemplateVersion 为空 -> 拒绝；
- DecisionKind 为空或不能映射 AlgorithmFamily -> 拒绝；
- ReportProfile 为空 -> 按 DecisionKind 推导。

ReportProfile 是可派生的呈现细分键；ReportType、TemplateVersion 与 DecisionKind 是不可猜测的核心身份。路由上下文无效时，执行器会明确归类失败，不会选择默认版本或任意 Builder。

### 10.2 从具体键放宽可选细分

Registry 的候选顺序只放宽 Algorithm/ReportProfile：

```text
decision + type + version + algorithm + profile
  -> decision + type + version + algorithm
  -> decision + type + version + profile
  -> decision + type + version
```

这些候选始终保留同一 DecisionKind、ReportType 与 TemplateVersion。不存在 family-only 候选，也不接受 ProductChannel。新模型 code 无需注册 Builder；同一机制的呈现差异可通过可选 Algorithm/Profile 细分。

### 10.3 TemplateVersion 永不跨版本回落

每个 fallback candidate 都保留原始 TemplateVersion。因此：

```text
请求 legacy-v2
  X 不会因为没有 v2 Builder 而回落到 legacy-v1
```

这是非常重要的不变量。跨 TemplateVersion 静默回落会让 Generation 身份声称使用 v2，实际内容却由 v1 生成，历史审计将失去意义。

### 10.4 新 DecisionKind 必须显式接入

当前 Key 必须包含合法 DecisionKind，注册和解析时从它派生 Family并校验；无 route 则 builder_not_found，不接受旧 family Builder 静默处理未知机制。一个 Builder 支持多个 DecisionKind 时通过 MultiKeyedBuilder 显式逐个声明。

新增 DecisionKind 需要注册具体 Key、冻结输入/内容契约和发布兼容测试；不能仅增加 Family 映射就视为报告链路完成。

## 11. 当前四类 Builder

| Builder | 路由机制 | 专用输入 | 当前实现特点 |
| --- | --- | --- | --- |
| `factor-scoring` | factor_scoring + score_range | FactorScoringFacts | 组装总分、等级、因子分、结论和建议 |
| `norm-profile` | factor_norm + norm_lookup | FactorScoringFacts | 当前复用 FactorScoringBuilder 的组装能力 |
| `task-performance` | task_performance + ability_level | FactorScoringFacts | 当前复用 FactorScoringBuilder 的组装能力 |
| `typology` | factor_classification + 四种 DecisionKind | PersonalityTypeFacts 或 TraitProfileFacts | 内部再选择 Adapter 与 Template |

### 11.1 FactorScoringBuilder

它把冻结 Factor model 与已提交 Outcome factor scores 交给 scoring assembler，生成量表类报告 Draft。具体文案只按冻结 OutcomeCode 查 InterpretationAssets；缺资产、缺代码或找不到对应文案时返回明确错误，不重匹配区间、不取末条规则、不按 risk level 发明结论或建议。

这使 Outcome 决定结果、Interpretation 组织文案的边界在失败路径也成立。资产和结果身份必须随模板/模型发布冻结，修改当前文案不能改变历史报告重放。规则由 [`scoring/scale_interpret.go`](../../../internal/apiserver/domain/interpretation/scoring/scale_interpret.go) 与直接回归测试保护。

### 11.2 NormProfileBuilder

它拥有独立 BuilderIdentity 和机制键，但当前委托 FactorScoringBuilder 构建 Draft。这说明：

- 生命周期和 Registry 已经能区分常模报告机制；
- 当前成品结构仍与因子计分报告相同；
- 后续若常模报告需要分布图、常模说明或专用章节，可以在不改变统一主链路的情况下替换其内部实现。

### 11.3 TaskPerformanceBuilder

它同样拥有独立机制身份，当前复用因子报告结构。未来增加正确率、反应时分布、任务阶段或能力雷达图时，应由 TaskPerformanceBuilder 吸收，而不是把认知任务分支写进 Executor。

### 11.4 TypologyBuilder

TypologyBuilder 当前支持四个 DecisionKind：

- pole composition；
- trait profile；
- nearest pattern；
- dominant factor。

它先根据输入存在 `PersonalityTypeFacts` 还是 `TraitProfileFacts` 选择内容组装路径，再按 AdapterKey 与 TemplateID 选择具体模板。

TemplateID 的当前解析策略是：

```text
TemplateID 精确命中
  -> 使用对应模板
TemplateID 缺失或未命中
  -> 拒绝路由，不生成报告
```

AdapterKey 只用于已发布 manifest 内的精确适配，不再承担“缺模板时猜测一个通用报告”的职责。ModelCatalog 发布校验与 Interpretation 运行时共同保护这条 fail-closed 契约。

## 12. Builder 的纯函数边界

一个合格 Builder 应近似满足：

```text
Build(frozen InterpretationInput, fixed builder/template code)
  -> deterministic Draft
```

因此 Builder 不应：

- 查询当前 ModelCatalog；
- 重新加载 Assessment 或 AnswerSheet；
- 调用外部 AI、随机数或当前时间生成正文；
- 创建 Report ID；
- 创建或修改 ReportGeneration / InterpretationRun；
- 保存 Report、Catalog 或 Outbox；
- 根据当前用户身份裁剪 Audience 内容；
- 修改 Outcome 中的结果事实。

当前默认 Builder 没有仓储依赖，也不持有生命周期服务，符合这个总体边界。

### 12.1 可重放不只取决于“没有数据库查询”

即使 Builder 是纯内存函数，以下变化仍会使同一输入产生不同内容：

- 修改默认解释文案；
- 修改区间回落规则；
- 修改 Adapter 默认选择；
- 修改 TemplateID 对应模板；
- 修改 Content 的组装顺序或字段语义。

所以真正的重放契约是“冻结输入 + 可定位的不可变 Builder/模板语义”，而不只是“Builder 不查数据库”。当前二进制保留两代 release manifest 和对应版本化 Builder 包装，发布模型显式冻结版本；
未来再发布新版本时，仍必须保留需要重放的旧 manifest 与实现，不能只改动当前版本常量。

## 13. 生产生成与运营 Preview

ModelCatalog 的 typology 预览会：

1. 用未发布模型构造一个临时 Assessment；
2. 在进程内执行 typology executor；
3. 构造临时 InterpretationInput；
4. 直接调用 `NewTypologyBuilder().Build`；
5. 返回分数与 Draft。

它刻意不进入生产 Interpretation 生命周期：

- 不消费 committed Outcome；
- 不创建 Generation / Run；
- 不持久化 InterpretReport；
- 不发送生成事件；
- 不触发重试治理。

这个边界是正确的：运营预览未发布模型时，不应制造正式测评和正式报告事实。

当前 Preview 与生产路径都使用默认 Builder Registry 和 `ResolveByMechanism`，新增特化路由不会再因直接构造 TypologyBuilder 而漂移。
Preview 仍不调用生产 Executor 或生命周期服务，只共享纯 Builder resolver 与内容构建机制。

## 14. 新增报告能力时怎样判断扩展点

### 14.1 新增一个已有机制的模型

例如新增一份采用 factor scoring + score range 的医学量表：

```text
发布模型冻结因子和解释规则
  -> Evaluation 产出标准 Outcome dimensions
  -> 现有 Outcome adapter
  -> 现有 FactorScoringBuilder
```

不应新增 model-code switch，也不应新增 Builder。

### 14.2 同一机制增加一种明确模板

例如人格类型报告增加一个新的内容模板：

1. 在 ModelCatalog 发布定义中冻结明确 TemplateID / AdapterKey；
2. 在 Interpretation 模板注册表增加对应模板；
3. 非空未知 TemplateID 必须失败；
4. 判断变化是否需要新 TemplateVersion；
5. 为生产和 Preview 增加一致性测试。

如果只是配置了新的 outcome 文案，而结构与模板行为未变，通常只需冻结发布素材，不需要新 Builder。

### 14.3 新增一种结果判定形态

如果 AlgorithmFamily 不变但 DecisionKind 新增：

1. 定义标准 Outcome facts；
2. 确认 InterpretationInput 是否已有充分表达；
3. 为现有 Builder 注册新 Key，或新增专用 Builder；
4. 禁止依赖 family-only fallback 偶然接入；
5. 验证错误输入会明确失败。

### 14.4 新增一种报告内容结构

如果现有 `report-content/v1` 无法表达，应：

1. 定义新 ContentSchemaVersion；
2. 发布新 TemplateVersion；
3. 决定旧 TemplateVersion 的 Builder 是否继续保留；
4. 确保新旧 Report 可以并存查询；
5. 更新客户端对 schema 的兼容策略。

不能只改 Report Content 字段而继续宣称 `report-content/v1`。

### 14.5 新增一个报告业务类型

增加 ReportType 会影响：

- Registry Key；
- Generation 幂等键；
- Report artifact 身份；
- 查询 catalog 的唯一性和当前报告选择；
- 客户端接口。

因此它不是“再注册一个模板”这么简单，必须连同生命周期和查询模型一起设计。

## 15. 已收敛边界与剩余改进

本篇早期识别的发布路由、成品来源与未知模板回落已经关闭。这里区分已成立事实和仍需继续跟踪的边界。

### 15.1 TemplateVersion 发布路由已关闭

当前 5 个 TemplateID 各有 `legacy-v1` 与 `2026-08-v1` 两个 release，ModelCatalog active snapshot 显式冻结 TemplateID/TemplateVersion，Outcome 继续冻结该身份。
发布、运行时路由与二进制 manifest 会互相校验。

后续发布新版本仍要遵守：新建 manifest、保留仍需重放的历史 manifest、冻结模型路由、跑 manifest 覆盖与 checksum 门禁，不能原地修改旧 release。

### 15.2 Report 成品来源自证已关闭

BuilderIdentity 与 ContentSchemaVersion 已固化到 Artifact，并与 generated event 一致提交。历史无来源字段只映射为 legacy/unknown，不借当前 Builder 猜测历史来源。

### 15.3 路由严格性已关闭

缺失 TemplateID/TemplateVersion、未知 release、非法 runtime spec 和不匹配的模型类型会在发布或生成阶段失败；运行时不再使用默认版本、目录猜测或未知 TemplateID 的通用回落。

### 15.4 仍需关注：Catalog 粒度

当前只运营 `standard` ReportType，`report_query_catalog` 仍以 Assessment 为一行当前来源。如果未来让多个 ReportType 或多个版本同时成为可查询成品，必须先扩展 Catalog 业务身份和查询契约。

### 15.5 仍需关注：旧 release 的长期保留

双版本目录使当前历史 Outcome 可重放，但未来代码清理不能仅根据“新写入只用当前版本”删除 `legacy-v1`。退出旧 release 需要先证明没有重放、恢复或历史补算需求，并取得单项数据治理决策。

### 15.6 仍需关注：解释规则区间契约

解释规则的完整性应继续由发布期验证和运行期明确失败保护，避免区间缺口、重叠或无序规则被内容回落掩盖。

### 15.7 仍需关注：Preview 与生产解析一致性

Preview 继续保持无持久化、无 Generation/Run，但新增 release 或特化路由时必须同时验证 Preview 与生产 resolver，不得形成两套内容选择规则。

### 15.8 仍需关注：多 ReportType

新增 ReportType 会改变 Generation 身份、Artifact、Catalog 和客户端契约；不能只注册一份模板就宣称完成。当前单一 `standard` 是明确的产品边界。

## 16. 必须保护的不变量

### 16.1 冻结输入不变量

- 生产报告只从 committed Outcome Record 构建输入；
- 不读取当前 AssessmentModel 替代 ReportInput；
- Outcome Payload 保存结果事实，ReportInput 保存报告所需冻结素材；
- schema v2 类型编码只能在同一 Record 的冻结输入内解析；
- 新 Outcome 必须显式保存完整模型与运行机制身份；
- Outcome schema 2 / ReportInput schema 3 的严格契约拒绝旧格式、缺失身份与默认模板补造。

### 16.2 路由不变量

- DecisionKind、ReportType 和 TemplateVersion 是核心路由身份；AlgorithmFamily 仅为派生属性；
- model code 不进入默认 Builder 路由；
- 重复 Key 在装配阶段失败；
- TemplateVersion 绝不跨版本 fallback；
- 非空未知的显式选择不应静默降级；
- 新 DecisionKind 必须通过显式测试证明由哪个 Builder 接入。

### 16.3 Builder 不变量

- Builder 只把 InterpretationInput 组装为 Draft；
- Builder 不访问仓储和外部可变状态；
- Builder 不拥有 Generation、Run、Report 提交和事件发送；
- Builder 不重新计分、不改变 Outcome 结论；
- BuilderIdentity 和 ContentSchemaVersion 非空且稳定；
- 同一 TemplateVersion 下的行为变化必须保持兼容，否则发布新版本。

### 16.4 历史重放不变量

- 新模型发布不能改变旧 Outcome 的报告输入；
- 自动重试不能读取新的解释素材；
- 同一冻结输入与同一模板语义应产生确定性内容；
- 新 TemplateVersion 产生新 Generation 和新成品，不覆盖旧成品；
- 报告应能追溯 Outcome、ModelVersion、TemplateVersion、Builder 和 ContentSchema。

## 17. 面试与设计追问

### 17.1 为什么不让 Builder 直接查询 ModelCatalog？

因为它会把报告生成绑定到“当前配置”，使失败重试和历史重放发生语义漂移。冻结 ReportInput 让报告只依赖测评发生时的发布资产，同时让 Builder 保持确定性和可测试性。

### 17.2 为什么 Outcome Payload 和 ReportInput 要分开？

Outcome Payload 是机器判定已经成立的结果事实；ReportInput 是解释这些事实所需的历史素材。分开后 Evaluation 不需要拥有报告正文结构，Interpretation 也不需要重新计算结果。

### 17.3 为什么按 AlgorithmFamily + DecisionKind 路由，而不是按 model code？

model code 是具体业务资产，数量会持续增长；AlgorithmFamily 与 DecisionKind 是可复用机制。按机制路由可以让同类模型配置化接入，只有异类结果形态才扩展 Builder。

### 17.4 为什么还需要 TemplateVersion？代码和 Git commit 不能追溯吗？

Git commit 只能说明部署过什么代码，不能成为业务 Generation 的稳定身份，也不能让新旧报告在数据中并存。TemplateVersion 把一代报告语义显式带入 Generation 和 Report，才能支持业务级幂等、重放和审计。

### 17.5 Builder 与设计模式中的 Builder 有什么关系？

这里的 Builder 更接近“策略 + 内容构建器”：Registry 根据机制键选择策略，具体 Builder 把结构化输入组装为 Draft。它不是为了逐步构造复杂对象而暴露 fluent API。
命名强调的是“只构建内容、不拥有生命周期”，解释时不必生硬套用经典 GoF Builder 模式。

### 17.6 Registry 的 fallback 是不是越灵活越好？

不是。fallback 只适合从产品特化回落到经过验证的通用机制。跨版本回落、未知 TemplateID 回落、未知 DecisionKind 被 family-only Builder 接收，都会把配置错误隐藏成看似成功的报告。

## 18. 代码与验证入口

| 主题 | 代码入口 |
| --- | --- |
| InterpretationInput | [`domain/interpretation/input/input.go`](../../../internal/apiserver/domain/interpretation/input/input.go) |
| Outcome -> Input 适配 | [`application/interpretation/automation/input`](../../../internal/apiserver/application/interpretation/automation/input/) |
| Outcome Record | [`port/evaluationfact/fact.go`](../../../internal/apiserver/port/evaluationfact/fact.go) |
| Outcome / ReportInput codec | [`port/evaluationfact/codec`](../../../internal/apiserver/port/evaluationfact/codec/) |
| Registry 与 fallback | [`domain/interpretation/rendering/registry.go`](../../../internal/apiserver/domain/interpretation/rendering/registry.go) |
| 默认 Builders | [`domain/interpretation/rendering/builders.go`](../../../internal/apiserver/domain/interpretation/rendering/builders.go) |
| 因子报告解释 | [`domain/interpretation/scoring`](../../../internal/apiserver/domain/interpretation/scoring/) |
| typology Adapter 与模板 | [`domain/interpretation/typology/patterns`](../../../internal/apiserver/domain/interpretation/typology/patterns/) |
| Preview 组合边界 | [`container/modules/modelcatalog/preview`](../../../internal/apiserver/container/modules/modelcatalog/preview/) |
| 生产执行器 | [`application/interpretation/automation/execution/executor.go`](../../../internal/apiserver/application/interpretation/automation/execution/executor.go) |
| 生成事件溯源字段 | [`domain/interpretation/events_outcome.go`](../../../internal/apiserver/domain/interpretation/events_outcome.go) |

```bash
go test ./internal/apiserver/port/evaluationfact/...
go test ./internal/apiserver/application/interpretation/automation/input
go test ./internal/apiserver/domain/interpretation/rendering
go test ./internal/apiserver/domain/interpretation/scoring
go test ./internal/apiserver/domain/interpretation/typology/...
go test ./internal/apiserver/container/modules/modelcatalog/preview
```
