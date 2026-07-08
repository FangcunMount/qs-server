# 机制导向目录终局与迁移路线

## 终局原则

**代码按机制组织，数据按测评组织。**

## 包名与 AlgorithmFamily 对照表

| Go 包名 | `AlgorithmFamily` 枚举 | 含义 | 典型测评 |
|---------|------------------------|------|----------|
| `scoring` | `factor_scoring` | 因子区间计分 | PHQ-9、GAD-7 |
| `typology` | `factor_classification` | pole/trait/pattern 分型 | MBTI、SBTI、BigFive |
| `norming` | `factor_norm` | 常模查表与投影 | Brief-2、Conners |
| `task_performance` | `task_performance` | 任务/能力表现 | SPM |

- **枚举**：API、种子、`AlgorithmFamily` 常量、持久化边界 — 保持 `factor_*`（除 `task_performance` 两端一致）。
- **包名**：`domain/evaluation`、`domain/interpretation`、`registry/mechanisms` 下的执行/报告实现。
- **modelcatalog metadata**：`modelcatalog/norming` ↔ `factor_norm`；`modelcatalog/task_performance` ↔ `task_performance`。

| 机制（包名） | 测评（配置） |
|-------------|-------------|
| scoring | PHQ-9、GAD-7、通用量表 |
| typology | MBTI、SBTI、BigFive |
| norming | Brief-2、Conners（规划） |
| task_performance | SPM、工作记忆任务（规划） |

## 终局目录（目标态）

```
domain/modelcatalog/
├── factor
├── norming              # 常模/综合指数 metadata（包名 norming；AlgorithmFamily 仍 factor_norm）
├── task_performance     # 任务表现 metadata
├── classification
└── legacy

domain/calculation/
├── scoring
├── projection
└── norm                 # 常模查表 + norm projection

application/evaluation/
├── registry/                 # 对外门面
│   └── mechanisms/
│       ├── scoring           # AlgorithmFamily: factor_scoring
│       ├── typology          # AlgorithmFamily: factor_classification
│       ├── norming           # AlgorithmFamily: factor_norm
│       └── task_performance
├── runtime/
└── calculationadapter/
```

## 三阶段迁移

### 阶段一：过渡（收尾）

- 已删除 `behavioral_rating/brief2`、`cognitive/spm` 过渡包（Round 3）。
- 仍保留 `adapter/{mbti,sbti,bigfive}` 作为 characterization-only 等价基线。
- 架构守卫测试禁止**新增**以测评 code 命名的 package。

### 阶段二：抽象（第二个同类模型出现时）

| 触发 | 动作 |
|------|------|
| Brief-2 + Conners | 抽 `calculation/norm`、`modelcatalog/norming` |
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
| 因子常模 metadata | `domain/modelcatalog/norming` |
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

## Round 9：domain scale 迁入 factor_scoring + materialize/registry 对齐（已完成）

**做**：删除 `domain/evaluation/scale`；实现迁入 `domain/evaluation/scoring`；materialize 工厂表与 registry path 对齐测试。

| 阶段 | 动作 |
|------|------|
| R9-1 | `domain/evaluation/scale/*` 迁入 `domain/evaluation/scoring`；删 `entry.go` 薄委托 |
| R9-2 | `materialize_paths.go` + `materialize_registry_test`：工厂 map 与 `RuntimeDescriptorRegistry` 对齐 |
| R9-3 | 架构守卫移除 `domain/scale` 白名单；`doc.go` 同步 |

**不做**：`domain/evaluation/personality` 迁入 `factor_classification`（留 Round 10）；assemble 完全 registry 驱动物化。

## Round 10：domain personality 迁入 factor_classification（已完成）

**做**：`domain/evaluation/personality/*` 整树迁入 `domain/evaluation/typology`；application 改引新路径；legacy adapter 白名单同步。

| 阶段 | 动作 |
|------|------|
| R10-1 | `personality/{configured,typology,adapter,profile,specialrule}` → `factor_classification/`；删 `entry.go` 薄委托 |
| R10-2 | 全仓 import `domain/evaluation/personality` → `factor_classification`；`architecture_test` 包名同步 |
| R10-3 | 架构守卫：adapter 白名单路径更新；禁止 application 直引 `domain/personality` |
| R10-4 | 文档 Round 10 节 |

