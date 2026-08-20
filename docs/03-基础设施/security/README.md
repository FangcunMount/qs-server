# Security

Security 是独立基础设施签署域，不能复用“业务模块主链已实现”的结论。工作负载身份、用户能力和资源 ownership 必须逐层成立；网络分段不能替代资源授权，JWT 也不能替代组织与 Testee/Profile 归属。

## 阅读顺序

1. [身份、服务与资源授权](./10-身份、服务与资源授权.md)：`security/ACL/resource ownership` 的 canonical 文档。
2. [敏感信息、网络暴露与验收](./20-敏感信息、网络暴露与验收.md)：Secret、日志、管理端点和网络正反向验收。

## 当前签署边界

- production YAML 声明 gRPC mTLS、ACL enabled 和 default deny；ACL 文件缺失/无效时 server 构造失败。这只是仓库配置意图，仍需 effective config 与实际握手证据。
- mTLS/方法 ACL 认证并限制服务身份，不能证明下游 RPC 对具体 User/Profile/Testee/Org 已做 ownership。
- Actor public Intake Profile 代表权和 collection Testee GET/PUT/care-context 的 IDOR 是当前源码确认的 P0；基础设施 security 必须保持 blocked，直到代码修复、跨用户负测和环境验收完成。
- apiserver IAM disabled/verifier 缺失时跳过认证的装配风险仍存在，不得写成统一 fail closed。

## 验证入口

```bash
go test -count=1 ./internal/pkg/grpc ./internal/pkg/httpauth ./internal/collection-server/application/testeeaccess ./internal/collection-server/transport/rest/...
```

生产签署还需要：真实 mTLS 正反向握手、方法 ACL、跨用户/跨机构资源访问负测、Secret 脱敏和管理端点网络不可达证明。
