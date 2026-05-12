# Survey 模块总结与后续演进

> 本文是 Survey 模块文档重建的收束篇。
>
> 前面几篇已经分别分析了：
>
> - `00-模型总览.md`：Survey 的作答事实域定位；
> - `01-Questionnaire模型与SubmissionSpec.md`：模板侧模型与可提交规格；
> - `02-AnswerSheet提交事实模型.md`：事实侧模型与提交上下文；
> - `03-答卷提交链路分析.md`：从 collection-server 到 outbox 的完整提交链路；
> - `04-事务幂等与Outbox出站.md`：durable boundary、IdempotencyKey 与 Outbox 出站。
>
> 本文用于回答：Survey 模块当前到底已经建设到什么程度？最终边界应该如何定义？还存在哪些补强点？未来支持 MBTI、Big Five、DISC 等新测评模型时，Survey 为什么应该基本保持稳定？Survey 模块如何用于面试和宣讲表达？

---

## 1. 结论先行

Survey 当前已经完成了一轮比较清晰的模型化重构。

它已经从“问卷与答卷 CRUD”升级为：

```text
以 Questionnaire 为模板聚合、以 SubmissionSpec 为可提交规格、以 AnswerSheet 为提交事实聚合、以 DurableStore + Outbox 为可靠出站边界的作答事实域。
```

Survey 当前最核心的设计成果可以概括为四句话：

```text
Questionnaire 管“可提交的模板”；
SubmissionSpec 管“这份模板如何被提交”；
AnswerSheet 管“一次已经发生的提交事实”；
DurableStore 管“提交事实、幂等记录和事件出站的一致性”。
```

一句话总结：

> **Survey 的边界已经基本立住：它只负责可靠采集作答事实，不负责解释这些答案。**

---

## 2. Survey 当前的系统位置

qs-server 当前不是普通问卷 CRUD 系统，而是围绕问卷收集、量表规则、异步评估、报告生成、统计运营和安全授权构建的测评后端系统。README 中也明确拆分了三条核心业务主轴：`Survey 管“填什么”`，`Scale 管“怎么算和怎么解释”`，`Evaluation 管“这一次测评执行后的结果”`。这正是 Survey 文档重建的根本前提。

Survey 位于整条测评链路的事实入口。

```text
Survey
  -> answersheet.submitted
  -> Evaluation
  -> Result / Report
  -> Statistics
```

它的上游是：

```text
Client / collection-server
```

它的下游是：

```text
Outbox / MQ / worker / Evaluation
```

它的职责不是完成整次测评，而是把一次作答稳定转化为业务事实。

---

## 3. Survey 的最终边界定义

Survey 的最终边界可以定义为：

```text
Survey 是作答事实域，负责问卷模板、提交规格、答卷事实、答案值、提交校验、提交幂等和答卷提交事件。
```

它负责：

| 能力 | 说明 |
| --- | --- |
| Questionnaire 模板建模 | 管理问卷 code、version、status、questions、validation rules |
| Questionnaire 生命周期 | draft / published / archived 等状态控制 |
| SubmissionSpec | 从已发布问卷生成可提交规格 |
| AnswerValue | 将 raw answer 转成类型化答案值 |
| AnswerValidator 协作 | 调用规则校验器验证答案合法性 |
| AnswerSheet.Submit | 创建完整提交事实 |
| SubmissionContext | 保存填写人、受试者、组织、任务上下文 |
| AnswerSheetSubmittedEvent | 声明答卷已提交 |
| Durable submit | 保存答卷、处理幂等、stage outbox |

它不负责：

| 不负责 | 应由谁负责 |
| --- | --- |
| 因子模型 | Scale |
| 计分规则 | Scale / Evaluation |
| 风险等级 | Scale / Evaluation |
| 解读规则 | Scale / Evaluation |
| Assessment 生命周期 | Evaluation |
| 报告生成 | Evaluation / ReportBuilder |
| 测评任务状态机 | Plan |
| 读侧统计投影 | Statistics |
| 用户认证授权真值 | IAM |

