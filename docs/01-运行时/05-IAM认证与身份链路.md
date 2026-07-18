# IAM 认证与身份链路

## 1. 结论

IAM 提供 token 校验、权限快照、能力判断和服务间认证。业务模块仍负责自身资源归属与用例授权，不能把 IAM 当成业务数据过滤器。

## 2. 链路

```text
credential
  -> middleware / verifier
  -> identity and tenant context
  -> application access service
  -> domain/resource ownership checks
```

## 3. 证据

组合入口在 `internal/apiserver/container/modules/iam` 和 platform wiring；共享认证能力在 `internal/pkg/iamauth`；具体资源访问范围在 actor/access 以及各模块 application service。
