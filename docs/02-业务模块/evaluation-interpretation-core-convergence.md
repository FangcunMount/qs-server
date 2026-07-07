# Evaluation / Interpretation 机制内核收敛

## 结论

- **ModelCatalog** 可按模型族组织（personality / scale / behavioral_rating / cognitive），承载模型资产差异。
- **Evaluation** 按执行机制组织（assessment / run / input / policy / pipeline），不认识具体测评 code。
- **Interpretation** 按报告机制组织（report / template / builder / rule / policy），不认识具体测评 code。
- **机制轴**：`AlgorithmFamily`（枚举）+ `DecisionKind` + `PayloadFormat`；执行代码包名见下表。

## 包名与 AlgorithmFamily 对照表

| Go 包名 | `AlgorithmFamily` 枚举 |
|---------|------------------------|
| `scoring` | `factor_scoring` |
| `typology` | `factor_classification` |
| `norming` | `factor_norm` |
| `task_performance` | `task_performance` |

## 阶段零决策（已锁定）

| 决策项 | 选择 |
|--------|------|
| Application 收敛路径 | **B→A**：先按机制族子包（factor_scoring / factor_classification / factor_norm / task_performance），终态收敛为纯 registry |
| run 聚合 | **不新增独立 run 聚合**；`domain/evaluation/run` 承载 attempt/failure/retry 执行阶段语义，`assessment` 保留生命周期与结果 |
| 机制轴 | `AlgorithmFamily` + `DecisionKind` + `PayloadFormat` |

## 三模块差异承载

| 模块 | 可按测评策略拆包？ | 承载什么差异 |
|------|-------------------|-------------|
| modelcatalog | 可以 | 模型结构、payload、配置差异 |
| calculation | 不应该 | 计算机制差异 |
| evaluation | 不应该 | 执行状态、pipeline、outcome assembly |
| interpretation | 不应该 | 报告结构、模板、解释规则 |
| application | 短期可以，长期收敛 | 用例编排、adapter、registry |

## 选择链（目标态）

```
PublishedModelSnapshot
  → AlgorithmFamily / PayloadFormat / DecisionKind
  → RuntimeDescriptorRegistry
  → EvaluationPipeline
  → AssessmentOutcome
  → Interpretation builder registry（机制键）
  → ReportTemplate + Rule
  → InterpretReport
```

## 终局目录

见 [mechanism-oriented-migration.md](./mechanism-oriented-migration.md) 与 `.cursor/rules/21-code-by-mechanism.mdc`。

## Round 5（已完成）

| 交付 | 说明 |
|------|------|
| 路由单点 | `ExecutionPath` 映射收敛到 `domain/evaluation/pipeline/resolve.go`；`runtime_path.go` 薄委托 |
| 机制键主路径 | `reporting/registry` 与 `writer` 优先 `MechanismReportBuilderKey`，`EvaluatorKey` 作 legacy fallback |
| 表征测试 | `pipeline/routing_equivalence_test.go`、`reporting/registry_mechanism_primary_test.go` |
| 架构守卫 | 禁止在 pipeline 外新增 `executionPathForFamily` / `algorithmFamilyFromModelKind` |

## Round 6（已完成）

| 交付 | 说明 |
|------|------|
| 实现宿主收敛 | `factor_scoring`/`factor_norm`/`task_performance` 承接 executor 实现；`scale`/`behavioral_rating`/`cognitive` 缩为 re-export |
| Reporting 机制命名 | `factor_scoring_report.go`、`norm_task_report.go` 为主；`ScaleReportBuilder` 等 deprecated 别名 |
| Materialize 表驱动 | `evaluatorFactories` / `reportBuilderFactories` / `scoreProjectorFactories` 按 `ExecutionPath` 注册 |
| 架构守卫 | application 层模型族白名单改为 re-export only |

## Round 7（已完成）

| 交付 | 说明 |
|------|------|
| typology 内联 | `factor_classification/` 承接原 `personality/typology` 全部实现 |
| deprecated 清债 | 删除 application `scale`/`behavioral_rating`/`cognitive`；characterization 直引 `factor_*` |
| Registry 桥接 | `DefaultRuntimeDescriptorRegistry()` 与 materialize 四条 `ExecutionPath` 对齐 |
| 测试迁移 | `factor_scoring/executor_test`、`factor_norm/*_test`、fixture 路径修正 |

## Round 8（已完成）

| 交付 | 说明 |
|------|------|
| Registry 驱动 descs | `DefaultEvaluationDescriptors` 从 `RuntimeDescriptorRegistry` 派生 execution path 再投影 |
| Catalog 导出 | `EvaluationCatalog.RuntimeDescriptorRegistry` 随 `ExportEvaluationCatalog` 注入 |
| Domain entry | application `factor_scoring` 经 `domain/evaluation/scoring` entry，不再直引 `scale` |
| 守卫 | `TestApplicationFactorMechanismsUseDomainEntryPackages` |

## Round 9（已完成）

| 交付 | 说明 |
|------|------|
| Domain scale 收敛 | `domain/evaluation/scoring` 承接原 `scale` 实现；删除过渡包 |
| Materialize 对齐 | `RegisteredEvaluatorPaths` 等与 registry 四条 path 等价测试 |
| 架构守卫 | domain `factor_scoring` 纳入 required packages；移除 `domain/scale` 白名单 |

## Round 10（已完成）

| 交付 | 说明 |
|------|------|
| Domain personality 收敛 | `domain/evaluation/typology` 承接 configured/typology/adapter/profile/specialrule |
| Import 全量切换 | 50+ 文件 `domain/evaluation/personality` → `factor_classification` |
| 守卫更新 | legacy adapter 白名单迁至 `factor_classification/adapter/*`；application 禁止回引 personality |

## Round 11（已完成）

| 交付 | 说明 |
|------|------|
| Interpretation 机制收敛 | `factor_classification` 承接 typology 报告；`factor_scoring` 承接 scale 报告 |
| Import 切换 | `builder`/`template`/application 改引机制包；移除 interpretation personality/score 过渡白名单 |
| 清债 | 删除重复 `domain/evaluation/personality` 目录 |

## Round 12（已完成）

| 交付 | 说明 |
|------|------|
| Legacy adapter 清债 | 删除 `adapter/{mbti,sbti,bigfive}`；characterization 改走 configured runtime |
| Materialize 单源 | `defaultPathMaterializations` 同时驱动 factory map 与 `RuntimeDescriptorRegistry` |
| 守卫 | 移除 assessment-code adapter 过渡白名单 |

## Round 13（已完成）

| 交付 | 说明 |
|------|------|
| Application registry 门面 | `application/evaluation/registry` 承接 catalog/typology 装配 API |
| Container 收敛 | compose/evaluation/interpretation 改引 registry，禁止直引 `factor_*` |
| 实现宿主保留 | `factor_*` 仍为 runtime materialize 内部实现，characterization 测试允许直引 |

## Round 14（已完成）

| 交付 | 说明 |
|------|------|
| Mechanisms 内联 | 顶层 `factor_*` 迁入 `registry/mechanisms/` 并删除旧路径 |
| Import 守卫 | application 禁止 legacy 顶层路径；container 禁止直引 mechanisms |
| 测试迁移 | characterization/runtime/domain 架构测试路径同步 |
