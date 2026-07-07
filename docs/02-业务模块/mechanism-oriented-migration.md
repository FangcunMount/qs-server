# 机制导向目录终局与迁移路线

## 终局原则

**代码按机制组织，数据按测评组织。**

| 机制（代码） | 测评（配置） |
|-------------|-------------|
| factor_scoring | PHQ-9、GAD-7、通用量表 |
| factor_classification / typology | MBTI、SBTI、BigFive |
| factor_norm | Brief-2、Conners（规划） |
| task_performance | SPM、工作记忆任务（规划） |

## 终局目录（目标态）

```
domain/modelcatalog/
├── factor
├── factor_norm          # 常模/综合指数 metadata
├── task_performance     # 任务表现 metadata
├── classification
└── legacy

domain/calculation/
├── scoring
├── projection
└── norm                 # 常模查表 + norm projection

application/evaluation/
├── calculationadapter
├── factor_scoring
├── factor_classification
├── factor_norm
└── task_performance
```

## 三阶段迁移

### 阶段一：过渡（收尾）

- 已删除 `behavioral_rating/brief2`、`cognitive/spm` 过渡包（Round 3）。
- 仍保留 `adapter/{mbti,sbti,bigfive}` 作为 characterization-only 等价基线。
- 架构守卫测试禁止**新增**以测评 code 命名的 package。

### 阶段二：抽象（第二个同类模型出现时）

| 触发 | 动作 |
|------|------|
| Brief-2 + Conners | 抽 `calculation/norm`、`modelcatalog/factor_norm` |
| SPM + 第二任务 | 抽 `calculation/task`、`modelcatalog/task_performance` 执行层 |
| MBTI/SBTI/BigFive | 收敛 report/detail 到 `personality_type` / `trait_profile` 机制 |

### 阶段三：退化为配置

测评 code（brief2、mbti、sbti、spm 等）仅存在于：

- `Algorithm` 枚举
- ModelCatalog payload / seed
- 测试 fixture / migration

不再存在于主干 package 名称中。

## Round 1 已完成（机制骨架）

| 交付 | 位置 |
|------|------|
| 常模查表 + projection | `domain/calculation/norm` |
| 因子常模 metadata | `domain/modelcatalog/factor_norm` |
| 任务表现 metadata | `domain/modelcatalog/task_performance` |
| Typology 机制 detail/report | `personality_type` / `trait_profile` generic assembler |
| 生产路径 | `configured` runtime（非 `adapter.DefaultRegistry()`） |
| 架构守卫 | `architecture_mechanism_test.go`、`.cursor/rules/21-code-by-mechanism.mdc` |

## Round 2：收缩过渡层 + 收紧契约（已完成）

**做**：应用层直引机制包；默认 registry 仅机制 adapter key；publish 必填 `decision.kind`；publish 拒绝 legacy adapter key。

| 阶段 | 动作 |
|------|------|
| R2-A | `behavioral_rating` application/snapshot 直引 `calcnorm` + `factor_norm`；`ApplyNormProjection` |
| R2-B | `cognitive/snapshot` 直引 `task_performance`；`spm` 缩为 re-export |
| R2-C | outcome/report registry 默认仅 `personality_type` + `trait_profile`；validator publish 拒绝 code adapter key |
| R2-D | `BuildPublishedSnapshot` 无 `decision.kind` 报错 |
| R2-E | application typology 不得 import `adapter/{mbti,sbti,bigfive}` |
| R2-F | 文档同步 |

### 阶段二：MBTI 收敛 — 已完成

report/detail 已收敛到 `personality_type` / `trait_profile` 机制 key；legacy assemble 保留供 characterization 显式注入。

## Round 3：删除过渡包 + 契约全链路对齐（已完成）

**做**：删除 `brief2`/`spm`；`detail_registry` 单轨；seed 对齐机制 key；报告路径统一走 mechanism template。

**不做**：Conners / SPM 执行层；删 `adapter/{mbti,sbti,bigfive}`。

| 阶段 | 动作 |
|------|------|
| R3-A | 删除 `brief2`/`spm`；测试迁至 `factor_norm`/`calcnorm`/`task_performance` |
| R3-B | `DefaultDetailAssemblerRegistry` 仅 2 key；`RegisterLegacyDetailAssemblers` 供 characterization |
| R3-C | seed_personality_typology 改机制 adapter key |
| R3-D | `report_template.go` + `buildMechanism*Report` 统一机制路径 |
| R3-E+F+G | characterization 标注、架构守卫、文档同步 |

过渡包白名单（characterization-only）：`adapter/{mbti,sbti,bigfive}`。

## Round 4：机制内核收敛（已完成）

**做**：沉淀 evaluation 执行内核（input/policy/run/pipeline）与 interpretation 报告机制骨架（report/template/builder/rule/policy）；application 按 factor_* 机制族物化；reporting registry 支持机制键。

