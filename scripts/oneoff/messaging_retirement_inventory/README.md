# 宿主工程退役：只读责任核对

该工具仅核对固定 `iam.mysql` / `qs.mysql` 原发布表的存在性、结构哈希和原状态字节分组。使用目标进程已有的明确数据库与 VERIFY_IDENTITY TLS 绑定，在有界、REPEATABLE READ / READ ONLY 事务内读取；不安装 schema、不启动 Relay、不回放或修改业务状态。

`publication_closed_rows` 只表示状态恰为 `published` / `confirmed`，不是业务效果完成、ACK、历史引用关闭或删表许可。NULL、大小写不同、尾随空格和未知状态都属于未关闭。缺表、缺状态字段、超时或身份不可读会失败，行数保持 null，不能据此推断零积压。结构哈希也不等于已验证历史正文及全部业务约束。

MySQL 伴随工具只有确认事务回滚和自有连接池关闭后才可报告完整；释放未知即失败。运输入口还要求进程成功退出，不能保留失败进程返回的“完整”声明。AI 审计也先关闭连接池，再输出最终结果。

`release-snapshot.yml` 的 `messaging_retirement_inventory=true` 入口要求完整 `reviewed_source` 等于工作流 SHA。该模式排除默认主机快照，不执行其 root Python 命令；仅复用现有 SSH/Docker 运输，在应用 UID 下运行临时只读伴随工具。AI 分支复用已安装的 `messaging_audit.inventory`，保存原文哈希与元数据，完整性/截断结果原样保留。它不是原正文导出、备份或恢复证明。Mongo 历史读取仍为明确源码依赖，不在此 MySQL 核对范围内。

M4 实际部署和回读完成前不执行现网核对。即使报告完整且所有发布状态关闭，`drop_authorized`、`business_responsibility_closed`、`original_body_export_verified` 和 `writers_excluded` 始终为 false。实际删除仍需逐对象的引用与业务责任证明、导出/恢复验证、全新库和升级库测试、回退下限，以及用户审查具体执行方案。

普通单元测试覆盖缺表、结构/字段读取失败和超时。既有 MySQL Native Metadata and Readiness CI 另在独立拥有的本地 MySQL 8.0.36 namespace 验证原状态字节、NULL、未知值与只读事务拒绝写入；不借用生产绑定。
