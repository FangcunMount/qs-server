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

## 4. 验证

运行 proto 生成/契约检查、server service 测试和两个调用方的定向测试。
