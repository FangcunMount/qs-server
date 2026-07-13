# ModelCatalog 模块

> 状态：已实现。本文只做模块边界和阅读地图，详细规则以下方 5 篇 canonical 文档为准。

## 1. 30 秒结论

ModelCatalog 管理“某个问卷版本应按什么模型语义被执行”的版本化模型资产：

```text
AssessmentModel
  可编辑的目录聚合
  + QuestionnaireBinding
  + DefinitionV2

Publish
  -> AssessmentSnapshot
  -> published-only resolver
  -> Evaluation / Plan / collection
```

`DefinitionV2` 是模型语义事实；published payload 是由 Definition 投影出的兼容 wire artifact。运行时只能读取 `published_assessment_models`，不能读取 draft、旧 `scales` collection，也不能从 payload 反向补齐 Definition 语义。

## 2. 模块边界

| ModelCatalog 负责 | ModelCatalog 不负责 |
| --- | --- |
| AssessmentModel 元数据、身份、生命周期与问卷绑定 | Questionnaire、Question 和 AnswerSheet |
| Measure、Calibration、Execution、Conclusion、Outcome、ReportMap | 一次 Assessment 的执行状态与 Outcome 实例 |
| 独立版本化 Norm 资料与 Definition 中的 NormRef | 生成和持久化 InterpretReport 实例 |
| 发布 AssessmentSnapshot 并提供 published-only 目录 | Plan 生命周期、Statistics 投影与 C 端页面编排 |

必须区分：

- AssessmentModel 是可编辑模型资产，不是一次 Assessment。
- Questionnaire 定义“问什么”，DefinitionV2 定义“如何把答案变成测量、结论和报告输入”。
- ProductChannel 是产品目录分类；Kind/SubKind/Algorithm 才是模型身份。
- AssessmentSnapshot 是运行时 read model，不是第二个可编辑聚合。

## 3. 文档地图

| 顺序 | 文档 | 核心问题 |
| --- | --- | --- |
| 10 | [领域模型](./10-领域模型.md) | AssessmentModel、Definition、Norm 与 AssessmentSnapshot 各自保护什么事实 |
| 20 | [核心设计：DefinitionV2 与模型扩展](./20-核心设计-DefinitionV2与模型扩展.md) | 五层 Definition、跨层校验和四类模型策略如何扩展 |
| 21 | [核心设计：模型身份、绑定与运行时路由](./21-核心设计-模型身份绑定与运行时路由.md) | 产品分类、模型身份、判定方式、执行家族、问卷绑定和版本如何解耦 |
| 30 | [关键链路：模型创建、编辑与发布](./30-关键链路-模型创建编辑与发布.md) | 从管理命令到 Definition 保存、快照发布和生命周期 effect 的执行顺序 |
| 31 | [关键链路：已发布模型解析与消费](./31-关键链路-已发布模型解析与消费.md) | Evaluation、Plan 和 collection 如何只读已发布模型并处理失败 |

设计新算法或模型种类时先看 `20` 和 `21`；排查后台发布问题看 `30`；排查执行期模型加载问题看 `31`。

## 4. 事实源与验证

| 主题 | 事实源 |
| --- | --- |
| Domain | [`internal/apiserver/domain/modelcatalog`](../../../internal/apiserver/domain/modelcatalog/) |
| Application | [`internal/apiserver/application/modelcatalog`](../../../internal/apiserver/application/modelcatalog/) |
| Port / payload projection | [`internal/apiserver/port/modelcatalog`](../../../internal/apiserver/port/modelcatalog/) |
| Mongo | [`internal/apiserver/infra/mongo/modelcatalog`](../../../internal/apiserver/infra/mongo/modelcatalog/) |
| Runtime materialization | [`internal/apiserver/infra/evaluationinput`](../../../internal/apiserver/infra/evaluationinput/)、[`infra/ruleset`](../../../internal/apiserver/infra/ruleset/) |
| REST / gRPC | [`api/rest/apiserver.yaml`](../../../api/rest/apiserver.yaml)、[`assessment_model_catalog.go`](../../../internal/apiserver/transport/grpc/service/assessment_model_catalog.go) |

```bash
go test ./internal/apiserver/domain/modelcatalog/...
go test ./internal/apiserver/application/modelcatalog/...
go test ./internal/apiserver/infra/modelcatalog ./internal/apiserver/infra/mongo/modelcatalog
go test ./internal/apiserver/container/modules/modelcatalog/...
make docs-hygiene
```
