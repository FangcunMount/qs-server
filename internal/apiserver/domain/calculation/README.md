# Calculation 规则语言

`domain/calculation` 只保留问卷和量表计分相关的领域规则语言：

- `StrategyType`：计分策略编码，如 `sum`、`average`、`weighted_sum`。
- 参数 key：如 `weights`、`precision`。
- `FormulaType` / `CalculationRule`：题目或因子上的计分规则声明。

执行器、策略注册表、批量计分、选项计分等实现细节不属于 domain，放在 `internal/apiserver/infra/ruleengine`。应用服务通过 `port/ruleengine` 消费执行能力。

M5 的 `application/evaluation/execute/native_kernel_golden_test.go` 用合法源码夹具运行真实 `RuntimeResolver` 与四个 native descriptor pipeline：Scale 区间/小数边界，BRIEF2 的 catalog NormLookup/人群缺失，四 pole 类型和显式 nearest_pattern，以及 SPM 冻结答案键/常模/能力 OutcomeCode。没有手工组装执行结果；SPM 夹具故意提供错误客户端 score，验证它只按冻结答案键计算。真实计算结果随后验证 hash/输入身份的派生字段隔离及 schema3 报告资产、常模和模板路由冻结。

`definition/testdata/hash_v1` 是另一组固定的合成序列化/历史回放 corpus，字节与 digest 不由内核测试重新生成；其中人工999分只验证展示不重匹配。两组证据均不代表105条生产模型完成语义验收或客户端正式发布。生产4个资产差异和25个 invalid head 继续阻断派生持久化删除。

CI 在现有普通测试之外核对 `^TestM5NativeKernelGolden` 恰好命中四项，再实际以 `-race -count=1` 执行并拒绝 SKIP：

```bash
go test -race -count=1 -run '^TestM5NativeKernelGolden' ./internal/apiserver/application/evaluation/execute
```
