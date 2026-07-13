# Survey 模块

> 状态：已实现。本文只做模块边界和阅读地图，详细规则以下方 5 篇 canonical 文档为准。

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

## 3. 文档地图

| 顺序 | 文档 | 核心问题 |
| --- | --- | --- |
| 10 | [领域模型](./10-领域模型.md) | Questionnaire 与 AnswerSheet 为什么是两个聚合，它们保护什么不变式 |
| 20 | [核心设计：题型与答案值抽象](./20-核心设计-题型与答案值抽象.md) | QuestionType、Question、原始值和 AnswerValue 如何联合扩展 |
| 21 | [核心设计：版本化与作答契约](./21-核心设计-版本化与作答契约.md) | head、snapshot、QuestionnaireRef 和 SubmissionSpec 如何稳定历史语义 |
| 30 | [关键链路：问卷创建与发布](./30-关键链路-问卷创建与发布.md) | 从管理命令到发布快照、binding、事件和缓存的执行顺序 |
| 31 | [关键链路：答卷提交校验与测评驱动](./31-关键链路-答卷提交校验与测评驱动.md) | 从请求、校验、事务和 Outbox 到 worker/assessment intake 的完整边界 |

阅读新题型实现时看 `20`；排查历史答卷版本时看 `21`；排查运行时问题时直接进入 `30` 或 `31`。

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
