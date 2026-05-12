# qs-server 模型化与服务化解读报告

> 本报告用于在文档重建前，对 qs-server 当前源码与系统架构进行一次总览式解读。
>
> 报告重点不是复述功能，而是回答三个问题：
>
> 1. qs-server 当前到底是一个什么类型的系统？
> 2. 当前源码在模块化、服务化、应用服务与领域模型建设上成熟到什么程度？
> 3. 后续文档重建与架构演进应该围绕哪些主轴展开？

---

## 1. 结论先行

qs-server 当前已经不是普通问卷 CRUD 系统，而是一个围绕 **问卷作答事实、量表规则、异步评估、报告生成、读侧统计和运行时治理** 构建的测评业务系统。

它的核心业务主轴可以概括为：

```text
Survey 管“用户填了什么”；
Scale 管“这些答案按照什么规则计算和解释”；
Evaluation 管“一次测评如何被执行、归档和产出报告”。
```

从当前源码与文档体系看，qs-server 已经具备比较清晰的三进程运行时边界：

```text
collection-server：前台 BFF 与提交保护层
qs-apiserver：主业务事实源与领域模型中心
qs-worker：事件消费者与异步评估驱动器
```

其中 `qs-apiserver` 是系统核心，承载 Survey、Scale、Evaluation、Actor、Plan、Statistics 等业务模块；`collection-server` 不拥有主业务聚合，只负责前台入口保护和提交代理；`qs-worker` 不直接拥有业务状态机，只通过内部调用推进异步链路。

### 1.1 当前总体评价

| 维度 | 评价 |
| --- | --- |
| 系统定位 | 已经从“问卷系统”升级为“测评业务系统” |
| 模块化 | Survey / Scale / Evaluation 主边界清楚，Actor / Plan / Statistics 辅助边界基本成立 |
| 服务化 | 三进程职责清楚，collection / apiserver / worker 分工合理 |
| 应用服务 | 已经按用例拆分，但部分模块仍有流程服务偏厚的风险 |
| 领域模型 | Survey 已经明显强模型化；Scale 有领域对象雏形；Evaluation 仍需进一步抽象 |
| 事件驱动 | Outbox + MQ + Worker 主链路清楚，具备可靠异步处理意识 |
| 文档基础 | 已有总览、运行时、业务模块、基础设施、专题分析、宣讲等结构，但还需要统一范式重建 |

### 1.2 最大亮点

qs-server 最大亮点不是“能创建问卷、提交答卷、生成报告”，而是它已经形成了一条比较完整的测评业务事实链路：

```text
AnswerSheet 提交事实
  -> answersheet.submitted 事件
  -> Assessment 创建
  -> assessment.submitted 事件
  -> Evaluation Pipeline / Engine 执行
  -> AssessmentScore / EvaluationResult
  -> InterpretReport / EvaluationReport
  -> Statistics 读侧投影
```

这条链路天然覆盖了中高级后端工程项目常见的关键能力：

- 作答事实建模；
- 规则模型管理；
- 异步评估；
- 事件出站可靠性；
- Worker 消费幂等；
- 业务状态机推进；
- 结果与报告归档；
- 高并发入口保护；
- 读侧统计投影；
- 运维观测与故障治理。

### 1.3 最大风险

当前最大风险不在 Survey，而在 Evaluation。

Survey / AnswerSheet 提交链路经过强模型重构后，已经具备 `Questionnaire.SubmissionSpec`、`AnswerSheet.Submit`、`SubmissionContext`、`SubmittedEvent` 等比较清晰的模型表达。

但 Evaluation 当前仍然容易被写成：

```text
Assessment + Scale 专属 Pipeline + 心理量表报告生成
```

如果后续支持 MBTI、Big Five、DISC 或其他人格/能力测评模型，而 Evaluation 仍然默认只理解 MedicalScale、FactorScore、RiskLevel、InterpretReport，那么系统会被迫在 Evaluation 内部堆积大量 `if model_type == ...` 的分支，导致 Evaluation 从“测评执行域”退化成“多模型流程脚本集合”。

因此，后续架构准备的核心原则应该是：

> 当前不急着增加 MBTI，但要让 Evaluation 不再默认等于 Scale Evaluation。

---

## 2. 系统定位：从问卷系统到测评业务系统

qs-server 的系统定位应该从“问卷&量表系统”进一步提升为：

> qs-server 是一个面向心理/医学测评场景的测评业务系统，负责从问卷作答事实出发，基于测评模型规则异步执行评估，生成可追踪、可归档、可统计的测评结果与报告。

这个定位比“问卷系统”更准确。

因为问卷只是入口事实，量表只是当前主要测评模型，真正的系统主线是：

```text
作答事实 -> 测评模型 -> 测评执行 -> 结果归档 -> 报告输出 -> 统计投影
```

