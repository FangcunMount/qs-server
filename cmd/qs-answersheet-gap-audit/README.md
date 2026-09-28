# AnswerSheet → Assessment 只读缺口审计

`qs-answersheet-gap-audit` 复用 `internal/apiserver/infra/answersheetgap` 的原事实校验与跨库分类，只读取带 `durable_acceptance.schema_version=1` 的新答卷。它不会重置 Outbox、发布消息、创建测评或调用模型。

运行前必须提供 `RM_QS_GAP_READ_ONLY=1`、`RM_QS_GAP_MYSQL_DSN`、`RM_QS_GAP_MONGO_URI`、`RM_QS_GAP_MONGO_DB` 环境变量；连接值不要放到命令行或日志。显式指定 `--after-id`（独占）、`--upper-id`（包含）和至少早于当前十分钟的 `--accepted-before`（RFC3339）。每次最多 20 页、每页最多 500 条。建议由有只读数据库权限的运维身份运行。

生产一次性工作流 `M6 QS-03 AnswerSheet Gap Read-only Audit` 从健康 API 容器读取现行连接配置，在独立、只读文件系统的临时容器内运行本命令。工作流通过标准输入传入连接 JSON（`--connections-stdin`），不把凭据放入命令行、日志或工件；仍必须显式提供 `RM_QS_GAP_READ_ONLY=1`。生产运行只输出分类计数，不生成逐项 ID 文件。工作流输入中的 API 镜像 SHA、答卷 ID 水位和时间截点必须与现场核对一致；发现确认缺口、未知或人工处理项时工作流失败并保留汇总，不自动修复。

```sh
qs-answersheet-gap-audit \
  --after-id 0 \
  --upper-id 1000000 \
  --accepted-before 2026-01-01T00:00:00Z \
  --page-size 100 --max-pages 1
```

命令标准输出唯一 JSON 包含固定输入水位、扫描数、分类计数和下一页游标；游标是答卷 ID，**不得原样写入公开日志**。生产工作流只公开分类汇总，隐藏三个 ID 水位，并在 `complete=false` 时失败；需要继续分页时由有权限的运维人员在私有环境使用游标。若需要逐项核对，可增加 `--detail-report /absolute/new/path.json`，命令以 `0600` 创建全新文件并最多记录 20 条需处理的答卷／原事件 ID；不要把该文件上传到公开 CI 日志或工件。`missing_confirmed` 只说明应有 Assessment 尚未观察到，仍需按原事实复查后由授权的宿主恢复流程处理；`unknown`、`manual_required` 不可自动重发。退出非零或无 JSON 均表示本次审计无效。扫描不会核对独立热度投影、已存在测评后的执行／报告效果或外部模型调用。
