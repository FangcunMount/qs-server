# Validation 规则语言

`domain/validation` 只保留答案校验相关的领域规则语言：

- `RuleType`：校验规则编码，如 `required`、`min_length`、`max_value`。
- `ValidationRule`：题目上的校验规则声明和值对象。

校验值接口、策略实现、默认 validator、批量校验等执行细节不属于 domain，放在 `internal/apiserver/infra/ruleengine`。应用服务通过 `port/ruleengine` 调用校验执行能力。