### 2.1 不应该把 Survey 理解成系统核心全部

Survey 很重要，但 Survey 只回答：

```text
用户填了哪份问卷？
填的是哪个问卷版本？
每道题提交了什么答案？
答案是否符合题型和校验规则？
```

Survey 不应该回答：

```text
这个答案得多少分？
这个因子属于什么风险等级？
这个人属于哪种人格类型？
这份报告应该怎么写？
```

这些问题属于 Scale / Evaluation / Report / 后续 Personality 模块。

### 2.2 不应该把 Scale 理解成所有测评模型的父类

Scale 当前负责 MedicalScale、Factor、ScoringRule、InterpretationRule、RiskLevel 等量表规则模型。它适合表达医学/心理量表类测评：

```text
题目答案 -> 因子得分 -> 总分/维度分 -> 风险等级 -> 解读规则 -> 报告建议
```

但它不适合承载所有测评模型。

例如未来的 MBTI 16 人格测评，其核心不是 RiskLevel，也不是医学量表因子，而是：

```text
E/I、S/N、T/F、J/P 四组偏好维度
  -> 维度倾向分
  -> 四字母人格类型
  -> 人格画像与解释内容
```

因此，Scale 应该保持专属化，不应该被改造成“万能测评模型”。

### 2.3 Evaluation 应该成为测评执行中心

Evaluation 的职责不是“专门为 Scale 算分”，而应该是：

```text
管理一次测评执行的生命周期；
基于某个 EvaluationModelRef 选择对应 Evaluator；
执行评估；
归档结果；
生成报告；
推进 Assessment 状态机；
发布测评生命周期事件。
```

当前阶段只有 ScaleEvaluator，一个实现就够了。

但是系统结构应该准备好未来扩展为：

```text
ScaleEvaluator
MBTIEvaluator
BigFiveEvaluator
DISCEvaluator
```

这就是 qs-server 下一阶段架构准备的核心。

---

## 3. 三进程服务化模型

qs-server 当前采用三进程协作模型。

```text
Client / Mini Program
  -> collection-server
  -> qs-apiserver
  -> Outbox / MQ
  -> qs-worker
  -> qs-apiserver
```

### 3.1 collection-server

`collection-server` 是前台入口保护层。

主要职责：

- 提供前台 REST BFF；
- 处理 RateLimit；
- 处理 SubmitQueue；
- 处理 SubmitGuard；
- 处理 submit-status 查询；
- 通过 gRPC 调用 qs-apiserver 保存答卷。

它不应该负责：

- 直接写主业务数据库；
- 直接创建 Assessment；
- 直接执行 Evaluation；
- 拥有 Survey / Scale / Evaluation 聚合；
- 消费业务 MQ。

它的定位可以概括为：

> collection-server 是前台高并发提交入口的保护层，不是主业务事实源。

### 3.2 qs-apiserver

`qs-apiserver` 是系统主业务中心。

主要职责：

- 承载 Survey / Scale / Evaluation / Actor / Plan / Statistics 等业务模块；
- 暴露管理端 REST；
- 暴露内部 gRPC；
- 维护主业务聚合；
- 负责 MySQL / MongoDB / Redis / Outbox 等基础设施适配；
- 维护 Assessment 状态机；
- 执行测评用例编排；
- 保存结果与报告。

它的定位可以概括为：

> qs-apiserver 是领域事实源和业务状态机中心。

### 3.3 qs-worker

`qs-worker` 是异步事件驱动器。

主要职责：

- 订阅 MQ；
- 分发事件；
- 执行 Ack / Nack；
- 通过 internal gRPC 回调 qs-apiserver；
- 推进评分、Assessment 创建、Evaluation 执行等异步任务。

它不应该负责：

- 直接写主业务表；
- 直接绕过 apiserver 修改 Assessment 状态；
- 自己实现第二套业务状态机；
- 持有 Survey / Evaluation 聚合规则。

它的定位可以概括为：

> qs-worker 是异步推进器，不是业务事实源。

---

## 4. 领域模型总览

qs-server 当前可以按照六个主要业务模块理解。

```text
Survey
Scale
Evaluation
Actor
Plan
Statistics
```

其中前三个是主轴模块，后三个是支撑模块。

---

## 5. Survey：作答事实域

### 5.1 模块定位

Survey 负责问卷结构与作答事实。

它回答的是：

```text
可以填什么？
用户实际填了什么？
这份答卷绑定的是哪个问卷版本？
提交时答案是否符合题型和校验规则？
```

Survey 不负责解释答案的医学/心理含义。

### 5.2 当前模型结构

Survey 内部可以拆成两个核心模型：

```text
Questionnaire：问卷模板聚合
AnswerSheet：答卷提交事实聚合
```

