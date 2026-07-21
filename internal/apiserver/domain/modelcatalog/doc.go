// Package modelcatalog 负责测评模型资产目录。
//
// # 概览
//
// v2 使用 Kind/SubKind/Algorithm 身份与 DefinitionV2 作为模型语义来源。
// published payload 是 port/modelcatalog/payload 下的编解码契约，不属于本领域包。
//
// KindCapability.Role 把产品通道和可执行模型家族分开。
// AssessmentModel.ProductChannel 是面向产品分类的 taxonomy 字段，不得驱动运行时执行路径选择。
// 可执行模型家族的约束应使用 ModelFamilyCapability。
//
//   - behavioral_rating / cognitive（CapabilityRoleModelFamily）：可执行家族；
//     Brief-2 落在 behavioral_rating+AlgorithmBrief2，Raven SPM 落在
//     cognitive+AlgorithmSPM；感觉统合 SPM 落在
//     behavioral_rating+AlgorithmSPMSensory。
//
// API 类型 behavior_ability 只是聚合 behavioral_rating 和 cognitive 列表的产品通道；
// 它不映射到领域 Kind。领域 KindBehavioralRating 是独立 behavioral_rating 运行时
// （assessmentmodel.behavioral_rating.default.v1 用于旧版默认模型；
// assessmentmodel.behavioral_rating.brief2.v1 用于 Brief-2）。
//
// KindCognitive 通过 ExecutionPathCognitive 描述符执行。类型学算法 custom_typology
// 与测评模型 Kind 无关。
// (assessmentmodel.cognitive.默认.v1 旧版; assessmentmodel.cognitive.spm.v1 用于 SPM)。
//
// # 根包门面（export.go）
//
// 根包仅保留 doc.go、errors.go、export.go。业务类型通过子包实现，根包 re-export：
//
//   - identity/: Product、Identity、Family
//   - assessmentmodel/: AssessmentModel 聚合
//   - definition/: Definition、MeasureSpec、Calibration、ReportMap
//   - factor/: Factor 和共享因子图元数据
//   - norm/: Norm 和 NormRef
//   - conclusion/: Risk/Type/Norm/Ability Conclusion
//   - taskperformance/: cognitive 测量 metadata
//
// 深 import 目标领域子包用于机制专用逻辑；跨机制常用读路径类型经 export.go 薄 re-export。
// 新增机制专用逻辑仍优先落在子包；仅将跨包高频叶子类型提升到根包别名。
// application 展示选项由 application/modelcatalog.CatalogQueryService 投影，
// 与 domain ModelFamilyCapability 分离。
//
// # 子包
//
//   - identity/: 产品概念、算法身份和执行家族
//   - assessmentmodel/: 后台可编辑测评模型配置聚合
//   - definition/: 测评模型定义主体
//   - binding/: 基础身份值、产品通道、ExecutionPath、ModelFamilyCapability
//   - factor/: 共享因子图元数据
//   - norm/: 常模资料与引用
//   - conclusion/: 解释与结果声明
//   - taskperformance/: cognitive 测量 metadata
package modelcatalog
