// Package modelcatalog 负责已发布测评模型资产。
//
// # 概览
//
// v2 引入 Kind/SubKind/Algorithm 身份、PublishedModelSnapshot，以及统一的人格类型学载荷。
// 迁移期仍保持旧版 ruleset.* 载荷格式可读；新的写入使用 assessmentmodel.* 载荷格式。
//
// KindCapability.Role 把产品通道和可执行模型家族分开。
// AssessmentModel.ProductChannel 是面向产品分类的 taxonomy 字段，不得驱动运行时执行路径选择。
// 可执行模型家族的约束应使用 ModelFamilyCapability。
//
//   - behavioral_rating / cognitive（CapabilityRoleModelFamily）：可执行家族；
//     Brief-2 落在 behavioral_rating+AlgorithmBrief2，SPM 落在 cognitive+AlgorithmSPM。
//
// API 类型 behavior_ability 只是聚合 behavioral_rating 和 cognitive 列表的产品通道；
// 它不映射到领域 Kind。领域 KindBehavioralRating 是独立 behavioral_rating 运行时
// （assessmentmodel.behavioral_rating.default.v1 用于旧版默认模型；
// assessmentmodel.behavioral_rating.brief2.v1 用于 Brief-2）。
//
// KindCustom 是保留的目录类型（API 选项禁用），与类型学算法 custom_typology
// 或计划 scheduleType=custom 无关。KindCognitive 通过 ExecutionPathCognitive 描述符执行。
// (assessmentmodel.cognitive.默认.v1 旧版; assessmentmodel.cognitive.spm.v1 用于 SPM)。
//
// # 机制命名（包名与枚举）
//
// 执行代码使用短包名；API/路由使用 AlgorithmFamily 枚举：
//
//	Go 包（evaluation）    AlgorithmFamily
//	scoring                factor_scoring
//	typology               factor_classification
//	norming                factor_norm
//	task_performance       task_performance
//
// See docs/02-业务模块/mechanism-oriented-migration.md §包名与 AlgorithmFamily 对照表.
//
// # 根包文件映射（Round 16 基线）
//
// 下列文件当前位于 modelcatalog 根包。Round 16+ 可能把它们移动到子包
// （identity/、routing/、catalog/、capability/、legacy/），并在根包保留类型别名。
//
// Identity — 草稿/已发布模型身份和产品分类体系（identity/ 子包）：
// - export.go: 根 类型别名。
// - identity/types.go: Kind、SubKind、Algorithm、DecisionKind。
// - identity/product_channel.go: ProductChannel。
// - legacy/personality_decision.go: FallbackPersonalityDecisionKind（仅 migration 读路径）。
//
// Routing — 执行家族和物化路径（routing/ 子包）：
// - export.go: 根 类型别名。
// - routing/algorithm_family.go: AlgorithmFamily，身份到家族的映射。
// - routing/execution_path.go: ExecutionPath。
// - routing/payload_format.go: 载荷格式常量和辅助函数。
//
// Catalog — 聚合、信封结构、校验（catalog/ 子包）：
// - export.go: 根 类型别名。
// - catalog/snapshot.go: PublishedModelSnapshot、ModelDefinition、QuestionnaireBinding。
// - catalog/aggregate.go: AssessmentModel、NewAssessmentModel。
// - catalog/definition.go: DefinitionPayload。
// - catalog/status.go: ModelStatus。
// - catalog/validation.go: 领域校验问题。
//
// Capability — API/目录操作守卫（capability/ 子包）：
// - export.go: 根 类型别名。
// - capability/capability.go、capability_role.go: KindCapability 矩阵。
// - capability/operation.go: CatalogOperation。
//
// Legacy / 兼容性 — 迁移读取器和 behavior_ability 产品通道（legacy/ 子包）：
// - legacy/alias.go: v1 Snapshot、Definition、RuleSetType 别名。
// - legacy/adapter.go: LegacyKindMapping、PublishedFromLegacy。
// - legacy/behavior_ability.go、behavior_ability_channel.go。
// - legacy/kind_mapping.go。
//
// 共享：
// - errors.go: 领域错误。
// - export.go: 根包到子包的门面别名。
// - architecture_test.go: 根包守卫（仅允许 doc/errors/export）。
//
// # 子包（按模型家族或机制元数据）
//
// - 身份/: 类型, 算法, 子类型, 判定类型, ProductChannel。
// - 路由/: 算法家族, ExecutionPath, 载荷格式。
// - 能力/: 类型能力, 目录操作。
// - 因子/: 共享因子快照、层级、计分/分类规格。
// - 常模ing/: 常模/复合 index 元数据（AlgorithmFamilyFactorNorm）。
// - task_performance/: 任务元数据（AlgorithmFamilyTaskPerformance）。
// - personality/: 类型学载荷、发布、校验器。
// - scale/: 量表定义和快照。
// - behavioral_rating/: behavioral_rating 快照（包括 Brief-2 画像）。
// - cognitive/: cognitive 快照（包括 SPM 画像）。
// - 目录/: AssessmentModel 聚合、已发布快照、校验。
// - 旧版/: v1 信封结构、适配器、behavior_ability 通道。
package modelcatalog
