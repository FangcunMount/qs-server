# 09-Evaluation 通用执行引擎专题

**本文回答**：为什么 Evaluation 不应该只是“结果模块”或“Scale 专用评估流水线”，而应该升级为通用测评执行引擎；它如何通过 ModelRef、Provider、Context 接入 Scale、MBTI、BigFive 等不同解释模型；Assessment、EvaluationRun、EvaluationResult、InterpretReport 分别承担什么职责；失败、重试、幂等、事件、报告生成和观测治理应该如何围绕 Evaluation 收敛。

---

## 30 秒结论

Evaluation 的核心职责不是“算量表分数”，也不是“保存报告结果”。

它真正负责的是：

```text
一次测评执行生命周期
```

也就是：

```text
Assessment created
  -> EvaluationRun started
  -> ModelRef resolved
  -> Provider context loaded
  -> Provider evaluated
  -> EvaluationResult saved
  -> Interpretation completed / failed
  -> InterpretReport generated
  -> report.generated
```

Evaluation 应该成为通用执行引擎，而不是具体模型逻辑容器。

| 对象 | 职责 | 不负责 |
| ---- | ---- | ------ |
| Assessment | 一次测评执行事实与状态机 | 不保存具体模型完整规则 |
| EvaluationRun | 一次执行尝试、失败阶段、重试记录 | 不表达最终业务报告 |
| EvaluationResult | Provider 输出的结构化结果快照 | 不保存完整模型规则事实 |
| InterpretReport | 本次报告事实、渲染数据、报告快照 | 不管理模型规则生命周期 |
| Interpretation Provider | 执行具体解释模型 | 不直接保存 Report，不直接发布事件 |
| Evaluation Engine | 编排执行生命周期 | 不依赖 MedicalScale / Factor / RiskLevel 等具体模型语义 |

一句话概括：

> **Evaluation 管的是“某次 AnswerSheet 基于某个 ModelRef 被执行、产生结果、生成报告、失败可追踪、重试可控制”的完整生命周期。Scale、MBTI、BigFive 只是不同 Provider。**

---

## 1. 为什么 Evaluation 不能继续被理解成“结果模块”

早期系统容易把 Evaluation 理解成：

```text
AnswerSheet + Scale -> Score / Report
```

这种理解在只支持医学量表时勉强够用，但它会带来两个问题。

### 1.1 它低估了 Evaluation 的过程性

一次测评不是单次函数调用，而是一个可失败、可重试、可观测的执行过程。

它至少包含：

```text
创建 Assessment
加载 AnswerSheet
加载 Questionnaire Snapshot
解析 ModelRef
查找 Provider
加载 Context
执行 Provider
保存 EvaluationResult
生成 InterpretReport
发布完成事件
```

其中任一步都可能失败。

如果 Evaluation 只是“结果模块”，这些失败就没有合适落点。

### 1.2 它会让 Evaluation 被 Scale 语义污染

如果 Evaluation 被理解成“量表结果模块”，它很容易依赖：

```text
MedicalScale
Factor
RiskLevel
InterpretationRule
```

这样一来，MBTI 接入时就会出现问题：

```text
MBTI 没有 RiskLevel；
MBTI 的 TypeCode 不是 Factor；
MBTI 的 TypeProfile 不是 InterpretationRule；
Evaluation 不能为了 MBTI 再长出一套 if/else。
```

所以 Evaluation 必须升级成通用执行引擎。

---

## 2. Evaluation 的新定位

Evaluation 的新定位是：

```text
Assessment Execution Engine
```

它的输入不是“Scale”，而是：

```text
AnswerSheetRef
QuestionnaireSnapshotRef
ModelRef
Actor / Testee / Org Context
```

它的输出不是“某种模型专用结果”，而是：

```text
EvaluationResult
InterpretReport
Lifecycle Events
```

它的执行方式不是直接调用某个 Scale service，而是：

```text
Registry.Resolve(model_type)
  -> Provider.LoadContext(model_ref)
  -> Provider.Evaluate(input, context)
```

这让 Evaluation 可以同时支持：

```text
ScaleProvider
MBTIProvider
BigFiveProvider
CareerInterestProvider
AIEnhancedProvider
```

---

## 3. Evaluation 核心对象

### 3.1 Assessment

Assessment 是一次测评执行事实。

它回答：

```text
谁在什么时候基于哪份 AnswerSheet、哪个 ModelRef，创建了一次测评执行？
这次执行当前处于什么状态？
最终结果和报告在哪里？
```

Assessment 应保存：