当前强模型重构后，Survey 已经形成比较清晰的提交链路：

```text
Published Questionnaire
  -> BuildSubmissionSpec
  -> SubmissionSpec.PrepareAnswers
  -> AnswerValue / ValidationTask
  -> AnswerValidator.ValidateAnswers
  -> AnswerSheet.Submit
  -> AnswerSheetSubmittedEvent
  -> DurableStore.CreateDurably
```

### 5.3 Questionnaire 的职责

Questionnaire 不只是问题列表。

它应该负责：

- 管理问卷 code / version；
- 管理题目结构；
- 管理题型和选项；
- 管理校验规则；
- 管理发布状态；
- 暴露可提交规格。

当前已经引入 `SubmissionSpec`，这是非常关键的模型进步。

`SubmissionSpec` 让 Questionnaire 可以明确表达：

```text
这份已发布问卷允许提交哪些题目；
每道题的真实题型是什么；
每道题有哪些校验规则；
客户端提交的答案是否属于这份问卷规格。
```

这样，application service 不再直接拼 question map，也不再直接信任 DTO 中的 question_type。

### 5.4 AnswerSheet 的职责

AnswerSheet 应该表达一次完整、不可变、可追踪的提交事实。

它至少包含：

```text
AnswerSheetID
QuestionnaireRef
SubmissionContext
Answers
FilledAt / SubmittedAt
DomainEvents
```

当前已经通过 `AnswerSheet.Submit(...)` 把提交行为收口到了领域模型中。

这说明 AnswerSheet 不再只是一个“答卷数据结构”，而是表达：

> 某个填写人在某个组织/任务上下文中，基于某个问卷版本，提交了一组合法答案，并由此产生 AnswerSheetSubmittedEvent。

### 5.5 SubmissionContext 的意义

`SubmissionContext` 是这轮 Survey 强模型重构中的关键对象。

它解决了过去的问题：

```text
AnswerSheet 只有 filler，不知道 testee / org / task；
事件 payload 需要外部传参拼装；
完整提交事实散落在 DTO / DurableMeta / Event 参数中。
```

引入 SubmissionContext 后，提交上下文进入领域模型，事件可以从 AnswerSheet 自身导出。

这使得 Survey 的模型表达更完整。

### 5.6 Survey 当前评价

| 维度 | 评价 |
| --- | --- |
| 模块边界 | 清楚，未侵入 Scale / Evaluation |
| Questionnaire | 已通过 SubmissionSpec 增强可提交语义 |
| AnswerSheet | 已从数据结构升级为提交事实聚合 |
| SubmissionContext | 已将填写人、受试者、组织、任务上下文纳入提交事实 |
| 领域事件 | SubmittedEvent 已能从提交事实中产生 |
| 应用服务 | 主要负责编排，业务语义明显下沉 |
| 后续小修 | 继续强化不可变性、slice clone、nil 入参防御、AnswerValue 语义方法 |

Survey 当前已经可以作为 qs-server 文档重建的第一个样板模块。

---

## 6. Scale：量表规则域

### 6.1 模块定位

Scale 负责当前系统最主要的测评模型：医学/心理量表。

它回答的是：

```text
这份量表绑定哪份问卷？
有哪些因子？
每个因子如何计分？
总分或因子分如何解释？
不同分数区间对应什么风险等级？
报告中的解读规则是什么？
```

Scale 不负责：

```text
保存用户答卷；
管理 Assessment 生命周期；
执行异步 Worker；
生成通用测评生命周期事件；
承载所有未来测评模型。
```

### 6.2 核心模型

Scale 当前可以围绕这些对象建模：

```text
MedicalScale
Factor
ScoringRule
InterpretationRule
RiskLevel
ScaleVersion / QuestionnaireRef
```

其中 `MedicalScale` 应该是聚合根。

它负责保护：

- 量表 code / version；
- 绑定的 QuestionnaireRef；
- 因子集合；
- 因子编码唯一；
- 总分因子唯一；
- 发布状态；
- 发布后不可变；
- 解读规则完整性。

### 6.3 当前风险

Scale 当前最大风险是被未来模型污染。

如果为了支持 MBTI，把 Scale 扩展成：

```text
MedicalScale + PersonalityScale + MBTIScale + DimensionScale
```

那么 Scale 会很快退化成“万能规则配置容器”。

这不是好方向。

Scale 应该保持专属化：

```text
Scale = 量表规则域
Personality / MBTI = 未来同级规则域
Evaluation = 通用测评执行域
```

### 6.4 当前阶段建议

当前不增加 MBTI 的前提下，Scale 应该做的不是泛化，而是把自己作为当前唯一 EvaluationModel 实现暴露出去。

也就是说：

