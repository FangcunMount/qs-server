// Package result 负责评估执行后的写入阶段。
//
// 写入器持久化报告（使用出箱暂存），然后投影分数和评估解释保存，然后通知等待者。
//
// 跨存储补偿不在这里处理。调用者必须将此包视为应用程序一致性边界，而不是模型特定评分逻辑。
//
// Round 2 边界：
//   - 写链主模型为 domain AssessmentOutcome（Execution 字段）。
//   - legacy_projection.go 是 application 内唯一 legacy 投影入口，供 characterization 与边界适配使用。
//   - types.go 的 NewOutcomeFromLegacyResult 是 legacy outcome 适配的唯一 application 入口。
//   - Writer 主路径 ApplyOutcome；resolveOutcomeKey 不再经 legacy 投影 fallback。
//
// 行为者：评估引擎 (Evaluation Engine / qs-worker)
// 职责：评估执行后的写入阶段
package result