这个边界必须保持稳定。

如果后续不断把风险、报告、人格类型、统计投影塞进 Survey，Survey 会重新退化成“问卷中心大泥球”。

---

## 4. 当前已经完成的模型化建设

### 4.1 Questionnaire 从题目列表升级为模板聚合

旧式问卷系统里，Questionnaire 往往只是：

```text
questionnaire table
question table
option table
```

当前 Survey 文档把 Questionnaire 明确为模板聚合。

它不仅保存题目，还承担：

```text
业务编码；
版本；
发布状态；
题型结构；
选项；
校验规则；
可提交规格生成。
```

这个升级非常重要。

因为它让 Questionnaire 不再只是后台配置，而是提交事实的模板源头。

### 4.2 SubmissionSpec 成为模板到事实之间的防腐层

SubmissionSpec 是本轮 Survey 强模型重构的关键成果。

它让提交链路从：

```text
application service 拆 Question 列表
```

升级为：

```text
Questionnaire.BuildSubmissionSpec()
```

它负责：

```text
固化 QuestionnaireRef；
固化 QuestionSpec；
校验 question_code 归属；
校验 question_type 一致性；
提供 validation rules；
输出 PreparedSubmissionAnswer。
```

这使得 application service 不再过度理解 Questionnaire 内部结构。

### 4.3 AnswerSheet 从答案集合升级为提交事实聚合

AnswerSheet 不再被理解为：

```text
answersheet + answers rows
```

而是被定义为：

```text
某个填写人在某个上下文中，基于某个确定问卷版本，提交的一组答案事实。
```

它包含：

```text
AnswerSheetID；
QuestionnaireRef；
SubmissionContext；
Answers；
FilledAt；
DomainEvents。
```

这使得 AnswerSheet 能够独立回答：

```text
谁为谁提交？
属于哪个组织？
来自哪个任务？
基于哪版问卷？
提交了哪些答案？
提交后产生了什么事件？
```

### 4.4 SubmissionContext 让提交上下文入模

SubmissionContext 解决了过去的割裂问题。

没有 SubmissionContext 时，完整提交事实可能散落在：

```text
DTO；
request context；
durable meta；
event payload；
worker handler。
```

现在 SubmissionContext 将这些上下文收口到 AnswerSheet：

```text
Filler；
Testee；
OrgID；
TaskID。
```

这让 `AnswerSheetSubmittedEvent` 可以从 AnswerSheet 自身导出。

### 4.5 Answer / AnswerValue 支撑题型扩展

Survey 已经意识到题型不是简单枚举。

题型扩展会同时影响：

```text
QuestionType；
Question / QuestionSpec；
RawSubmissionAnswer；
AnswerValue；
AnswerValue factory；
AnswerValidator；
Storage mapper；
Query DTO；
Frontend renderer。
```

当前 AnswerValue 至少抽象出：

```text
StringValue；
NumberValue；
OptionValue；
OptionsValue。
```

这让答案不再完全以 `map[string]any` 的方式在业务层传播。

### 4.6 Durable submit 关闭事件丢失窗口

Survey 的提交链路没有使用：

```text
Save AnswerSheet
Publish MQ
```

而是使用：

```text
Save AnswerSheet
Save Idempotency Record
Stage Outbox Events
```

这让答卷保存和事件出站之间有了可靠边界。

qs-server README 也明确：系统关键事件通过 Outbox 可靠出站，再进入 MQ 驱动 worker；Outbox 负责业务数据库与消息出站之间的一致性，worker 消费不承诺 exactly-once，业务侧必须幂等。

---

## 5. 当前 Survey 的核心链路

Survey 的最终提交链路可以压缩为：

```text
collection-server
  -> qs-apiserver SubmissionService
  -> resolve published Questionnaire
  -> Questionnaire.BuildSubmissionSpec
  -> SubmissionSpec.PrepareAnswers
  -> AnswerValue + ValidationTask
  -> AnswerValidator.ValidateAnswers
  -> AnswerSheet.Submit
  -> DurableStore.CreateDurably
  -> Outbox answersheet.submitted
```

