# Actor

Actor 管参与者、关系、测评入口和访问上下文；它不拥有问卷、测评执行或报告状态机。

## 1. 领域模型

当前领域子包位于 `internal/apiserver/domain/actor`：

| 子域 | 核心事实 |
| --- | --- |
| `testee` | 受试者档案 |
| `clinician` | 医生身份 |
| `operator` | 后台操作员与角色投影 |
| `relation` | Clinician-Testee 关系与分配策略 |
| `assessmententry` | 对外测评入口与摄入上下文 |

跨模块引用使用 `TesteeRef` 等轻量标识，不共享 Actor 聚合的可变状态。

## 2. 应用服务

`internal/apiserver/application/actor` 按 `testee`、`clinician`、`operator`、`assessmententry`、`access`、`actorctx` 分用例组织。重点区分：

- 生命周期/管理服务：修改 Actor 自身事实；
- 查询服务：读取投影；
- access/actorctx：为其它模块解析访问范围与参与者上下文；
- assessment entry intake：把公开 token 入口转换为受控业务上下文。

## 3. 关键路径

```text
公开 AssessmentEntry token
  -> 解析入口与组织/医生上下文
  -> 确认或创建 Testee
  -> 生成 intake 结果
  -> 交给后续 Survey/Evaluation journey
```

医生分配受试者时，由 relation 的领域策略校验关系，再由 application service 编排 repository 与权限投影。

## 4. 权限边界

IAM 证明“调用者是谁、具有什么能力”；Actor access service 解析“可以访问哪些受试者”；具体业务模块仍需校验自身资源和组织归属。

## 5. 证据与验证

- domain：`internal/apiserver/domain/actor`。
- application：`internal/apiserver/application/actor`。
- 装配：`internal/apiserver/container/modules/actor`。
- transport：Actor REST/gRPC exports 与 handler/service。
- 验证：actor domain/application/container 定向测试和相关访问控制测试。

状态：`已实现`（本轮核对到类型、服务分区和装配入口；更细状态机文档列为待补证据）。
