# 08-多解释模型扩展专题--从 Scale 到 MBTI

**本文回答**：为什么 qs-server 不能继续把 Scale 当作所有解释能力的中心；为什么 MBTI 不应该被实现成 MedicalScale 的一个特殊类型；为什么需要抽象 Interpretation Model；ScaleProvider、MBTIProvider、BigFiveProvider 应该如何作为同级 Provider 接入 Evaluation；新增解释模型时，会如何影响事件、持久化、Redis、统计、安全、观测和文档体系。

---

## 30 秒结论

qs-server 下一阶段的核心演进，不是“继续增强 Scale”，而是从：

```text
Survey -> Scale -> Evaluation
```

升级为：

```text
Survey -> Interpretation Model -> Concrete Models -> Evaluation
```

其中：

| 层次 | 职责 | 示例 |
| ---- | ---- | ---- |
| Survey | 作答事实 | Questionnaire、AnswerSheet、AnswerValue |
| Interpretation Model | 解释模型接入协议 | ModelRef、Provider、Context、Registry |
| Concrete Models | 具体解释模型规则 | Scale、MBTI、BigFive、职业兴趣测评 |
| Evaluation | 通用测评执行引擎 | Assessment、EvaluationRun、EvaluationResult、InterpretReport |

核心判断：

> **Scale 是一个具体解释模型，不是解释模型抽象层。MBTI 与 Scale 同级，应该通过统一 Interpretation Provider 接入 Evaluation，而不是被塞进 MedicalScale。**

如果把 MBTI 塞进 Scale，短期可能少写一些代码，但长期会导致：

```text
MedicalScale 被 MBTI TypeCode / TypeProfile 污染；
Factor / RiskLevel 被滥用；
Evaluation 继续依赖 Scale 语义；
后续 BigFive / 职业兴趣测评继续污染 Scale；
系统无法成为多解释模型平台。
```

正确方向是：

```text
ScaleProvider implements InterpretationProvider
MBTIProvider implements InterpretationProvider
BigFiveProvider implements InterpretationProvider
```

---

## 1. 问题背景：为什么从 Scale 扩展到 MBTI 是架构拐点

在 qs-server 早期，Scale 几乎等同于“解释模型”。

当系统主要处理医学量表时，这样的叙事是自然的：

```text
Questionnaire 收集答案；
MedicalScale 定义因子、计分规则和解释规则；
Evaluation 根据量表规则生成结果和报告。
```

但当系统计划支持 MBTI 后，问题就变了。

MBTI 不是 MedicalScale 的一个小变体，它有自己的模型语义：

```text
Dimension
Preference
TypeCode
TypeProfile
ReportTemplate
```

而医学量表更自然的语义是：

```text
MedicalScale
Factor
ScoringSpec
RiskLevel
InterpretationRule
```

二者都可以“解释答卷”，但解释方式、规则结构、结果结构和报告表达都不同。

所以支持 MBTI 不是简单加字段，而是逼着系统回答一个根本问题：

```text
qs-server 到底是“医学量表系统”，还是“支持多种解释模型的测评执行平台”？
```

如果目标是后者，Scale 就必须从“解释能力中心”降级为“具体解释模型之一”。

---

## 2. 旧模型：Survey / Scale / Evaluation 的边界问题

旧主线可以简化为：

```text
Survey
  -> 提供 Questionnaire / AnswerSheet

Scale
  -> 提供 MedicalScale / Factor / InterpretationRule

Evaluation
  -> 读取 AnswerSheet + Scale，生成 Assessment / Report
```

在只支持 Scale 时，它的问题不明显。

但新增 MBTI 后，系统会遇到三个选择。

### 2.1 选择一：把 MBTI 塞进 Scale

也就是：

```text
MedicalScale
  + MBTI dimension
  + MBTI type code
  + MBTI type profile
```

这是最容易写坏的方案。

表面上它复用了 Scale 的维护链路、查询链路和测评链路，但实际后果是：

```text
MedicalScale 不再只表达医学量表；
Factor 被迫表达 MBTI dimension；
RiskLevel 被迫表达 TypeCode；
InterpretationRule 被迫表达 TypeProfile；
Scale 变成万能规则容器。
```

这会让 Scale domain 失去边界。

### 2.2 选择二：在 Evaluation 里直接 if/else