对应职责如下。

| 阶段 | 职责 |
| --- | --- |
| collection-server | 前台提交保护、限流、排队、重复抑制 |
| SubmissionService | 编排提交用例 |
| QuestionnaireRepository | 加载可提交问卷版本 |
| Questionnaire | 判断是否可提交，生成 SubmissionSpec |
| SubmissionSpec | 校验题目归属与题型一致性 |
| AnswerValue factory | 将 raw value 转成类型化答案 |
| AnswerValidator | 执行 required/min/max 等规则校验 |
| AnswerSheet.Submit | 创建完整提交事实并产生事件 |
| DurableStore | 保存答卷、处理幂等、stage outbox |
| Outbox Relay | 将事件可靠发布到 MQ |

这条链路已经具备清晰的 DDD + 六边形架构特征：

```text
入站层不做核心业务；
应用层负责编排；
领域层保护不变量；
基础设施层负责持久化和消息出站；
worker 只做异步驱动。
```

---

## 6. Survey 当前成熟度评价

| 维度 | 评价 |
| --- | --- |
| 模块定位 | 清楚，作答事实域边界已经成立 |
| 聚合拆分 | Questionnaire / AnswerSheet 拆分合理 |
| 模板侧模型 | SubmissionSpec 是关键亮点 |
| 事实侧模型 | AnswerSheet.Submit + SubmissionContext 已经比较完整 |
| 题型扩展 | 已经识别 QuestionType 与 AnswerValue 双侧扩展 |
| 提交流程 | application service 编排较清晰 |
| 幂等与 Outbox | durable boundary 设计合理 |
| 与 Scale 边界 | 当前保持良好，没有侵入因子/风险/报告 |
| 与 Evaluation 边界 | 通过 answersheet.submitted 解耦，方向正确 |
| 文档基础 | 已经形成比较完整的模块文档样板 |

综合判断：

```text
Survey 模块已经达到“可作为 qs-server 文档重建样板”的成熟度。
```

它不是完美状态，但主模型已经立住。

---

## 7. 仍需补强的地方

### 7.1 AnswerValue 语义仍可增强

当前 AnswerValue 仍然以 `Raw()` 为中心。

这在工程上可用，但长期容易让领域层重新退回 `any`。

后续可以逐步补充：

```text
Kind()
IsEmpty()
AsString()
AsNumber()
OptionCode()
OptionCodes()
```

目标是：

```text
持久化和 JSON 可以使用 Raw；
领域规则和校验尽量使用强语义方法。
```

### 7.2 DTO 中 question_type 的长期处理

当前 DTO 中仍然带 `question_type`。

短期可以保留，用于兼容和校验。

长期更理想的方向是：

```text
客户端只提交 question_code + value；
服务端完全通过 SubmissionSpec 推导 question_type。
```

这样可以进一步减少客户端对题型事实源的影响。

### 7.3 Getter 与 slice 不可变性

强模型需要注意内部状态暴露。

需要继续检查：

```text
Questions 是否返回 clone；
Answers 是否返回 clone；
Events 是否返回 clone；
OptionsValue 是否 clone 输入输出；
SubmissionContext 是否避免暴露内部可变指针。
```

这些不是大模型问题，但会影响领域对象的封装质量。

### 7.4 DurableStore 实现一致性

Mongo repository 当前对 nil sheet 有明确错误防御。

application 层 wrapper 也应保持一致，不应该静默返回 nil。

建议统一策略：

```text
nil sheet 是编程错误，应直接返回 error。
```

### 7.5 题型扩展测试矩阵

后续每次新增题型，都应该按矩阵补测试：

```text
Questionnaire.BuildSubmissionSpec；
SubmissionSpec.PrepareAnswers；
AnswerValue factory；
AnswerValidator；
Mongo mapping；
Query DTO；
端到端提交链路。
```

否则题型扩展很容易只在 API 层能提交，但在校验、存储或下游评估中出问题。

---

## 8. 后续演进方向

### 8.1 短期：继续补强 Survey 内部封装