```text
Scale 仍然是 Scale；
但 Evaluation 不再把 Scale 当作唯一前提；
ScaleEvaluator 是当前唯一 Evaluator。
```

---

## 7. Evaluation：测评执行域

### 7.1 模块定位

Evaluation 是 qs-server 当前最需要继续抽象的模块。

它不应该只是 Scale 的执行脚本，而应该负责一次测评执行的完整生命周期：

```text
Assessment 创建
Assessment 提交
Assessment 执行中
Assessment 解释完成
Assessment 失败
结果归档
报告归档
事件发布
```

### 7.2 Evaluation 的核心概念

后续 Evaluation 应该围绕这些概念稳定下来：

```text
Assessment
EvaluationModelRef
Evaluator
EvaluationInput
EvaluationOutput
EvaluationResult
EvaluationReport
```

其中最关键的是 `EvaluationModelRef`。

当前版本可以只有：

```text
ModelType = scale
```

但是 Assessment 不应该直接把 Scale 当作唯一模型前提，而应该引用：

```text
EvaluationModelRef {
  Type: scale
  Code: xxx
  Version: xxx
}
```

这样未来支持 MBTI 时，只需要新增：

```text
ModelType = mbti
MBTIModel
MBTIEvaluator
MBTIResult
MBTIReport
```

而不是重写 Assessment 主生命周期。

### 7.3 Assessment 的职责

Assessment 应该表达一次测评执行。

它不是答卷，也不是量表，也不是报告。

它应该回答：

```text
这次测评属于谁？
基于哪份 AnswerSheet？
基于哪种 EvaluationModel？
当前执行到什么状态？
是否已生成结果？
是否已生成报告？
失败原因是什么？
是否可以重试？
```

理想状态机可以表达为：

```text
pending
  -> submitted
  -> interpreting
  -> interpreted
  -> failed
```

也可以根据当前实现继续保留更简化状态，但核心原则不变：

> Assessment 状态迁移必须由 Evaluation 模型控制，不应该由 Worker 或具体 Evaluator 直接绕过。

### 7.4 Evaluation Engine 的职责

Evaluation Engine 应该只做一件事：

```text
根据 EvaluationModelRef.Type 选择对应 Evaluator，并执行评估。
```

当前唯一实现是：

```text
ScaleEvaluator
```

后续才会增加：

```text
MBTIEvaluator
BigFiveEvaluator
DISCEvaluator
```

当前不要提前实现 MBTI，但要把 Scale 专属 pipeline 包装成显式的 ScaleEvaluator。

### 7.5 Result / Report 的抽象

当前 Scale 结果可能包含：

```text
TotalScore
FactorScores
RiskLevel
Interpretations
```

但通用 EvaluationResult 不应该被这些字段绑死。

建议分两层：

```text
EvaluationResult：通用结果归档
ScaleEvaluationResult：Scale 专属 payload
```

Report 也一样。

当前 Scale 报告可以包含：

```text
factor score section
risk interpretation section
suggestion section
```

未来 MBTI 报告可能包含：

```text
type overview section
dimension preference section
strength section
relationship section
career section
```

所以 EvaluationReport 应该是通用结构，具体报告内容由不同 Evaluator / ReportBuilder 生成。

### 7.6 Evaluation 当前评价

| 维度 | 评价 |
| --- | --- |
| 模块重要性 | 最高，是未来扩展多测评模型的关键 |
| 当前成熟度 | 已有 assessment / engine 拆分，但仍需确认 Scale 耦合程度 |
| 主要风险 | Evaluation 被 Scale Pipeline 和心理量表报告结构绑死 |
| 当前重构目标 | 先引入 EvaluationModelRef / Evaluator 抽象，不新增 MBTI |
| 文档重点 | 讲清 Assessment 生命周期、Engine 调度、Result/Report 归档 |

---

## 8. Actor：测评参与者与身份投影域

### 8.1 模块定位

Actor 负责表达测评业务中的人和角色。

它不应该替代 IAM，也不应该重新实现认证授权。

它更像 qs-server 内部的业务身份投影：

```text
Testee：受试者
Clinician：医生 / 解读者
Operator：后台操作员
Guardian：监护人
Filler：填写人
```

### 8.2 与 IAM 的关系

IAM 是身份与权限事实源。

qs-server 中的 Actor 应该只保存测评业务所需的本地投影或引用，不应该把 IAM 的完整用户、账号、角色、权限模型复制一遍。

可以这样理解：

```text
IAM 负责“这个调用者是谁，有没有权限”；
Actor 负责“这个人在测评业务里扮演什么角色”。
```

### 8.3 与 Survey / Evaluation 的关系

Survey 的 SubmissionContext 会引用 Filler / Testee / Org / Task。

Evaluation 的 Assessment 会引用 Subject / Testee / Org / Plan 信息。

