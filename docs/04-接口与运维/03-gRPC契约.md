# gRPC 契约

## 1. 事实源

proto 位于 [`api/grpc/proto`](../../api/grpc/proto/)，生成代码位于 `api/grpc/gen`。不要手改生成文件。

## 2. 两类调用

- collection -> apiserver：前台 BFF 把受保护请求交给业务中心；
- worker -> apiserver：异步 handler 通过 internal service 驱动 application use case。

## 3. 约束

- transport DTO 与领域模型分离；
- 组织、身份、请求标识和幂等标识按用例需要显式传递；
- domain/application error 在 gRPC 边界映射为稳定 status；
- proto 兼容性遵循字段号稳定和增量演进原则。

## 4. AnswerSheet 持久结果回读

`AnswerSheetService.LookupAnswerSheetSubmission` 是 collection -> apiserver
的 additive 内部 RPC。请求显式携带 writer、idempotency key、问卷、testee、
task、origin 和 answers，不携带客户端不可控的 org。apiserver 先按 writer/key
读取已持久 AnswerSheet，再使用已存 org 与本次稳定输入计算 fingerprint：

- `found=true` 必须同时返回非零 `id`；
- miss 返回 `found=false`；
- 同 key 不同内容返回 `AlreadyExists`；
- Mongo 或内部读取失败返回 `Unavailable`；
- 取消和 deadline 保持 `Canceled` / `DeadlineExceeded`。

这个 RPC 不访问 questionnaire、binding、attribution、IAM/ProfileLink，不启动
transaction，也不写 Outbox。旧 apiserver 的 `Unimplemented` 仅用于滚动升级
期间回退既有 `SaveAnswerSheet` durable path。

## 5. 验证

运行 proto 生成/契约检查、server service 测试和两个调用方的定向测试。
