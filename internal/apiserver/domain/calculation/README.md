# Calculation 规则语言

`domain/calculation` 只保留问卷和量表计分相关的领域规则语言：

- `StrategyType`：计分策略编码，如 `sum`、`average`、`weighted_sum`。
- 参数 key：如 `weights`、`precision`。
- `FormulaType` / `CalculationRule`：题目或因子上的计分规则声明。

执行器、策略注册表、批量计分、选项计分等实现细节不属于 domain，放在 `internal/apiserver/infra/ruleengine`。应用服务通过 `port/ruleengine` 消费执行能力。
