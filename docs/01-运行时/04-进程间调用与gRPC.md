# 进程间调用与 gRPC

## 1. 结论

collection 与 worker 都通过 gRPC 把业务写入交回 apiserver。REST/OpenAPI 与 gRPC/proto 是不同契约，不能靠 prose 文档复制维护字段。

## 2. 事实源

- proto：`api/grpc/proto`。
- 生成代码：`api/grpc/gen`。
- apiserver service：`internal/apiserver/transport/grpc/service`。
- collection/worker client：各自 infra 或 client 适配层。

## 3. 约束

调用链必须透传请求标识、租户/组织上下文和必要身份信息；服务间认证与用户身份不能混成同一概念。错误码映射应在 transport 边界完成。
