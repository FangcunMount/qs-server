# gRPC 服务基础设施

## 结论

`internal/pkg/grpc` 是 qs-server 的通用 gRPC server 集成层；业务服务注册与 handler 位于 `internal/apiserver/transport/grpc`。本包复用 component-base 的 recovery、logging、mTLS identity、ACL 与 audit 能力，同时接入 IAM TokenVerifier、组织授权快照和 qs-server 自己的配置校验。

生产是否启用 TLS、mTLS、IAM auth、ACL、audit 或 reflection，必须回到 `configs/apiserver.prod.yaml`、`configs/grpc-acl.prod.yaml` 与实际部署证据核对；代码支持不等于环境已经启用。

## 职责边界

| 层 | 当前入口 | 责任 |
| --- | --- | --- |
| 通用 server | `internal/pkg/grpc/server.go` | listener、消息大小、连接年龄、health、reflection、拦截器链 |
| 配置 | `internal/pkg/grpc/config.go` | TLS/mTLS、IAM auth、ACL、audit 与 server 参数 |
| ACL 加载 | `internal/pkg/grpc/acl_loader.go` | strict、non-empty、default-deny 配置校验 |
| 请求上下文 | `internal/pkg/grpc/context.go` | User、Org、mTLS service identity 与 Request ID 投影 |
| 业务注册 | `internal/apiserver/transport/grpc/registry.go` | 将各模块 application service 注册到 server |
| handler | `internal/apiserver/transport/grpc/service` | Proto DTO、鉴权后的用例调用与错误映射 |
| 机器契约 | `api/grpc/proto` | 手写 Proto 真源；生成代码不作为首要编辑入口 |

本包不拥有领域授权规则，也不把 transport identity 当作资源 ownership。资源级授权仍由 application/domain service 完成。

## 一元拦截器顺序

`buildUnaryInterceptors` 当前按以下顺序组装：

1. recovery；
2. Request ID 传播/生成；
3. logging；
4. mTLS identity（启用时）；
5. IAM authentication（启用且 TokenVerifier 可用时）；
6. `ExtraUnaryAfterAuth`，当前用于授权快照等进程级扩展；
7. ACL（启用时）；
8. audit（启用时）。

ACL 运行策略必须是 `deny`，配置文件缺失、为空、方法名未知或与运行策略不一致都会令 server 构造失败。新增 RPC 时必须同步 Proto、service registry、collection/worker client method 清单与生产 ACL。

## 验证

```bash
go test ./internal/pkg/grpc
go test ./internal/apiserver/transport/grpc/...
```

重点回归：

- `acl_identity_matrix_test.go`：注册 RPC、客户端身份和生产 ACL 的闭包；
- `acl_loader_test.go`：default-deny 与非法配置 fail-closed；
- `requestid_test.go`：请求 ID 传播；
- `proto_contract_test.go`、`architecture_test.go`：Proto 与 transport 边界。

历史上已删除的自建 grpcserver 包和旧 transport 目录只存在于 Git 历史，不再是当前代码入口。