也就是：

```text
if model_type == scale:
    load MedicalScale
    calculate FactorScore
    build ScaleReport

if model_type == mbti:
    load MBTIModel
    calculate TypeCode
    build MBTIReport
```

这能避免污染 Scale，但会污染 Evaluation。

Evaluation 会不断堆积具体模型细节：

```text
FactorScore
RiskLevel
TypeCode
TypeProfile
TraitScore
CareerInterest
AIInterpretation
```

最后 Evaluation 会变成一个巨大的模型分发器，而不是通用执行引擎。

### 2.3 选择三：抽象 Interpretation Model

也就是：

```text
Evaluation
  -> ModelRef
  -> Registry.Resolve(model_type)
  -> Provider.LoadContext
  -> Provider.Evaluate
  -> EvaluationResult
  -> InterpretReport
```

这是更稳妥的长期方案。

它保留了具体模型的独立性，也让 Evaluation 只关心通用执行生命周期。

---

## 3. 新模型：Interpretation Model 抽象层

Interpretation Model 不是一个新的“万能业务模块”，而是一组接入协议。

它要解决的问题是：

```text
不同解释模型的规则结构不同；
不同解释模型的结果结构不同；
不同解释模型的报告结构不同；
但一次测评执行的生命周期相同。
```

因此它提供统一抽象：

```text
ModelRef
InterpretationProvider
InterpretationContext
InterpretationRegistry
EvaluationInput
EvaluationResult contract
```

### 3.1 ModelRef

ModelRef 表示一次 Assessment 使用哪个解释模型。

建议最小结构：

```text
ModelType
ModelCode
ModelVersion
```

示例：

```text
scale:ADHD_PARENT:1.0.0
mbti:MBTI_STANDARD:1.0.0
bigfive:BIGFIVE_STANDARD:1.0.0
```

ModelRef 的价值是：

- 让 Evaluation 不直接依赖 MedicalScale ID。
- 让 Scale / MBTI / BigFive 可以用同一种引用方式进入执行链路。
- 让历史报告可追溯到当时使用的模型版本。
- 为 ReEvaluationJob 和规则版本治理打基础。

---

### 3.2 InterpretationProvider

Provider 是具体模型接入 Evaluation 的执行契约。

建议语义：

```text
LoadContext(ctx, modelRef)
Evaluate(ctx, input, context)
```

职责边界：

| 方法 | 职责 | 不应做 |
| ---- | ---- | ------ |
| LoadContext | 加载只读规则上下文 | 不保存本次执行结果 |
| Evaluate | 根据输入和上下文产生结构化结果 | 不直接写 Report 主表，不直接发事件 |

Provider 的核心价值是让不同模型同级接入：

```text
ScaleProvider
MBTIProvider
BigFiveProvider
```

---

### 3.3 InterpretationContext

Context 是 Provider 执行所需的只读上下文。

ScaleContext 可能包含：

```text
MedicalScale snapshot
Factor rules
Scoring specs
Interpretation rules
Questionnaire binding
```

MBTIContext 可能包含：

```text
MBTIModel snapshot
Dimension rules
Question mappings
Type profiles
Report template
Questionnaire binding
```

Context 可以缓存，但不是事实源。

事实源应仍在具体模型 repository。

---

### 3.4 InterpretationRegistry

Registry 负责根据 ModelType 找到 Provider。

示例：

```text
scale -> ScaleProvider
mbti -> MBTIProvider
bigfive -> BigFiveProvider
```

它让 Evaluation 避免写成 if/else 分发器。

如果 Provider 未注册，应产生明确失败：

```text
provider_not_found
```

并进入 `interpretation.failed` 或 Assessment failed 语义，而不是 panic 或静默跳过。

---

## 4. Scale 的新定位：具体医学量表解释模型

Scale 的新定位应该是：

```text
Concrete Interpretation Model: MedicalScale
```

它仍然非常重要，但它不再是所有解释能力的抽象中心。

### 4.1 Scale 应该保留的语义

Scale domain 应继续专注：

```text
MedicalScale
Factor
ScoringSpec
InterpretationRule
RiskLevel
QuestionnaireRef
Published Snapshot
```

这些是医学量表规则资产。

Scale 文档应继续讲：

