# Survey 模块

> 状态：已实现。本文是 Survey 的阅读入口，只维护模块定位、文档地图和事实源，不复制各专题正文。

## 1. 30 秒结论

Survey 是 qs-server 的版本化作答契约与作答事实层。它的核心不是问卷 CRUD 或事件投递，而是四个相互约束的领域问题：

```text
Questionnaire
  定义“某一版本允许问什么”

AnswerSheet
  记录“针对该版本实际答了什么”

Question / AnswerValue
  定义题型能力、原始输入形状与领域答案语义

Submission Contract
  用已发布快照校验题型、选项、显示条件和校验规则
```

`Questionnaire code + version` 是两个聚合之间的稳定契约键。发布、持久化、Outbox 和后续测评交接都很重要，但它们是保护和传递这些领域事实的机制，不是 Survey 的定义中心。

## 2. 模块边界

| Survey 负责 | Survey 不负责 |
| --- | --- |
| 问卷 head、发布快照和 active version | Assessment Model 的 Definition、Binding 和 Published Snapshot |
| 题型能力、答案值语义、选项、校验和显示条件 | 评估机制、因子计算、常模和结论规则 |
| AnswerSheet、AnswerValue、提交上下文和版本化作答契约 | Assessment / EvaluationRun / Outcome |
| 答卷持久化、业务幂等和 `answersheet.submitted` 出站 | 报告生成、解释策略和最终展示 |
| 问卷变更事件和缓存失效信令 | Plan 任务生命周期与 Statistics 聚合 |

最重要的反例：

- `Questionnaire` 不是 `AssessmentModel`。
- `Questionnaire` 发布不等于测评模型发布。
- `AnswerSheet` 不是评估结果；提交成功也不等于评估或报告完成。
- 题目选项分值是基础答卷计分输入，不是模型执行与解释规则的全部事实源。
- `questionnaire.changed`、缓存失效信令和 `answersheet.submitted` 分别属于不同可靠性等级，不能互相替代。

## 3. 文档地图

| 顺序 | 文档 | 回答的问题 |
| --- | --- | --- |
| 10 | [领域模型：Questionnaire](./10-领域模型-Questionnaire.md) | 问卷、题目、版本、head/发布快照如何建模 |
| 11 | [领域模型：AnswerSheet](./11-领域模型-AnswerSheet.md) | 提交上下文、答案值和答卷事实如何建模 |
| 12 | [题型与答案值类型系统](./12-题型与答案值类型系统.md) | Question 能力、原始值和 AnswerValue 如何联合建模 |
| 20 | [版本化作答契约](./20-版本化作答契约.md) | head、published snapshot、active version 和 QuestionnaireRef 如何稳定历史语义 |
| 21 | [提交规格与答案校验](./21-提交规格与答案校验.md) | SubmissionSpec 如何将发布问卷变成服务端作答契约 |
| 22 | [新增题型与答案类型 SOP](./22-新增题型与答案类型SOP.md) | 一次完整扩展必须同步哪些领域、契约、存储和测试点 |
| 30 | [关键路径：问卷创建编辑与发布](./30-关键路径-问卷创建编辑与发布.md) | 从管理接口到发布快照、绑定同步和事件的完整路径 |
| 31 | [关键路径：答卷提交与校验](./31-关键路径-答卷提交与校验.md) | 从提交请求到构造 AnswerSheet 的完整校验路径 |
| 32 | [关键路径：答卷可靠落库与出站](./32-关键路径-答卷可靠落库与出站.md) | 幂等、Mongo 事务、Outbox 和 post-commit 如何协作 |
| 33 | [关键路径：答卷计分与测评交接](./33-关键路径-答卷计分与测评交接.md) | worker 如何把答卷交给跨模块 assessment intake |
| 80 | [模块边界与协作](./80-模块边界与协作.md) | Survey 与 Actor、ModelCatalog、Evaluation、Plan、Statistics 的边界 |
| 90 | [分层架构与代码索引](./90-分层架构与代码索引.md) | domain/application/infra/transport/container 从哪里进入 |

推荐按表中顺序阅读。修改具体能力时，可直接进入对应关键路径和代码索引。

## 4. 核心主链路

```mermaid
flowchart LR
    Q["Questionnaire head"]
    P["Published snapshot"]
    T["Question type contract"]
    S["SubmissionSpec"]
    V["AnswerValue"]
    A["AnswerSheet"]
    O["Mongo Outbox"]
    W["qs-worker"]
    E["Assessment intake"]

    Q -->|发布| P
    P -->|冻结题型与规则| T
    T -->|派生| S
    S -->|校验并归一化| V
    V -->|构造 Answer| A
    A -->|同事务写入| O
    O --> W
    W --> E
```

这条链路的成功边界分三段：

1. 问卷发布成功：head、发布快照和 active version 已更新。
2. 答卷提交成功：AnswerSheet 与 `answersheet.submitted` Outbox 已可靠落库。
3. 后续测评成功：worker 调用 assessment intake 后创建或复用 Assessment；该阶段不属于 Survey 提交事务。

## 5. 当前事实源

| 事实 | 源码 / 契约 |
| --- | --- |
| 模块装配 | [`internal/apiserver/container/modules/survey`](../../../internal/apiserver/container/modules/survey/) |
| Questionnaire 聚合 | [`internal/apiserver/domain/survey/questionnaire`](../../../internal/apiserver/domain/survey/questionnaire/) |
| AnswerSheet 聚合 | [`internal/apiserver/domain/survey/answersheet`](../../../internal/apiserver/domain/survey/answersheet/) |
| 应用用例 | [`internal/apiserver/application/survey`](../../../internal/apiserver/application/survey/) |
| Mongo 持久化 | [`internal/apiserver/infra/mongo/questionnaire`](../../../internal/apiserver/infra/mongo/questionnaire/)、[`internal/apiserver/infra/mongo/answersheet`](../../../internal/apiserver/infra/mongo/answersheet/) |
| REST / gRPC | [`internal/apiserver/transport/rest/routes_survey.go`](../../../internal/apiserver/transport/rest/routes_survey.go)、[`api/grpc/proto`](../../../api/grpc/proto/) |
| 事件可靠性 | [`configs/events.yaml`](../../../configs/events.yaml)、[`internal/pkg/eventing/catalog/spec.go`](../../../internal/pkg/eventing/catalog/spec.go) |

## 6. Verify

```bash
go test ./internal/apiserver/domain/survey/...
go test ./internal/apiserver/application/survey/...
go test ./internal/apiserver/container/modules/survey/...
make docs-hygiene
```
