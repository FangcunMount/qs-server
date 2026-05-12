# AnswerSheet 模型：AnswerSheet / Answer / AnswerValue

> 本文是 Survey 模块文档的第三篇。
>
> 上一篇《01-Questionnaire模型-Questionnaire-Question-SubmissionSpec》讲清了模板侧模型：`Questionnaire` 管“可提交的问卷模板”，`SubmissionSpec` 管“这份已发布问卷如何被提交”。
>
> 本文聚焦事实侧模型：`AnswerSheet` 如何表达一次完整作答事实，`SubmissionContext` 为什么必须入模，`Answer / AnswerValue` 如何承载单题答案事实，`Answer.Score` 的边界是什么，以及 `AnswerSheetSubmittedEvent` 如何从提交事实中产生。

---

## 1. 结论先行

`AnswerSheet` 是 Survey 模块中的**答卷提交事实聚合**。

它负责表达：

```text
谁，在什么业务上下文中，基于哪份问卷版本，正式提交了哪些答案。
```

它不负责表达：

```text
这些答案如何组成因子；
这些答案如何计算总分；
这些答案对应什么风险等级；
这些答案应该生成什么报告；
这次测评执行到了什么状态。
```

这些属于 Scale / Evaluation。

一句话概括：

> **AnswerSheet 是一次作答事实，不是草稿、不是测评结果、不是报告。**

AnswerSheet 模型的核心结构可以概括为：

```text
AnswerSheet
├── ID
├── QuestionnaireRef
├── SubmissionContext
├── Answers
├── FilledAt
├── Score
└── DomainEvents
```

其中：

```text
QuestionnaireRef 说明基于哪份问卷版本；
SubmissionContext 说明谁为谁在什么上下文中提交；
Answers 保存每道题的类型化答案；
DomainEvents 记录 AnswerSheetSubmittedEvent。
```

---

## 2. 本文边界

本文只讲 AnswerSheet 事实模型。

本文重点：

```text
AnswerSheet 聚合根；
QuestionnaireRef；
SubmissionContext；
Answer；
AnswerValue；
Answer.Score；
AnswerSheetSubmittedEvent。
```

本文不展开：

```text
collection-server 到 qs-apiserver 的服务链路；
SubmissionService 的完整提交流程；
DurableStore / Idempotency / Outbox；
Worker 与 Evaluation 消费事件链路。
```

这些由后续文档承接：

```text
03-测评服务查询与提交链路.md
04-测评提交事件幂等与Outbox出站链路.md
```

---

## 3. 为什么 AnswerSheet 是聚合根

`AnswerSheet` 是一次提交事实的聚合根。

它内部包含：

```text
QuestionnaireRef；
SubmissionContext；
Answer 集合；
提交时间；
领域事件。
```

这些对象共同表达一次完整提交事实，不能被拆散理解。

DDD 中聚合根用于控制聚合内部对象的访问，并维护聚合整体一致性；外部对象通常只应引用聚合根，而不是直接修改聚合内部对象。

对于 AnswerSheet 来说，聚合根需要保护：

```text
答卷必须引用确定问卷版本；
答卷必须有提交上下文；
答卷必须有答案集合；
同一题目不能重复提交；
提交时间必须有效；
提交事实成立后必须产生提交事件。
```

因此，外部不应该直接绕过 AnswerSheet 去修改内部 Answers。

正确的模型语义是：

```text
AnswerSheet.Submit(...)
```

而不是：

```text
NewAnswerSheet + 外部随意 SetAnswers / SetContext / SetEvents
```

---

## 4. AnswerSheet 的核心结构

AnswerSheet 可以抽象为：

```text
AnswerSheet
├── ID
├── QuestionnaireRef
├── SubmissionContext
├── Answers
├── FilledAt
├── Score
└── DomainEvents
```

字段可以分成四类。

| 类型 | 字段 | 说明 |
| --- | --- | --- |
| 标识信息 | ID | 答卷提交事实的唯一标识 |
| 模板引用 | QuestionnaireRef | 指向确定 QuestionnaireCode + QuestionnaireVersion |
| 提交上下文 | SubmissionContext | 填写人、受试者、组织、任务等上下文 |
| 答案事实 | Answers / FilledAt / Score | 单题答案、提交时间、基础分值 |
| 治理信息 | DomainEvents | 提交事实产生后的领域事件 |

AnswerSheet 的核心不是“答案数组”，而是一次完整业务事实。

