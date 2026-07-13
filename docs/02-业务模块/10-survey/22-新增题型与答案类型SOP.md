# 新增题型与答案类型 SOP

## 1. 本文回答

本文给出在 qs-server 中新增题型或答案值语义时的完整实施顺序、代码落点、兼容性检查和验证门槛。

## 2. 30 秒结论

当前新增题型不是单点注册操作。题型构造使用 factory registry，但答案值、提交校验、gRPC 解码、规则引擎视图和持久化重建仍有分散分支。

完成定义是：

```text
可创建和发布题目
  + 可通过所有提交入口解码
  + 可由发布快照校验
  + 可构造领域 AnswerValue
  + 可执行需要的校验/计分
  + 可写入并从 Mongo 无损重建
  + REST / gRPC / collection 契约一致
  + 新旧数据和客户端兼容策略明确
```

缺少任一项，都不能宣布题型已完整支持。

## 3. 实施前先定义类型契约

开始改代码前，先完成下表。不允许用 `any` 或 JSON 形状代替领域语义设计。

| 问题 | 必须给出的结论 |
| --- | --- |
| 题型表达什么业务意图？ | 例如日期、矩阵选择、文件、排序，不是“新增一个字符串” |
| 是否可作答？ | 决定 SubmissionSpec 的 required/额外答案处理 |
| 原始值形状是什么？ | JSON、gRPC string 和 Go 解码后类型 |
| 领域值语义是什么？ | 复用现有 AnswerValue，还是新建专用值对象 |
| 是否有选项或其它结构定义？ | 发布校验和提交规格需要什么元数据 |
| 需要哪些 validation view？ | string / number / array 或新视图 |
| 是否参与 Survey 基础计分？ | single selection / multiple selections / number 或不参与 |
| 历史值如何重建？ | BSON 形状、类型标识和兼容解码 |
| 旧客户端如何表现？ | 忽略、降级显示、禁止发布，或通过版本化契约隔离 |

## 4. Step 1：扩展 Questionnaire 领域模型

### 4.1 新增 QuestionType

在 `domain/survey/questionnaire/types.go` 定义稳定的序列化值。该值会进入：

- Questionnaire Mongo document；
- AnswerSheet Mongo document；
- REST / gRPC 契约；
- 已发布问卷和历史答卷。

因此它是持久化契约，发布后不能随意重命名或改变大小写。

### 4.2 实现 Question

在 `question.go` 中定义具体题型，明确它如何实现：

- 通用题干和提示；
- placeholder；
- options 或其它结构定义；
- validation rules；
- calculation rule；
- show controller。

如果现有 `Question` 接口无法表达新题型的结构，先决定是添加通用能力、专用子接口，还是新的领域对象。不应把无类型 map 直接塞入 `QuestionCore`。

### 4.3 注册工厂

实现 `QuestionFactory` 并通过 `RegisterQuestionFactory` 注册。工厂必须拒绝题型特有的结构缺失，例如 Radio/Checkbox 不允许空 options。

### 4.4 补充发布校验

检查 `Validator.ValidateQuestion / ValidateForPublish`：

- 最小结构是否完整；
- 子元素 code 是否非空且唯一；
- 校验/计算/显示规则是否适用于该题型；
- 问卷发布后是否足以构建提交规格。

## 5. Step 2：扩展版本化提交规格

`SubmissionSpec` 是新题型能否真正提交的核心门槛。需要检查：

1. `submissionQuestionSpec` 是否需要保留新的题型元数据；
2. `BuildSubmissionSpec` 是否从 Question 正确提取这些数据；
3. `PrepareAnswers` 是否需要新的结构校验；
4. `isAnswerable` 是否应调整；
5. `isEmptyAnswerValue` 是否理解新值的空语义；
6. ShowController 是否能用新值作为条件源。

如果新题型有候选集、坐标、行列、文件类型或日期范围等特有结构，必须在此层用发布快照验证，不能只依赖前端控件。

## 6. Step 3：扩展 AnswerValue

### 6.1 先判断是否需要新值对象

不同题型可以复用同一 AnswerValue，前提是领域语义相同。例如 Text 和 Textarea 都是自由文本，因此共享 `StringValue`。

传输形状相同不代表领域语义相同。Radio 的 option code 和 Text 的文本都是 string，但必须分别是 `OptionValue` 和 `StringValue`。

### 6.2 实现归一化构造

更新 `CreateAnswerValueFromRaw`，明确：

- 允许的 Go 输入类型；
- 是否接受历史 JSON/BSON 解码形状；
- 如何防御性复制 slice/map；
- `Raw()` 返回什么稳定形状；
- 什么是空值。

