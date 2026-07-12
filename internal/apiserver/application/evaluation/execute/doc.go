// Package execute 评估执行引擎
//
// 负责调度一次测评执行，由 qs-worker 消费 evaluation.requested 后调用
//
// 设计说明：
// execute 只承担通用编排：claim EvaluationRun、加载 Assessment、解析输入快照、
// 选择评分执行器、可靠提交 Outcome，并统一收口评分失败。
//
// 行为者：评估引擎 (Evaluation Engine / qs-worker)
// 职责：执行一次通用测评编排
// 变更来源：测评执行流程、失败收口、结果写入边界
// 说明：此服务由 qs-worker 调用，消费 evaluation.requested
package execute