所以 Actor 是 Survey 和 Evaluation 的共同支撑模块。

### 8.4 当前文档重建重点

Actor 模块文档应该重点回答：

```text
Actor 和 IAM 的边界是什么？
Filler / Testee / Guardian / Operator 有什么区别？
SubmissionContext 为什么需要 Actor 引用？
Assessment 中的测评对象如何表达？
```

---

## 9. Plan：测评计划与任务编排域

### 9.1 模块定位

Plan 负责测评任务编排。

它回答的是：

```text
谁需要做哪项测评？
在哪个时间窗口内完成？
绑定哪份问卷？
绑定哪种测评模型？
任务状态如何推进？
任务完成后是否触发提醒或统计？
```

### 9.2 Plan 对未来多测评模型非常关键

如果未来支持 MBTI，系统需要知道：

```text
这份 AnswerSheet 提交后，到底应该走 ScaleEvaluator 还是 MBTIEvaluator？
```

这个决策不应该由 Survey 做。

Survey 只知道 Questionnaire / AnswerSheet。

更合理的位置是 Plan 或 EvaluationModelResolver。

因此，Plan 后续可以逐步承载：

```text
Task -> QuestionnaireRef -> EvaluationModelRef
```

当前版本可以仍然只解析到 Scale，但模型表达上要为 EvaluationModelRef 预留位置。

### 9.3 当前文档重建重点

Plan 模块文档应该重点回答：

```text
Plan / Task 与 AnswerSheet 的关系是什么？
Task 如何约束受试者、问卷和提交窗口？
Task 如何影响 Assessment 创建？
未来如何通过 Task 绑定不同 EvaluationModel？
```

---

## 10. Statistics：读侧统计投影域

### 10.1 模块定位

Statistics 负责读侧统计聚合。

它不应该成为主业务状态机的一部分，也不应该反向影响 Survey / Evaluation 的写入链路。

它主要消费业务事件或读取事实数据，构建查询友好的统计视图。

### 10.2 通用统计与模型专属统计要分层

为了未来支持多测评模型，Statistics 需要分成两层。

第一层是通用测评统计：

```text
提交量
完成量
失败量
评估耗时
报告生成量
任务完成率
机构维度统计
受试者维度统计
```

第二层是模型专属统计：

```text
Scale：风险等级分布、因子分分布、量表完成情况
MBTI：人格类型分布、维度倾向分布
BigFive：五维人格分布
```

当前阶段只有 Scale，但统计模型不要被 Scale-only 字段绑死。

### 10.3 当前文档重建重点

Statistics 文档应该重点回答：

```text
哪些统计来自通用 Assessment 生命周期？
哪些统计来自 Scale 专属结果？
读侧投影如何重建？
缓存与热数据如何治理？
统计延迟如何解释？
```

---

## 11. 应用服务层评价

qs-server 当前 application 层已经有比较明确的用例拆分。

以 Survey / AnswerSheet 提交链路为例，application service 主要做：

```text
DTO 校验
加载 Questionnaire
构建 SubmissionSpec
准备答案与校验任务
调用 AnswerValidator
调用 AnswerSheet.Submit
调用 DurableStore
返回提交结果
```

这是健康方向。

### 11.1 健康的 application service

一个健康的 application service 应该做：

```text
编排用例；
加载聚合；
调用领域行为；
控制事务边界；
调用 repository / port；
处理幂等；
组织返回 DTO。
```

它不应该做：

```text
直接判断领域状态是否合法；
直接拼装领域事件 payload；
直接修改聚合内部字段；
把复杂业务规则写成流程脚本；
绕过领域对象生成结果。
```

### 11.2 当前重点关注点

接下来最需要检查的是 Evaluation application service。

核心问题包括：

```text
Assessment 状态转换是否由 Assessment 聚合保护？
EvaluateAssessment 是否只是编排，还是包含大量 Scale 业务判断？
Pipeline handler 是应用流程步骤，还是领域规则承载者？
Report 生成是否和 Scale 强耦合？
失败和重试是否被状态机建模？
```

---

## 12. 事件与 Outbox 评价

qs-server 当前使用事件系统串联异步主链路。

核心原则是：

```text
业务状态由 apiserver 管；
事件由 Outbox 可靠出站；
MQ 负责投递；
Worker 负责消费和回调；
业务侧负责幂等。
```

这个方向是正确的。

### 12.1 Survey 事件

Survey 应该发布：

```text
answersheet.submitted
```

该事件只表达答卷提交事实。

它不应该表达：

```text
应该使用哪个 Scale；
应该生成哪类报告；
应该执行哪个具体评估 pipeline。
```

### 12.2 Evaluation 事件

Evaluation 应该发布测评生命周期事件：

