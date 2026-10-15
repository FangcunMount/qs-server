# Operator 子域

## 定位

`Operator` 是 IAM User 在 QS 业务域中的员工投影，负责本地员工身份、机构归属、联系方式和激活状态。IAM 仍是角色分配与权限授权的唯一事实源。

`operators.roles` 保存 IAM 直接角色投影，只用于管理端展示、查询和审计，不参与请求授权判定。SQL102 退出重复持久化列 `effective_roles`；现用响应的 `effective_roles` 仍从 `roles` 派生，IAM 投影接口的角色校验保持原语义。

## 授权边界

- 路由能力由请求期 IAM AuthZ Snapshot 判断。
- 对象级授权由业务对象加载后的 IAM AuthZ Check 判断。
- Operator 聚合不提供 `HasRole`、`CanEvaluate` 等角色名授权旁路。
- 员工角色编辑只调用 `ReplaceManagedAssignments` 原子替换 QS 可管理的直接角色。
- 停用员工只改变 QS 业务激活状态，不撤销 IAM Assignment；业务入口通过 active operator 门禁拒绝停用员工。

## 角色投影

```text
当前公司 Scoped Assignment
  -> LoadOperatorRoleProjection
  -> DirectRoles (+ EffectiveRoles 同值) + PolicyVersion
  -> operators.roles 本地非权威投影
  -> 响应 effective_roles 从 roles 派生
```

IAM 提交成功但快照尚未追上提交版本时，员工操作仍返回成功，并把 `authz_projection_pending` 标记为 `true`。投影通过以下路径收敛：

- 当前请求加载到新 Snapshot 时，以策略版本为提示重新读取当前公司的角色投影并持久化；
- 管理操作提交后主动等待并刷新；
- apiserver 每 10 分钟在分布式 lease 下扫描 pending 记录并重试。

IAM 不可用时保留旧投影和 pending 标记；授权链本身仍 fail-closed，不能回退到本地角色。

## 领域职责

- `Operator`：维护员工本地状态和 IAM 投影证据。
- `Validator`：校验机构、用户、资料字段和可提交的 QS 直接角色。
- `Factory`：按 IAM User 幂等获取或创建员工。
- `Editor` / `Lifecycler`：修改本地资料及激活状态。
- `Repository`：持久化聚合；`PendingProjectionRepository` 提供有界 pending 扫描。

角色能力与对象条件均不在本子域重复建模。当前可分配角色：`qs:admin`、`qs:content_manager`、`qs:assessment_operator`、`qs:evaluation_plan_manager`、`qs:result_reviewer`。

## M4-C 发布边界

本候选必须在已验收的 clean101/40 上，由独立离线执行器完成 SQL102，再启动新业务 runtime。旧 `effective_roles` 为无默认值的 NOT NULL 列，省略该列的新写入不能在旧 schema 上发布；旧镜像也不能接管102/40库。

现代 prepare、selected/orphan exit 和 recovery 预检仅接受单行 cleanSQL102，使用固定现行字段和新摘要格式。旧未执行指纹必须重新预检；已有完成责任的原始 JSON 与 checksum 不重写，恢复只重新开放本地身份，不恢复角色、照护关系或二维码。后续 SQL103 等发布必须在其任务中重新审核并更新该工具的确切 schema 门禁，不能临时放行任意未来版本。

本候选源码和普通测试不证明生产两列值分布、具名值备份、完整停写、失败恢复、业务启动或30/60分钟/RPO0；这些仍由现有模块问题清单和版本验收台账记录。