```text
答案必须和问卷版本绑定；
答案必须和填写上下文绑定；
答案必须能驱动后续 Evaluation；
答案提交后需要可靠产生领域事件。
```

---

## 5. 后端为什么不维护 AnswerSheet 草稿

Survey 后端只保存正式提交后的 AnswerSheet。

```text
前端草稿：用户填写过程中的临时体验；
后端 AnswerSheet：提交成功后的业务事实。
```

如果后端也维护草稿，会引入复杂状态：

```text
draft；
submitted；
cancelled；
expired；
resumed；
auto_saved。
```

这会带来额外问题：

| 问题 | 说明 |
| --- | --- |
| 状态边界变复杂 | 很难判断何时形成正式作答事实 |
| 并发编辑复杂 | 多端编辑、恢复草稿、自动保存需要冲突处理 |
| 事件时机模糊 | 不清楚什么时候产生 answersheet.submitted |
| 与 Plan 混杂 | 草稿过期、任务窗口、提醒策略会侵入 Survey |

当前更清晰的设计是：

```text
前端可以保存填写过程中的草稿；
后端只接收正式提交；
提交成功即产生 AnswerSheet；
AnswerSheet 创建即表示作答事实发生。
```

所以 AnswerSheet 不需要复杂状态机。

它的领域行为重点是：

```text
Submit
```

---

## 6. QuestionnaireRef：答卷的模板版本引用

`QuestionnaireRef` 是 AnswerSheet 中最重要的引用之一。

它通常包含：

```text
QuestionnaireRef
├── QuestionnaireCode
├── QuestionnaireVersion
└── QuestionnaireTitle
```

它表达：

```text
这份答卷是基于哪一份、哪一版问卷模板提交的。
```

### 6.1 为什么不能只保存 QuestionnaireCode

Questionnaire 是会演进的模板。

如果 AnswerSheet 只保存 QuestionnaireCode，会出现问题：

```text
题目后来新增了；
题型后来修改了；
选项后来调整了；
校验规则后来变化了；
基础分值后来变化了；
历史答卷不知道当时基于哪版模板提交。
```

所以 AnswerSheet 必须保存：

```text
QuestionnaireCode + QuestionnaireVersion
```

这也是后续 Scale / Evaluation 校验一致性的基础。

### 6.2 与 SubmissionSpec 的关系

QuestionnaireRef 应来自 `SubmissionSpec`。

```text
Questionnaire.BuildSubmissionSpec()
  -> SubmissionSpec.QuestionnaireRef()
  -> AnswerSheet.Submit(questionnaireRef, ...)
```

这样可以保证：

```text
AnswerSheet 引用的问卷版本和提交规格来自同一个已发布 Questionnaire。
```

---

## 7. SubmissionContext：提交上下文值对象

`SubmissionContext` 是 AnswerSheet 提交事实的一部分。

它用于回答：

```text
这份答卷是谁填的？
这份答卷是为谁填的？
这份答卷属于哪个组织？
这份答卷是否来自某个任务？
```

可以抽象为：

```text
SubmissionContext
├── Filler
├── Testee
├── OrgID
└── TaskID
```

### 7.1 为什么 SubmissionContext 必须入模

如果没有 SubmissionContext，提交事实会被拆散到多个地方：

```text
AnswerSheet 只保存答案；
DTO 保存 testee_id / org_id / task_id；
DurableMeta 保存幂等上下文；
Event payload 再由 application 临时拼接。
```

这样会导致：

```text
AnswerSheet 自己无法完整说明这次提交发生在什么业务上下文中；
AnswerSheetSubmittedEvent 无法自然从 AnswerSheet 导出；
Evaluation / Plan / Statistics 消费事件时缺少稳定上下文来源。
```

SubmissionContext 入模后，AnswerSheet 可以完整表达：

```text
某个填写人，在某个组织/任务上下文下，为某个受试者提交了这份答卷。
```

### 7.2 Filler 与 Testee

`Filler` 和 `Testee` 必须区分。

| 场景 | Filler | Testee |
| --- | --- | --- |
| 成人自评 | 本人 | 本人 |
| 家长代填儿童量表 | 家长 | 儿童 |
| 医生访谈录入 | 医生 | 患者 |
| 老师填写观察问卷 | 老师 | 学生 |

`Filler` 回答：

```text
谁执行了填写动作？
```

`Testee` 回答：

```text
这份答卷是为谁填写的？
```