```text
AssessmentID
AnswerSheetID
QuestionnaireRef
ModelRef
Actor / Testee / Org
Status
ResultRef
ReportRef
FailureReason
CreatedAt / UpdatedAt
```

Assessment 不应保存：

```text
完整 MedicalScale 规则
完整 MBTIModel 规则
完整 TypeProfile 正文
完整 ReportTemplate
```

这些属于具体模型事实源。

---

### 3.2 EvaluationRun

EvaluationRun 是一次执行尝试。

它回答：

```text
这次 Assessment 第几次执行？
执行到了哪个 phase？
失败原因是什么？
是否可重试？
耗时多少？
```

建议记录：

```text
RunID
AssessmentID
AttemptNo
ModelRef
Phase
Status
StartedAt
FinishedAt
FailureCode
FailureMessage
Retryable
TraceID
```

EvaluationRun 的价值：

- 失败可追踪。
- 重试可审计。
- 性能可分析。
- 不把所有过程状态塞进 Assessment 主表。

---

### 3.3 EvaluationResult

EvaluationResult 是 Provider 执行后的结构化结果快照。

它回答：

```text
这次解释模型执行产生了什么结构化结果？
```

Scale 结果可能是：

```text
factor_scores
risk_level
interpretation_sections
```

MBTI 结果可能是：

```text
dimension_scores
preference_result
type_code
type_profile_ref
```

BigFive 结果可能是：

```text
trait_scores
trait_percentiles
trait_profiles
```

EvaluationResult 应能表达差异，但不应让 Assessment 主表长出大量模型专用字段。

---

### 3.4 InterpretReport

InterpretReport 是报告事实。

它回答：

```text
这次测评最终给用户或医生展示的报告是什么？
```

它可能包含：

```text
ReportID
AssessmentID
ModelRef
ResultRef
Sections
RenderData
SnapshotRefs
GeneratedAt
```

InterpretReport 与 EvaluationResult 的区别：

| 对象 | 偏向 |
| ---- | ---- |
| EvaluationResult | 结构化计算结果 |
| InterpretReport | 面向展示和业务交付的报告快照 |

一个模型可以有结果，但报告生成失败。

所以 `interpretation.completed` 不等于 `report.generated`。

---

## 4. Evaluation 执行链路

推荐主链路：

```text
answersheet.submitted
  -> CreateAssessmentFromAnswerSheet
  -> assessment.created
  -> CompleteAssessment
  -> assessment.completed
  -> CompleteInterpretation
  -> interpretation.completed / interpretation.failed
  -> GenerateReportFromInterpretation
  -> report.generated
```

展开为：

```text
Assessment created
  -> create EvaluationRun
  -> load AnswerSheet / Questionnaire snapshot
  -> load ModelRef
  -> resolve Provider
  -> load Provider Context
  -> execute Provider
  -> save EvaluationResult
  -> mark interpretation completed / failed
  -> generate InterpretReport
  -> publish report.generated
```

### 4.1 为什么拆成 assessment.completed 和 interpretation.completed

`assessment.completed` 表示 Evaluation 层的测评准备和执行阶段完成。

`interpretation.completed` 表示具体 Provider 已完成解释。

二者分开有几个好处：

- 可以区分 Evaluation 编排失败和 Provider 执行失败。
- 可以为不同模型统计执行耗时。
- 可以让报告生成独立观察和重试。
- 可以避免旧的 `assessment.interpreted` 语义过重。

### 4.2 为什么 report.generated 需要单独事件

报告生成是独立事实。

即使 Provider 执行成功，报告仍可能因为：

- ReportTemplate 缺失。
- Mongo 写入失败。
- 渲染数据构建失败。
- ObjectStorage 写入失败。
- wait-report notification 失败。

而失败。

因此，`report.generated` 应在 InterpretReport durable save 成功后再发出。

---

## 5. Provider 接入模型

Evaluation 不应该知道具体模型算法。

它只知道 Provider contract。

```text
type Provider interface {
    LoadContext(ctx, modelRef) (Context, error)
    Evaluate(ctx, input, context) (Result, error)
}
```

这是概念示意，不要求代码完全照抄。

### 5.1 ScaleProvider

ScaleProvider 负责：

```text
load MedicalScale snapshot
load Factor / ScoringSpec / InterpretationRule
calculate factor scores
produce scale result
```

它输出的是 EvaluationResult，不直接保存 Report。

### 5.2 MBTIProvider

MBTIProvider 负责：

```text
load MBTIModel snapshot
load DimensionRule / QuestionMapping / TypeProfile
calculate preference result
produce TypeCode
produce MBTI result
```