**不做**：`domain/interpretation/personality` 收敛；删 `adapter/{mbti,sbti,bigfive}`；assemble registry 驱动物化。

## Round 11：interpretation 机制包收敛（已完成）

**做**：`domain/interpretation/personality` → `factor_classification`；`domain/interpretation/score` → `factor_scoring`；清重复 `domain/evaluation/personality`。

| 阶段 | 动作 |
|------|------|
| R11-1 | `personality/*` → `interpretation/factor_classification`（含 typology 子包）；`score/*` → `interpretation/factor_scoring` |
| R11-2 | 全仓 import 切换；`builder`/`template` 改引机制包 |
| R11-3 | 删重复 `domain/evaluation/personality`；架构守卫移除 interpretation personality/score 白名单 |
| R11-4 | 文档 Round 11 节 |

**不做**：删 `adapter/{mbti,sbti,bigfive}`；assemble 完全 registry 驱动物化（留 Round 12）。

## Round 12：legacy adapter 清债 + materialize 单源（已完成）

**做**：删 `adapter/{mbti,sbti,bigfive}`；characterization/configured 测试改引 configured + typology reference；materialize 与 `DefaultRuntimeDescriptorRegistry` 共用 `defaultPathMaterializations`。

| 阶段 | 动作 |
|------|------|
| R12-1 | 删 legacy adapter 子包；`adapter` 仅保留 `ModelAdapter`/`Registry` + `configured` |
| R12-2 | characterization 用 `scoreBigFiveCharacterization`；configured 等价测试改 typology reference |
| R12-3 | `materialization_specs.go` 单源驱动 factory map 与 registry |
| R12-4 | 架构守卫移除 adapter 过渡白名单；文档 Round 12 节 |

## Round 13：application 纯 registry 终态（已完成）

**做**：`application/evaluation/registry` 成为 container 唯一装配入口；`factor_*` 降为 runtime 内部实现宿主。

| 阶段 | 动作 |
|------|------|
| R13-1 | 新增 `registry`：`DefaultEvaluationDescriptors`、`DefaultTypologyRegistry`、typology 类型别名 |
| R13-2 | container/compose 改引 `registry.TypologyRegistry`；modelcatalog 薄委托 |
| R13-3 | `preview` 改引 registry；架构守卫禁止 container 直引 `factor_*` |
| R13-4 | 文档 Round 13 节 |

**R14 已承接**：`factor_*` 迁入 `registry/mechanisms/` 并删除顶层路径；characterization 改引 mechanisms（测试允许）。

## Round 14：mechanisms 内联 + 顶层 factor_* 删除（已完成）

**做**：`factor_*`/`task_performance` 迁入 `registry/mechanisms/`；全仓 import 切换；禁止 application 引用旧顶层路径。

| 阶段 | 动作 |
|------|------|
| R14-1 | `application/evaluation/{factor_*,task_performance}` → `registry/mechanisms/*` |
| R14-2 | runtime/registry/characterization import 全量切换 |
| R14-3 | 架构守卫：`TestApplicationDoesNotImportLegacyFactorMechanismHosts`；container 禁止直引 mechanisms |
| R14-4 | 文档 Round 14 节 |

## Round 15：机制包语义重命名（方案 B，已完成）

**做**：`factor_*` 机制包改为语义短名；`AlgorithmFamily` 字符串保留 `factor_*`（双层兼容）。

| 旧包 | 新 Go 包名 | `AlgorithmFamily` 枚举 | 子包调整 |
|------|-----------|------------------------|----------|
| `factor_scoring` / `range_scoring` | `scoring` | `factor_scoring` | 合并原 `domain/evaluation/scoring`（FactorScorer） |
| `factor_classification` | `typology` | `factor_classification` | `typology/`→`patterns/`，`profile/`→`trait/` |
| `factor_norm` | `norming` | `factor_norm` | — |
| `task_performance` | `task_performance` | `task_performance` | — |
| `modelcatalog/factor_norm` | `modelcatalog/norming` | `factor_norm` | metadata 包名对齐 |

`personality/typology`（测评 payload）、`task_performance` metadata、`application/evaluation/scoring`（快照）保持原名。

## Round 16：ModelCatalog 根包 Phase A 整理（已完成）

