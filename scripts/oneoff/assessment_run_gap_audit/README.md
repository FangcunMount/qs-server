# Assessment → Evaluation Run 只读缺口审计

用于 M6-06 的 QS-04 生产核对。命令只读取 `assessment` 与 `runtime_checkpoint`，在 MySQL 只读事务中按固定 Assessment ID 区间扫描，最多 1,000 行，不发布消息、不重置 Outbox、不执行模型。

输入必须指定 `--after-id`（不含）、`--upper-id`（含）和 `--submitted-before`（RFC3339、至少早于当前 10 分钟）。Assessment 的 `submitted_at` 是 UTC+8 墙上时间；命令把截止时间转换为 UTC+8 后再交给 SQL 比较。`--connections-stdin` 接收私有 JSON `{"mysql_dsn":"..."}`；不应把 DSN 放在公开命令行或日志中。默认仅输出截止时间和分类计数；受控私有运行才加 `--include-ids` 获取候选 ID，所有 ID 在 JSON 中以十进制字符串表示，避免 JavaScript 数值精度丢失。

分类中的 `candidate_never_claimed` 仅表示：仍为 `submitted`、有冻结模型、超过宽限期且没有有效 Run。它**不是** Broker 丢失的定论或重发授权；须逐 ID 核对原 Outbox、现役消费者、业务关联和冻结配置。`run_present` 表示已有 Run，绝不可走“从未接单”修复；执行中和模型结果未知必须交原租约／人工治理。`manual_required` 为状态或提交时间异常。退出码 0 为完整且无候选／异常，2 为完整但有候选／异常，3 为超过行数预算，1 为输入／连接／读取错误。

区间应从标准 Profile 切换后的已核定下界开始。输出的 `next_after_id` 只用于**同一固定快照的分页**；新提交可能发生在已扫 ID 中，所以持续巡检必须重扫最近时间窗口，不能无限向前推进单个 ID 游标并宣称无缺口。每次扫描固定上界与宽限期，完成前不得以部分页计为通过。此命令不提供自动修复或生产调度。