它同样输出 EvaluationResult，不直接保存 Report。

### 5.3 Provider 不应该做什么

Provider 不应该：

- 修改 Assessment 状态。
- 保存 InterpretReport 主事实。
- 直接发布 Outbox event。
- 修改 AnswerSheet。
- 修改具体模型规则。
- 读取用户权限。
- 做统计投影。

这些分别属于 Evaluation、Survey、Concrete Model、Security、Statistics。

---

## 6. Evaluation 的状态机

建议状态语义：

```text
created
running
completed
interpretation_failed
report_generated
failed
retrying
```

也可以根据当前代码实际状态命名收敛，但语义上要覆盖：

| 状态 | 语义 |
| ---- | ---- |
| created | Assessment 已创建 |
| running | 正在执行 |
| completed | Evaluation 执行阶段完成 |
| interpretation_failed | Provider 执行失败 |
| report_generated | 报告已生成 |
| failed | 测评失败 |
| retrying | 正在重试 |

关键点：

```text
AnswerSheet submitted 不等于 Assessment completed；
Assessment completed 不等于 Interpretation completed；
Interpretation completed 不等于 Report generated。
```

---

## 7. 失败分类

Evaluation 失败应该按阶段分类。

建议失败阶段：

```text
load_answersheet
load_questionnaire
resolve_model_ref
resolve_provider
load_context
evaluate_provider
save_result
generate_report
save_report
publish_event
notify_waiter
```

建议 failure reason：

```text
answersheet_not_found
questionnaire_snapshot_missing
model_ref_invalid
provider_not_found
context_load_failed
rule_invalid
questionnaire_mismatch
evaluate_failed
result_save_failed
report_build_failed
report_save_failed
event_stage_failed
waiter_notify_failed
```

这些 reason 应该是有界枚举，不要直接把原始 error string 当业务状态。

---

## 8. 重试与幂等

Evaluation 是异步链路，必须天然支持重复事件和重试。

### 8.1 重试原则

重试应该基于：

```text
AssessmentID
ModelRef
AnswerSheetID
AttemptNo
```

而不是重新创建一份新的业务事实。

### 8.2 幂等原则

至少要保证：

- 同一个 AnswerSheet 不重复创建多个有效 Assessment。
- 同一个 Assessment 的重复事件不会重复生成多个有效 Report。
- EvaluationResult 有唯一约束或幂等写入语义。
- InterpretReport 有唯一约束或版本语义。
- Outbox event 可重复消费但业务副作用幂等。

### 8.3 重试不能改变历史规则引用

如果 Assessment 已经绑定某个 ModelRef，重试时默认应继续使用原 ModelRef。

否则会出现：

```text
第一次按 scale:ADHD:1.0.0 失败；
重试时模型已经更新到 1.1.0；
结果不可追溯。
```

如果需要用新规则重算，必须进入显式 ReEvaluationJob。

---

## 9. 事件与 Outbox 边界

Evaluation 相关关键事件应可靠出站。

```text
assessment.created
assessment.completed
interpretation.completed
interpretation.failed
assessment.failed
report.generated
```

### 9.1 事件表达阶段事实

事件不应该表达具体算法。

错误示例：

```text
scale.factor_scored
mbti.type_calculated
```

这些更适合成为 Provider 内部日志或 EvaluationResult 内容。

系统级事件应表达阶段事实：

```text
interpretation.completed
interpretation.failed
report.generated
```

### 9.2 规则变化事件不是执行事件

```text
report.changed
```

表示模型规则或目录变化。

它不表示某次 Assessment 完成。

它不应默认触发历史 Assessment 重算。

---

## 10. 数据边界

### 10.1 Assessment

适合 MySQL 或结构化存储，因为它需要：

- 状态查询。
- 按 ID 查询。
- 按 AnswerSheet 查询。
- 按 Testee / Org 查询。
- 幂等约束。
- 状态流转。

### 10.2 EvaluationResult

可以按当前实现选择 MySQL / Mongo，但语义上它属于 Evaluation 事实源。

它不属于 Scale，也不属于 MBTI。

### 10.3 InterpretReport

报告更像文档快照，适合 Mongo 或文档型持久化。

但无论存在哪，都应归 Evaluation 报告事实源。

### 10.4 具体模型规则

Scale 规则归 Scale。

MBTI 规则归 MBTI。

BigFive 规则归 BigFive。

Evaluation 只保存引用和执行结果，不保存完整规则定义。

---

## 11. 观测指标

Evaluation 通用引擎需要按模型类型观测。

推荐指标：