二者在自评场景中可能相同，在代填或观察场景中通常不同。

### 7.3 OrgID 与 TaskID

`OrgID` 表示组织 / 机构上下文。

`TaskID` 表示这次提交是否来自某个测评任务。

Survey 不负责完整 Plan 状态机。

但 Survey 应保存 TaskID 引用，供后续模块使用：

```text
Evaluation 用于创建 Assessment；
Plan 用于任务状态推进；
Statistics 用于任务完成率统计。
```

---

## 8. Answer：单题答案事实

`Answer` 是 AnswerSheet 聚合内部的单题答案值对象。

它表达：

```text
用户对某一道题实际提交了什么答案。
```

可以抽象为：

```text
Answer
├── QuestionCode
├── QuestionType
├── AnswerValue
└── Score
```

### 8.1 QuestionCode

`QuestionCode` 用于定位题目。

它的解释上下文来自：

```text
AnswerSheet.QuestionnaireRef
```

也就是说：

```text
Q001 不是全局孤立含义；
Q001 必须结合 QuestionnaireCode + QuestionnaireVersion 解释。
```

### 8.2 QuestionType

`QuestionType` 表示该答案对应的题型。

注意：

```text
QuestionType 的事实源不是客户端 DTO；
QuestionType 应来自 Questionnaire / SubmissionSpec。
```

客户端提交 question_type 时，只能作为待校验输入。

最终进入 Answer 的 question_type 应以模板规格为准。

### 8.3 AnswerValue

`AnswerValue` 是类型化答案值。

它避免业务层到处传递：

```text
map[string]any；
interface{}；
raw json。
```

AnswerValue 的具体类型由 QuestionType 决定。

### 8.4 Score

`Score` 表示单题基础分。

它的边界要严格限定：

```text
Answer.Score 是答案在当前问卷模板下的基础分；
它不是因子分；
它不是总分；
它不是风险等级；
它不是报告结论。
```

---

## 9. AnswerValue：类型化答案值

`AnswerValue` 是 Survey 答案模型的关键扩展点。

它用于表达不同题型下的答案结构。

常见类型包括：

```text
StringValue；
NumberValue；
OptionValue；
OptionsValue；
EmptyValue。
```

它们与题型的关系通常是：

| QuestionType | AnswerValue | 说明 |
| --- | --- | --- |
| Radio | OptionValue | 单选选项编码 |
| Checkbox | OptionsValue | 多选选项编码集合 |
| Text | StringValue | 短文本 |
| Textarea | StringValue | 长文本 |
| Number | NumberValue | 数值输入 |
| Section | EmptyValue / StringValue | 分组说明类内容 |

### 9.1 为什么需要 AnswerValue

如果直接保存 raw value，会导致：

```text
题型语义丢失；
校验器要处理大量 any 类型；
存储结构难以稳定；
查询结果难以还原；
新增题型缺少统一扩展点。
```

AnswerValue 的价值是：

```text
在进入领域模型前完成类型收敛；
让 Answer 保存稳定的答案事实；
让校验、存储、查询、Evaluation 消费都有明确输入。
```

### 9.2 AnswerValue 的设计原则

AnswerValue 应该具备：

```text
明确类型；
不可随意变更；
能安全导出原始值；
能被存储层稳定映射；
能被校验器稳定读取。
```

可以逐步增强：

```text
Kind()
IsEmpty()
AsString()
AsNumber()
OptionCode()
OptionCodes()
```

目标是减少 `Raw() any` 在领域层传播。

---

## 10. Answer.Score 的边界

Survey 可以承载单题基础分值。

例如：

```text
Radio 题：
A = 0 分
B = 1 分
C = 2 分

用户选择 B
Answer.Score = 1
```

这个分数来自 Questionnaire / Option / QuestionSpec。

它表达的是：

```text
当前答案在当前问卷模板下对应的基础分。
```

### 10.1 Answer.Score 可以做什么

Answer.Score 可以用于：

```text
保存单题基础分快照；
减少后续 Evaluation 再查选项基础分的成本；
为 ScoringSpec 执行提供输入；
支持后续审计：当时这个答案基础分是多少。
```

### 10.2 Answer.Score 不能做什么

Answer.Score 不能被当成：

```text
FactorScore；
TotalScore；
RiskLevel；
InterpretationResult；
ReportConclusion。
```

这些属于 Scale / Evaluation。

边界表：

