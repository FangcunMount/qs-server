# 核心设计：冻结输入、Builder 与模板路由

> 状态：本文已按当前源码重写。它描述的是报告生成的输入与扩展机制，不展开 Generation、Run、重试和可靠提交；这些生命周期问题由下一篇文档说明。

## 1. 本文回答

本文集中回答五个问题：

1. 为什么报告生成必须使用 Evaluation 提交时冻结的事实，而不能重新查询当前 ModelCatalog；
2. Outcome 结果事实、ReportInput 报告素材和 InterpretationInput 分别是什么；
3. Interpretation 怎样兼容不同 Outcome schema，又不让 Builder 感知历史存储格式；
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
FromOutcomeRecord                 防腐与兼容适配边界
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
AlgorithmFamily + DecisionKind + ReportType + TemplateVersion
+ optional Algorithm / ProductChannel / ReportProfile
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
| Outcome 历史 schema 不同 | Evaluation fact codec、Outcome adapter | 否 |
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

## 4. 三层输入模型

### 4.1 第一层：Outcome Record

Evaluation 提交的只读 Record 是跨模块事实边界。与本文相关的字段包括：

| 字段 | 含义 | Interpretation 的用法 |
| --- | --- | --- |
| `ID` | Outcome 唯一身份 | 形成 Generation 来源与追踪关系 |
| `OrgID` | 组织身份 | 形成报告关联事实，不直接授予访问权限 |
| `AssessmentID` | 测评身份 | 关联报告与查询索引 |
| `TesteeID` | 受试者身份 | 形成参与者查询范围 |
| `Model` | 模型 kind、subkind、algorithm、code、version、title | 构建报告模型身份 |
| `Runtime` | AlgorithmFamily、DecisionKind、PayloadFormat | 解析 Builder 与解码 payload |
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
| `Model` | kind、subkind、algorithm、code、version、title、channel、family | 报告展示和追踪 |
| `Runtime` | AlgorithmFamily、DecisionKind、PayloadFormat | Builder 机制路由 |
| `Result` | Primary、Level | 已成立的总结果事实 |
| `Report` | type、version、algorithm、channel、profile、adapter、template | 报告路由与模板选择 |
| `FactorScoring` | 因子模型与因子结果 | 因子计分、常模与任务类报告 |
| `PersonalityType` | 类型详情 | 人格类型类报告 |
| `TraitProfile` | 特质详情 | 连续特质画像类报告 |

Builder 只依赖这一层。它不需要知道 Outcome 是 schema v1 还是 v2，也不需要理解 MongoDB 中的 JSON 结构。

## 5. FromOutcomeRecord 是防腐边界

生产报告链路的关键适配器是 `application/interpretation/automation/input.FromOutcomeRecord`。它承担四类工作。

### 5.1 解码并验证跨模块事实

适配器先调用：

```text
DecodeExecution(record)
DecodeReportInput(record)
```

任一步解码失败，报告生成不会带着半完整数据继续执行。由于输入映射发生在 Starter 创建 Generation / Run 之前，这类错误当前不会形成 InterpretationRun；它属于生产链路中仍需在可观测性上继续补强的边界。

### 5.2 补齐历史兼容身份

旧 Outcome 可能没有显式保存完整运行机制。当前适配器提供有限兼容：

- AlgorithmFamily 为空时，尝试根据 kind、subkind、algorithm 推导；
- DecisionKind 为空时，根据 AlgorithmFamily 填入默认 DecisionKind；
- 旧 scale 模型 algorithm 为空时，兼容为 `scale_default`；
- 旧 typology 模型 algorithm 为空时，兼容为 `personality_typology`；
- ReportProfile 根据 DecisionKind 推导。

这些是读取历史数据的兼容策略，不是新数据可以继续省略身份的理由。新的 Outcome 应显式冻结完整运行时身份；否则一个“默认值”将同时承担历史兼容和当前业务规则，后续很难安全演进。

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

