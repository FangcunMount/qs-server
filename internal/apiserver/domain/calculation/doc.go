// Package calculation 针对模型输入执行计分和投影规则。
//
// 边界：
//   - ModelCatalog 负责模型结构和规则配置（因子、常模、策略）。
//   - Calculation 执行这些规则并生成 calculation.Result。
//   - Evaluation 编排一次测评执行，并通过应用层适配器把计算结果映射为
//     outcome.Execution.
//
// Calculation 是无状态计算内核，不得导入 modelcatalog、factor、question 或其他领域资产。
// 调用方负责把自身领域资产转换为 calculation 的中性输入（ScoreNode、ScoreValue），
// 并消费 calculation.Result。
package calculation
