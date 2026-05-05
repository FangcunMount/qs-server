# ClientBundle 与外部客户端注入

**本文回答**：runtime composition 中外部客户端、SDK helper、service auth、WeChat、OSS、IAM、gRPC client 等如何被创建和注入；为什么 application 不能直接创建外部客户端；ClientBundle 概念应该如何在文档中使用。

---

## 30 秒结论

| 客户端/外部依赖 | 当前装配位置 |
| --------------- | ------------ |
| IAM Client / IAMModule | Container Stage，通过 `NewIAMModuleWithRuntimeOptions` |
| IAM Backpressure | Resource Stage 构建，Container Stage 注入 IAMModule runtime options |
| IAM AuthzSnapshotLoader | IAMModule 暴露，transport/runtime/integration 使用 |
| AuthzVersion Subscriber | Integration Stage 创建并启动 |
| WeChat QR / Subscribe | Integration Stage 调 Container Init 方法 |
| OSS PublicObjectStore | WeChat/QR 集成初始化中使用 |
| gRPC Server AuthzSnapshot | Transport Stage 从 Container 构建 GRPC bootstrap deps |
| REST / gRPC Deps | Container 构建 BuildRESTDeps / BuildGRPCDeps |
| ServiceAuth | 各 infra IAM helper 中创建，用于 PerRPC metadata |

一句话概括：

> **ClientBundle 不是一个必须存在的万能 struct，而是文档上的概念：所有外部 runtime client 必须在组合根中创建，并以窄接口注入业务或 transport。**

---

## 1. 为什么需要 ClientBundle 视角

外部客户端包括：

- IAM SDK client。
- TokenVerifier。
- AuthzSnapshotLoader。
- Messaging subscriber/publisher。
- WeChat SDK adapter。
- OSS client。
- gRPC service auth credentials。
- Redis-backed SDK cache。

这些不应由 handler/application 随手创建。原因：

- 生命周期不可控。
- 配置散落。
- 测试困难。
- credential 泄漏。
- backpressure 无法注入。
- shutdown 无法统一。

ClientBundle 视角要求：

```text
创建在 runtime composition；
以 port/interface 注入；
由 lifecycle 管理关闭。
```

---

## 2. IAM 客户端注入

Container Stage 中：

```text
NewIAMModuleWithRuntimeOptions(ctx, config.IAMOptions, IAMModuleRuntimeOptions{
  Limiter: resources.containerInput.containerOptions.Backpressure.IAM,
})
```

说明：

- IAMModule 在 container 初始化前创建。
- IAM limiter 来自 Resource Stage。
- IAMModule 后续提供 TokenVerifier、AuthzSnapshotLoader、WeChat app config、recipient resolver 等能力。

---

## 3. AuthzVersion Subscriber

Integration Stage 中：

1. 获取 `c.IAMModule.AuthzSnapshotLoader()`。
2. 读取 `IAMOptions.AuthzSync`。
3. 如果 enabled：
   - 创建 subscriber。
   - 生成 channel。
   - 调 `SubscribeVersionChanges`。
4. 失败时 warning，并关闭 subscriber。

该 subscriber 在 shutdown 时 Stop + Close。

---

## 4. WeChat / OSS 客户端注入

Integration Stage 调：

```text
c.InitQRCodeService(WeChatOptions, OSSOptions)
c.InitMiniProgramTaskNotificationService(WeChatOptions)
```

这表示：

- WeChat/OSS 属于外部集成初始化。
- 它们依赖 Container 中已有的 IAM、cache、object storage 等能力。
- 不是 Resource Stage 的基础资源。
- 不是业务模块初始化的一部分。

---

## 5. Transport Deps

Transport Stage 不直接访问 Container 内部字段，而是调用：

```text
container.BuildRESTDeps(rateLimitOptions)
container.BuildGRPCDeps(grpcServer)
container.BuildServerGRPCBootstrapDeps()
```

这样 REST/gRPC 注册只消费明确 deps，不负责创建业务对象。

---

## 6. ServiceAuth

ServiceAuthHelper 实现 gRPC PerRPCCredentials：

```text
GetRequestMetadata -> authorization: Bearer token
```

它应由 infra wrapper 创建，并暴露窄能力：

- GetToken。
- GetRequestMetadata。
- ServiceIdentity。
- Stop。

不要在调用点手写 metadata。

---

## 7. 生命周期要求

任何新增 client/helper 必须说明：

| 问题 | 要求 |
| ---- | ---- |
| 谁创建 | Resource/Container/Integration/Transport 哪个 stage |
| 谁持有 | Container / IAMModule / local stage output |
| 谁使用 | port / application / transport |
| 谁关闭 | lifecycle shutdown deps |
| 是否可选 | nil 时如何降级 |
| 是否有 backpressure | 是否注入 limiter |
| 是否有 credential | 禁止泄漏 |

---

## 8. 常见误区

### 8.1 “业务用到外部 API 就在 service 里 new client”

不应。client 属于 runtime dependency。

### 8.2 “ClientBundle 必须是一个真实结构体”

不一定。这里是设计视角；代码可以用多个 deps struct 承载。

### 8.3 “integration stage 只做第三方 SDK”

不只。它还包括 authz version sync 这类跨系统订阅。

### 8.4 “Transport 可以顺便创建 use case”

不应。Transport 只注册 route/service。

---

## 9. 修改指南

新增外部 client：

1. 明确是否基础资源、业务模块依赖、integration、transport helper。
2. 定义 port/interface。
3. 在正确 stage 创建。
4. 注入 ContainerOptions 或 Container 字段。
5. 明确 nil/degraded 行为。
6. 明确 Stop/Close。
7. 补 tests/docs。

---

## 10. Verify

```bash
go test ./internal/apiserver/process
go test ./internal/apiserver/container
go test ./internal/apiserver/infra/iam
go test ./internal/apiserver/infra/wechatapi
go test ./internal/apiserver/infra/objectstorage/...
```
