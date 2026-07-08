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
// # 根包门面（export.go）
//
// 根包仅保留 doc.go、errors.go、export.go。业务类型通过子包实现，根包 re-export：
//
//   - binding/: Kind、SubKind、Algorithm、ModelFamilyCapability（执行/生命周期守卫）
//   - publishing/: PublishedModelSnapshot、AlgorithmFamily、PayloadFormat
//   - factor|scoring|norming|typology|taskperformance/: 机制载荷与快照
//   - legacy/: v1 信封、迁移适配器、behavior_ability 产品通道
//
// 深 import 机制子包（factor/typology 等）用于机制专用逻辑；跨机制常用读路径类型经 export.go 薄 re-export。
// 新增机制专用逻辑仍优先落在子包；仅将跨包高频叶子类型提升到根包别名。
// application 展示选项见 application/modelcatalog/option.ModelCatalogOption（与 domain ModelFamilyCapability 分离）。
//
// # 子包（机制八包，已与磁盘对齐）
//
//   - binding/: 身份、产品通道、ModelFamilyCapability
//   - publishing/: 发布聚合、快照构建、PayloadFormat
//   - factor/: 共享因子图元数据
//   - scoring/: 因子计分定义与快照
//   - typology/: 类型学载荷、发布、校验
//   - norming/: 常模/行为评定快照
//   - taskperformance/: 任务表现快照
//   - legacy/: v1 兼容读取
package modelcatalog
