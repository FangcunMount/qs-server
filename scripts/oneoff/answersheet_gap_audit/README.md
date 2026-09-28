# QS-03 答卷效果缺口只读核对

`answersheet_gap_audit` 只读扫描带 `durable_acceptance.schema_version=1` 的新答卷，核对原标准 Mongo Outbox 与 MySQL Assessment。它不会发送消息、修改 Outbox、创建测评或保存扫描水位；历史无标记答卷不在范围内。

运行者须显式固定答卷 ID 区间和接单截止时间，并使用有界单次预算。数据库连接从环境变量读取，避免在命令行和审计记录中暴露密码：

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

示例 ID 和时间仅展示参数格式，不能当生产水位或宽限期使用。生产执行前须先从业务事实确定上界、宽限期和只读账号，并确认 `answersheets` 上已有键序为 `durable_acceptance.schema_version, deleted_at, domain_id` 的 `idx_answersheet_durable_audit`；命令会在该索引缺失、键序不匹配或无法核验时停止。一次输出包含固定区间、截止时间、每条原事件 ID、分类和 `next_after_id`。扫描达到 `max-sheets` 时 `complete=false`，应沿用原上界和截止时间续扫，不能把部分结果写成全量无缺口。

退出码：`0` 表示该固定区间完整且无需关注，`2` 表示完整但存在 `missing_confirmed`、`delivery_pending`、`unknown` 或 `manual_required`，`3` 表示达到预算但尚未扫完，`1` 表示参数、连接、扫描或输出失败。`assessment_present` 只证明测评行存在，不证明后续评估、报告或通知成功；`missing_confirmed` 是人工核对候选，**不是自动重投授权**。任何跨库读错误都不能当作效果缺失。

恢复须另用原事件和冻结 Admission，经有审计、授权和效果重查的宿主入口；不得重置全部 `published` 行。当前代码尚未提供该生产写入入口或定时调度。
