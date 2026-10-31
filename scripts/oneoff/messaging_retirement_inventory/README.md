# 宿主工程退役：只读责任核对

该工具仅核对固定 `iam.mysql` / `qs.mysql` 原发布表的存在性、结构哈希、实际 schema head 和原状态字节分组。版本头缺失、多行或 dirty 都会使核对失败；记录实际 head 不表示该版本已获退役批准。使用目标进程已有的明确数据库与 VERIFY_IDENTITY TLS 绑定，在有界、REPEATABLE READ / READ ONLY 事务内读取；不安装 schema、不启动 Relay、不回放或修改业务状态。

`publication_closed_rows` 只表示状态恰为 `published` / `confirmed`，不是业务效果完成、ACK、历史引用关闭或删表许可。NULL、大小写不同、尾随空格和未知状态都属于未关闭。缺表、缺状态字段、超时或身份不可读会失败，行数保持 null，不能据此推断零积压。结构哈希也不等于已验证历史正文及全部业务约束。

MySQL 伴随工具只有确认事务回滚和自有连接池关闭后才可报告完整；释放未知即失败。运输入口还要求进程成功退出，不能保留失败进程返回的“完整”声明。AI 审计也先关闭连接池，再输出最终结果。

`release-snapshot.yml` 的 `messaging_retirement_inventory=true` 入口要求完整 `reviewed_source` 等于工作流 SHA。该模式排除默认主机快照，不执行其 root Python 命令；仅复用现有 SSH/Docker 运输，在应用 UID 下运行临时只读伴随工具。AI 分支固定应用 UID `10001:10001` 和镜像启动解释器 `/app/.venv/bin/python`，通过已安装应用的 `Settings()` 读取原数据库及 TLS 绑定，复用 `messaging_audit.inventory`，保存原文哈希与元数据，完整性/截断结果原样保留。失败回执只记录阶段、异常类型和整数驱动码，不记录异常正文、连接串或凭据；无有效回执时，读写效果保持未知，不能推断空表。它不是原正文导出、备份或恢复证明。Mongo 历史读取仍为明确源码依赖；新增集合核对见下文，不能由 SQL 结果替代。

M4 实际部署和回读完成前不执行现网核对。即使报告完整且所有发布状态关闭，`drop_authorized`、`business_responsibility_closed`、`original_body_export_verified` 和 `writers_excluded` 始终为 false。实际删除仍需逐对象的引用与业务责任证明、导出/恢复验证、全新库和升级库测试、回退下限，以及用户审查具体执行方案。

普通单元测试覆盖缺表、结构/字段读取失败和超时。既有 MySQL Native Metadata and Readiness CI 另在独立拥有的本地 MySQL 8.0.36 namespace 验证原状态字节、NULL、未知值与只读事务拒绝写入；不借用生产绑定。

## Mongo 固定集合核对

同一运输支持 `qs.mongo`，固定读取应用显式绑定的 `qs.rm_outbox`，以已校验的独立伴随二进制调用现有元数据工具的 `--messaging-retirement-inventory` 入口。旧元数据入口及其固定历史 head 不变。

计数只读取状态字段，采用单次 `readConcern=snapshot` 聚合与二进制字符串比较；只有精确字符串 `published` 归为发布状态关闭，NULL、缺字段、数组、大小写及尾随空格均保留为其他状态。查询前后复核 schema head/dirty 与集合 UUID/目录哈希；缺集合或身份变化不是零行。该过程不声明多次读取处于同一事务、不证明停写或业务责任关闭，不导出消息正文。

复用既有十条命令预算、响应上限及游标释放校验，另限总读取 25 秒、聚合 2 秒；结束时关闭自有连接池，释放不确定即失败。原生 CI 必须在独占、认证 Mongo 7.0.31 副本集执行旧元数据验收及新增只读账户/原状态类型计数验收，两者均 RUN/PASS、零跳过。源码兼容与原生测试通过不等于已读取生产集合。
