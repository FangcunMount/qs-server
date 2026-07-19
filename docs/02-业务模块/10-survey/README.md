# Survey 模块

> 状态：主体已实现；“独立问卷提交后不创建 Assessment”、“严格拒绝不可见题答案”和“受理幂等元数据收入 AnswerSheet document”仍是规划改造。本文只做模块边界和阅读地图，详细规则以下方 canonical 文档为准。

## 1. 30 秒结论

Survey 建立版本化作答契约，并保存作答事实：

```text
Questionnaire
  定义“某一版本允许问什么”

Question / AnswerValue
  定义“每种题如何回答”

AnswerSheet
  记录“针对该版本实际答了什么”
```

`Questionnaire code + version` 是 Questionnaire 与 AnswerSheet 之间的稳定契约键。发布、Mongo 事务、Outbox 和 worker 是保护与传递这些事实的机制，不是 Survey 的领域中心。

Questionnaire 可以独立发布并作为信息收集器使用，此时 AnswerSheet 就是业务终点；Questionnaire 只有与已发布 AssessmentModel 绑定后，才构成能够继续执行 Evaluation 的完整测评。

AnswerSheet 只表示一次正式、最终提交，系统不定义客户端与服务端之间的草稿或暂存协议。在可靠受理时，AnswerSheet 已经冻结问卷引用、提交上下文和原始答案；单题基础分与总基础分由后续 Survey scoring 按精确问卷版本异步补充。基础题分是可重建的延迟派生属性，不属于 `202 Accepted` 的成立条件。

## 2. 模块边界

| Survey 负责 | Survey 不负责 |
| --- | --- |
| Questionnaire head、published snapshot 和 active version | AssessmentModel Definition、Binding 和模型发布 |
| 题型、选项、AnswerValue、校验和显示条件 | 因子、常模、风险结论和解释规则 |
| AnswerSheet、SubmissionContext 和基础题分 | Assessment、Outcome 和 InterpretReport |
| AnswerSheet 可靠落库与 `answersheet.submitted` | Plan 生命周期和 Statistics 读模型 |

关键反例：

- Questionnaire 不是 AssessmentModel。
- AnswerSheet 不是测评结果。
- 答卷提交成功不等于 Assessment、Evaluation 或报告已完成。
- 跨 Survey、ModelCatalog、Plan 和 Evaluation 的编排属于 `application/journey/assessmentintake`。

> **规划改造：独立问卷终止边界。** 业务目标是未找到 AssessmentModel binding 时，在 AnswerSheet 与基础题分完成后正常结束，不创建 Assessment。当前 `assessmentintake.Service.Ensure` 仍会创建一个不会自动提交的 unbound pending Assessment；这是已确认的实现偏差，不应被当作目标模型。
>
> **规划改造：不可见题答案严格拒绝。** 服务端应使用同一份最终提交中的控制题答案重新计算 ShowController。只要提交包含当前不可见问题的答案，就应拒绝整份提交，而不是忽略隐藏答案或部分接收。当前共享校验器只在 required 检查时使用可见性，尚未保护该契约。
>
> **当前不足：基础计分状态不可区分。** AnswerSheet 当前以 `score=0` 创建，异步计分后再写回单题分和总分，但没有独立 scoring status 或 scored timestamp。因此无法仅凭 `score=0` 判断“尚未计分”还是“已计分且真实结果为零”。具体状态模型留待代码改造时确定。

## 3. 文档地图

| 顺序 | 文档 | 核心问题 |
| --- | --- | --- |
| 10 | [领域模型](./10-领域模型.md) | Questionnaire 与 AnswerSheet 为什么是两个聚合，它们保护什么不变式 |
| 20 | [核心设计：题型与答案值抽象](./20-核心设计-题型与答案值抽象.md) | QuestionType、Question、原始值和 AnswerValue 如何联合扩展 |
| 21 | [核心设计：版本化与作答契约](./21-核心设计-版本化与作答契约.md) | head、snapshot、QuestionnaireRef 和 SubmissionSpec 如何稳定历史语义 |
| 22 | [核心设计：数据存储与一致性](./22-核心设计-数据存储与一致性.md) | MongoDB 文档模型、head/snapshot 共集合、可靠受理事务和幂等存储如何保护事实 |
| 30 | [关键链路：问卷维护与发布](./30-关键链路-问卷维护与发布.md) | 问卷如何维护、独立发布，以及绑定模型后如何由 Assessment Release 联合发布 |
| 31 | [关键链路：答卷校验与可靠受理](./31-关键链路-答卷校验与可靠受理.md) | 从入口预检、精确版本校验和 AnswerSheet 建立，到幂等与 Outbox 原子提交 |
| 32 | [关键链路：从作答事实到测评执行](./32-关键链路-从作答事实到测评执行.md) | durable Outbox、Worker 和 Journey 如何派生基础题分并幂等进入 Evaluation |

阅读新题型实现时看 `20`；排查历史答卷版本时看 `21`；分析 MongoDB 文档、事务或幂等边界时看 `22`；排查问卷发布时看 `30`；排查答卷为什么不能受理时看 `31`；排查答卷已受理但 Assessment 尚未就绪时看 `32`。

## 4. 事实源与验证

| 主题 | 事实源 |
| --- | --- |
| Domain | [`internal/apiserver/domain/survey`](../../../internal/apiserver/domain/survey/) |
| Application | [`internal/apiserver/application/survey`](../../../internal/apiserver/application/survey/) |
| Mongo | [`internal/apiserver/infra/mongo/questionnaire`](../../../internal/apiserver/infra/mongo/questionnaire/)、[`answersheet`](../../../internal/apiserver/infra/mongo/answersheet/) |
| Transport | [`routes_survey.go`](../../../internal/apiserver/transport/rest/routes_survey.go)、[`api/grpc/proto`](../../../api/grpc/proto/) |
| Event contract | [`configs/events.yaml`](../../../configs/events.yaml)、[`internal/pkg/eventing/catalog`](../../../internal/pkg/eventing/catalog/) |

```bash
go test ./internal/apiserver/domain/survey/...
go test ./internal/apiserver/application/survey/...
go test ./internal/apiserver/container/modules/survey/...
make docs-hygiene
```