| 分值类型 | 归属 | 说明 |
| --- | --- | --- |
| OptionScore | Survey / Questionnaire | 选项基础分配置 |
| Answer.Score | Survey / AnswerSheet | 单题基础分事实 |
| FactorScore | Evaluation | 因子得分结果 |
| TotalScore | Evaluation | 总分结果 |
| RiskLevelResult | Evaluation | 风险等级命中结果 |

---

## 11. AnswerSheet.Submit 的领域语义

`AnswerSheet.Submit` 是 AnswerSheet 聚合的核心领域入口。

它表达：

```text
创建一份正式提交事实。
```

### 11.1 Submit 应该接收什么

Submit 的输入可以抽象为：

```text
AnswerSheetID；
QuestionnaireRef；
SubmissionContext；
Answers；
FilledAt。
```

这些信息共同构成一次完整提交事实。

### 11.2 Submit 应该保护什么

Submit 至少应保护以下不变量。

| 不变量 | 说明 |
| --- | --- |
| ID 不能为空 | 答卷事实必须有唯一标识 |
| QuestionnaireRef 合法 | 必须引用确定问卷版本 |
| SubmissionContext 合法 | 必须知道提交上下文 |
| Answers 非空 | 正式提交不能没有答案 |
| QuestionCode 不重复 | 同一答卷中同一题不能重复提交 |
| FilledAt 合法 | 提交时间不能是非法零值 |
| 产生 SubmittedEvent | 提交事实成立后必须有事件 |

### 11.3 Submit 不应该做什么

Submit 不应该：

```text
加载 Questionnaire；
判断 question_code 是否属于问卷；
执行 required/min/max 等校验规则；
计算因子分；
判断风险等级；
生成报告；
创建 Assessment。
```

这些职责分别属于：

| 职责 | 归属 |
| --- | --- |
| 加载已发布问卷 | Application Service / Repository |
| 判断题目归属 | SubmissionSpec |
| 执行答案规则校验 | AnswerValidator |
| 计算和解释 | Scale / Evaluation |
| 创建 Assessment | Evaluation |

---

## 12. AnswerSheetSubmittedEvent

`AnswerSheetSubmittedEvent` 是 AnswerSheet 提交事实产生后的领域事件。

它表达：

```text
一份答卷事实已经正式提交。
```

它不表达：

```text
Assessment 已经创建；
Evaluation 已经完成；
报告已经生成；
风险等级已经计算。
```

### 12.1 事件应该从 AnswerSheet 导出

事件 payload 应该来自 AnswerSheet 自身。

典型字段包括：

```text
AnswerSheetID；
QuestionnaireCode；
QuestionnaireVersion；
Filler；
Testee；
OrgID；
TaskID；
SubmittedAt。
```

这些字段之所以能从 AnswerSheet 导出，是因为：

```text
QuestionnaireRef 已经入模；
SubmissionContext 已经入模；
FilledAt 已经入模。
```

如果事件 payload 还需要由 application 临时拼接，说明 AnswerSheet 模型本身还不完整。

### 12.2 事件的边界

`AnswerSheetSubmittedEvent` 是 Survey 对外声明的事实。

它的语义是：

```text
后续模块可以开始处理这份答卷。
```

但后续怎么处理，不属于这个事件本身。

```text
使用哪份 Scale；
是否创建 Assessment；
是否生成报告；
是否更新 Plan；
是否更新 Statistics。
```

这些属于下游模块。

---

## 13. AnswerSheet 与 Questionnaire 的边界

AnswerSheet 不拥有 Questionnaire。

它只保存：

```text
QuestionnaireRef
```

边界如下。

| 概念 | 归属 |
| --- | --- |
| Questionnaire | Survey / template aggregate |
| Question | Questionnaire 内部模型 |
| SubmissionSpec | Questionnaire 的提交规格 |
| AnswerSheet | Survey / fact aggregate |
| Answer | AnswerSheet 内部值对象 |

关系是：

```text
Questionnaire 产生 SubmissionSpec；
SubmissionSpec 准备提交答案；
AnswerSheet 引用 QuestionnaireRef 并保存作答事实。
```

AnswerSheet 不反向修改 Questionnaire。

---

## 14. AnswerSheet 与 Scale / Evaluation 的边界

AnswerSheet 是 Scale / Evaluation 的事实输入，但不依赖它们。

### 14.1 与 Scale

Scale 定义：

```text
Factor；
ScoringSpec；
InterpretationRules。
```

