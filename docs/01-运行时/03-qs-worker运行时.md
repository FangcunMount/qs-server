# qs-worker 运行时

## 1. 结论

`qs-worker` 消费领域事件并调用 apiserver internal gRPC。它负责消息处理语义，不直接拥有业务事务或主业务存储。

## 2. 当前主链事件

- `answersheet.submitted`
- `evaluation.requested`
- `evaluation.outcome.committed`
- `evaluation.failed`
- `interpretation.report.generated`
- `interpretation.report.failed`

最终清单以 `configs/events.yaml` 为准。

## 3. 处理原则

- handler 只做解析、幂等/重复抑制、调用和 Ack/Nack 决策；
- 业务状态迁移在 apiserver application/domain；
- 可重试错误与永久错误应保留不同的消息语义；
- worker 关闭前停止接收新消息并等待在途处理结束。