短期建议做小步补强，不建议大拆。

优先级：

```text
P0：统一 nil sheet 错误处理；
P1：Events / Questions / Answers getter 返回 clone；
P2：AnswerValue 增加 Kind / IsEmpty；
P3：补 SubmissionSpec 和 AnswerSheet.Submit 单元测试；
P4：补题型扩展矩阵文档；
P5：补端到端提交链路测试。
```

### 8.2 中期：配合 Evaluation 抽象

中期重点不是改 Survey，而是让 Evaluation 抽象出来。

当前 Survey 只发布：

```text
answersheet.submitted
```

未来 Evaluation 应该基于：

```text
AnswerSheet.QuestionnaireRef
SubmissionContext.TaskID
Plan / ModelResolver
```

解析出：

```text
EvaluationModelRef
```

这意味着：

```text
Survey 不需要知道 Scale；
Survey 也不需要知道 MBTI；
Survey 只需要持续提供稳定 AnswerSheet 事实。
```

### 8.3 长期：支持多测评模型时 Survey 仍应稳定

未来支持 MBTI、Big Five、DISC 等模型时，Survey 不应该大改。

原因是这些变化属于：

```text
测评规则模型变化；
结果解释模型变化；
报告结构变化；
Evaluation Evaluator 扩展。
```

而不是：

```text
作答事实模型变化。
```

Survey 只需要保证：

```text
可以定义对应问卷；
可以提交对应答案；
可以保存 QuestionnaireRef；
可以保存 AnswerSheet；
可以发出 answersheet.submitted。
```

如果 MBTI 需要新的题型，例如 Rating、MatrixRadio、Slider，那么 Survey 的变化也只应该落在题型扩展机制上，而不是把 MBTI 规则塞进 Survey。

---

## 9. 为什么未来支持 MBTI 时 Survey 基本不变

MBTI 的核心变化是解释模型，不是作答事实模型。

Survey 仍然只管：

```text
题目；
选项；
答案；
提交；
事件。
```

MBTI 需要的是：

```text
维度映射；
E/I、S/N、T/F、J/P 偏好计算；
TypeCode；
人格画像；
人格报告。
```

这些不属于 Survey。

更合理的架构是：

```text
Survey
  -> AnswerSheet
  -> answersheet.submitted
  -> Evaluation
       -> MBTIEvaluator
       -> MBTIResult
       -> MBTIReport
```

而不是：

```text
Survey
  -> MBTIQuestionnaire
  -> MBTIAnswerSheet
  -> MBTIReport
```

所以，未来接入 MBTI 时，Survey 最多需要新增题型支持，而不应该新增 MBTI 领域规则。

---

## 10. Survey 与下一阶段 Evaluation 抽象的关系

下一阶段 qs-server 的关键架构准备是：

```text
Evaluation 不再默认等于 Scale Evaluation。
```

这对 Survey 的要求是：

```text
稳定提供 AnswerSheetSubmittedEvent；
事件中包含 QuestionnaireRef；
事件中包含 SubmissionContext；
AnswerSheet 可被 Evaluation 查询；
AnswerValue 足够表达提交答案。
```

只要 Survey 做到这些，Evaluation 就可以自行决定：

```text
这份答卷对应 ScaleEvaluator；
这份答卷对应 MBTIEvaluator；
这份答卷对应其他 Evaluator。
```

Survey 不需要参与这个决策。

---

## 11. Survey 文档体系总结

当前 Survey 模块文档已经形成以下结构：

```text
00-模型总览.md
01-Questionnaire模型与SubmissionSpec.md
02-AnswerSheet提交事实模型.md
03-答卷提交链路分析.md
04-事务幂等与Outbox出站.md
05-Survey模块总结与后续演进.md
```

这套文档覆盖了：

| 文档 | 重点 |
| --- | --- |
| 00 | Survey 的域定位、整体模型和边界 |
| 01 | Questionnaire、SubmissionSpec、题型模板侧扩展 |
| 02 | AnswerSheet、SubmissionContext、AnswerValue、提交事实 |
| 03 | 从 collection 到 apiserver 再到 outbox 的提交链路 |
| 04 | durable boundary、IdempotencyKey、Outbox、Worker 边界 |
| 05 | 模块总结、成熟度评价和后续演进 |

