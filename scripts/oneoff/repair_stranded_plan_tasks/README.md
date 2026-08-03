# repair_stranded_plan_tasks

安全修复 Plan Task 的稳定 `due_at` 与错过开放窗口的历史 `pending`。默认 `audit`；
`apply`/`rollback` 必须显式 `--confirm`。工具不会发布 `task.expired` 事件，因此不会触发历史通知。

## 构建

```bash
go build -o /tmp/repair_stranded_plan_tasks ./scripts/oneoff/repair_stranded_plan_tasks
```

DSN 必须通过环境变量注入，并启用 `parseTime=true` 和
`loc=Asia%2FShanghai`；工具会为每个 MySQL session 固定 `time_zone=+08:00`。不要把 DSN 写进命令历史：

```bash
export MYSQL_DSN='user:password@tcp(host:3306)/qs?parseTime=true&loc=Asia%2FShanghai'
export REPAIR_DIR="$(mktemp -d)"
```

## 生产步骤（机构 1）

固定 cutoff 后先只读审计：

```bash
/tmp/repair_stranded_plan_tasks \
  --mode audit --org-id 1 --cutoff-at '2026-08-03T00:00:00+08:00' \
  --checkpoint-file "$REPAIR_DIR/checkpoint.json" \
  --audit-file "$REPAIR_DIR/plan-task-repair.jsonl.gz"
```

首次审计要求 `eligible_missed=108574`，并且 `dirty_active=0`、
`inactive_plan_or_enrollment=0`、`invalid_terminal_enrollment=0`。完成数据库快照备份后，
使用新的 checkpoint 路径执行：

```bash
/tmp/repair_stranded_plan_tasks \
  --mode apply --confirm --operator "$USER" --org-id 1 \
  --cutoff-at '2026-08-03T00:00:00+08:00' \
  --checkpoint-file "$REPAIR_DIR/apply-checkpoint.json" \
  --audit-file "$REPAIR_DIR/plan-task-repair.jsonl.gz"
```

同一命令可在中断后续跑。Apply 先为全机构 Task 回填 `planned_at+7` 个上海自然日，
再用 version CAS 将干净、active/active 的 stale pending 置为
`expired/missed_open_window`，最后关闭全部 Task 已终态的 Enrollment。

验证使用新的 checkpoint；成功条件为 `due_at_null=0`、`stale_pending=0`，且无人工项：

```bash
/tmp/repair_stranded_plan_tasks \
  --mode verify --org-id 1 --cutoff-at '2026-08-03T00:00:00+08:00' \
  --checkpoint-file "$REPAIR_DIR/verify-checkpoint.json" \
  --audit-file "$REPAIR_DIR/plan-task-repair.jsonl.gz"
```

## 回滚

回滚使用 apply 的 checkpoint（其中保存 run_id）和同一审计文件。每条记录都要求当前
version 和全部修复后字段仍与审计一致；检测到任何后续业务变化立即停止：

```bash
/tmp/repair_stranded_plan_tasks \
  --mode rollback --confirm --org-id 1 \
  --cutoff-at '2026-08-03T00:00:00+08:00' \
  --checkpoint-file "$REPAIR_DIR/apply-checkpoint.json" \
  --audit-file "$REPAIR_DIR/plan-task-repair.jsonl.gz"
```

源数据回滚后必须重新执行 Statistics repair、validate、publish。审计文件和数据库快照须按
变更单保存；不要在修复后、Statistics 重建前开启 `missed_expiration_enabled`。

## Statistics 重建顺序

Apply/verify 完成后，从本次机构 Task 的最早业务创建日开始修复；`TO_DATE` 固定为最近一个
完整上海自然日。先在数据库确认日期边界：

```sql
SET time_zone = '+08:00';
SELECT DATE(MIN(COALESCE(business_created_at, created_at))) AS from_date
FROM assessment_task
WHERE org_id = 1 AND deleted_at IS NULL;
```

避开 nightly publish，以 7 天窗口依次执行 repair、validate，最后只 publish 最近完整日：

```bash
export FROM_DATE='2025-01-01' # 替换为上面查询结果
export TO_DATE='2026-08-02'   # 替换为执行时最近完整上海自然日

QS_STATISTICS_TOKEN='***' go run ./scripts/oneoff/rebuild_statistics \
  --base-url http://127.0.0.1:8081 --org-ids 1 \
  --from "$FROM_DATE" --to "$TO_DATE" --window-days 7 \
  --reason plan_task_due_repair --mode repair --confirm

QS_STATISTICS_TOKEN='***' go run ./scripts/oneoff/rebuild_statistics \
  --base-url http://127.0.0.1:8081 --org-ids 1 \
  --from "$FROM_DATE" --to "$TO_DATE" --window-days 7 \
  --reason plan_task_due_validate --validate-only

QS_STATISTICS_TOKEN='***' go run ./scripts/oneoff/rebuild_statistics \
  --base-url http://127.0.0.1:8081 --org-ids 1 \
  --from "$TO_DATE" --to "$TO_DATE" \
  --reason plan_task_due_publish --mode publish --confirm
```

Validate 必须全部返回 `inserted=0`、`conflict=0`；publish 成功并切换缓存 Generation 后，
才能把生产 `missed_expiration_enabled` 改为 `true`。