- MedicalScale 模型设计。
- Factor 和 InterpretationRule。
- 生命周期维护。
- 问卷绑定。
- 查询读模型。
- Scale 与 Evaluation 的联动。

### 4.2 Scale 不应该继续承担的职责

Scale 不应该承担：

- MBTI TypeCode。
- MBTI TypeProfile。
- BigFive TraitProfile。
- Assessment 状态机。
- EvaluationRun。
- InterpretReport 生命周期。
- 通用 Provider Registry。
- 所有解释模型列表。

这些应该分别落到：

```text
MBTI concrete model
BigFive concrete model
Interpretation Model abstraction
Evaluation
Statistics ReadModel
```

---

## 5. MBTI 的新定位：Scale 同级的具体解释模型

MBTI 应该作为一个新的 Concrete Model。

建议模型：

```text
MBTIModel
├── ModelCode
├── ModelVersion
├── QuestionnaireRef
├── Status
├── Dimensions
├── QuestionMappings
├── TypeProfiles
├── ReportTemplate
└── Metadata
```

### 5.1 MBTIModel

MBTIModel 是规则聚合根。

它负责：

- 维护 MBTI 模型编码和版本。
- 绑定 Questionnaire。
- 管理四组维度规则。
- 管理题目到维度的映射。
- 管理 16 种 TypeProfile。
- 管理报告模板。
- 发布后冻结。

### 5.2 DimensionRule

DimensionRule 表示 MBTI 的维度规则。

示例：

```text
E/I
S/N
T/F
J/P
```

它和 Scale 的 Factor 不一样。

Factor 通常表示医学量表中的症状因子或能力因子；MBTI Dimension 表示人格偏好轴。

不要为了复用字段，把 DimensionRule 塞进 Factor。

### 5.3 TypeProfile

TypeProfile 表示类型画像。

示例：

```text
INTJ
ENFP
ISTP
```

TypeProfile 不是 RiskLevel。

RiskLevel 表达风险程度；TypeProfile 表达人格类型解释。

把 TypeProfile 塞进 RiskLevel 会直接破坏语义。

---

## 6. Evaluation 的新定位：通用测评执行引擎

Evaluation 不应关心具体模型内部算法。

它应该关心：

```text
这次 Assessment 使用哪个 ModelRef；
Provider 是否能加载；
Context 是否能加载；
Provider 是否执行成功；
EvaluationResult 是否保存；
InterpretReport 是否生成；
失败是否可重试。
```

Evaluation 的核心流程应是：

```text
Assessment
  -> EvaluationRun
  -> Resolve ModelRef
  -> Load Provider Context
  -> Provider Evaluate
  -> Save EvaluationResult
  -> Save InterpretReport
  -> Publish events
```

### 6.1 EvaluationResult 应该支持模型差异

EvaluationResult 需要表达通用结构，同时允许模型差异。

Scale 结果可能包含：

```text
factor_scores
risk_level
interpretation_sections
```

MBTI 结果可能包含：

```text
dimension_scores
preference_result
type_code
type_profile_ref
```

EvaluationResult 可以通过结构化 payload 或 typed result snapshot 表达差异。

但不要把每个模型的专用字段都塞进 Assessment 主表。

### 6.2 InterpretReport 应该保存报告事实

InterpretReport 表示本次报告快照。

它可以包含：

```text
model_type
model_code
model_version
result_snapshot_ref
sections
render_data
created_at
```

报告事实归 Evaluation，不归 Scale 或 MBTI 规则仓储。

---

## 7. 新主链路

多解释模型后的主链路：

```text
AnswerSheet submitted
  -> Assessment created
  -> Assessment completed
  -> Interpretation completed / failed
  -> Report generated
```

展开为：

```text
answersheet.submitted
  -> CreateAssessmentFromAnswerSheet
  -> assessment.created
  -> CompleteAssessment
  -> assessment.completed
  -> CompleteInterpretation
  -> Resolve ModelRef
  -> Provider.LoadContext
  -> Provider.Evaluate
  -> interpretation.completed / interpretation.failed
  -> GenerateReportFromInterpretation
  -> report.generated
```

这条链路中：

- Survey 只负责 AnswerSheet。
- Evaluation 负责 Assessment 和执行生命周期。
- Interpretation Model 负责 Provider 抽象。
- Concrete Models 提供具体规则。
- Report 由 Evaluation 持久化。

