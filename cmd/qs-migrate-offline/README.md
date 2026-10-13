# 离线双库迁移执行器

`qs-migrate-offline` 只拥有两条数据库连接和一次已批准的嵌入式 up 执行。默认只读；不启动 API、Collection、Worker、业务 Container、Scheduler、relay、NSQ、索引自动 reconcile 或种子数据。它不是业务服务、备份工具、恢复工具或全局维护框架。

完整停写、生产备份对恢复、逐对象责任和首个 runtime 启动边界由 [One-off、恢复与生产证据](../../docs/03-基础设施/migration-recovery/20-One-off、恢复与生产证据.md#7-m4-离线退役维护窗口)维护。本文只定义执行器输入与输出。

## 构建与只读检查

从已审核的 clean exact SHA 构建，保留源码 SHA、二进制摘要和实际平台。`pkg/version.GitCommit` 必须为构建时的40位小写SHA；没有 `--source-sha` 覆盖参数。

```bash
go build -ldflags "-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$(git rev-parse HEAD)" \
  -o /private/task-owned/qs-migrate-offline ./cmd/qs-migrate-offline
sha256sum /private/task-owned/qs-migrate-offline
```

凭据通过受限环境输入 `QS_OFFLINE_MYSQL_DSN`、`QS_OFFLINE_MONGO_URI`；不放进CLI、命令历史或文档。MySQL DSN必须选择显式指定的数据库；Mongo集群必须提供稳定replica-set身份。所有连接和查询仅属于这两库，不加载普通服务配置或业务资源。

```bash
/private/task-owned/qs-migrate-offline --mysql-database=qs --mongo-database=qs
```

只读JSON输出包含编译SHA、完整up/down资源摘要、实际选中库身份摘要、初始clean版本对和实际嵌入目标head。不会生成批准、查询业务正文、消费批准或证明业务已经冻结。未知、dirty、未来、未声明、低于100/39、pristine或旧对象复现均拒绝。

## 独立文件批准

写执行使用 `--apply --approval-file=<absolute-path> --approval-sha256=<raw-sha256>`。批准是具名责任人审查的原始JSON文件，0600普通文件、单硬链接、无symlink路径；采用与冷启动批准相同的私有文件检查。默认只读模式不接收批准参数。

批准格式如下；尖括号必须替换成真实证据，模板本身无法通过：

```json
{
  "format_version": 1,
  "kind": "qs_schema_pair_offline_upgrade",
  "operation_id": "<digits>-<attempt>",
  "expires_at": "<UTC RFC3339 Z, at most 24 hours>",
  "approved_source_sha": "<compiled source SHA>",
  "migration_resources_sha256": "<actual complete embedded resources digest>",
  "mysql_database": "<selected MySQL database>",
  "mongo_database": "<selected Mongo database>",
  "mysql_identity_sha256": "<read-only inspection identity>",
  "mongo_cluster_sha256": "<read-only inspection identity>",
  "initial_mysql_version": 100,
  "initial_mongo_version": 39,
  "target_mysql_version": 100,
  "target_mongo_version": 39,
  "approval_summary_sha256": "<reviewed approval digest>",
  "freeze_scope_sha256": "<all writer and autonomous expiry freeze evidence digest>",
  "backup_pair_manifest_sha256": "<complete same-window backup-pair manifest digest>",
  "restore_receipt_sha256": "<complete actual backup-pair restore receipt digest>",
  "startup_receipt_sha256": "<isolated complete runtime startup rehearsal digest>",
  "responsibility_sha256": "<per-object exit responsibility digest>",
  "backup_pair_location": "<real restricted storage reference, no credentials>",
  "data_owner": "<named data owner>",
  "recovery_owner": "<named recovery owner>",
  "runtime_frozen": true,
  "runtime_not_started": true,
  "window_started_at": "<actual T0, UTC RFC3339 Z>",
  "restore_duration_seconds": 1,
  "startup_reserve_seconds": 300,
  "backup_retention_days": 30
}
```

两个目标版本必须来自当前二进制的只读输出，不能照抄示例数字。当前main资源仍为100/39；仅合入执行器不会产生101或40。SQL-only101/39及Mongo-only101/40分别需要独立迁移资源、已审核版本对和各自批准，旧迁移字节保持不变。

执行器验证绑定、时效和字段完整性，不会自动证实外部存储可恢复或进程已停止。证据摘要、备份位置和负责人由批准者核实；缺失、unknown/pending、零摘要、恢复耗时缺失或超过3600秒、保留不足30天、已启动runtime均拒绝。此前四旧对象的恢复例外不能用于本批准。

批准在开始DDL前再次绑定实际初始版本对、身份和原始文件，一次消费生成同目录 `.consumed` 并fsync文件及目录。失败、不确定或无DDL的apply也消费；不能删除marker重试。只读检查不会消费。已完成阶段的fresh restart必须有新批准，绑定当前真实clean版本对；dirty绝不接管。

## 执行与结果

执行只经过 `PreflightCompatibilityPair → SQL Migrator → Mongo Migrator → VerifyComplete`。AutoSeed固定false，不能传force、down、任意版本、任意migration源或dirty清理参数。SQL完成并实际回读后才允许Mongo阶段。

正常30分钟从入口关闭开始，包含重启与实际核验；批准必须保留至少30秒、少于30分钟的`startup_reserve_seconds`，具体值依据完整隔离启动演练。默认总连接/执行deadline15分钟，`--timeout`最多30分钟，并受T0+30分钟减启动预留和expires_at较早者约束。offline的post-floor SQL/Mongo命令使用该context；连接、provider元数据和断开采用有界transport超时。超时或取消不证明服务器DDL已停止：继续保持业务停机，确认实际server活动和metadata，再作恢复决定。

JSON结果报告stage、批准是否已消费、已完成后端的changed值及最终verified。失败后端的changed值省略，并标记`migration_outcome_unknown`；非零退出要求保留原始批准/marker和另行只读盘点，不能把false或进程退出解读为“没有写入”。`verified=true`只证明资源目标的clean版本对，逐对象结构、完整数据恢复、业务ready和生产业务验收仍独立执行。

所有业务进程在执行和只读验收期间继续停止。发出第一个新业务runtime启动命令就是不可用旧全量备份覆盖的提交点，即使启动失败或尚未ready也一样。该工具不启动runtime、不签发生产ready、不解除冻结。

## 验证

普通参数/文件批准测试：

```bash
go test -count=1 ./cmd/qs-migrate-offline ./internal/pkg/migration
```

真实隔离数据库测试：

```bash
go test -race -tags=integration -count=1 -timeout=8m -v \
  -run '^TestOfflineNative' ./internal/pkg/migration
```

要求 `QS_M3_TEST_MYSQL_DSN`、`QS_SERVER_TEST_MONGO_URI` 指向隔离MySQL和MongoRS，Mongo数据库prefix含test/contract；缺失依赖即失败。测试只使用独立SQL schema和随机Mongo数据库，测试FS未来资源不进入生产embed。合成批准仅用于逻辑测试，不是生产备份/恢复证明。