```text
assessment.created
assessment.submitted
assessment.interpreting
assessment.interpreted
assessment.failed
report.generated
```

这些事件最好携带 `EvaluationModelRef`。

这样未来支持 MBTI 时，事件结构可以保持稳定。

### 12.3 Worker 边界

Worker 消费事件后，应该通过 internal gRPC 回调 apiserver。

Worker 不应该直接写主业务表。

如果 Worker 直接修改 Assessment 状态，就会破坏 apiserver 作为主业务事实源的边界。

---

## 13. 基础设施与运行时治理评价

qs-server 当前不只是业务模型项目，也包含比较完整的运行时治理能力。

核心治理能力包括：

```text
RateLimit
SubmitQueue
SubmitGuard
Backpressure
LockLease
Worker concurrency
Health Check
Metrics
pprof
governance status
```

这些能力让 qs-server 的工程表达超出普通 CRUD 系统。

### 13.1 collection-server 的保护链

前台提交链路中，collection-server 可以形成：

```text
RateLimit
  -> SubmitQueue
  -> SubmitGuard
  -> gRPC Client
  -> qs-apiserver
```

这条链路的价值是：

```text
入口限流
削峰填谷
重复提交抑制
明确 Retry-After
避免前台流量直接打穿 apiserver
```

### 13.2 apiserver 的主业务治理

apiserver 负责：

```text
事务写入
幂等记录
Outbox staging
Assessment 状态机
Result / Report 持久化
内部 gRPC API
```

它是所有业务事实的主边界。

### 13.3 worker 的异步治理

worker 负责：

```text
MQ 消费
并发控制
Ack / Nack
失败重试
回调 apiserver
```

worker 的治理重点不是“自己做业务”，而是“稳定推进异步任务”。

---

## 14. 当前系统的主要欠缺

### 14.1 Evaluation 抽象还需要加强

这是最高优先级。

当前阶段不支持 MBTI，但应该提前把 Evaluation 从 Scale 专属链路中解耦。

建议逐步引入：

```text
EvaluationModelRef
Evaluator
EvaluationInput
EvaluationOutput
EvaluationResult
EvaluationReport
```

当前唯一实现是：

```text
ScaleEvaluator
```

### 14.2 Scale 不应继续泛化

Scale 应该保持量表规则域，不应该承担未来所有测评模型。

否则会出现：

```text
Factor 既表示量表因子，又表示 MBTI 维度；
RiskLevel 既表示医学风险，又表示人格倾向；
InterpretationRule 既表示分数区间解释，又表示人格画像解释。
```

这会污染模型语言。

### 14.3 Result / Report 需要分层

当前报告如果过度绑定：

```text
FactorScore
RiskLevel
Interpretation
```

未来接入其他模型会很困难。

应该区分：

```text
通用 EvaluationReport
Scale 专属 ScaleReportPayload
未来 MBTI 专属 MBTIReportPayload
```

### 14.4 Statistics 需要避免 Scale-only

统计应该同时支持：

```text
通用 Assessment 生命周期统计
模型专属结果统计
```

当前只有 Scale 没问题，但模型设计不要被 Scale 锁死。

### 14.5 文档范式还需要统一

当前 docs 已经有不少内容，但后续重建时需要统一成固定模板：

```text
模型总览
模型服务详细拆解
关键链路分析
代码实现评价
模块总结与宣讲口径
```

这样每个模块都能形成统一阅读体验。

---

## 15. 后续架构准备路线

当前不增加 MBTI 的前提下，建议按下面路线推进。

### P0：稳定 Survey 文档样板

Survey 已经完成较强模型化，可以先作为文档重建样板。

建议文档：

```text
docs/02-业务模型/survey/
├── 00-模型总览.md
├── 01-Questionnaire模型与SubmissionSpec.md
├── 02-AnswerSheet提交事实模型.md
├── 03-答卷提交链路分析.md
├── 04-事务幂等与Outbox出站.md
└── 05-Survey模块总结与后续演进.md
```

### P1：梳理 Scale 模型

Scale 文档重点不是泛化，而是讲清：

```text
MedicalScale 聚合根
Factor 模型
ScoringRule
InterpretationRule
RiskLevel
发布生命周期
与 QuestionnaireRef 的关系
```

### P2：重构 Evaluation 概念语言

先不大改代码，先定概念：

```text
Assessment = 一次测评执行
EvaluationModelRef = 测评模型引用
Evaluator = 某类测评模型执行器
ScaleEvaluator = 当前唯一 Evaluator
EvaluationResult = 通用结果归档
EvaluationReport = 通用报告归档
```

### P3：将当前 Scale Pipeline 包装成 ScaleEvaluator

不要一上来重写 Evaluation。

第一步只是把当前 Scale 流程包起来：