---

## 8. 事件系统影响

新增解释模型后，事件系统必须区分两类事件。

### 8.1 一次测评执行事件

这类事件表达某次 Assessment 的阶段事实：

```text
answersheet.submitted
assessment.created
assessment.completed
interpretation.completed
interpretation.failed
assessment.failed
report.generated
```

它们通常是 durable_outbox。

### 8.2 规则变化事件

这类事件表达模型规则或目录变化：

```text
scale.changed
interpretation-model.changed
mbti-model.published
mbti-model.archived
```

它们通常是 best_effort 或轻量治理事件。

### 8.3 关键不变量

```text
interpretation.completed 表示某次解释执行完成；
interpretation-model.changed 表示规则变化；
二者不能混用。
```

规则变化不应默认触发历史 Assessment 重算。

历史重算必须建模为显式：

```text
ReEvaluationJob
RepairJob
BackfillJob
```

---

## 9. Data Access 影响

新增 MBTI 后，持久化边界应保持清晰。

### 9.1 规则事实

MBTI 规则事实建议进入具体模型 repository，例如 Mongo Document：

```text
mbti_models
```

包含：

```text
model_code
model_version
status
questionnaire_ref
dimensions
question_mappings
type_profiles
report_template
```

### 9.2 执行事实

一次测评执行结果进入 Evaluation：

```text
Assessment
EvaluationRun
EvaluationResult
InterpretReport
```

### 9.3 统计事实

统计查询进入 Statistics ReadModel：

```text
statistics_interpretation_model_daily
statistics_mbti_type_daily
statistics_mbti_dimension_daily
```

### 9.4 缓存事实

Redis 只缓存可回源数据：

```text
MBTIModelListCache
PublishedInterpretationModelListCache
MBTIContextCache
```

Redis 不是规则事实源。

---

## 10. Redis / Cache 影响

新增解释模型后，缓存分为几类。

### 10.1 发布态模型列表

可使用 StaticList：

```text
PublishedScaleListCache
MBTIModelListCache
BigFiveModelListCache
PublishedInterpretationModelListCache
```

注意：StaticList 只保存列表摘要，不保存完整规则正文。

### 10.2 Provider Context cache

Context cache 用于减少规则加载开销。

例如：

```text
ScaleContextCache
MBTIContextCache
```

它必须可回源具体模型 repository。

### 10.3 WarmupTarget

新增模型后可注册：

```text
static.interpretation_model:{modelType}:{modelCode}:{modelVersion}
static.interpretation_model_list:published
static.mbti_model_list:published
```

Warmup 只让缓存变热，不修复业务事实，也不重算历史测评。

---

## 11. Statistics 影响

MBTI 会产生新的统计口径。

示例：

```text
MBTI TypeCode 分布
MBTI Dimension 偏好分布
不同模型执行量分布
不同模型报告生成量分布
不同模型失败率
```

建议 ReadModel：

```text
statistics_interpretation_model_daily
statistics_mbti_type_daily
statistics_mbti_dimension_daily
```

关键原则：

```text
TypeProfile 文案属于 MBTI 规则事实源；
TypeCode 分布属于 Statistics ReadModel；
EvaluationResult 是统计投影输入；
Redis QueryCache 只缓存高频查询结果。
```

不要把完整 TypeProfile 复制进统计表。

---

## 12. Security 影响

多解释模型扩展后，权限也要拆开。

### 12.1 模型规则管理权限

```text
read_interpretation_models
manage_interpretation_models
```

面向：

```text
Scale / MBTI / BigFive 规则配置、发布、归档、查询
```

### 12.2 报告访问权限

```text
read_interpretation_reports
```

面向：

```text
用户测评报告，例如 Scale 报告、MBTI 报告、BigFive 报告
```

### 12.3 关键不变量

```text
能管理 MBTI 规则，不等于能查看用户 MBTI 报告；
能查看用户报告，不等于能修改模型规则。
```

建议 IAM resource：

```text
qs:interpretation_models
qs:interpretation_reports
```

---

## 13. Observability 影响

指标应能按模型类型观察，但不能引入高基数 label。

推荐指标：

```text
qs_interpretation_execution_total{model_type,provider,phase,result}
qs_interpretation_execution_duration_seconds{model_type,provider,phase,result}
qs_interpretation_failure_total{model_type,provider,phase,reason}
qs_evaluation_lifecycle_total{stage,result}
```