这可以作为后续 Scale、Evaluation、Actor、Plan、Statistics 模块文档重建的样板。

每个模块都可以继续采用：

```text
模型总览
模型服务详细拆解
关键链路分析
工程一致性边界
模块总结与后续演进
```

---

## 12. Survey 模块的代码证据链

| 主题 | 代码路径 |
| --- | --- |
| Questionnaire 聚合 | `internal/apiserver/domain/survey/questionnaire/questionnaire.go` |
| Questionnaire 生命周期 | `internal/apiserver/domain/survey/questionnaire/lifecycle.go` |
| Question 模型 | `internal/apiserver/domain/survey/questionnaire/question.go` |
| SubmissionSpec | `internal/apiserver/domain/survey/questionnaire/submission_spec.go` |
| AnswerSheet 聚合 | `internal/apiserver/domain/survey/answersheet/answersheet.go` |
| SubmissionContext / QuestionnaireRef | `internal/apiserver/domain/survey/answersheet/types.go` |
| Answer / AnswerValue | `internal/apiserver/domain/survey/answersheet/answer.go` |
| AnswerSheetSubmittedEvent | `internal/apiserver/domain/survey/answersheet/events.go` |
| AnswerValue 校验适配 | `internal/apiserver/domain/survey/answersheet/validation_adapter.go` |
| 提交服务主流程 | `internal/apiserver/application/survey/answersheet/submission_service.go` |
| 问卷解析 | `internal/apiserver/application/survey/answersheet/submission_questionnaire_resolver.go` |
| 答案准备 | `internal/apiserver/application/survey/answersheet/submission_answer_assembler.go` |
| 提交 finalizer | `internal/apiserver/application/survey/answersheet/submission_finalizer.go` |
| durable store 接口 | `internal/apiserver/application/survey/answersheet/durable_store.go` |
| durable store wrapper | `internal/apiserver/application/survey/answersheet/transactional_durable_store.go` |
| Mongo durable submit | `internal/apiserver/infra/mongo/answersheet/durable_submit.go` |
| 事件契约 | `configs/events.yaml` |
| collection 入口 | `internal/collection-server/transport/rest/handler/answersheet_handler.go` |
| internal gRPC 契约 | `internal/apiserver/interface/grpc/proto/internalapi/internal.proto` |

---

## 13. Verify

Survey 模块常用验证命令：

```bash
go test ./internal/apiserver/domain/survey/...
go test ./internal/apiserver/application/survey/...
go test ./internal/apiserver/infra/mongo/answersheet/...
```

涉及 collection-server：

```bash
go test ./internal/collection-server/application/answersheet/...
go test ./internal/collection-server/transport/rest/handler/...
```

涉及事件、文档或契约：

```bash
make docs-hygiene
make docs-verify
```

全量质量入口：

```bash
make test
make lint
make docs-hygiene
```

---

## 14. 面试与宣讲口径

### 14.1 30 秒版本

```text
Survey 是 qs-server 的作答事实域，不负责报告和风险解释。
我把它拆成 Questionnaire 和 AnswerSheet 两个聚合：Questionnaire 负责可发布、可提交的问卷模板，并通过 SubmissionSpec 暴露提交规格；AnswerSheet 负责一次完整提交事实，通过 SubmissionContext 保存填写人、受试者、组织和任务上下文，并在 Submit 时产生 answersheet.submitted。
提交链路最后通过 DurableStore 同时保存答卷、幂等记录和 outbox 事件，保证后续 Evaluation 能被可靠驱动。
```

### 14.2 3 分钟版本