## 6. schema v1、schema v2 与历史兼容

### 6.1 版本化的目标

Outcome schema 版本回答的是“持久化结果事实怎样解码”，不等于报告模板版本：

| 版本概念 | 保护对象 |
| --- | --- |
| Outcome `SchemaVersion` | Evaluation Payload 的数据结构 |
| `PayloadFormat` | 运行机制产生的 payload 格式身份 |
| `TemplateVersion` | 一代完整报告生成语义 |
| `ContentSchemaVersion` | 最终报告 Content 的结构 |

它们可以独立演进，不能用一个 `v1/v2` 同时指代所有层次。

### 6.2 typology schema v1

历史 typology payload 可能已经包含大量报告详情，例如类型名称、一句话描述、优势、弱点、建议、图片和来源信息。codec 会恢复为 `PersonalityTypeDetail` 或 `TraitProfileDetail`，适配器再直接转成 Interpretation facts。

这条路径的意义是兼容已存在事实，而不是鼓励 Evaluation 继续产出报告文案。

### 6.3 typology schema v2

schema v2 将 Evaluation 事实收敛为更小的分类事实，例如：

```text
ClassificationFact
├── TypeCode
├── Pattern
├── MatchPercent / Similarity
├── IsSpecial
└── SpecialTrigger
```

Interpretation 使用 `TypeCode` 在同一 Outcome 的冻结 typology ReportInput 中查找名称、说明、画像、建议、图片和来源信息。

关键点是：

> 查找发生在历史冻结输入内，不回退到当前 ModelCatalog。

如果 TypeCode 在冻结输入中不存在，当前实现直接返回错误。这比读取当前配置“尽量生成一份报告”更可靠，因为后者会掩盖 Outcome 与发布模型不一致的问题。

### 6.4 因子模型与常模素材

因子计分报告从冻结 snapshot 恢复因子 code、title、max score、total 标识和解释规则，再把 Outcome dimensions 中的分数与其结合。

常模报告还会使用冻结常模表和 Outcome 已有的 T 分数恢复 conclusion、suggestion 等展示内容。当前代码在个别维度缺少 Level 时还会补齐 Level code/label。这个行为已经接近“重新判定结果”的边界，后续应明确：

- Interpretation 可以根据 Outcome level 选择文案；
- Interpretation 不应在 Outcome 未给出 level 时重新决定 level；
- 如果报告必需 level，Evaluation 应把它作为完整结果事实提交。

因此，本文把当前实现记录为事实，但不把“Interpretation 补算 Level”固化为目标设计。

## 7. 报告路由的七个身份

Registry 的路由键由四个核心字段和三个可选细分字段组成：

```text
Key
├── AlgorithmFamily   必需：计算机制族
├── DecisionKind      必需：结果判定形态
├── ReportType        必需：报告业务类型
├── TemplateVersion   必需：报告生成语义版本
├── Algorithm         可选：具体算法身份
├── ProductChannel    可选：产品渠道
└── ReportProfile     可选：呈现形态
```

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

当前生产适配器固定使用 `legacy-v1`，ModelCatalog 尚未发布或绑定明确的报告模板版本。因此目前只是建立了版本身份骨架，还没有完成真正的模板资产版本治理。

### 7.5 Algorithm、ProductChannel 与 ReportProfile

三个可选字段用于在共享机制内进一步区分呈现：

| 字段 | 适合解决的问题 | 不应承担的职责 |
| --- | --- | --- |
| Algorithm | 同一机制族中算法特有的报告差异 | 代替 AlgorithmFamily |
| ProductChannel | 医疗、行为干预等渠道的产品呈现差异 | 代替组织授权或前端 Audience |
| ReportProfile | scale、norm、task、personality_type、trait_profile 等内容形态 | 代替 DecisionKind |