示例：

```text
qs_interpretation_execution_duration_seconds{model_type="mbti",provider="mbti",phase="evaluate",result="success"}
```

禁止 label：

```text
model_code
model_version
assessment_id
answer_sheet_id
report_id
type_code
```

MBTI TypeCode 分布应进入 Statistics ReadModel，不应进入 Prometheus label。

Governance endpoint 可以 drill down 到具体模型，但 summary metrics 必须低基数。

---

## 14. Governance 影响

新增解释模型后，治理层应能观察：

```text
model_type=scale
model_type=mbti
model_type=bigfive
```

治理对象包括：

- 模型列表缓存状态。
- Provider Context cache 状态。
- WarmupTarget 状态。
- Interpretation execution 状态。
- Interpretation queue / backlog。
- Failed interpretation list。
- Report generation lag。

Governance 可以提供 drill-down，但默认不应修改业务事实。

特别注意：

```text
规则变化后，governance 可以预热缓存；
不能自动重算历史 Assessment；
历史重算必须通过显式任务。
```

---

## 15. 新增 MBTI 的推荐实施步骤

建议顺序：

```text
1. 固化 ModelRef / Provider / Context / Registry
2. 将 ScaleProvider 适配到统一 Provider 契约
3. 定义 MBTIModel domain
4. 定义 MBTI repository / document / mapper / migration
5. 实现 MBTIProvider.LoadContext
6. 实现 MBTIProvider.Evaluate
7. 扩展 EvaluationResult / InterpretReport 的 MBTI 结果表达
8. 接入事件链路 interpretation.completed / failed
9. 增加 MBTIModelListCache / ContextCache / WarmupTarget
10. 增加 MBTI TypeCode / Dimension ReadModel
11. 增加 read/manage interpretation models 和 read reports 权限
12. 增加 metrics / governance drill-down
13. 增加 tests / docs / runbook
```

不建议顺序：

```text
1. 先改 Scale 表结构
2. 把 MBTI 维度塞进 Factor
3. 把 TypeCode 塞进 RiskLevel
4. 在 Evaluation 写 if model_type == mbti
5. 把结果字段塞进 Assessment 主表
```

---

## 16. 设计收益

### 16.1 模型扩展成本更低

新增模型时主要新增：

```text
Concrete Model domain
Provider
Context loader
Result mapper
Report template
DataAccess / Redis / Statistics / Security / Observability 接入
```

而不是重写 Evaluation 主流程。

### 16.2 Evaluation 更稳定

Evaluation 只管执行生命周期：

```text
created
completed
interpretation completed / failed
report generated
failed / retry
```

不再绑定 MedicalScale 语义。

### 16.3 Scale 模型更纯粹

Scale 继续表达医学量表，不被 MBTI / BigFive 污染。

### 16.4 基础设施可复用

事件、缓存、统计、安全、观测都可以围绕通用模型维度扩展。

---

## 17. 设计代价

### 17.1 抽象成本增加

需要新增：

- Provider contract。
- Registry。
- Context model。
- ModelRef。
- Result contract。
- Provider tests。

### 17.2 调试链路更长

MBTI 报告不生成时，需要查：

```text
AnswerSheet
Assessment
ModelRef
Registry
MBTIProvider
MBTIContext
EvaluationResult
InterpretReport
Outbox
Worker
```

### 17.3 文档和测试要求更高

每新增一个模型，都要同步：

- 业务模块文档。
- data-access SOP。
- redis cache SOP。
- event catalog。
- security capability。
- observability metrics。
- governance runbook。

这是多模型平台的必要代价。

---

## 18. 设计不变量

1. Scale 是具体医学量表模型，不是解释模型抽象层。
2. MBTI 与 Scale 同级，都是具体解释模型。
3. BigFive / 职业兴趣测评也应作为同级模型接入。
4. Evaluation 不直接依赖 MedicalScale / Factor / RiskLevel。
5. Evaluation 通过 ModelRef / Provider / Context 执行模型。
6. Provider 不直接保存 Report 主事实。
7. Provider 不直接发布业务事件。
8. Context 是只读规则快照，不是事实源。
9. 规则事实在具体模型模块。
10. 执行事实在 Evaluation。
11. 统计事实在 Statistics ReadModel。
12. Redis 只缓存可回源数据。
13. Metrics 使用低基数 model_type，不使用 model_code。
14. 管理模型规则权限和读取用户报告权限必须拆开。
15. 规则变化事件不默认触发历史 Assessment 重算。