**目标**：减根包平铺杂乱感；**不删** `behavior_ability` / migration kinds 等兼容语义（留 Phase B/C）。

**策略**：子包承载实现 + 根包 `modelcatalog` 保留类型别名（对外 import 路径不变）。

### 终态目录

```text
domain/modelcatalog/
├── doc.go              # 模块地图（文件索引）
├── errors.go           # 域错误
├── export.go           # 根包类型别名 → 子包（过渡期）
├── identity/           # Kind / Algorithm / SubKind / DecisionKind / ProductChannel
├── routing/            # AlgorithmFamily / ExecutionPath / PayloadFormat
├── catalog/            # AssessmentModel 聚合、Definition、PublishedSnapshot、校验
├── capability/         # KindCapability（合并 ModelFamilyCapability）、CatalogOperation
├── legacy/             # v1 信封、KindMapping、behavior_ability 槽位（扩展现有 legacy/）
├── factor/             # （已有）通用 Factor 构件
├── norming/            # （已有）常模 metadata
├── task_performance/   # （已有）
├── personality/        # （已有）
├── scale/              # （已有）
├── behavioral_rating/  # （已有）
└── cognitive/          # （已有）
```

### 文件迁移清单（根包 36 个 → 子包）

| 现文件 | 目标子包 | 备注 |
|--------|----------|------|
| `types.go`, `types_test.go` | `identity/` | Kind/Algorithm/SubKind/DecisionKind |
| `product_channel.go`, `product_channel_test.go` | `identity/` | |
| `personality_decision.go` | `identity/` | FallbackPersonalityDecisionKind |
| `algorithm_family.go`, `algorithm_family_test.go` | `routing/` | |
| `algorithm_identity_test.go` | `routing/` | |
| `execution_path.go` | `routing/` | |
| `payload_format.go`, `payload_format_test.go` | `routing/` | |
| `aggregate.go`, `aggregate_*_test.go` | `catalog/` | |
| `definition.go` | `catalog/` | DefinitionPayload |
| `snapshot.go` | `catalog/` | v2 PublishedModelSnapshot |
| `status.go` | `catalog/` | ModelStatus |
| `validation.go`, `validation_factor_test.go` | `catalog/` | |
| `capability.go`, `capability_test.go` | `capability/` | **先合并** `model_family_capability*` |
| `capability_role.go` | `capability/` | |
| `operation.go`, `operation_test.go` | `capability/` | |
| `legacy_alias.go` | `legacy/` | v1 Snapshot 信封 |
| `legacy_adapter.go` | `legacy/` | PublishedFromLegacy 等 |
| `legacy_behavior_ability_capability.go` | `legacy/` | |
| `behavior_ability.go`, `behavior_ability_test.go` | `legacy/` | |
| `behavior_ability_channel.go`, `behavior_ability_channel_test.go` | `legacy/` | |
| `doc.go` | 根包 | 增补模块地图 |
| `errors.go` | 根包 | |

### 实施顺序（减循环依赖）

| 步骤 | 动作 | 验证 |
|------|------|------|
| R16-1 | `doc.go` 文件地图（零行为变更） | 已完成 |
| R16-2 | 合并 `ModelFamilyCapability` → `KindCapability` | 已完成 |
| R16-3 | 建 `identity/`，迁 types + product_channel；根包 `export.go` 别名 | 已完成 |
| R16-4 | 建 `routing/`，迁 algorithm_family + execution_path + payload_format | 已完成 |
| R16-5 | 建 `capability/`，迁 capability + operation | 已完成 |
| R16-6 | 扩 `legacy/`，收拢 legacy_* + behavior_ability* | 已完成 |
| R16-7 | 建 `catalog/`，迁 aggregate + snapshot + validation | 已完成 |
| R16-8 | 架构测试：`domain/modelcatalog` 根包仅 doc/errors/export | 已完成 |

### 根包 `export.go` 示例（保持对外 API）

```go
package modelcatalog

import "…/modelcatalog/identity"

type Kind = identity.Kind
type Algorithm = identity.Algorithm
// …其余别名同理
```

### 不做（Phase B/C，需产品/数据窗口）

- 删除 `KindBehaviorAbility` / `RuntimeViaScaleLegacy`
- 删除 `KindMBTIMigration` / flat-kind 快照解码
- 合并 `behavioral_rating/snapshot` 与 `scale/snapshot` 解析框架