```text
EvaluationEngine
  -> dispatch by ModelType
  -> ScaleEvaluator
  -> 当前旧 pipeline
```

行为保持不变，但依赖方向改变。

### P4：Result / Report 通用化

在不破坏当前 Scale 结果的前提下，逐步引入通用 Result / Report 存储接口。

### P5：重建 Statistics 分层

把统计分成：

```text
Assessment 生命周期统计
Scale 结果统计
```

为未来 MBTI 类型分布统计预留位置。

### P6：专题文档总结架构演进

建议新增专题文档：

```text
docs/05-专题分析/08-从量表评估到通用测评执行引擎.md
```

该文档回答：

```text
为什么当前不增加 MBTI？
为什么要提前抽象 Evaluation？
为什么 Scale 不应该泛化？
EvaluationModelRef 解决什么问题？
未来 MBTI 如何无侵入接入？
```

---

## 16. 文档重建总方案

建议后续 docs 重建为以下结构。

```text
docs/
├── 00-总览/
│   ├── README.md
│   ├── 01-系统定位与业务主轴.md
│   ├── 02-三进程协作总览.md
│   ├── 03-领域模型地图.md
│   ├── 04-核心链路--从答卷到报告.md
│   ├── 05-源码事实矩阵.md
│   └── 06-qs-server模型化与服务化解读报告.md
│
├── 01-运行时/
│   ├── README.md
│   ├── 01-qs-apiserver启动与组合根.md
│   ├── 02-collection-server运行时.md
│   ├── 03-qs-worker运行时.md
│   ├── 04-进程间调用与gRPC.md
│   ├── 05-事件消费与回调链路.md
│   └── 06-优雅关闭与资源释放.md
│
├── 02-业务模型/
│   ├── README.md
│   ├── survey/
│   ├── scale/
│   ├── evaluation/
│   ├── actor/
│   ├── plan/
│   └── statistics/
│
├── 03-基础设施/
│   ├── README.md
│   ├── event/
│   ├── data-access/
│   ├── redis/
│   ├── resilience/
│   ├── security/
│   ├── observability/
│   └── integration/
│
├── 04-接口与契约/
│   ├── README.md
│   ├── 01-REST API契约.md
│   ├── 02-gRPC API契约.md
│   ├── 03-事件契约.md
│   ├── 04-配置契约.md
│   └── 05-错误码与响应模型.md
│
├── 05-专题分析/
│   ├── README.md
│   ├── 01-为什么拆分Survey-Scale-Evaluation.md
│   ├── 02-为什么同步提交但异步评估.md
│   ├── 03-为什么需要collection保护层.md
│   ├── 04-为什么使用Outbox.md
│   ├── 05-为什么需要读侧统计聚合.md
│   ├── 06-IAM嵌入式SDK边界分析.md
│   ├── 07-系统演进路线.md
│   └── 08-从量表评估到通用测评执行引擎.md
│
└── 06-宣讲/
    ├── README.md
    ├── 01-项目一句话定位.md
    ├── 02-三分钟项目讲解.md
    ├── 03-五分钟架构讲解.md
    ├── 04-核心链路讲解稿.md
    ├── 05-高频追问与回答.md
    └── 06-证据链索引.md
```

---

## 17. 每个业务模块的统一文档模板

每个业务模块建议统一采用下面结构。

```text
README.md
00-模型总览.md
01-模型服务详细拆解.md
02-关键链路分析.md
03-接口与契约映射.md
04-代码实现评价与重构建议.md
05-模块总结与宣讲口径.md
```

### 17.1 模型总览

回答：

```text
这个模块解决什么业务问题？
核心聚合根是什么？
实体和值对象有哪些？
领域服务有哪些？
模块边界是什么？
不负责什么？
```

### 17.2 模型服务详细拆解

回答：

```text
application service 有哪些？
domain service 有哪些？
repository port 有哪些？
infra adapter 有哪些？
用例编排和领域行为如何协作？
```

### 17.3 关键链路分析

回答：

```text
一次核心业务动作从入口到持久化如何完成？
状态如何变化？
事件在哪里产生？
事务和幂等如何保证？
失败和重试如何处理？
```

### 17.4 接口与契约映射

回答：

```text
REST API 如何映射 application service？
gRPC API 如何映射内部用例？
事件契约如何映射领域事件？
配置项如何影响模块行为？
```

### 17.5 代码实现评价与重构建议

回答：

```text
当前实现完成了什么？
当前实现还有什么欠缺？
哪些地方不要重构？
哪些地方应该下一阶段重构？
```

### 17.6 模块总结与宣讲口径

回答：

```text
30 秒怎么讲？
3 分钟怎么讲？
5 分钟怎么讲？
面试官可能追问什么？
代码证据链在哪里？
```

