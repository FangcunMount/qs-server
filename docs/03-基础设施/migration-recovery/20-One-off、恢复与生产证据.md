# One-off、恢复与生产证据

## 1. 当前结论

one-off 不是“运行一次的普通脚本”，而是独立变更程序。每个工具都必须声明权威范围、读写集合、幂等键、dry-run、apply-time recheck、备份/恢复、退出码和生产证据。缺少任一关键条件时，应降级为只读审计或 blocked。

本页是 `one-off/recovery/production evidence` 主题的 canonical 文档。

## 2. 安全分级

| 级别 | 含义 | 最低要求 |
| --- | --- | --- |
| audit-only | 只读扫描，不改变业务事实 | 固定上界、稳定 cursor、结果清单、非零 finding 退出语义 |
| dry-run capable | 能计算候选变更但默认不写 | 权威范围、fingerprint、过期时间、apply 前重查 |
| controlled write | 经授权写入或修复 | exact manifest、批次幂等、备份、逐批复验、停止条件 |
| destructive | 删除/覆盖不可轻易恢复的数据 | 独立审批、目标清单、业务/合规影响、可验证恢复、双人复核 |
| blocked | 当前语义无法证明安全 | 禁止 apply；只能补分析、测试和保护条件 |

`cleanup_orphaned_assessment_documents` 当前必须是 blocked：AnswerSheet 与 `answersheet.submitted` Outbox 在同一 Mongo 事务持久化，Assessment 由 worker 异步确保；“暂时没有 MySQL Assessment”不能证明 AnswerSheet 是孤儿。该工具还未把 durable Outbox 纳入候选保护/备份，apply 会制造新的反向不一致。

这里的 blocked 目前是文档、变更流程和运维授权层面的阻断，不是二进制 fail-closed：源码仍解析并执行 `--apply`、`--hard-delete` 与 `--skip-backup`。在代码级拒绝开关或移除可执行分发完成前，必须继续限制工具制品/生产凭证访问；任何人都不能把本页或门禁通过解读为“写删路径已在技术上禁用”。

## 3. blocked 工具的退出条件

至少全部满足后，才允许另起变更重新评估：

1. 使用权威业务状态定义目标，而不是仅用跨库暂时缺失；
2. 同时核对 AnswerSheet、durable Outbox、消息 retry/dead-letter、Assessment terminal 状态；
3. 冻结 exact 候选清单和 fingerprint，并设置有效期；
4. apply 时逐条重查，任何状态变化立即跳过；
5. 备份所有将修改的文档及关联 Outbox，完成恢复演练；
6. 设定批量、速率、停止阈值、写后双向一致性复验；
7. 获得独立生产授权。文档合并和测试通过不构成授权。

## 4. 恢复流程

```text
发现异常
  -> 先阻断继续写入/自动调度（按故障域）
  -> 保存 exact SHA、effective config、时间窗和原始证据
  -> 只读盘点，冻结候选与原因
  -> 选择 replay / rebuild / repair / restore
  -> 在隔离环境演练
  -> 经授权小批执行并逐批复验
  -> 完整复扫 + 业务链路验证
  -> 写入证据台账并设置失效期
```

Statistics coordinator 等长任务若留下 stale `running` ledger，不能仅靠“不要启动新批次”作为恢复方案；恢复必须定义 stale 判定、owner、重入/标失败规则和缓存/投影复验。请求 context 已取消时，失败结算还需要独立可用的清理 context，这一代码缺口在修复前保持 gap。

## 5. 生产证据最小字段

每次真实 one-off 或恢复记录：

- 工具名、source/deployed SHA、二进制/镜像 digest；
- observed_at、environment、owner、审批引用；
- 脱敏 effective-config hash、命令和参数 fingerprint；
- 只读候选清单 hash、扫描上界、读写集合；
- dry-run/apply/复验结果和退出码；
- backup/restore 证据；
- limitations、expires_on、supersedes。

数量、耗时、主机数和 finding 属于这一时点记录，不进入 `scripts/oneoff/README.md` 或 migration sidecar。

## 6. 验证入口与未闭合 gap

```bash
go test -count=1 ./scripts/oneoff/...
go test -count=1 ./internal/pkg/migration
```

这些测试只覆盖工具逻辑和静态契约。当前仍需真实环境验证：

- 目标库只读 dry-run 与候选人工抽样；
- 备份可恢复性和 apply-time 竞争窗口；
- MQ replay/dead-letter 与跨库异步延迟；
- stale Statistics run 恢复；
- blocked AnswerSheet cleanup 的重新设计与独立授权。