当前默认 Builder 主要注册在 AlgorithmFamily + DecisionKind 层，三个字段更多是未来的精细路由能力。ReportProfile 当前由 DecisionKind 推导。

## 8. 五个容易混淆的“模板身份”

下面五个字段处在不同层次：

| 概念 | 示例 | 回答的问题 | 当前落点 |
| --- | --- | --- | --- |
| `TemplateVersion` | `legacy-v1` | 使用哪一代不可变报告生成语义？ | Input、Registry、Generation、Report |
| `TemplateID` | `mbti`、`sbti`、`bigfive` | typology Builder 内选择哪个具体内容模板？ | 冻结 ReportInput -> Input.Report |
| `AdapterKey` | `personality_type`、`trait_profile`、`mbti` | 将 typology facts 交给哪种内置适配方式？ | 冻结 ReportInput -> Input.Report |
| `BuilderIdentity` | `factor-scoring`、`typology` | 实际是哪一个 Builder 实现生成的？ | Builder、生成事件、日志 |
| `ContentSchemaVersion` | `report-content/v1` | Draft/Report Content 使用什么结构？ | Builder、生成事件 |

### 8.1 TemplateVersion 不是 TemplateID

`legacy-v1` 表示整套报告生成语义的兼容发布；`mbti` 表示 TypologyBuilder 内的一个具体内容模板。一个 TemplateVersion 可以包含多个 TemplateID。

如果 MBTI 文案或结构发生不兼容变化，不能只在同一个 TemplateID 后面偷偷更换代码；应判断是否需要新 TemplateVersion，保证历史 Outcome 重放仍然可以定位旧语义。

### 8.2 AdapterKey 不是 Algorithm

Algorithm 描述模型如何计算；AdapterKey 描述 Interpretation 怎样选择内置报告适配器。当前没有显式 AdapterKey 时，TypologyBuilder 会根据 model algorithm 回退：

- `mbti` -> MBTI adapter；
- `sbti` -> SBTI adapter；
- `bigfive` -> BigFive adapter；
- 其他 -> 通用 personality type 或 trait profile adapter。

这种回退用于兼容旧输入。新发布模型应尽量冻结明确 AdapterKey，避免 Interpretation 从算法名称猜测呈现策略。

### 8.3 BuilderIdentity 不是 Builder 路由键

Registry 根据机制 Key 选择 Builder，而不是根据 `BuilderIdentity` 反查。BuilderIdentity 是执行证据，用来回答“最终是哪段实现生成了内容”。

当前它已经进入 `interpretation.report.generated` 事件和执行日志，但没有固化进 InterpretReport artifact。若事件丢失或仅持有 Report 正文，无法从成品自身确认具体 Builder；这是后续成品可追溯性需要补强的地方。

### 8.4 ContentSchemaVersion 不是 TemplateVersion

同一 Content schema 可以被多个 Builder 或 TemplateVersion 复用；反过来，升级 Content schema 也通常需要评估是否发布新 TemplateVersion。

当前所有默认 Builder 都返回 `report-content/v1`。这个值进入生成事件，但同样没有保存在 Report 成品中。

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

### 10.1 先补齐最小默认值

从 InterpretationInput 构造 RoutingContext 时，当前实现会：

- ReportType 为空 -> `standard`；
- TemplateVersion 为空 -> `legacy-v1`；
- DecisionKind 为空 -> 按 AlgorithmFamily 取兼容默认值；
- ReportProfile 为空 -> 按 DecisionKind 推导。

如果 AlgorithmFamily 或最终 DecisionKind 仍为空，路由上下文无效，执行器会将其归类为不支持的机制，而不是随便选择一个 Builder。

### 10.2 从最具体键回落到通用键

Registry 会先尝试完整 Key，再逐步去掉可选细分字段。可以把当前候选顺序概括为：

```text
family + decision + type + version + algorithm + channel + profile
  -> 去掉部分 profile / channel / algorithm 组合
  -> family + decision + type + version
  -> family + type + version
```