### 风险

- import 循环：`routing` 依赖 `identity`；`capability` 依赖 `routing` + `identity`；`catalog` 最后迁。
- 全仓 `modelcatalog.Kind` 通过别名可不变；**禁止**外部直引新子包（架构守卫可选）。

## Round 17–22：机制键主路径 + Interpretation 单轨 + Run 内存骨架（已完成）

| Round | 动作 |
|-------|------|
| R17 | `Outcome.RuntimeDescriptorKey` 三元组与 Evaluate 主路径接线 |
| R18 | `writer.ResolveByMechanism` 机制键报告生成；删除 EvaluatorKey 报告分叉 |
| R19 | BR/COG characterization 热修（暴露路由三轨分裂） |
| R20 | `EvaluationRun` 域模型 + Evaluate 内存编排 |
| R21 | `KindCapability` / `ModelCatalogOption` 拆型 |
| R22 | characterization harness 与 production assemble 对齐 |

## Round 23：执行路由单源化（已完成）

**做**：`pipeline.ExecutionRoutingFromSnapshot` 为 Evaluate / Interpretation / materialize 单源；删除 reporting `reportAlgorithmFamilyFromSnapshot` Kind-switch；characterization 注入 `RuntimeDescriptorRegistry`。

| 验证 | `execution_routing_test.go`、`routing_equivalence_test.go`、全量 characterization |

## Round 24：Descriptor 执行 + Run 落库 + Typology 子路由（已完成）

| 子轨 | 动作 |
|------|------|
| R24-A | `RuntimeResolver` descriptor-primary 走族级 `familyEvaluators`；`runtime_descriptor_primary: true` |
| R24-B | `evaluation_run` 表 + `assessment.current_run_id`；Execute Start/Success/Fail 落库 |
| R24-C | typology `MechanismKeys()` 按 `DecisionKind` 注册；`resolveEvaluatorKey` 无 typology 分支扩展 |

**刻意保留（R25+）**：删除 `EvaluatorKey`、Run REST、worker 重试编排、Statistics 投影统一。

## Round 25：机制键三轨对齐 + Descriptor-only 执行 + Run Attempt 递增（已完成）

| 子轨 | 动作 |
|------|------|
| R25-A | ScoreProjector / EventAssembler `ResolveByMechanism`；`writer.prepare` 三轨机制键；`DefaultMechanismEventAssemblers` |
| R25-B | `MaterializeFamilyEvaluators` + `WithFamilyEvaluators`；descriptor 命中不经 `EvaluatorRegistry` 族级 dispatch；legacy 仅 typology alias |
| R25-C | `NewEvaluationRunWithAttempt` / `NextEvaluationRun`；Start 前读 latest retryable run；worker 日志 `evaluation_run_hint` |

**刻意保留（R26+）**：worker 基于 DB `Retryable` 的 ack/nack 编排、Statistics 投影统一、DB 列删除、Phase B/C legacy kinds。

## Round 26：EvaluationRun 只读 REST + Operating 失败列表（已完成）

| 动作 | 说明 |
|------|------|
| Port | `ListByAssessmentID`、`ListRetryableFailed` |
| Application | `runquery` 查询服务；`ProtectedQueryService` 挂 org 隔离 |
| REST 业务 | `GET /api/v1/evaluations/assessments/:id/runs`、`.../runs/latest` |
| REST 内部 | `GET /internal/v1/evaluation-runs/failed?retryable=true`（`CapabilityOrgAdmin`） |

**刻意保留（R27+）**：run 创建/取消 REST；worker 重试策略改造。

## Round 27：KindCapability → ModelCatalogOption Registry（已完成）

| 动作 | 说明 |
|------|------|
| `option.Registry` | 启动时从 `DefaultCatalogOptions` + `DefaultFamilyCapabilities` 物化 |
| Application | `options.go` / `operation_policy.go` / `kind_mapper.go` 不再直引 `KindCapability` |
| 等价测试 | `TestRegistryAllowsMatchesDefaultCapabilities` |

## Round 28：EvaluatorKey 退役 → ExecutionIdentity（已完成）

