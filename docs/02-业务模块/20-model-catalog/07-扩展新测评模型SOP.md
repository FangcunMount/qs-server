# 扩展新测评模型 SOP

## 1. 适用场景

新增医学量表、人格模型、行为能力测评或其它可执行测评模型。

---

## 2. 步骤

1. 定义模型业务身份：`AssessmentKind`、子类型和执行算法。
2. 在 `domain/modelcatalog/capability.go` 登记 `KindCapability`（含 `RuntimeExecutable` / `ExecutionPath`）。
3. 设计 `ModelPayload` 与发布快照解析（`domain/modelcatalog/<kind>/snapshot`）。
4. 注册 `evaldomain.ModelDescriptor` 并加入 `DefaultEvaluationDescriptors()` 顺序表。
5. 在 `application/evaluation/runtime/materialize.go` 接入 evaluator / report builder / score projector 分支。
6. 在 `infra/evaluationinput` 接入 input provider 与 published catalog 解码。
7. 在 `port/evaluationinput` 定义 payload 与 catalog 接口。
8. 补 characterization 单模块 + cross-module 测试；跑 `AssertRegistryKeyParity` / `kind_landing_test.go`。
9. 更新统计口径：需要新指标时再调整 `statistics`。

参考实现：`behavioral_rating`（`behavioral_rating.default.v1` → scale 计分引擎投影）。

---

## 2.1 Runtime 落地检查清单（源码为准）

| 检查项 | 落点 |
| ------ | ---- |
| 能力矩阵 | `domain/modelcatalog/capability.go` |
| 执行路径 | `domain/modelcatalog/execution_path.go` + `domain/evaluation/runtime_path.go` |
| Descriptor | `domain/evaluation/registry.go` + `container/modules/modelcatalog/default_descriptors.go` |
| Evaluator | `application/evaluation/runtime/materialize.go` |
| Report Builder | 同上 + `application/interpretation/reporting/` |
| Score Projector | `application/evaluation/runtime/materialize.go`（scale-like runtime） |
| Input Provider | `infra/evaluationinput/providers.go` |
| 契约测试 | `container/modules/modelcatalog/kind_landing_test.go` |

---

## 3. 验收标准

| 检查项 | 标准 |
| ------ | ---- |
| 模型资产 | 可创建、校验、发布、查询 |
| 快照 | 执行记录能回溯模型快照 |
| 执行 | Evaluation 不依赖旧 `scale` 核心概念 |
| 报告 | InterpretReport 能识别模型身份 |
| 文档 | 本目录和相关模块链路已更新 |

---

## 4. 禁止事项

- 不要把新模型直接写成 Survey 题目逻辑。
- 不要让 Evaluation 读取可变草稿配置。
- 不要把报告文案塞进模型执行结果。
- 不要重新建立新的独立核心模块目录。