这一策略支持“通用机制 Builder + 少量产品特化 Builder”：

- 没有特化 Builder 时，同机制回落到通用实现；
- 某个 algorithm、channel 或 profile 需要特殊报告时，可以注册更具体 Key；
- 新模型 code 不需要注册新 Builder。

### 10.3 TemplateVersion 永不跨版本回落

每个 fallback candidate 都保留原始 TemplateVersion。因此：

```text
请求 legacy-v2
  X 不会因为没有 v2 Builder 而回落到 legacy-v1
```

这是非常重要的不变量。跨 TemplateVersion 静默回落会让 Generation 身份声称使用 v2，实际内容却由 v1 生成，历史审计将失去意义。

### 10.4 family-only 回落必须谨慎

最后一级候选允许 `AlgorithmFamily + ReportType + TemplateVersion`，不再包含 DecisionKind。它为一个 Builder 兼容整个机制族提供了空间，但也带来风险：

- 新增一个 DecisionKind 时，可能被旧的 family-level Builder 静默接收；
- Builder 实际并不了解这种新结果形态，却没有得到 `builder_not_found`；
- 扩展错误会表现为内容缺失或默认展示，而不是明确失败。

因此，family-only Builder 必须主动声明它真的能处理该机制族内所有 DecisionKind。对于结果形态差异明显的 family，应优先注册明确的 DecisionKind 键。

## 11. 当前四类 Builder

| Builder | 路由机制 | 专用输入 | 当前实现特点 |
| --- | --- | --- | --- |
| `factor-scoring` | factor_scoring + score_range | FactorScoringFacts | 组装总分、等级、因子分、结论和建议 |
| `norm-profile` | factor_norm + norm_lookup | FactorScoringFacts | 当前复用 FactorScoringBuilder 的组装能力 |
| `task-performance` | task_performance + ability_level | FactorScoringFacts | 当前复用 FactorScoringBuilder 的组装能力 |
| `typology` | factor_classification + 四种 DecisionKind | PersonalityTypeFacts 或 TraitProfileFacts | 内部再选择 Adapter 与 Template |

### 11.1 FactorScoringBuilder

它把冻结 Factor model 与 Outcome factor scores 交给 scoring assembler，生成量表类报告 Draft。具体因子解释优先使用冻结解释规则；没有配置结论时，代码会按 risk level 生成默认文案。

当前解释规则存在一个兼容回落：如果没有区间精确命中但规则列表非空，会使用最后一条规则。这个行为可能掩盖规则区间缺口，目标设计更适合显式失败或由发布校验保证区间完整。

默认文案本身也是报告语义。只要它仍由代码生成，就必须受 TemplateVersion 约束；否则修改一段 Go 文案也会改变历史重放结果。

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
未命中
  -> 根据 AdapterKey 回落
仍未命中
  -> 使用通用模板
