# 门店运营身份准备

`operator-prepare preflight|apply|status` 为已审核的 IAM 用户创建停用、无角色的 QS Operator。它调用 Actor 生命周期应用服务，不创建 IAM 用户，不赋权，不启用，不恢复已退役身份。

输入由 `OPERATOR_PREPARE_INPUT` 指向权限为 `0600` 的 JSON 文件，字段为 `OrgID`、`UserID`、`ActorID`、`Name`、`RequestID`、`Reason`。不接受密码、角色或 Scope 字段。使用正常 QS 配置参数连接服务。

每次执行都必须指定新的 `OPERATOR_PREPARE_REPORT` 路径。报告以 `0600` 保存；日志只输出摘要。先运行 `preflight`，审核人员、公司及报告，再将报告的 `fingerprint` 作为 `OPERATOR_PREPARE_FINGERPRINT` 运行 `apply`。命令持有目标用户的共享人员变更锁并重新核实事实。它不会暂停其他服务或进行跨 IAM/QS 事务。

状态：

- `pending`：尚无 Operator，目标 IAM 身份有效且没有授权，可以审核后创建。
- `created_inactive`：本次已创建停用 Operator，仍不可作为可用门店账号交付。
- `existing_inactive`：当前已有同公司、同名、同审核操作人创建的停用无角色身份；不重复写入。这是当前状态观察，不证明归属于当前请求，也不是历史迁移完成凭证。
- 出错：检查受限报告；不得通过删除现有身份、改变公司或直接更新状态来绕过冲突。

`status` 同样核对当前身份、人员和授权事实；人员被启用、授权或退役后将报告冲突，不把准备记录当作自动覆盖依据。准备成功后，启用和按门店配置角色必须在后续 Scope 发布切换中执行，并独立验证。
