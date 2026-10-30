# QS-03 答卷效果缺口只读核对

`answersheet_gap_audit` 只读扫描带 durable acceptance 标记的答卷。v2 核对同一 Mongo 主库快照中的原事务计划、精确 committed witness、原 wire 和冻结 Admission；v1 保留历史标准 Outbox 校验。未知版本和缺失/冲突凭证列为需人工核对，不作为空扫描略过。历史无标记答卷不在范围内；不发送消息、不修改消息状态、不创建测评、不保存扫描水位。

运行者须显式固定答卷 ID 区间和接单截止时间，并使用有界单次预算。数据库连接从环境变量读取，避免在命令行和审计记录中暴露密码：

生产手动工作流 `M6 QS-03 AnswerSheet Gap Read-only Audit` 复用此命令，核对精确健康 API 镜像及二进制摘要，在独立只读容器中运行；连接 JSON 经标准输入传递（`--connections-stdin`），MySQL 查询使用只读事务。工作流只在公开日志显示分类汇总，不显示原命令的逐项 ID／分页游标；范围不完整或有需关注分类时失败，不自动修复。直接运行本命令会输出逐项 ID，结果必须留在私有运维环境。

```sh
export MONGO_URI='从现有只读运维配置读取'
export MONGO_DB='从现有只读运维配置读取'
export MYSQL_DSN='从现有只读运维配置读取'

audit_binary=$(mktemp -t qs-gap-audit.XXXXXX)
trap 'rm -f "$audit_binary"' EXIT
go build -o "$audit_binary" ./scripts/oneoff/answersheet_gap_audit
"$audit_binary" \
  --after-id=0 --upper-id=90010077 \
  --accepted-before=2026-09-28T12:00:00+08:00 \
  --batch-size=100 --max-sheets=1000 --timeout=2m
```

示例 ID 和时间仅展示参数格式，不能当生产水位或宽限期使用。生产执行前须先从业务事实确定上界、宽限期和只读账号，并确认 `answersheets` 上已有键序为 `durable_acceptance.schema_version, deleted_at, domain_id` 的非稀疏、非隐藏、非部分索引 `idx_answersheet_durable_audit`；命令会在该索引缺失、键序不匹配或无法核验时停止，查询也显式指定该索引。一次输出包含固定区间、截止时间、每条原事件 ID、分类和 `next_after_id`。扫描达到 `max-sheets` 时 `complete=false`，应沿用原上界和截止时间续扫，不能把部分结果写成全量无缺口。

`--upper-id` 最大为 `9223372036854775807`：Mongo BSON 的整数边界是有符号 64 位，不能把 `uint64` 最大值作为“全量”上界。运行前还须固定切换水位；无 durable acceptance 标记的切换前答卷不应被当成当前事务链的投递缺口。

退出码：`0` 表示该固定区间完整且无需关注，`2` 表示完整但存在 `missing_committed`、`missing_confirmed`、`delivery_pending`、`unknown` 或 `manual_required`，`3` 表示达到预算但尚未扫完，`1` 表示参数、连接、扫描或输出失败。`missing_committed` 仅表示 v2 本地提交凭证成立但没有 Assessment，不证明 Broker 已确认；`missing_confirmed` 属于 v1 历史发布确认路径。两者均为人工核对候选，**不是自动重投授权**。

`assessment_present` 只证明测评行存在，不证明后续评估、报告或通知成功。软删除的 Assessment 仍代表曾经接单，分类为 `manual_required/assessment_previously_accepted`，不能当作可重新创建的缺口。Mongo 与 MySQL 不构成共同事务快照，任何跨库读错误都不能当作效果缺失；恢复前必须再次捕获，并由原 Intake 唯一键处理正常消费者竞态。

恢复须另用原事件和冻结 Admission，经有审计、授权和效果重查的[单份原答卷恢复工具](../answersheet_gap_recover/README.md)；不重置发布记录。本轮只在 CI 自建的隔离数据库验收，不连接或操作现库，不新增定时修复任务。