| 子轨 | 动作 |
|------|------|
| R28-A | `AssertExecutionPathParity`；reporting 注册表 mechanism 主索引 + legacy `ExecutionIdentity` 回退 |
| R28-B | `ExecutionIdentity` 值对象；`ModelDescriptor` 删 `Key`；`resolveExecutionIdentity` |
| R28-C | 删除 `domain/evaluation/key.go`；characterization 契约迁移 |

**刻意保留（R29+）**：Phase B/C behavior_ability / migration kinds；worker 重试编排。

## Round 29：ModelCatalog 遗留收敛（已完成）

| 子轨 | 动作 |
|------|------|
| R29-B | 删除 `behavior_ability` List/Options 运行时双列表聚合；`List?kind=behavior_ability` 返回 400；Options 仍暴露 channel 元数据 |
| R29-C1 | 运行时删除 `mbti`/`sbti` flat `LegacyKindMapping` 读路径（仅保留 `scale` 迁移映射） |
| R29-C2 | infra snapshot 解码单入口：`infra/modelcatalog/published_decode.go`（BR + scale ruleset） |

## Round 30：Worker ack/nack 对齐 evaluation_run.retryable（已完成）

| 动作 | 说明 |
|------|------|
| Proto | `EvaluateAssessmentResponse` / `GenerateReportFromAssessmentResponse` 增 `retryable`、`run_id`、`failure_kind` |
| gRPC | 失败路径经 `RunQueryService.FindLatestByAssessmentID` 填充 run 元数据 |
| Worker | 优先读 `resp.Retryable`：`true` → nack；`false` + `status=failed` → ack |

## Round 31：ExecutionIdentity 遗留层清理（已完成）

| 动作 | 说明 |
|------|------|
| 应用层 | `ResolveOutcomeKey` / `resolveExecutionIdentity` 统一 `ExecutionIdentity()`；删除 `EvaluatorKey()` 包装 |
| 注册表 | 移除 `MaterializeLegacyEvaluators`；evaluator registry 空壳 + familyEvaluators 主路径 |

**刻意保留（R34+）**：DB 列删除、Conners 新 mechanism、`migration 000039` 索引（explain 需要时）。

## Round 32–40：终局收敛（已完成）

| Round | 动作 |
|-------|------|
| R32 | Typology `decision.kind` 唯一来源 payload/snapshot；`DecisionKindForIdentity` 不再为 personality 做 algorithm fallback；`legacy.FallbackPersonalityDecisionKind` 仅 migration 读路径 |
| R33 | Brief-2 publish 强制 `brief2.primary_dimension_code`；runtime `total/gec` 仅 legacy snapshot 兜底 |
| R34 | `calculation.ValidateScoreNodes` 接入 behavioral_rating publish（application 层）；`weighted_sum` 缺权重升为 error |
| R35 | 新建 `domain/evaluation/event`；assessment 事件经 compat 重导出；清理 architecture stale 白名单 |
| R36 | `calculationadapter/score.go` 抽取 score/level 转换 |
| R37 | `descriptorDrivenExecutor` 支持三件套 pipeline；有 registry 时禁止无 snapshot 的 legacy evaluator fallback |
| R38 | `option.Registry` 注册 `behavior_ability` product channel 元数据 |
| R39 | `MechanismReportBuilderKey` 扩展 `Algorithm`/`ProductChannel`；registry 逐级 fallback |
| R40 | `domain/evaluation/run/checkpoint.go` 定义 `CheckpointSeam` 契约；合表迁移后续轮次 |

## Round 41–50：终局对齐去测评类型包（已完成）

| Round | 动作 |
|-------|------|
| R41 | `domain/evaluation/event` 落地事件构造；`assessment` 保留 compat 适配 |
| R42 | `domain/calculation/scoring` 承接量表计分；`evaluation/scoring` 瘦身为 ACL |
| R43 | `domain/calculation/classification` 承接 trait 机制；legacy MBTI/SBTI scorer 迁至 application typology/legacy |
| R44 | 删除 `evaluation/norming`、`evaluation/task_performance` 占位包；新增 evaluation 架构守卫 |
| R45 | `policy/report_type.go` 承接 `ResolveReportType`；interpretation 子包结构守卫 |
| R46 | `interpretation/scoring` 暴露 `BuildFactorScoringReport` 机制入口 |
| R47 | typology patterns 以 `mechanism_assembler` 为主；legacy `*_assembler` 标记 transitional |
| R48 | reporting 架构守卫禁止新增测评 code builder 名；`legacy_report_aliases` 保留 deprecated ACL |
| R49 | `legacy_sbti_payload` / `from_sbti` 标注 legacy ACL |
## Round 51–60：瘦 transitional 包 + Application 层重组（已完成）

