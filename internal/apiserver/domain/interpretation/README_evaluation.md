# Evaluation Interpretation Domain

本包只保留测评解读的领域语言和值对象：

- 风险等级、解读规则、解读配置。
- 阈值、区间、组合规则的声明形态。
- 领域层可复用的校验错误。

解读策略注册、默认文案提供者、批量执行器和并发执行都不属于 domain。它们位于 `internal/apiserver/infra/interpretengine`，并通过 `internal/apiserver/port/interpretengine` 暴露给 application pipeline。

## 分层边界

```text
domain/evaluation/interpretation
  规则语言和值对象

port/interpretengine
  application 消费的执行端口和执行 DTO

infra/interpretengine
  阈值/区间策略、默认文案、执行器 registry

application/evaluation/engine/pipeline
  组装执行 DTO、调用端口、应用结果和持久化
```

## 扩展方式

新增解读规则类型时：

1. 在 domain 中增加稳定的规则语言和值对象。
2. 在 `port/interpretengine` 增加 application 需要的最小执行 DTO 字段。
3. 在 `infra/interpretengine` 实现策略并在 adapter 构造时注册。
4. 在 pipeline seam tests 中锁定 fallback、风险等级和默认文案语义。

domain 不直接调用执行器，也不保存 package-level singleton。