```text
Survey 模块解决的是“作答事实可信”的问题。

在 qs-server 里，Survey 不等于整个测评系统。Survey 只管用户填了什么，Scale 管怎么算和怎么解释，Evaluation 管这次测评执行后的结果。

Survey 内部我拆成两个核心聚合。Questionnaire 是模板聚合，管理问卷 code、version、status、questions、validation rules。它通过 BuildSubmissionSpec 暴露“已发布问卷如何被提交”的规格。SubmissionSpec 会校验 question_code 是否属于当前问卷版本、question_type 是否与模板一致，并输出 PreparedSubmissionAnswer。

AnswerSheet 是事实聚合，表达某个填写人在某个业务上下文中，基于某个确定问卷版本提交了一组答案。它通过 QuestionnaireRef 绑定问卷版本，通过 SubmissionContext 保存 filler、testee、org、task，通过 Answer / AnswerValue 保存类型化答案，并在 AnswerSheet.Submit 时产生 answersheet.submitted 领域事件。

提交链路上，collection-server 负责限流、排队、重复提交保护；qs-apiserver 的 SubmissionService 负责编排；AnswerValidator 负责执行规则校验；DurableStore 在一个持久化边界内保存 AnswerSheet、写入幂等记录，并把事件 stage 到 Outbox。这样即使 MQ 暂时不可用，事件也不会丢。

所以 API 返回提交成功，只代表作答事实已经可靠保存、事件已经进入出站链路，不代表 Evaluation 或报告已经完成。
```

### 14.3 5 分钟版本讲解结构

```text
1. 先讲 Survey 的边界：只管作答事实，不管解释结果。
2. 再讲两个聚合：Questionnaire 是模板，AnswerSheet 是事实。
3. 讲 SubmissionSpec：把可提交规格从 application helper 收回模型。
4. 讲 AnswerSheet.Submit：提交上下文、答案值、领域事件收口。
5. 讲 DurableStore + Outbox：答卷保存、幂等、事件出站的一致性。
6. 最后讲未来演进：MBTI 不应该改 Survey，应该扩展 Evaluation / Evaluator。
```

### 14.4 高频追问

| 追问 | 回答要点 |
| --- | --- |
| Survey 和 Scale 怎么分？ | Survey 管作答事实，Scale 管量表规则和解释规则 |
| Survey 和 Evaluation 怎么分？ | Survey 发出 answersheet.submitted，Evaluation 管 Assessment、结果和报告 |
| 为什么拆 Questionnaire 和 AnswerSheet？ | 模板和事实生命周期不同，必须避免模板变化污染历史答卷 |
| SubmissionSpec 的价值是什么？ | 把可提交规格显式建模，防止 application 拆聚合和客户端污染题型事实 |
| SubmissionContext 为什么入模？ | 让 AnswerSheet 自己完整表达谁填、为谁填、在哪个组织和任务下提交 |
| AnswerValue 有什么价值？ | 让答案从 raw any 变成类型化值对象，支撑题型扩展和校验 |
| 为什么需要 Outbox？ | 保证答卷保存和事件出站之间的一致性，避免事件丢失 |
| 幂等键为什么不属于 AnswerSheet？ | 它是请求去重元数据，不是答卷事实本身 |
| 未来支持 MBTI，Survey 是否要大改？ | 不应该。MBTI 是解释模型变化，Survey 仍只保存问卷和答卷事实 |

---

## 15. 最终判断

Survey 模块当前已经完成了 qs-server 中非常关键的一轮模型化建设。

它的最终价值不是“能保存问卷和答卷”，而是：

```text
它为整个测评系统提供了稳定、可追溯、可事件驱动的作答事实源。
```

当前最重要的结论是：

```text
Survey 已经基本稳定；
后续重心应该转向 Scale 和 Evaluation；
其中 Evaluation 是下一阶段架构准备的重点。
```

Survey 后续只需要继续做小步增强：

```text
更强的 AnswerValue 语义；
更稳的题型扩展机制；
更严的不可变性封装；
更完善的提交链路测试；
更完整的 event / outbox 观测。
```

但是它的主边界不应该再变化。

一句话收束：

> **Survey 是“作答事实源”，不是“测评解释中心”。这个边界守住了，qs-server 后续支持 Scale、MBTI、Big Five 或其他测评模型时，架构才不会被反复推倒。**