| Round | 动作 |
|-------|------|
| R51 | patterns 测评 DTO/converters 迁至 `application/.../typology/legacy`；domain patterns 仅保留 generic detail |
| R52 | `specialrule` 下沉 `calculation/classification`；configured 改引 calculation |
| R53 | 删除 `evaluation/typology/trait` ACL；调用方直引 `calculation/classification` |
| R54 | `InterpretReport` 等实现迁入 `interpretation/report`、`rule`、`builder`；根包 facade re-export |
| R55 | `application/evaluation/scoring` 写路径迁入 `outcome/scoring`；消除与 mechanisms/scoring 命名冲突 |
| R56 | 删除 `report_sbti`/`report_bigfive`；`report_input_mapper` 仅机制 detail + legacy 适配 |
| R57 | legacy outcome 装配收口至 `legacy/outcome_*`；`algorithm_aliases` 标记 deprecated |
| R58 | `interpretation/reporting` 拆 `registry`/`writer`/`projection` 子包 + 根 facade |
| R59 | 删除 domain `*_assembler` 薄包装；统一 `BuildPersonalityTypeReport` + mechanism template |
| R60 | `application/architecture_transitional_test.go` 守卫 + 本文档 R51–R60 表 |

## Round 61–77：终局对齐去测评 code 结构化（已完成）

| Round | 动作 |
|-------|------|
| R61 | 删除 `SPMProfile`；cognitive snapshot 仅 `taskperf.ApplyNormMetadata` 投影 |
| R62 | `Brief2Profile` → 机制中性 `NormingProfile`；norm_enricher/port 改读 generic |
| R63 | `PayloadFormatForCognitive/BehavioralRating` 去 Algorithm switch；legacy format 标 deprecated |
| R64 | publish 侧 `normingDecisionSpec` 由 norming metadata 驱动 |
| R65 | evaluation 根级 registry/execution_identity/runtime_path/input 归入 `pipeline/` |
| R66 | 删 `domain/evaluation/scoring`；桥接下沉 application；直引 `calculation/scoring` |
| R67 | typology patterns DTO 迁至 `application/evaluation/outcome/typology` |
| R68 | configured/adapter 上移 application；删 `domain/evaluation/typology` 整树 |
| R69 | interpretation `*_detail.go` 外迁 `application/.../legacy`；删 typology 薄包装 |
| R70 | 删 descriptor/report evaluator fallback；三件套缺失显式 error |
| R71 | modelcatalog 机制子包脚手架：`scoring/typology/taskperformance/binding/publishing` |
| R72 | `cognitive/snapshot` → `taskperformance/snapshot`（cognitive 留 compat seam） |
| R73 | `behavioral_rating/snapshot` → `norming/snapshot` |
| R74 | `scale/snapshot` → `scoring/snapshot` |
| R75 | `personality/typology` → `typology`（全量改 import；personality/typology 目录清空） |
| R76 | `identity/routing/catalog/capability` 经 `binding/publishing/export.go` 收敛；`task_performance` → `taskperformance` |
| R77 | modelcatalog 顶层包守卫 + 本文档 R61–R77 表；`系统设计文档` §19/§20.7 标注演进态 |

## Round 78–86：modelcatalog 机制化收官（已完成）

| Round | 动作 |
|-------|------|
| R78 | `identity`/`capability`/`QuestionnaireBinding` 实体迁入 `binding`；旧包降 compat re-export |
| R79 | `routing`/`catalog` 聚合迁入 `publishing`；根 `export.go` 仅引 binding/publishing/legacy |
| R80 | family `publish` 下沉 `publishing.SnapshotBuilder`；norming 校验/metadata 机制化 |
| R81 | `scoring/definition` seam re-export `scale/definition` 全部符号 + 契约测试 |
| R82 | infra/application/container/ruleset 分批改 import 至 `scoring/definition`（BSON 不变） |
| R83 | 物理迁移 `scale/definition` → `scoring/definition`；`scale/` 仅留 compat seam |
| R84 | `application/modelcatalog` service/bridge/gateway 去模型族命名；架构守卫禁回退文件名 |
| R85 | §20.5 `primary_dimension_code` 配置化；§20.6 `DecisionKind` 去 algorithm 推断 |
| R86 | 终局守卫收口（顶层八包 + scale compat）+ 文档 §19/§20.7 转正 + 本文档 R78–R86 表 |

