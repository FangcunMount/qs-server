// Package evaluation 评估应用服务
//
// 负责测评执行、Outcome 事实提交、Assessment/score 投影与评分查询。
// 写路径以 Outcome.Execution + EvaluatorKey 为主线：execute 按 EvaluatorKey
// 路由到各模型家族 Executor，所有成功结果通过 EvaluationCommitter 可靠提交。
//
// assessment 包按行为者拆分测评生命周期与读查询服务。
// execute 包负责评估执行、失败收口与 outbox 时序。
// 报告查询、报告等待及 legacy interpreted 投影由 Interpretation/Journey
// 负责，不属于 Evaluation application 的能力。
package evaluation