| 阶段 | 动作 |
|------|------|
| R4-A | `domain/evaluation/{input,policy,run,pipeline}` 稳定内核；`RuntimeDescriptorRegistry` 按 AlgorithmFamily/PayloadFormat 路由 |
| R4-B | `application/evaluation/{factor_scoring,factor_classification,factor_norm,task_performance}` 机制族包；`runtime/materialize` 改走机制路径 |
| R4-C | `domain/interpretation/{report,template,builder,rule,policy}` 报告机制骨架；机制 report builder（FactorScoring/Typology/NormProfile/TaskPerformance） |
| R4-D | `reporting/registry` 增加 `MechanismReportBuilderKey` + `ResolveByMechanism` |
| R4-E | 架构守卫 + `21-code-by-mechanism.mdc` + 收敛文档同步 |

**不做**：删除 `scale/personality/score` 过渡实现宿主；纯 registry 终态（去掉 factor_* 子包）。

## Round 5：路由单点 + 机制键主路径（已完成）

**做**：`ExecutionPath` 映射单点化；interpretation `Resolve`/`Writer` 机制键优先。

| 阶段 | 动作 |
|------|------|
| R5-1 | `pipeline/resolve.go` 为唯一 `ModelKind→ExecutionPath` 实现；`runtime_path.go` 委托；`routing_equivalence_test` |
| R5-2 | `mechanism_key.go`；`registry.Resolve`/`writer.resolveReportBuilder` 机制优先；`registry_mechanism_primary_test` |
| R5-3 | 架构守卫 + 收敛文档 Round 5 节 |

**不做**：`factor_*` 内联宿主；`RuntimeDescriptorRegistry` 接入 assemble。

## Round 6：Middle Man 消除 + 机制命名为主（已完成）

**做**：`factor_*` 承接真实实现宿主；模型族 application 包缩为 re-export；reporting 以机制命名为主；`runtime/materialize` 表驱动工厂注册。

| 阶段 | 动作 |
|------|------|
| R6-A | `factor_scoring` 内联原 `scale` 实现；`factor_norm`/`task_performance` 内联原 `behavioral_rating`/`cognitive`；三模型族包仅 `aliases.go` + 测试 |
| R6-B | `FactorScoringReportBuilder`/`NormProfileReportBuilder`/`TaskPerformanceReportBuilder` 为主类型；`legacy_report_aliases.go` 保留 deprecated 别名 |
| R6-C | `runtime/materialize.go` 改为 `ExecutionPath→工厂` map，去掉 `switch path` |
| R6-D | 架构守卫白名单注释同步；收敛文档 Round 6 节 |

**不做**：`factor_classification` 内联 `personality/typology`（~30 文件，留 Round 7）；`RuntimeDescriptorRegistry` 接入 assemble；删除 deprecated 包（需 characterization 迁移）。

## Round 7：typology 内联 + 清债 + Registry 桥接（已完成）

**做**：`factor_classification` 承接 typology 实现；删除 application 层 deprecated 包；characterization 改引机制包；`RuntimeDescriptorRegistry` 最小桥接。

| 阶段 | 动作 |
|------|------|
| R7-1 | characterization 改 import `factor_scoring`/`factor_norm`/`task_performance`/`factor_classification` |
| R7-2 | 删除 `scale`/`behavioral_rating`/`cognitive` application 包；测试迁至 `factor_*` |
| R7-3 | `personality/typology` 实现迁入 `factor_classification/`（~31 文件）；生产路径改引机制包 |
| R7-4 | `runtime/descriptor_registry.go` 注册 4 条 `ExecutionPath` 等价 descriptor + 覆盖测试 |
| R7-5 | 架构守卫白名单同步；收敛文档 Round 7 节 |

**不做**：assemble 完全切到 `RuntimeDescriptorRegistry` 驱动（留 Round 8）；删 domain 层 `personality/scale` 过渡宿主。

## Round 8：Registry 驱动 assemble + domain entry 重定向（已完成）

**做**：`DefaultEvaluationDescriptors` 从 `RuntimeDescriptorRegistry` 派生 path；catalog 导出 registry；application `factor_scoring` 改引 domain entry。

| 阶段 | 动作 |
|------|------|
| R8-1 | `runtime/descriptors.go`：`ExecutionPathsFromRegistry` + `FilterExecutablePaths`；`default_descriptors` 改走 registry |
| R8-2 | `EvaluationCatalog.RuntimeDescriptorRegistry`；`ExportEvaluationCatalog` 注入 |
| R8-3 | `domain/factor_scoring/entry` 扩展 alias；application 禁止直引 `domain/evaluation/scale` |
| R8-4 | `pipeline.Registry.HasAlgorithmFamily`；架构守卫 + 文档 Round 8 节 |

**不做**：删 `domain/evaluation/scale`/`personality` 宿主（留 Round 9）；materialize 改读 registry 工厂表。
