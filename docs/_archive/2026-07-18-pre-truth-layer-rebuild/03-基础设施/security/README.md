# security

security 模块是 qs-server 的安全支撑层，用于消费 IAM 身份、构建访问上下文、校验能力边界和服务间认证。

## 1. 这个模块解决什么问题

它解决“小程序、后台和内部服务调用进来后，qs-server 如何知道是谁、属于哪个组织、能做什么”的问题。

## 2. 它在 qs-server 中处于什么位置

security 位于 HTTP / gRPC 入口和业务服务之间。qs-server 不负责认证中心本身，而是消费 IAM 身份和权限结果。

## 3. 整体架构是什么

请求携带 IAM token 或服务身份；入口解析 Principal；应用层构建 OrgScope、AuthzSnapshot 和 CapabilityDecision；业务服务按能力校验。

## 4. 关键链路有哪些

| 链路 | 文档 |
| --- | --- |
| 整体架构 | [01-安全模块整体架构.md](01-安全模块整体架构.md) |
| IAM 身份透传 | [02-IAM身份透传链路.md](02-IAM身份透传链路.md) |
| 服务间认证 | [03-服务间认证链路.md](03-服务间认证链路.md) |
| 访问上下文 | [04-访问上下文与权限快照.md](04-访问上下文与权限快照.md) |
| 安全边界 | [05-安全边界与降级.md](05-安全边界与降级.md) |

## 5. 为什么选择当前方案

认证和组织权限属于 IAM 体系，qs-server 只消费身份、权限和服务身份，避免把认证中心逻辑复制到测评服务内。

## 6. 代码事实源

- [../../../internal/pkg/iamauth](../../../internal/pkg/iamauth)
- [../../../internal/pkg/securityplane](../../../internal/pkg/securityplane)
- [../../../internal/pkg/securityprojection](../../../internal/pkg/securityprojection)
- [../../../internal/apiserver/application/authz](../../../internal/apiserver/application/authz)
- [../../../internal/apiserver/infra/iam](../../../internal/apiserver/infra/iam)
- [../../../internal/collection-server/infra/iam](../../../internal/collection-server/infra/iam)