该函数同时被新提交和 Mongo 重建使用，所以只验证 HTTP 请求是不够的。

## 7. Step 4：扩展校验与计分视图

### 7.1 Validation

检查 `AnswerValueAdapter`：

- `IsEmpty`是否对新值正确；
- `AsString / AsNumber / AsArray` 是否能暴露规则引擎需要的语义；
- 是否需要新 validation rule 及对应 ruleengine 实现。

不应为了复用现有规则而将结构化值无损语义压成 `fmt.Sprintf`。如果新值需要新视图，应扩展 port 契约。

### 7.2 Scoring

检查 `NewScorableValue` 适配器与 `AnswerScorer`：

- 新题型是否参与 Survey 基础计分；
- 如果参与，它使用 single/multiple/number 哪种视图；
- 题目定义中的计分元数据是否能在发布快照中完整保留；
- 不参与计分时是否明确 skip，而不是意外得 0 分后伪装成已处理。

模型因子、常模和结论规则仍属于 ModelCatalog/Evaluation，不随新题型进入 Survey。

## 8. Step 5：扩展应用组装

检查两条组装路径：

| 路径 | 当前入口 | 检查点 |
| --- | --- | --- |
| 问卷命令 -> Question | `question_command_assembler.go` | DTO 是否能表达新题型参数，是否生成完整 `QuestionParams` |
| PreparedSubmissionAnswer -> Answer | `submission_answer_assembler.go` | AnswerValue 能否构造，validation task 是否携带服务端规则 |

应用层可以组装 DTO 和 port task，但不应在此复制题型特有不变式。如果 assembler 出现“题型为 X 时重新判断选项合法性”，说明规则应回到 domain specification。

## 9. Step 6：保护持久化往返

### 9.1 Questionnaire

Questionnaire PO 当前保存通用字段、options、validation rules、calculation rule 和 show controller。如果新题型需要新结构：

1. 扩展 `QuestionPO`；
2. 更新 `QuestionnaireMapper.ToPO`；
3. 更新 `mapQuestions` 的 PO -> QuestionParams 重建；
4. 增加往返测试；
5. 明确旧 document 缺少新字段时的默认行为。

### 9.2 AnswerSheet

AnswerSheet PO 保存 `question_type + value.value`，mapper 读取时调用 `CreateAnswerValueFromRaw`。新增 AnswerValue 时必须验证：

- BSON 写入形状稳定；
- BSON 解码后的 Go 类型在构造函数允许范围内；
- slice/map 不因共享引用被外部修改；
- 旧 AnswerSheet 仍能重建；
- 未知题型是显式失败、跳过，还是保留 raw value，策略必须明确。

当前 mapper 对无法重建的单个 Answer 会跳过，所以往返测试必须检查答案数量和值，不能只检查 mapper 未返回 nil。

## 10. Step 7：对齐 REST、gRPC 与 collection

### 10.1 gRPC

`answersheet.proto` 使用 string 承载 answer value，`decodeAnswerValue` 再按 question type 解码。新值形状需要：

- 定义稳定的 string/JSON 编码；
- 更新 `decodeAnswerValue`；
- 更新详情返回时的值编码；
- 增加合法、空值、边界值和错误值测试。

只更新 proto 注释不会让服务端理解新值。

### 10.2 collection-server

检查：

- REST request DTO 是否接受新 JSON 形状；
- `AnswerConverter` 是否需要归一化；
- gRPC client 是否无损转发；
- SubmitQueue 序列化和幂等 payload 是否稳定；
- 旧小程序客户端不识别新题型时的发布/展示策略。

### 10.3 REST / OpenAPI

同步：

- 问卷管理 DTO 的题型参数；
- 答卷 value 的 schema 与示例；
- 题型命名和大小写；
- 必要的 enum 或 discriminator；
- 生成的 `api/rest/apiserver.yaml`。

当前 OpenAPI 中部分题型描述仍使用 `single_choice / multi_choice` 示例性用语，而 domain 精确值是 `Radio / Checkbox`。新题型实施时必须以实际 transport/domain 契约校正描述，不得继续复制模糊示例。

## 11. Step 8：建立分层测试