```text
qs_evaluation_lifecycle_total{stage,result}
qs_interpretation_execution_total{model_type,provider,phase,result}
qs_interpretation_execution_duration_seconds{model_type,provider,phase,result}
qs_interpretation_failure_total{model_type,provider,phase,reason}
```

示例：

```text
qs_interpretation_execution_duration_seconds{model_type="mbti",provider="mbti",phase="evaluate",result="success"}
```

禁止：

```text
model_code
model_version
assessment_id
answer_sheet_id
report_id
type_code
```

这些高基数字段应进入 logs / trace / governance drill-down。

---

## 12. Governance 排障入口

Evaluation 的治理入口应能区分：

```text
Assessment status
EvaluationRun status
Interpretation execution status
Report generation status
Provider context cache status
Worker backlog
Outbox lag
```

按模型类型 drill-down：

```text
model_type=scale
model_type=mbti
model_type=bigfive
```

典型排障：

```text
assessment.created 正常增长，但 assessment.completed 不增长
  -> 卡在 Evaluation 执行准备阶段

assessment.completed 正常增长，但 interpretation.completed 不增长
  -> 卡在 Provider 执行阶段

interpretation.completed 正常增长，但 report.generated 不增长
  -> 卡在报告生成阶段
```

---

## 13. Evaluation 与 Survey 的边界

Survey 负责：

```text
Questionnaire
AnswerSheet
Answer validation
answersheet.submitted
```

Evaluation 负责：

```text
Assessment
EvaluationRun
EvaluationResult
InterpretReport
assessment / interpretation / report events
```

Survey 不应该：

- 同步执行 Provider。
- 修改 Assessment 状态。
- 生成 InterpretReport。
- 重试 EvaluationRun。

Evaluation 不应该：

- 修改 Questionnaire。
- 修改 AnswerSheet。
- 修改答题规则。
- 接管题型校验。

---

## 14. Evaluation 与 Scale / MBTI 的边界

Concrete Model 负责规则。

Evaluation 负责执行。

| 事项 | 归属 |
| ---- | ---- |
| MedicalScale / Factor | Scale |
| MBTIModel / TypeProfile | MBTI |
| ModelRef | Interpretation Model / Evaluation input |
| Provider contract | Interpretation Model |
| Provider execution orchestration | Evaluation |
| EvaluationResult | Evaluation |
| InterpretReport | Evaluation |
| TypeCode 分布统计 | Statistics ReadModel |

---

## 15. Evaluation 与 Statistics 的边界

Statistics 读取 Evaluation 事实，不反向修改 Evaluation。

典型投影：

```text
assessment_completed_count
interpretation_completed_count
interpretation_failed_count
report_generated_count
mbti_type_distribution
mbti_dimension_distribution
```

Statistics ReadModel 不应该保存完整 EvaluationResult 或完整 TypeProfile。

它只保存查询优化投影。

---

## 16. 设计收益

### 16.1 支持多解释模型

Evaluation 不绑定 Scale 后，可以接入：

- Scale。
- MBTI。
- BigFive。
- 职业兴趣测评。
- AI 增强解释。

### 16.2 失败可治理

失败不再只是“报告生成失败”，而能明确：

```text
provider_not_found
context_load_failed
rule_invalid
evaluate_failed
report_save_failed
```

### 16.3 重试可控制

通过 EvaluationRun 记录尝试，避免重复创建业务事实。

### 16.4 观测更清晰

可以看到：

- 哪个模型慢。
- 哪个 phase 失败。
- 哪个 Provider 失败率高。
- 报告生成是否成为瓶颈。

---

## 17. 设计代价

| 代价 | 表现 |
| ---- | ---- |
| 抽象增加 | Provider、Context、Registry、Result contract |
| 链路变长 | AnswerSheet -> Assessment -> Provider -> Result -> Report |
| 测试更多 | Provider contract、EvaluationRun、Retry、Outbox、Report |
| 排障跨层 | Survey / Event / Worker / Evaluation / Provider / Report |
| 文档复杂 | 需要讲清楚执行事实、规则事实和报告事实 |

这些代价是值得的，因为它换来了多模型扩展能力和执行治理能力。

---

## 18. 替代方案分析

### 18.1 Evaluation 直接依赖 Scale

优点：

- 初期简单。
- 代码路径短。

缺点：

- MBTI 接入困难。
- Evaluation 被 MedicalScale 语义污染。
- BigFive 等新模型继续恶化问题。

结论：不适合下一阶段。

### 18.2 Evaluation 内部 if/else 分发模型

优点：

- 不污染 Scale。
- 实现直观。