AnswerSheet 提供：

```text
QuestionnaireRef；
Answers；
Answer.Score。
```

Scale 不应该被 AnswerSheet 直接引用。

### 14.2 与 Evaluation

Evaluation 负责：

```text
加载 AnswerSheet；
解析 EvaluationModel；
加载规则；
计算 FactorScore；
匹配 RiskLevel；
生成 Report。
```

AnswerSheet 只负责：

```text
提供作答事实；
通过 SubmittedEvent 通知事实已发生。
```

---

## 15. 当前模型成熟度评价

| 方面 | 评价 |
| --- | --- |
| AnswerSheet 聚合边界 | 已与 Questionnaire 模板模型分离 |
| QuestionnaireRef | 能追溯答卷基于哪份问卷版本 |
| SubmissionContext | 能表达填写人、受试者、组织、任务上下文 |
| Answer 模型 | 能承载单题答案事实 |
| AnswerValue | 能支撑类型化答案和题型扩展 |
| Answer.Score | 已限定为单题基础分，不越界到测评结果 |
| Submit 语义 | 创建即提交，不维护后端草稿状态机 |
| SubmittedEvent | 能从 AnswerSheet 自身导出事件 payload |
| 模块边界 | 未侵入 Scale / Evaluation 的解释职责 |

综合判断：

```text
AnswerSheet 模型已经能够承担 Survey 事实侧的核心职责：保存一份合法、类型化、可追溯的作答事实，并为后续 Evaluation 提供事件起点。
```

---

## 16. 后续演进方向

### 16.1 AnswerValue 语义增强

建议逐步增强：

```text
Kind()
IsEmpty()
AsString()
AsNumber()
OptionCode()
OptionCodes()
```

目标是减少 `Raw() any` 在领域层传播。

### 16.2 AnswerSheet Snapshot

可以考虑为查询和事件输出提供只读 snapshot。

例如：

```text
AnswerSheetSnapshot
├── ID
├── QuestionnaireRef
├── SubmissionContext
├── Answers
└── FilledAt
```

用于避免外部直接持有内部 Answers slice。

### 16.3 SubmittedEvent 快照化

如果后续 AnswerSheet 模型继续演进，可以确保 SubmittedEvent 保存必要字段快照，而不是只保存 ID。

这样 Worker 消费时可以更稳定地感知提交上下文。

### 16.4 与 EvaluationModelRef 协作

未来支持 MBTI、Big Five、DISC 时，AnswerSheet 不需要感知具体模型。

它继续提供：

```text
QuestionnaireRef；
SubmissionContext；
Answers；
answersheet.submitted。
```

具体使用哪个 Evaluator，由 Plan / EvaluationModelResolver / Evaluation 决定。

---

## 17. 不建议做的事情

| 不建议 | 原因 |
| --- | --- |
| 给 AnswerSheet 增加复杂草稿状态机 | 后端答卷是提交事实，草稿属于前端体验 |
| 让 AnswerSheet 持有完整 Questionnaire | 会混淆模板聚合与事实聚合 |
| 让 AnswerSheet 判断题目是否属于问卷 | 这是 SubmissionSpec 的职责 |
| 让 AnswerSheet 执行 required/min/max 校验 | 这是 AnswerValidator 的职责 |
| 在 AnswerSheet 中保存 FactorScore | 会污染 Survey 与 Evaluation 边界 |
| 把 Answer.Score 当成风险结果 | Answer.Score 只是单题基础分 |
| 在 SubmittedEvent 中声明 Assessment 已创建 | submitted 事件只表达答卷已提交 |
| 让 AnswerSheet 决定 EvaluationModel | 模型选择属于 Plan / EvaluationModelResolver / Evaluation |

---

## 18. 代码锚点

| 类型 | 路径 |
| --- | --- |
| AnswerSheet 聚合 | `internal/apiserver/domain/survey/answersheet/answersheet.go` |
| SubmissionContext / QuestionnaireRef | `internal/apiserver/domain/survey/answersheet/types.go` |
| Answer / AnswerValue | `internal/apiserver/domain/survey/answersheet/answer.go` |
| AnswerSheet 领域事件 | `internal/apiserver/domain/survey/answersheet/events.go` |
| AnswerSheet 仓储端口 | `internal/apiserver/domain/survey/answersheet/repository.go` |
| Answer 校验适配 | `internal/apiserver/domain/survey/answersheet/validation_adapter.go` |
| SubmissionSpec | `internal/apiserver/domain/survey/questionnaire/submission_spec.go` |
| 提交应用服务 | `internal/apiserver/application/survey/answersheet/submission_service.go` |
| 答案准备 | `internal/apiserver/application/survey/answersheet/submission_answer_assembler.go` |
| 提交 finalizer | `internal/apiserver/application/survey/answersheet/submission_finalizer.go` |