| 层 | 必须保护的行为 |
| --- | --- |
| Question factory | 最小合法构造成功，缺少特有参数失败 |
| Validator | 可发布/不可发布边界 |
| SubmissionSpec | 题型匹配、结构值、required、ShowController、未知值 |
| AnswerValue | 每种允许原始类型、拒绝类型、防御性复制 |
| Validation/scoring | 规则视图与得分语义 |
| Questionnaire mapper | Question -> PO -> Question 不丢结构和规则 |
| AnswerSheet mapper | AnswerValue -> BSON -> AnswerValue 不丢类型和值 |
| gRPC codec | string/JSON 往返、非法值错误 |
| application submission | 新题型可从发布问卷走到 AnswerSheet |
| collection integration | REST -> queue -> gRPC 值形状不变 |

至少补一条完整纵向测试，防止各单元测试都通过，但 question type 字符串或 value 形状在层与层之间不一致。

## 12. 兼容性决策

发布新题型前必须回答：

1. 旧版 apiserver 读到新题型 Mongo 数据时会怎样？
2. 旧 collection-server 会如何转发值？
3. 旧小程序是否会展示、忽略或损坏该题？
4. 已发布问卷是否需要新版本，而不是就地修改？
5. 新值是否能在事件、导出、统计和管理详情中无损表示？

如果客户端不具备能力协商，安全默认是禁止将新题型发布给未支持的客户端，而不是假设 string value 可以自动兼容所有语义。

## 13. 完成检查表

- [ ] 题型领域意图、可作答性和值语义已定义。
- [ ] QuestionType 的持久化名称已确定。
- [ ] Question 实现、工厂和发布校验已完成。
- [ ] SubmissionSpec 能使用发布快照验证新值。
- [ ] AnswerValue 及其 raw/BSON 形状已定义。
- [ ] validation/scoring 适配已完成或明确 skip。
- [ ] Questionnaire 和 AnswerSheet 持久化往返无损。
- [ ] REST、gRPC、collection 和客户端契约已对齐。
- [ ] 旧数据与旧客户端兼容策略已评审。
- [ ] 分层测试和至少一条端到端表征测试已通过。
- [ ] 题型矩阵、OpenAPI/proto 和本 SOP 已同步。

## 14. 代码索引与 Verify

| 扩展面 | 主要路径 |
| --- | --- |
| QuestionType / factory | [`domain/survey/questionnaire/types.go`](../../../internal/apiserver/domain/survey/questionnaire/types.go)、[`question.go`](../../../internal/apiserver/domain/survey/questionnaire/question.go)、[`factory.go`](../../../internal/apiserver/domain/survey/questionnaire/factory.go) |
| 发布与提交校验 | [`validator.go`](../../../internal/apiserver/domain/survey/questionnaire/validator.go)、[`submission_spec.go`](../../../internal/apiserver/domain/survey/questionnaire/submission_spec.go)、[`submission_validation.go`](../../../internal/apiserver/domain/survey/questionnaire/submission_validation.go) |
| AnswerValue / adapters | [`domain/survey/answersheet/answer.go`](../../../internal/apiserver/domain/survey/answersheet/answer.go)、[`validation_adapter.go`](../../../internal/apiserver/domain/survey/answersheet/validation_adapter.go)、[`scoring_service.go`](../../../internal/apiserver/domain/survey/answersheet/scoring_service.go) |
| application assembler | [`application/survey/questionnaire/question_command_assembler.go`](../../../internal/apiserver/application/survey/questionnaire/question_command_assembler.go)、[`application/survey/answersheet/submission_answer_assembler.go`](../../../internal/apiserver/application/survey/answersheet/submission_answer_assembler.go) |
| Mongo mapper | [`infra/mongo/questionnaire/mapper.go`](../../../internal/apiserver/infra/mongo/questionnaire/mapper.go)、[`infra/mongo/answersheet/mapper.go`](../../../internal/apiserver/infra/mongo/answersheet/mapper.go) |
| gRPC codec | [`transport/grpc/service/answersheet.go`](../../../internal/apiserver/transport/grpc/service/answersheet.go)、[`api/grpc/proto/answersheet/answersheet.proto`](../../../api/grpc/proto/answersheet/answersheet.proto) |
| collection adapter | [`collection-server/application/answersheet`](../../../internal/collection-server/application/answersheet/)、[`collection-server/infra/grpcclient/answersheet_client.go`](../../../internal/collection-server/infra/grpcclient/answersheet_client.go) |

```bash
go test ./internal/apiserver/domain/survey/...
go test ./internal/apiserver/application/survey/...
go test ./internal/apiserver/infra/mongo/questionnaire ./internal/apiserver/infra/mongo/answersheet
go test ./internal/apiserver/transport/grpc/service -run 'AnswerSheet|AnswerValue|DecodeAnswerValue'
go test ./internal/collection-server/application/answersheet/... ./internal/collection-server/infra/grpcclient/...
make docs-hygiene
```