---

## 18. 面试与宣讲表达

qs-server 适合被讲成一个中高级 Go 后端项目。

不建议这样讲：

```text
我做了一个问卷系统，可以创建问卷、提交答卷、生成报告。
```

这个表达太弱。

建议这样讲：

```text
我做的是一个面向心理/医学测评场景的测评业务系统。
系统采用 collection-server、qs-apiserver、qs-worker 三进程协作。
前台提交由 collection-server 做限流、排队和重复提交抑制；
核心业务事实由 qs-apiserver 管理；
答卷提交后通过 Outbox 可靠出站，再由 worker 异步驱动 Assessment 创建、量表计分、风险解释和报告生成。
在领域建模上，我把 Survey、Scale、Evaluation 拆成三个核心业务域：
Survey 只保存作答事实，Scale 只管理量表规则，Evaluation 管理一次测评执行生命周期。
最近我还对 Survey / AnswerSheet 提交链路做了强模型重构，引入 SubmissionSpec、SubmissionContext 和 AnswerSheet.Submit，让提交事实、规则校验和领域事件更清晰地收口到模型中。
```

### 18.1 30 秒版本

```text
qs-server 是一个测评业务系统，不是普通问卷 CRUD。
它通过 collection-server、qs-apiserver、qs-worker 三进程协作，实现前台可靠提交、后台异步评估、报告生成和统计投影。
领域上拆成 Survey、Scale、Evaluation：Survey 管作答事实，Scale 管量表规则，Evaluation 管测评执行生命周期。
```

### 18.2 3 分钟版本

```text
qs-server 面向心理/医学测评场景。系统入口是 collection-server，它负责前台 BFF、限流、排队和重复提交抑制，避免用户高峰直接打穿主业务服务。

真正的业务事实源是 qs-apiserver。apiserver 内部按领域拆成 Survey、Scale、Evaluation、Actor、Plan、Statistics 等模块。其中 Survey 负责问卷与答卷事实，Scale 负责量表规则和解释规则，Evaluation 负责 Assessment 生命周期和报告产出。

用户提交答卷后，系统会同步保存 AnswerSheet，并通过 Outbox stage answersheet.submitted 事件。Outbox relay 把事件投递到 MQ，qs-worker 消费后通过 internal gRPC 回调 apiserver，推进评分、Assessment 创建和 Evaluation 执行。worker 不直接写业务表，业务状态机仍然由 apiserver 控制。

最近我重点重构了 Survey / AnswerSheet 链路。Questionnaire 通过 SubmissionSpec 暴露可提交规格，AnswerSheet 通过 Submit 创建完整提交事实，SubmissionContext 把填写人、受试者、组织和任务上下文纳入模型，SubmittedEvent 从聚合自身产生。这让 application service 从业务判断中解放出来，主要负责编排用例、调用校验端口和 durable store。
```

### 18.3 5 分钟版本强调点

5 分钟讲解可以按以下顺序展开：

```text
1. 系统不是问卷 CRUD，而是测评业务系统；
2. 三进程协作解决前台高峰、主业务事实源和异步评估的边界问题；
3. Survey / Scale / Evaluation 三个核心领域分别承载作答事实、规则模型和执行生命周期；
4. Outbox + MQ + Worker 解决同步提交与异步评估之间的一致性和解耦；
5. Survey 强模型重构展示 DDD 能力：SubmissionSpec / SubmissionContext / AnswerSheet.Submit / SubmittedEvent；
6. 下一阶段计划把 Evaluation 从 Scale 专属 pipeline 抽象为通用测评执行引擎，为 MBTI 等新测评模型做架构准备。
```

---

## 19. 最终判断

qs-server 当前已经具备继续投入的价值。

它的工程亮点不是单点技术，而是多个能力围绕同一条业务主线形成了组合：

```text
领域建模
应用服务编排
三进程运行时边界
事件驱动
Outbox 可靠出站
Worker 异步推进
入口保护
结果报告归档
读侧统计投影
IAM 安全接入
运维观测治理
```

当前阶段最正确的路线是：

```text
先把现有 Survey / Scale / Evaluation 模型讲清楚；
再把 Survey 作为样板完成文档重建；
然后分析 Scale；
最后重点重构和重写 Evaluation；
在不增加 MBTI 的前提下，提前完成 EvaluationModelRef / Evaluator / Result / Report 的抽象准备。
```

一句话总结：

> qs-server 当前应该从“问卷量表系统”升级表达为“以 Scale 为当前唯一模型实现的通用测评执行系统”；Survey 已经完成较好的作答事实建模，Scale 应保持量表规则域专属化，Evaluation 则需要从 Scale 专属执行链路中抽象出来，为下一阶段接入 MBTI 等多测评模型做架构准备。