```

这里存在一个值得治理的隐患：未知且非空的 TemplateID 与“未指定 TemplateID”目前没有区别，都会静默回落。运营配置拼写错误可能不会阻止发布或生成，只会产生通用报告。更稳妥的契约是：

- TemplateID 为空：允许按 AdapterKey 选择默认模板；
- TemplateID 非空但不存在：明确失败，不能静默回落。

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

所以真正的重放契约是“冻结输入 + 可定位的不可变 Builder/模板语义”，而不只是“Builder 不查数据库”。TemplateVersion 正是为后半部分建立身份，但当前代码还没有保留多代 Builder 实现，也没有从发布模型解析版本，能力尚未闭环。

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

但当前 Preview 直接构造 TypologyBuilder，生产路径通过 Registry 解析，两条路径的 Builder 选择可能随未来扩展发生漂移。更合理的演进方向是共享一个“无持久化的 Builder resolver / renderer”，而不是让 Preview 调用生产 Executor 或生命周期服务。

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

## 15. 当前设计问题与后续改进

本篇发现的问题已在 [设计问题与重构清单](./90-设计问题与重构清单.md) 汇总。这里先说明与输入和路由直接相关的项目。

### 15.1 TemplateVersion 尚未成为发布资产

当前生产适配器统一写入 `legacy-v1`，ModelCatalog 没有在模型发布时明确绑定报告模板版本。结果是：

- Generation 虽然具备版本身份，但业务发布不能选择它；
- 修改 Builder 或模板代码时，没有旧版本实现可供历史重放；
- `legacy-v1` 容易变成一个长期不变的名字，内部行为却持续变化。

目标是让发布版本明确冻结报告生成语义，并能保留至少仍需重放的历史版本实现。

### 15.2 Report 成品缺少生成实现自证

BuilderIdentity 与 ContentSchemaVersion 已进入 generated event，但 InterpretReport 本身没有保存它们。建议将二者固化到成品元数据，使单独读取 Report 也能回答：

- 哪个 Builder 生成；
- 使用什么内容 schema；
- 与 TemplateVersion 是否匹配。

### 15.3 typology runtime spec 解码错误被忽略

适配器当前只在 `ToRuntimeSpec()` 成功时写入 TemplateID 和 AdapterKey，失败时继续生成。这样可能把有问题的冻结配置降级为算法默认 Adapter。

目标契约应区分：

- 历史输入确实没有 runtime spec：走明确兼容分支；
- 输入声明存在 spec 但格式非法：报告生成失败，并留下可治理错误。

### 15.4 未知 TemplateID 静默回落

非空错误 TemplateID 应被视为配置或冻结输入错误，不应与空值使用相同默认逻辑。

### 15.5 解释规则区间缺口被最后一条规则掩盖

`findInterpretRuleWithRangeFallback` 在无命中时使用最后一条规则。这会让缺口、重叠或无序规则难以暴露。优先改进方向是发布期验证区间完整性，运行期无命中时明确失败或使用有业务定义的默认规则。

### 15.6 常模映射可能补写结果等级

Interpretation 当前可能根据冻结常模和 T 分数补 Level。这与“Outcome 决定结果，Interpretation 只组织解释”的目标边界存在张力。应把缺失的 Level 修正到 Evaluation 事实，而不是长期由报告层兜底。

### 15.7 输入解码失败缺少 Generation / Run 证据

FromOutcomeRecord 在 Starter 之前执行。若 Payload 或 ReportInput 解码失败，没有 InterpretationRun 记录当前停在哪一步。后续应考虑建立可审计的准入失败事实，同时避免为明显非法或不存在的 Outcome 制造错误 Generation。

### 15.8 family-only fallback 可能误接新机制

Registry 应增加测试或注册约束，防止未来新增 DecisionKind 被过宽 Builder 静默承接。

### 15.9 Preview 与生产解析可能漂移

Preview 应继续保持无持久化、无生产生命周期，但可以与生产共享纯 Builder resolver 和输入校验规则。

## 16. 必须保护的不变量

### 16.1 冻结输入不变量

- 生产报告只从 committed Outcome Record 构建输入；
- 不读取当前 AssessmentModel 替代 ReportInput；
- Outcome Payload 保存结果事实，ReportInput 保存报告所需冻结素材；
- schema v2 类型编码只能在同一 Record 的冻结输入内解析；
- 新 Outcome 必须显式保存完整模型与运行机制身份；
- 兼容默认值只服务可识别的历史数据。

### 16.2 路由不变量

- AlgorithmFamily、DecisionKind、ReportType 和 TemplateVersion 是核心路由身份；
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

这里的 Builder 更接近“策略 + 内容构建器”：Registry 根据机制键选择策略，具体 Builder 把结构化输入组装为 Draft。它不是为了逐步构造复杂对象而暴露 fluent API。命名强调的是“只构建内容、不拥有生命周期”，解释时不必生硬套用经典 GoF Builder 模式。

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