## Round 87+：过渡包清零（进行中）

| Round | 动作 |
|-------|------|
| R87 | 剩余调用方改引 `scoring/definition`；删除 `modelcatalog/scale/` 整树；顶层守卫禁 `scale/` |
| R88 | `identity/routing/catalog/capability` 调用方改引 `binding/publishing`；删除四过渡包 |
| R89 | `personality publish` 迁入 `publishing.SnapshotBuilder`；`typology` 去 `modelcatalog` 根依赖；删 `cognitive/behavioral_rating/task_performance` domain 包 |
| R90 | `application/modelcatalog/{personality,behavioral_rating,cognitive}` → `{typology,norming,taskperformance}`；删 `personality/seed`；seed 调用方直引 `legacy`；守卫禁回退目录 |
| R91 | `container/modules/modelcatalog` 装配机制化：`assemble_{typology,norming,taskperformance}`；`Module.{Typology,Norming,TaskPerformance}`；守卫禁回退 assemble 文件名 |
| R92 | `publishing` 快照/载荷解码机制化：`typology_payload`/`snapshot_{typology,norming,taskperformance}`；`TypologyPayloadAndRuntimeSpecFromModel`；守卫禁回退 publishing 文件名 |

过渡态说明（R109 后对外 REST/gRPC 与 collection 内部 session/cache/warmup 命名已收敛 typology；领域 `KindPersonality`、`ProductChannelPersonality`、`PayloadFormatPersonalityTypologyV1` 等历史数据值仍保留 personality 字样）：

## Round 106–107：运行审计与 API 身份补全（R107）

| Round | 动作 |
|-------|------|
| R106 | modelcatalog 机制包结构收官（见 `docs/系统设计文档.md` §19） |
| R107 | `EvaluationRun` 补齐 `trace_id` / `input_snapshot_ref`；evaluation 与 typology catalog 的 `ModelIdentity` 暴露 `product_channel` / `algorithm_family`；report routing 已支持 Algorithm/ProductChannel key 精确命中与 broad fallback 测试；Audience/ReportProfile 在 R112 入键 |

## Round 108：旧接口与兼容层下线（R108）

| 轮次 | 内容 |
|------|------|
| R108 | breaking 下线 `/api/v1/personality-*` REST、`personalitymodel` gRPC、application `Personality*` DTO/deprecated alias；唯一公开面为 `/api/v1/typology-*` + `typologymodel` gRPC；OpenAPI drift 字段已重生成修复；`KindPersonality` 数据值与 `PersonalityModel*` 缓存信号名在 R108 当轮仍保留 |

## Round 110：v1 legacy 测评读端点下线（R110）

| 轮次 | 内容 |
|------|------|
| R110 | breaking 下线 collection v1 量表投影读端点 `GET /api/v1/assessments`（列表/`:id`/`:id/report`）与 `GET /api/v1/answersheets/:id/assessment`；删除 `LegacyAssessment*` DTO 与 `v1_projection` 投影层；报告可见因子过滤下沉到 v2 `GetAssessmentReport`；`ResolveAssessmentByAnswerSheetID` gRPC 消费端下线（reader 能力暂留）；`algorithm` REST 查询过滤此前已不存在。测评读统一 `/api/v2/assessments*` 与 `/api/v1/typology-assessments*` |

## Round 109：内部旧命名闭环（R109）

| 轮次 | 内容 |
|------|------|
| R109 | `collection-server/application/personalitysession` 迁移为 `typologysession`；OpenAPI schema 不再导出 `personalitysession.*`；collection container 字段、cache signal、governance warmup、L1 cache 配置从 `PersonalityModel*` / `personality_model_cache_changed` / `static.personality_model` / `personality_cache` 收敛为 typology 命名；新增架构/契约测试防止旧命名回流 |

## Round 111：factor_scoring 原生 RuntimeDescriptor triple（R111）