---

## 19. 常见误区

### 19.1 “MBTI 就是另一种量表”

不准确。MBTI 是解释模型，但不是 MedicalScale。

### 19.2 “复用 Scale 最省事”

短期省事，长期会污染 Scale、Evaluation 和统计体系。

### 19.3 “Provider 直接生成 Report 就行”

不建议。Provider 应输出结构化结果；报告事实应由 Evaluation 统一保存。

### 19.4 “Context cache 可以当规则事实源”

不行。Context cache 必须可回源具体模型 repository。

### 19.5 “model_code 可以作为 metrics label”

不建议。model_code / model_version 容易形成高基数，应该进入日志或 governance drill-down。

### 19.6 “规则改了就应该自动重算历史报告”

不应默认如此。历史报告应追溯当时的 ModelRef / RuleSnapshotRef。重算必须显式建模。

---

## 20. 代码锚点

### Interpretation Model 文档

- `docs/02-业务模块/interpretation-model/README.md`
- `docs/02-业务模块/interpretation-model/01-解释模型抽象--ModelRef-Provider-Context模型设计.md`
- `docs/02-业务模块/interpretation-model/02-解释模型接入链路--注册-加载-执行-结果返回.md`
- `docs/02-业务模块/interpretation-model/03-新增解释模型链路--以MBTI接入为例.md`
- `docs/02-业务模块/interpretation-model/04-解释模型分层架构与事实源索引.md`

### Scale

- `internal/apiserver/container/assembler/scale.go`
- `internal/apiserver/application/scale`
- `internal/apiserver/domain/assessmentmodel/scale/definition`
- `docs/02-业务模块/scale/README.md`

### Evaluation

- `internal/apiserver/container/assembler/evaluation.go`
- `internal/apiserver/application/evaluation`
- `internal/apiserver/domain/evaluation`
- `docs/02-业务模块/evaluation/README.md`
- `docs/02-业务模块/evaluation/03-Evaluation引擎链路--模型解析-规则加载-执行-报告生成.md`

### Cross-cutting

- `docs/03-基础设施/event/01-事件目录与契约.md`
- `docs/03-基础设施/data-access/05-新增持久化能力SOP.md`
- `docs/03-基础设施/redis/04-QueryCache与StaticList.md`
- `docs/03-基础设施/security/05-新增安全能力SOP.md`
- `docs/03-基础设施/observability/01-Metrics指标体系.md`
- `docs/03-基础设施/observability/04-GovernanceEndpoint与排障SOP.md`

---

## 21. Verify

```bash
go test ./internal/apiserver/application/evaluation/...
go test ./internal/apiserver/domain/evaluation/...
go test ./internal/apiserver/application/scale/...
go test ./internal/apiserver/domain/assessmentmodel/scale/definition/...
go test ./internal/apiserver/infra/mongo/...
go test ./internal/apiserver/infra/cachequery
```

如果新增 Provider contract：

```bash
go test ./internal/apiserver/application/...
go test ./internal/apiserver/domain/...
```

如果新增事件：

```bash
go test ./internal/pkg/eventcatalog
go test ./internal/worker/handlers
```

如果修改文档：

```bash
make docs-hygiene
git diff --check
```

---

## 22. 下一跳

| 目标 | 文档 |
| ---- | ---- |
| 为什么拆分 Survey / Interpretation Model / Evaluation | `01-为什么拆分Survey-InterpretationModel-Evaluation.md` |
| 为什么同步提交但异步测评执行 | `02-为什么同步提交但异步测评执行.md` |
| Evaluation 通用执行引擎专题 | `09-Evaluation通用执行引擎专题.md` |
| 解释模型事件与缓存治理专题 | `10-解释模型事件与缓存治理专题.md` |
| Scale 模块 | `../02-业务模块/scale/README.md` |
| Interpretation Model 模块 | `../02-业务模块/interpretation-model/README.md` |
| Evaluation 模块 | `../02-业务模块/evaluation/README.md` |
