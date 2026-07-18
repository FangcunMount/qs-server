# apiserver REST

## 1. 事实源

[`api/rest/apiserver.yaml`](../../api/rest/apiserver.yaml) 是当前对外 REST 契约。Swagger 注解和 handler 是生成来源；本文不复制端点清单。

## 2. 服务责任

apiserver REST 面向后台管理、业务查询、公开入口和治理能力。路由进入 transport 后应调用 application service，不直接操作 repository。

## 3. 鉴权核对

检查 OpenAPI `security`、middleware、IAM capability、actor access scope 和业务资源归属。某个端点在 OpenAPI 标记 public，只表示无需用户 Bearer token，不代表其 token/签名/租户规则可以省略。

## 4. 验证

```bash
make docs-rest
make docs-verify
```

再运行受影响 handler/application 定向测试。
