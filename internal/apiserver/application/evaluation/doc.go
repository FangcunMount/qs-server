// Package evaluation 评估应用服务
//
// 负责测评执行、结果写入与查询编排。写路径以 AssessmentOutcome + EvaluatorKey 为主线：
// execute 按 EvaluatorKey 路由到各模型家族 Executor；interpretation/reporting.Writer 以 Outcome.Execution 为权威，
// legacy EvaluationResult 投影仅保留在 outcome/legacy 边界（characterization 适配）。
//
// assessment 包按行为者拆分测评生命周期与读查询服务。
// interpretation 包负责报告构建、得分投影与 assessment 状态落库。
// execute 包负责评估执行、失败收口与 outbox 时序。
package evaluation
