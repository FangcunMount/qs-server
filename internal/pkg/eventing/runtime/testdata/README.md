# 固定旧版本消息契约样本

`retired-wire-contracts.json` 来自 QS 提交 `96fc4df42f6af9f6ce22e2184394b1a07df2ffdd` 所固定的 `component-base v0.6.11`。执行 `bash scripts/testing/capture-retired-wire-contracts.sh` 可重新捕获；脚本归档原提交并使用原 go.mod，不能使用当前 SDK 生成预期。

全部输入都是合成测试值。样本包含九条可靠事件和六条外围事件的原始业务 JSON、直接发送 wire、旧 Relay 解码再编码 wire，以及十一类解码输入对应的原始结果、元数据或错误。外层 JSON 使用字符串保存原始字节，避免缩进重写原始 JSON。旧错误中的解析器包名只在当前解码测试比较时按既有规则转换。

当前测试保留事件目录／路由核对、完整字节一致、稳定消息身份、UTC+8、消费者载荷和异常输入行为。这些样本不替代实际 NSQ 投递、数据库事务或恢复验收。实际旧 provider 集成测试仍另行维护，不将当前 SDK 对端冒充旧版本。