---

## 19. Verify

修改 AnswerSheet / Answer / AnswerValue 后，建议执行：

```bash
go test ./internal/apiserver/domain/survey/answersheet/...
go test ./internal/apiserver/application/survey/answersheet/...
```

如果改动涉及 SubmissionSpec 协作：

```bash
go test ./internal/apiserver/domain/survey/questionnaire/...
go test ./internal/apiserver/application/survey/answersheet/...
```

如果改动涉及存储映射：

```bash
go test ./internal/apiserver/infra/mongo/answersheet/...
```

如果改动涉及事件契约：

```bash
make docs-hygiene
```

---

## 20. 面试与宣讲口径

### 20.1 30 秒版本

```text
AnswerSheet 是 Survey 域中的答卷提交事实聚合，不是草稿、不是报告、不是测评结果。
它保存 QuestionnaireRef、SubmissionContext 和一组类型化 Answers，表达某个填写人在某个业务上下文中，基于某个问卷版本提交了什么答案。
AnswerSheet.Submit 会保护提交事实的不变量，并产生 AnswerSheetSubmittedEvent，作为后续 Evaluation 的起点。
```

### 20.2 3 分钟版本

```text
在 Survey 模块里，Questionnaire 是模板，AnswerSheet 是事实。

AnswerSheet 表达的是一次正式提交事实：谁填的、为谁填的、属于哪个组织和任务、基于哪份问卷版本、提交了哪些答案。它内部包含 QuestionnaireRef、SubmissionContext、Answer 集合、FilledAt 和领域事件。

这里有几个边界很重要。第一，AnswerSheet 后端没有草稿状态。前端可以保存填写过程，但后端只保存提交成功后的业务事实。第二，AnswerSheet 只引用 QuestionnaireRef，不持有完整 Questionnaire，避免模板变化污染历史事实。第三，Answer 使用 AnswerValue 保存类型化答案，避免 raw any 在领域层扩散。第四，Answer.Score 只是单题基础分，不是因子分、风险等级或报告结论。

AnswerSheet.Submit 是提交事实的领域入口，它负责校验 ID、QuestionnaireRef、SubmissionContext、Answers、FilledAt 等基本不变量，并产生 AnswerSheetSubmittedEvent。它不负责加载问卷、不负责执行 required/min/max 校验、不负责计算因子分或生成报告。这些职责分别由 SubmissionSpec、AnswerValidator、Scale 和 Evaluation 承担。
```

### 20.3 高频追问

| 追问 | 回答要点 |
| --- | --- |
| AnswerSheet 是聚合根吗？ | 是，它保护一次提交事实的整体一致性 |
| 为什么后端没有草稿？ | 后端只保存正式提交事实，草稿属于前端填写体验 |
| 为什么需要 QuestionnaireRef？ | 追溯答卷基于哪份问卷版本提交 |
| 为什么需要 SubmissionContext？ | 让答卷自己表达填写人、受试者、组织、任务上下文 |
| Filler 和 Testee 区别？ | Filler 是填写动作执行者，Testee 是被测评对象 |
| AnswerValue 解决什么？ | 把 raw value 收敛成类型化答案事实 |
| Answer.Score 是测评结果吗？ | 不是，只是单题基础分，FactorScore 和 RiskLevel 属于 Evaluation |
| SubmittedEvent 表达什么？ | 只表达答卷已提交，不表达评估已完成 |
| AnswerSheet 是否依赖 Scale？ | 不依赖，Scale/Evaluation 后续消费 AnswerSheet 事实 |

---

## 21. 下一篇文档

下一篇建议维护：

```text
03-测评服务查询与提交链路.md
```

重点回答：

```text
collection-server 在 Survey 服务链路中的职责；
qs-apiserver 如何作为 Survey 领域事实源；
问卷查询链路如何组织；
答卷提交链路如何从 DTO 进入 SubmissionService；
SubmissionService 如何编排 Questionnaire / SubmissionSpec / AnswerValidator / AnswerSheet.Submit。
```
