# repair_stranded_plan_tasks

该工具修复 Plan Task 的调度轮次、稳定 `due_at` 与错过开放窗口的 stranded `pending`。默认 `audit`；`apply`/`rollback` 必须显式 `--confirm`。工具不会发布历史 `task.expired` 通知事件。

本页只保留稳定工具契约。机构、cutoff、候选数、目标 URL、执行人和结果必须写入本次审批/evidence，不能复制旧生产参数。

## 1. 构建与安全输入

```bash
go build -o /tmp/repair_stranded_plan_tasks ./scripts/oneoff/repair_stranded_plan_tasks
export MYSQL_DSN='<secret from protected environment>'
export REPAIR_DIR="$(mktemp -d)"
export TARGET_ORG_ID='<approved org id>'
export CUTOFF_AT='<approved RFC3339 cutoff>'
```

DSN 必须启用 `parseTime=true`、`loc=Asia%2FShanghai`；工具为 MySQL session 固定 `time_zone=+08:00`。不得把 DSN、审计文件或生产候选清单提交仓库。

## 2. Audit

```bash
/tmp/repair_stranded_plan_tasks \
  --mode audit --org-id "$TARGET_ORG_ID" --cutoff-at "$CUTOFF_AT" \
  --checkpoint-file "$REPAIR_DIR/audit-checkpoint.json" \
  --audit-file "$REPAIR_DIR/plan-task-repair.jsonl.gz"
```

在申请 apply 前必须：

1. 冻结命令、source/deployed SHA、cutoff、候选清单 hash 与过期时间；
2. 人工处理全部 `schedule_inference_ambiguous`、`schedule_revision_invalid`、`dirty_active`、`inactive_plan_or_enrollment`、`invalid_terminal_enrollment`；
3. 核对候选数量与数据库只读抽样，不能沿用历史 expected count；
4. 完成目标范围数据库快照和恢复演练；
5. 确认 Statistics/nightly 和其它修复任务不会并发。

## 3. Apply 与 Verify

```bash
/tmp/repair_stranded_plan_tasks \
  --mode apply --confirm --operator "$USER" \
  --org-id "$TARGET_ORG_ID" --cutoff-at "$CUTOFF_AT" \
  --checkpoint-file "$REPAIR_DIR/apply-checkpoint.json" \
  --audit-file "$REPAIR_DIR/plan-task-repair.jsonl.gz"

/tmp/repair_stranded_plan_tasks \
  --mode verify --org-id "$TARGET_ORG_ID" --cutoff-at "$CUTOFF_AT" \
  --checkpoint-file "$REPAIR_DIR/verify-checkpoint.json" \
  --audit-file "$REPAIR_DIR/plan-task-repair.jsonl.gz"
```

Apply 顺序保持为 schedule revision/defined-at → stable due-at → stale pending terminal → terminal enrollment。每批使用 version CAS；影响行数必须与候选完全一致，否则回滚该批。审计文件落盘后才提交事务，提交后才推进 checkpoint。

Verify 只有在 `schedule_defined_at_null`、`schedule_revision_invalid`、`due_at_null`、`stale_pending`、`unclosed_terminal_enrollment` 和人工项全部为 0 时通过。具体结果写入 dated evidence，不回写本页。

## 4. Rollback 边界

```bash
/tmp/repair_stranded_plan_tasks \
  --mode rollback --confirm \
  --org-id "$TARGET_ORG_ID" --cutoff-at "$CUTOFF_AT" \
  --checkpoint-file "$REPAIR_DIR/apply-checkpoint.json" \
  --audit-file "$REPAIR_DIR/plan-task-repair.jsonl.gz"
```

Rollback 逐条要求当前 version 和修复后字段仍与审计一致；任何后续业务变化都 fail closed。若 Statistics 已生成不可变 `task_schedule_defined/task_schedule_terminal` Fact，工具会在写入前拒绝 rollback；此时只能恢复包含 Fact 的完整快照或用更高 revision 前向修正。

## 5. Statistics 重建

从目标机构 Task 的最早业务日期到最近完整上海自然日，按受控窗口依次执行 Statistics repair、validate，最后 publish。日期、窗口、base URL、token 与 org 必须由本次变更提供；不得使用旧 runbook 的固定值。

Validate 必须无 insert/conflict，publish 成功并切换 cache generation 后，才能另行评估 `missed_expiration_enabled`。工具执行不自动授权变更 scheduler/config。

## 6. 验证与证据

```bash
go test -count=1 ./scripts/oneoff/repair_stranded_plan_tasks
```

本地测试不证明真实数据安全。生产证据至少包含审批、exact SHA、effective config、audit/apply/verify manifest hash、备份/恢复、逐批结果、Statistics 重建、limitations 与失效期。