缺点：

- Evaluation 会膨胀成模型分发器。
- 每新增模型都改 Evaluation 主流程。
- 不利于 Provider contract 测试。

结论：短期可跑，长期不可维护。

### 18.3 Provider 化 Evaluation Engine

优点：

- Evaluation 生命周期稳定。
- 模型独立演进。
- Provider 可测试。
- 横切能力可复用。

缺点：

- 抽象成本更高。
- 需要更多文档和测试。

结论：更适合多解释模型平台。

---

## 19. 设计不变量

1. Evaluation 不直接依赖 MedicalScale / Factor / RiskLevel。
2. Evaluation 通过 ModelRef / Provider / Context 执行模型。
3. Provider 不直接保存 InterpretReport。
4. Provider 不直接发布业务事件。
5. Assessment 是执行事实，不是规则事实。
6. EvaluationRun 是执行尝试，不是最终报告。
7. EvaluationResult 是结构化结果快照，不是模型规则。
8. InterpretReport 是报告事实，不是规则模板。
9. 重试默认使用原 ModelRef。
10. 规则变化不默认触发历史重算。
11. `interpretation.completed` 不等于 `report.generated`。
12. Statistics 不反向修改 Evaluation。
13. Redis 不成为 Evaluation 事实源。
14. Metrics 不使用高基数模型字段。
15. Worker 不直接执行具体模型算法，只调用 InternalService。

---

## 20. 常见误区

### 20.1 “Evaluation 就是算分”

不对。算分只是某些 Provider 的内部步骤。Evaluation 管的是执行生命周期。

### 20.2 “Provider 可以直接生成报告”

不建议。Provider 输出结构化结果，报告事实由 Evaluation 保存。

### 20.3 “assessment.completed 就是报告完成”

不对。报告完成应看 `report.generated`。

### 20.4 “重试时用最新模型规则更合理”

不一定。默认应使用原 ModelRef，保证历史可追溯。使用新规则应走 ReEvaluationJob。

### 20.5 “EvaluationResult 可以随便塞 JSON”

不应无约束。可以支持模型差异，但要有 result contract、版本和测试。

---

## 21. 代码锚点

### Evaluation

- `internal/apiserver/container/modules/evaluation/assemble.go`
- `internal/apiserver/application/evaluation`
- `internal/apiserver/domain/evaluation`
- `docs/02-业务模块/30-evaluation/README.md`
- `docs/02-业务模块/30-evaluation/10-领域模型.md`
- `docs/02-业务模块/30-evaluation/31-关键链路-Worker执行与报告驱动.md`

### Interpretation Model

- `docs/02-业务模块/40-interpretation/README.md`
- `docs/02-业务模块/20-model-catalog/README.md`
- `docs/05-专题分析/01-为什么拆分Survey-InterpretationModel-Evaluation.md`

### Worker / Event

- `internal/worker/handlers/answersheet_handler.go`
- `internal/worker/handlers/assessment_handler.go`
- `internal/worker/handlers/interpretation_handler.go`
- `docs/03-基础设施/event/03-Outbox可靠出站链路.md`
- `docs/03-基础设施/event/04-MQ发布与消费链路.md`

### Cross-cutting

- `docs/03-基础设施/data-access/04-读写模型分离.md`
- `docs/03-基础设施/security/04-访问上下文与权限快照.md`
- `docs/03-基础设施/observability/03-指标设计.md`
- `docs/03-基础设施/observability/06-告警与故障定位.md`

---

## 22. Verify

```bash
go test ./internal/apiserver/application/evaluation/...
go test ./internal/apiserver/domain/evaluation/...
go test ./internal/worker/handlers
go test ./internal/pkg/eventcatalog
```

如果新增 Provider：

```bash
go test ./internal/apiserver/application/...
go test ./internal/apiserver/domain/...
go test ./internal/apiserver/infra/mongo/...
go test ./internal/apiserver/infra/cachequery
```

如果修改文档：

```bash
make docs-hygiene
git diff --check
```

---

## 23. 下一跳

| 目标 | 文档 |
| ---- | ---- |
| 多解释模型扩展专题 | `08-多解释模型扩展专题--从Scale到MBTI.md` |
| 解释模型事件与缓存治理专题 | `10-解释模型事件与缓存治理专题.md` |
| Evaluation 模块 README | `../02-业务模块/30-evaluation/README.md` |
| Evaluation 执行链路 | `../02-业务模块/30-evaluation/31-关键链路-Worker执行与报告驱动.md` |
| Interpretation Model README | `../02-业务模块/40-interpretation/README.md` |
