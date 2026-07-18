# Security

## 1. 结论

安全链分为用户身份、服务身份、权限能力、租户/组织范围和资源归属。任何一层都不能替代其它层。

## 2. 链路

```text
credential verification
  -> principal / service identity
  -> tenant and organization context
  -> IAM capability decision
  -> actor access scope
  -> module resource ownership check
```

## 3. 事实源

IAM/platform 组合、`internal/pkg/iamauth`、transport middleware、actor/access 和各模块 application service。公开接口清单以 OpenAPI 的 security 声明和 handler 行为共同核对。

## 4. 降级原则

身份、权限或归属无法确认时默认拒绝；缓存或 IAM 依赖故障不得扩大访问范围。
