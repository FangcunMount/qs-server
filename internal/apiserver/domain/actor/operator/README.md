# Operator 子域

## 定位

`Operator` 是 IAM User 在 QS 业务域中的员工投影，负责本地员工身份、机构归属、联系方式和激活状态。IAM 仍是角色分配、角色继承和权限授权的唯一事实源。

`staff.roles` 保存 IAM 直接角色投影；`staff.effective_roles` 保存包含继承关系的有效角色投影。两者只用于管理端展示、查询和审计，不参与请求授权判定。

## 授权边界

- 路由能力由请求期 IAM AuthZ v3 Snapshot 判断。
- 对象级授权由业务对象加载后的 IAM AuthZ v3 Check 判断。
- Operator 聚合不提供 `HasRole`、`CanEvaluate` 等角色名授权旁路。
- 员工角色编辑只调用 `ReplaceManagedAssignments` 原子替换 QS 可管理的直接角色。
- 停用员工只改变 QS 业务激活状态，不撤销 IAM Assignment；业务入口通过 active operator 门禁拒绝停用员工。

## 角色投影

```text
IAM Assignment / RoleInheritance
  -> GetAuthorizationSnapshot
  -> DirectRoles + EffectiveRoles + PolicyVersion
  -> staff 本地非权威投影
```

IAM 提交成功但快照尚未追上提交版本时，员工操作仍返回成功，并把 `authz_projection_pending` 标记为 `true`。投影通过以下路径收敛：

- 当前请求加载到新 Snapshot 时顺带持久化；
- 管理操作提交后主动等待并刷新；
- apiserver 每 10 分钟在分布式 lease 下扫描 pending 记录并重试。

IAM 不可用时保留旧投影和 pending 标记；授权链本身仍 fail-closed，不能回退到本地角色。

## 领域职责

- `Operator`：维护员工本地状态和 IAM 投影证据。
- `Validator`：校验机构、用户、资料字段和可提交的 QS 直接角色。
- `Factory`：按 IAM User 幂等获取或创建员工。
- `Editor` / `Lifecycler`：修改本地资料及激活状态。
- `Repository`：持久化聚合；`PendingProjectionRepository` 提供有界 pending 扫描。

角色能力、继承闭包和对象条件均不在本子域重复建模。
