# Assessment Model 模块文档

> Assessment Model 是 qs-server 的 **统一测评模型资产层**。
>
> 它负责管理医学量表、人格模型等测评模型资产的后台配置、发布快照、模型绑定和执行 payload。旧 `scale/personalitymodel` 不再作为独立核心模块表达，而是归入 `assessment-model` 的具体模型资产与兼容能力。

---

## 1. 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 文档业务名 | `assessment-model` |
| 当前代码包名 | `internal/apiserver/container/modules/assessmentmodel` |
| 注册名 | canonical：`assessmentmodel`；legacy：`scale`、`personalitymodel` |
| 一句话职责 | 管测评模型资产，不管用户答卷事实，也不管一次测评执行状态 |
| 典型对象 | `AssessmentKind`、`PublishedModelSnapshot`、`QuestionnaireBinding`、Model Payload、Model Descriptor |
| 具体模型资产 | Scale / MBTI / SBTI / BigFive 等 |
| 下游协作 | Evaluation 读取发布快照和 payload 执行测评；Interpretation Model / Report 读取模型身份与结果生成报告 |

---

## 2. Assessment Model 管什么

Assessment Model 负责：

```text
AssessmentKind；
AssessmentModelSnapshot；
Model Binding；
Model Payload；
Scale / MBTI / BigFive 等模型资产抽象；
模型发布与查询；
模型目录缓存和发布态读取。
```

它不负责：

```text
用户答卷事实；
一次测评执行状态；
报告持久化；
任务调度；
读侧统计聚合。
```

重点：`scale` 不再是独立核心模块，而是 `assessment-model` 下的一类医学量表模型资产。代码层仍保留 `application/scale`、`infra/mongo/scale`、`scale` 注册名等兼容路径，阅读时要把它们理解为 Assessment Model 的旧能力路径。

---

## 3. 当前代码锚点

| 类型 | 路径 |
| ---- | ---- |
| 容器模块 | `internal/apiserver/container/modules/assessmentmodel` |
| canonical 注册 | `internal/apiserver/container/modules/registry.go` 中的 `PackageAssessmentModel` |
| legacy 注册名 | `internal/apiserver/container/modules/assessmentmodel/module.go` 中的 `scale`、`personalitymodel` |
| 领域模型 | `internal/apiserver/domain/assessmentmodel` |
| 医学量表模型资产 | `internal/apiserver/domain/assessmentmodel/scale` |
| 应用服务 | `internal/apiserver/application/assessmentmodel` |
| 旧 Scale 应用路径 | `internal/apiserver/application/scale` |
| 旧 PersonalityModel 应用路径 | `internal/apiserver/application/personalitymodel` |
| 端口 | `internal/apiserver/port/assessmentmodel` |

---

## 4. 文档目录

| 文档 | 说明 |
|------|------|
| [01-统一测评模型后台配置](./01-统一测评模型后台配置.md) | Draft 生命周期、REST 契约、状态机 |
| [02-人格测评模型定义格式](./02-人格测评模型定义格式.md) | `assessmentmodel.personality.typology.v1` + `RuntimeSpec` |
| [03-发布快照与执行链路](./03-发布快照与执行链路.md) | PublishedModelSnapshot -> Evaluation -> Report |
| [Catalog目录缓存（L1+L2）](../../03-基础设施/redis/10-Catalog目录L1-L2缓存.md) | C 端目录读双层缓存、信令失效、配置与排障 |
| [旧 Scale 兼容入口](../scale/README.md) | 医学量表规则细节，后续应逐步收口为 Assessment Model 的具体模型资产文档 |

---

## 5. 与其他模块的边界

| 协作对象 | 关系 |
| -------- | ---- |
| Survey | Survey 提供问卷和答卷事实；Assessment Model 只引用问卷绑定，不保存 AnswerSheet |
| Evaluation | Evaluation 读取 Assessment Model 的发布快照和 payload，完成一次测评执行 |
| Interpretation Model / Report | Report builder 使用模型身份、执行结果和 adapter 生成 `InterpretReport` |
| Statistics | Statistics 可以投影模型维度指标，但不成为模型资产事实源 |

---

## 6. Verify

修改 Assessment Model 代码或文档后，按范围选择：

```bash
go test ./internal/apiserver/container/modules/assessmentmodel/...
go test ./internal/apiserver/application/assessmentmodel/...
go test ./internal/apiserver/domain/assessmentmodel/...
make docs-hygiene
git diff --check
```
