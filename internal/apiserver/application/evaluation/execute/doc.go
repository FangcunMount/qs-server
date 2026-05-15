// Package execute 评估执行引擎
//
// 负责调度一次测评执行，由 qs-worker 消费 AssessmentSubmittedEvent 后调用
//
// 设计说明：
// execute 只承担通用编排：加载 Assessment、解析输入快照、选择模型执行器、写入结果并统一收口失败；具体模型解释由 Scale/MBTI 等 executor 实现。
//
// 行为者：评估引擎 (Evaluation Engine / qs-worker)
// 职责：执行一次通用测评编排
// 变更来源：测评执行流程、失败收口、结果写入边界
// 说明：此服务由 qs-worker 调用，消费 AssessmentSubmittedEvent
package execute