| 轮次 | 内容 |
|------|------|
| R111 | `registry/mechanisms/scoring/pipeline_components.go` 实现原生 `InputAssembler/Calculator/OutcomeAssembler`；`runtime.AttachNativePipelines` 仅对 `AlgorithmFamilyFactorScoring` 装配；`AttachEvaluatorPipelines` 跳过已原生化 family；`scoring.Executor` 与 `pipeline_bridge` 保留供 fallback/characterization；norming/task_performance/typology 仍 evaluator-backed |

## Round 111b：factor_norm / task_performance 原生 RuntimeDescriptor triple（R111b）

| 轮次 | 内容 |
|------|------|
| R111b | `norming/pipeline_components.go` 与 `task_performance/pipeline_components.go` 实现原生 triple（Calculator 克隆 scale payload 后委托 `scoring.Executor`，OutcomeAssembler 分别做 `ApplyFactorProjections` / `NormalizeOutcome`）；`AttachNativePipelines` 扩展装配两 family；`AttachEvaluatorPipelines` 同步 skip；Executor 与 bridge 保留；仅 `factor_classification`（typology）仍 evaluator-backed |

## Round 111c：factor_classification（typology）原生 RuntimeDescriptor triple（R111c）

| 轮次 | 内容 |
|------|------|
| R111c | `typology/pipeline_components.go` 实现原生 triple（Calculator 经 configured `runnerForIdentity` 调用 `algorithmRunner.buildOutcome`，OutcomeAssembler pass-through）；`AttachNativePipelines` 装配 `AlgorithmFamilyFactorClassification`；四条机制均已原生化，仅 `pipeline_bridge` 与 legacy Executor 保留供 fallback |

## Round 111d：删除 evaluator-backed bridge（R111d）

| 轮次 | 内容 |
|------|------|
| R111d | 删除 `execute/pipeline_bridge.go`（`EvaluatorPipelineComponents` / `evaluatorCalculator`）与 `runtime/pipeline_attach.go`（`AttachEvaluatorPipelines`）；上下文辅助迁至 `execute/descriptor_pipeline_context.go`；生产仅 `AttachNativePipelines` 装配；family `Executor` 仍保留供无 snapshot 的 evaluator registry fallback |

## Round 112：报告 Audience / ReportProfile 路由入键（R112）

| 轮次 | 内容 |
|------|------|
| R112 | 新增 `domain/interpretation/policy.ReportProfile`；`MechanismReportBuilderKey` / `ReportRoutingContext` additive 扩展 `Audience` / `ReportProfile`；`MechanismKeyFallbackCandidates` 扩展逐级剥离并 dedupe；v1 生成路径默认空值，broad builder 行为不变；生产注册未新增 audience/profile 专用 builder |

## Round 113：跨库写入对账与补偿（R113）

| 轮次 | 内容 |
|------|------|
| R113 | 新增 `application/evaluation/consistency`：`Scan` 检测 report 已出站但 MySQL status 未 interpreted、计分产物已落库但 status 仍 submitted；`RepairInterpretedFinalization` 幂等重放 reporting 末步（`ApplyOutcome` + assessment Save）；`evaluation_run` 与 `analytics_projector_checkpoint` 仍未物理合并 |

## Round 114：四机制 Audience / ReportProfile 生产注册（R114）

| 轮次 | 内容 |
|------|------|
| R114 | `policy.ReportProfile` 常量与 `ReportProfileForDecisionKind`；`ReportRoutingContextFromOutcome` 按 decision 填充 profile（v1 audience 仍空）；`ExpandAudienceProfileBuilders` 为 scoring/norming/task_performance/typology 注册 participant/clinician/admin/profile 键；clinician 经 `VisibilityPolicy` 过滤 `model_extra`；生产 `buildReportBuilderRegistry` 物化后展开 |

## Round 115：CheckpointSeam 物理合表（R115）

| 轮次 | 内容 |
|------|------|
| R115 | migration `000040` 创建 `runtime_checkpoint` 并 backfill `evaluation_run` / `analytics_projector_checkpoint` 后 drop 旧表；`infra/mysql/checkpoint` 实现 `evaluationrun.Repository` + `CheckpointSeam` + analytics projector 幂等；statistics journey 与 evaluation execute/runquery 改读新表；cleanup 脚本同步 |
