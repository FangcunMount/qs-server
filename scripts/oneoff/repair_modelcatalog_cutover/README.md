# repair_modelcatalog_cutover

G5 维护窗口专用的 current-only ModelCatalog 修复工具。历史 Assessment、AnswerSheet、Outcome、Report、任务和统计事实已清零后，它从仍在使用的 published head 重新物化 active snapshot，并在一个 MongoDB 事务内：

- 删除 retained archived Model snapshot；
- 按当前 family Handler 重新生成 `AlgorithmFamily`、`DecisionKind` 和冻结 DefinitionV2 layers；
- 使用 `CanonicalContentHash` 写入 DefinitionV2 hash；
- 清除 `payload`、`definition_payload`、`is_active_published` 等退役字段；
- 保持 Model code/version、Questionnaire 精确绑定、发布时间和 head revision 不变。

该工具不会猜测或改写 Norm，也不会修改 Questionnaire。任一 Norm、DefinitionV2、head/snapshot、Questionnaire 绑定、索引、migration 或历史清零证据不满足条件时，整个 apply 都会 fail closed。

## 前置条件

1. 所有 qs-server API、Collection/Worker、Outbox Relay、Evaluation、Interpretation 和 Statistics 写流量已停止。
2. MySQL 与 MongoDB 已成对备份并完成恢复预检。
3. 历史测评事实已经清零；`assessment_plan` 可以保留，但 `assessment_task` 与 `plan_enrollment` 必须为空。
4. MongoDB 必须是可写 Replica Set primary。
5. `MYSQL_DSN` 与 `MONGO_URI` 通过环境变量提供，不得写入命令历史。

## Dry-run

默认只读。它会打印每个 active snapshot 的 canonical identity/hash，以及精确的阻断原因：

```bash
go build -o /tmp/repair-modelcatalog-cutover \
  ./scripts/oneoff/repair_modelcatalog_cutover/

set -o pipefail

/tmp/repair-modelcatalog-cutover \
  --mongo-db qs \
  --timeout 10m \
  | tee /tmp/modelcatalog-repair-dry-run.txt

QS_REPAIR_DRY_RUN_RC=$?
echo "退出码=$QS_REPAIR_DRY_RUN_RC"
```

退出码为 `2` 时不得加 `--apply`。先处理列出的 Norm、绑定或 Definition 问题，再重新 dry-run。

## Apply

只有 dry-run 退出 `0` 且输出 `PASS: repair plan is complete and apply-safe` 时才允许执行：

```bash
set -o pipefail

/tmp/repair-modelcatalog-cutover \
  --mongo-db qs \
  --timeout 10m \
  --apply \
  | tee /tmp/modelcatalog-repair-apply.txt

QS_REPAIR_RC=$?
echo "退出码=$QS_REPAIR_RC"
```

成功必须同时满足：

- 退出码为 `0`；
- 输出 `MODELCATALOG_CUTOVER_REPAIR_OK`；
- post-apply 重新盘点无阻断；
- archived snapshot 与 legacy model document 均为 0；
- active snapshot 数量、code/version、Questionnaire 绑定保持不变。

随后重新运行 `verify_definition_v2_cutover`。未达到 audit 退出 `0`、五类 smoke 未通过前，不得恢复生产写流量。

## 退出码

| Code | 含义 |
| --- | --- |
| 0 | dry-run 可安全 apply，或 apply 与 post-apply 验证成功 |
| 1 | 连接、查询、事务或 post-apply 证据不可用 |
| 2 | dry-run/apply 前置检查发现阻断，未写数据 |

## 生命周期

该命令只服务于本次 G5 current-only cutover。台账关闭并保存执行证据后，应从当前工作树删除；需要追溯时使用 Git 历史。
