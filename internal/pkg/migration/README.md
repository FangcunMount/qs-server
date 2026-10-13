# 数据库迁移

> 状态：当前有效
>
> 适用范围：`internal/pkg/migration` 的 MySQL 与 MongoDB schema migration
>
> 事实来源：`migrate.go`、`resources.go`、`retirement_pair.go`、`retirement_driver.go`、`internal/apiserver/bootstrap/database.go` 与嵌入式迁移文件

## 结论

qs-server 在 apiserver 启动阶段按配置执行 MySQL 与 MongoDB 向上迁移。迁移文件嵌入二进制，不依赖运行目录挂载 SQL 或 JSON 文件。

迁移版本机制只保证同一版本不会被正常重复执行，不保证迁移无损。仓库中存在 `DROP`、结构替换、数据清理和遗留对象退役迁移；任何生产迁移都必须按迁移内容评估风险、备份和验收。

当前实现的运行时入口只暴露 `Run()` 向上迁移：

- MySQL：`NewMigrator(db, config)`；
- MongoDB：`NewMongoMigrator(client, config)`；
- dirty 状态会阻断继续迁移；
- 当前目录末端版本为 MySQL `100`、MongoDB `39`；MySQL `86` 为传输死信增加物理投递身份，`87` 新建任务开放提醒逐收件账本，`88` 原子冻结一次开放事件的完整收件身份集合，`89` 在原业务事务内索引 Assessment 与其原始评估请求事件 ID（不回填或重放旧消息），`90` 增加单条缺口恢复的宿主审批结果账本（不自动重投）。`91`～`95` 为 AI MQ 增加持久消息、失败状态、旧命令归属、统一准入门禁和技术观测；原消息及未确认责任须保留。仓库目录版本未代表生产已执行或启用；生产实际版本以数据库只读查询和[当前版本定档验收台账](../../../docs/00-总览/09-当前版本定档验收台账.md)为准。`87`／`88` 一旦写入真实提醒责任，应用回退时须保留两张表；`89`／`90` 的事件身份与人工恢复证据亦须保留，不执行删除账本或关联的 down 迁移。

MySQL `99`、MongoDB `38` 的历史证据槽和索引仍在已发布的 schema 中。历史回填已取消，额外历史证据读取已退出；本批迁移保护重构不删除这些字段、索引或其他业务结构。后续退出须另行盘点数据、演练、授权和发布。

## 固定退役边界与后续版本

MySQL `100`／MongoDB `39` 是不可逆的兼容链退役边界，不是未来迁移的版本上限。对已安装库，启动必须证明 MySQL `domain_event_outbox`、`ai_bridge_commands`、`ai_messaging_legacy_commands` 和 MongoDB `domain_event_outbox` 均已缺失。身份、可见性、权限、网络或 dirty 结果未知均阻断；正常启动不代替受控流程删除现存旧对象。历史迁移字节及退役正文绑定保留。

`resources.go` 从嵌入文件形成实际版本清单和目标 head，并通过 `declaredSchemaPairs` 明确声明可运行的 clean 版本对。当前仍是 `99/38` 的历史升级入口和 `100/39` 的最终状态。新增迁移必须同时声明已审核的完整版本对；不存在于资源清单的版本、未来版本和任意组合不能靠数值大于退役边界获得通过。

同一次启动先证明初始版本对，再执行 SQL 到该 release 的目标。只有迁移、cleanup、连接释放和最终 clean 回读成功，才允许 Mongo 从未变化的初始版本继续到自身目标。两个 backend 最终达到目标、身份不变且旧对象缺失后才能完成启动。同一 live pair 任一步失败或响应未知即失效。`100/38` 仅允许在同次已证明 SQL 成功的阶段存在，新进程不接管该部分状态。

后续互相独立的 DDL 可以分别发布，例如明确声明 `101/39`，再发布 `101/40`。该示例没有对应的新业务迁移文件，也不授权数据库动作。只有被源码声明并实际回读为 clean 的完整状态可以重启；dirty、未声明中间状态和未知 DDL 结果必须保留现场，依据该版本独立演练的方案前向修复。

## 新空库的批准

新空双库继续使用 `migration.retirement-bootstrap-authorization-file` 和 `migration.retirement-bootstrap-authorization-sha256`，不新增 CLI。文件须为真实私有绝对路径、`0600` 单链接文件，摘要绑定已批准的原始字节；目录为空不构成批准。

当前批准格式为 `format_version: 2`、`kind: qs_schema_pair_pristine_bootstrap`。它绑定实际编译 SHA、选定 MySQL 库与 Mongo 集群身份、该二进制的两个目标 head、完整嵌入迁移资源摘要和有效期。资源摘要是按 MySQL、MongoDB 目录及文件名顺序，将全部 up/down 文件记录为 `[backend, path, sha256]` 的 JSON 数组后再计算 SHA256。原 v1 仅绑定退役末尾资源，不能授权当前格式。

批准文件的生成方可通过只读的 `EmbeddedMigrationResources()` 读取实际编译目标 head 与完整资源摘要，避免复制摘要算法或固定目标版本。运行闭环集成测试使用这个接口生成 v2 夹具；该只读接口不创建批准文件，也不绕过真实库身份和空库预检。

批准在 SQL 开始前复核并一次消费；失败或部分冷启动不重新使用该文件。只有批准的新空库才允许完整历史升级后删除新生成的空旧对象。正常恢复不能使用冷启动批准，不能借关闭 DDL、down、force 或直接清除 dirty 绕过保护。

本批真实数据库测试覆盖当前 `100/39`、仅在测试 FS 中的未来 `101/40`、独立阶段重启和部分 DDL 失败。源码和测试不是生产发布证明；本重构的实际发布及后续数据退出须独立验收，四对象此前的执行事实仍见[退役工具说明](../../../scripts/database/compatibility-retirement.md)。

## 目录与职责

```text
internal/pkg/migration/
├── migrate.go
├── driver.go
├── driver_mysql.go
├── driver_mongo.go
├── resources.go
├── retirement_pair.go
├── retirement_driver.go
└── migrations/
    ├── mysql/
    │   ├── 000001_*.up.sql
    │   └── 000001_*.down.sql
    └── mongodb/
        ├── 000001_*.up.json
        └── 000001_*.down.json
```

MySQL 和 MongoDB 各自维护独立的 `schema_migrations` 状态，版本号只在同一种后端内连续，不要求两个后端同步。

标准 Outbox 迁移 MySQL `84` 新建宿主 `rm_outbox`、`qs_rm_replay_requests`、`qs_rm_replay_items`；MongoDB `36` 为新 `rm_outbox` 和请求流水集合建索引。它们不迁移或删除当时的旧 Outbox；旧对象已由之后的独立退役流程退出。MySQL down 只在三张新表都为空时删除结构；一旦写入新消息或授权证据，应用回退须保留兼容结构。Mongo down 保留集合、文档和索引，重新 up 可重复应用。正常 apiserver 启动只执行 up；这些 down 规则不构成生产回滚许可。

MySQL `85` 只为现有 `system_governance_action_runs` 增加按组织、动作、状态与 ID 查询待核对记录的索引；down 只移除该索引，不删除审计数据。

## 启动时序

apiserver 的数据库 bootstrap 执行顺序为：

1. 建立数据库连接；
2. 无论 DDL 是否启用，都联合核对配置选定的 MySQL 与 MongoDB 身份、clean head、可见性和四个退役对象的状态；两个连接和选定库都必须存在，覆盖库名须与两个业务库一致；
3. `migration.enabled=false` 只跳过 DDL，仍要求两个库已达到本 release 的目标 pair；pristine 或待升级状态拒绝，不消费冷启动批准；
4. DDL 启用时先执行 MySQL，再在已证明的 SQL 成功阶段执行 MongoDB；
5. MongoDB migration 完成后协调并验证 ModelCatalog 与报告目录索引，最后只读复验完整目标 pair；
6. 任一步失败都会中止启动，不带着部分完成状态提供服务。

正常开发启动：

```bash
make run-apiserver
```

或显式运行二进制：

```bash
./bin/qs-apiserver --config=configs/apiserver.dev.yaml
```

当前仓库没有独立的 `make migrate-up`、`make migrate-down` 或 migration CLI。不要在文档、脚本或操作手册中假设这些命令存在。

## 配置

apiserver YAML 中的配置为：

```yaml
migration:
  enabled: true
  autoseed: false
  database: qs
```

默认值由应用 options 提供：`enabled=true`、`autoseed=false`。环境变量覆盖必须使用 apiserver 前缀，例如：

```bash
QS_APISERVER_MIGRATION_ENABLED=false make run-apiserver
```

无前缀的 `MIGRATION_ENABLED` 不会按当前配置加载规则覆盖该字段。关闭迁移只用于明确的开发或运维场景，不能作为绕过 dirty 状态或失败迁移的恢复手段。

## 新增迁移

### MySQL

在 `migrations/mysql/` 下增加同版本号的一对文件：

```text
000070_change_name.up.sql
000070_change_name.down.sql
```

要求：

- 版本号使用当前 MySQL 最大版本的下一个值；
- `up` 只承担一个清晰的 schema 或数据治理目标；
- `down` 明确可恢复到什么程度；若数据不可逆，不能声称可以完整回滚；
- DDL、DML、索引和约束变更都要评估锁表、耗时与线上数据分布；
- 对关键迁移增加空库建库、历史版本升级或专用集成测试。

### MongoDB

在 `migrations/mongodb/` 下增加同版本号的一对 JSON 文件：

```text
000033_change_name.up.json
000033_change_name.down.json
```

文件内容是操作数组，例如：

```json
[
  {
    "createIndexes": "example_collection",
    "indexes": [
      {
        "key": { "org_id": 1, "created_at": -1 },
        "name": "idx_example_org_created"
      }
    ]
  }
]
```

支持的操作和约束以 `mongodb_driver.go` 及 [MongoDB migration 说明](migrations/mongodb/README.md)为准，不要凭借 Mongo Shell 语法推断 JSON driver 一定支持。

## 风险分级

| 类型 | 例子 | 最低验收要求 |
|---|---|---|
| 增量变更 | 新表、新集合、新索引、可空字段 | 空库与升级路径测试、性能影响检查 |
| 数据改写 | backfill、状态归一化、唯一键收敛 | 只读盘点、影响行数、幂等性、失败恢复方案 |
| 破坏性退役 | `DROP TABLE`、`drop` 集合、删除字段或历史数据 | 调用与查询依赖清零、备份、恢复演练、明确授权、执行后盘点 |

破坏性迁移至少需要：

1. 扫描当前代码、脚本、定时任务、接口与跨仓消费者；
2. 对目标对象做生产只读盘点，确认行数、集合数、索引和近期读写；
3. 明确备份位置、保留期和恢复验证方式；
4. 在维护窗口执行，并记录迁移前后版本、耗时和影响量；
5. 运行应用健康检查、关键业务烟测与数据完整性查询；
6. 将结果回填到版本验收台账。

迁移文件存在 `down` 不代表数据可恢复。很多退役迁移的 `down` 只能重建空结构，无法复原已删除的数据。

## dirty 状态处理

当 `schema_migrations` 标记为 dirty 时，`Run()` 会返回错误并中止启动。处理顺序：

1. 保留现场，确认后端、版本、失败日志和已执行的语句或操作；
2. 检查目标 schema 与数据处于“未执行、部分执行、已执行但未结算”的哪一种状态；
3. 根据该版本的 `up/down` 和备份制定恢复或补偿方案；
4. 在测试环境复现并验证方案；
5. 经明确审批完成该版本的前向修复或备份恢复，另行核对实际结构与版本元数据一致；不得靠 force 或清除 dirty 跳过未知结果；
6. 重新启动并完成 schema、数据和业务验收。

不要把直接修改 `schema_migrations.dirty` 作为首选操作。仅清除 dirty 标记可能导致版本记录与真实 schema 永久分叉。

## 回滚边界

应用内 `Migrator` 当前没有 `Rollback()` 方法，正常启动路径也不执行 down migration。`.down.sql` 与 `.down.json` 用于迁移契约、受控测试和经审批的外部恢复流程。

生产回滚优先级通常是：

1. 停止继续放大影响；
2. 回退应用版本或关闭受影响入口；
3. 根据迁移性质选择前向修复、受控 down 或备份恢复；
4. 重新验证版本状态和业务数据。

对于已经删除的数据，应用回退和 down migration 都不能替代备份恢复。MySQL `100`／MongoDB `39` 及之后的应用内迁移只允许前进，不跨越或降低已安装的退役边界。

## 验证

提交 migration 变更前至少运行：

```bash
go test ./internal/pkg/migration
git diff --check
make docs-hygiene
```

先配置专用测试 `QS_M3_TEST_MYSQL_DSN` 和 `QS_SERVER_TEST_MONGO_URI`（Mongo replica-set），再运行与 CI 相同的迁移保护检查：

```bash
QS_M3_REQUIRE_DATABASE=1 QS_SERVER_TEST_MONGO_DB_PREFIX=qs_m3_test \
  go test -race -tags=integration -count=1 -timeout=8m \
  -run '^(TestSchemaPairNative|TestDisabledDDLNative)' \
  ./internal/pkg/migration ./internal/apiserver/bootstrap
```

该组测试只使用显式测试端点及独立随机 schema，缺少要求的数据库即失败；验证 cold bootstrap 一次消费、完整资源绑定、未来版本阶段、dirty/旧对象复现、禁用 DDL 与借用资源生命周期。真实数据库、升级、异常中断测试通过仍不能代替生产 schema 和业务验收。

生产验收应记录：

- MySQL 与 MongoDB 的版本和 dirty 状态；
- 迁移执行耗时与错误日志；
- 目标表、集合、索引和数据量；
- 三个进程的实际构建 SHA、健康状态与关键链路烟测；
- 备份和恢复证据。

## 相关文档

- [当前版本定档验收台账](../../../docs/00-总览/09-当前版本定档验收台账.md)
- [本地开发与配置约定](../../../docs/00-总览/06-本地开发与配置约定.md)
- [配置与环境变量](../../../docs/04-接口与运维/05-配置与环境变量.md)
- [部署与端口](../../../docs/04-接口与运维/06-部署与端口.md)

## 已退役运营身份的显式恢复

`cmd/operator-recover` 仅用于经过确认的维护操作，不是后台注册或启用接口。它恢复原 Operator ID，不恢复医生绑定、旧二维码或旧角色。后续角色和 Scope 分配必须独立完成。

- `preflight`：读取当前身份、退出记录和 IAM 权限，输出确定性事实指纹，不写授权或身份数据。允许在干净的 MySQL 78 版、尚无恢复归档表时执行。
- `apply`：要求已执行 79 版迁移、暂停人员及授权写入，并提交本次预演指纹。事务内保存原退出审计、恢复空角色身份、移除已归档退出的阻断记录。
- `status`：校验恢复归档并报告 `pending` 或 `historical_completed`，同时输出当前 Operator 事实。历史完成不表示当前事实未变化，也不表示业务验收通过；重复 Apply 不覆盖后续变化。

输入使用 `OPERATOR_RECOVERY_INPUT` 指向受限 JSON 文件，包含 `OrgID`、`ActorID`、`OperatorID`、`ExpectedVersion`、`ExpectedPolicyVersion`、`RequestID`、`Reason`。目标版本是退出完成后的版本；策略版本必须来自当次核实。报告路径由 `OPERATOR_RECOVERY_REPORT` 指定，必须是尚不存在的新文件，工具以 `0600` 创建。输入不接受未知字段或多个 JSON 对象。

Apply 另需 `OPERATOR_RECOVERY_FINGERPRINT` 与 `OPERATOR_RECOVERY_MAINTENANCE=true`。维护标识是外部写入已暂停的确认，不会自动暂停服务，也不能将 QS 与 IAM 变成一个事务。运行日志只输出状态摘要；人员事实、指纹与错误详情保存到受限报告。

79 版新增 `operator_recovery_archives`。归档包含恢复记录、原退出任务、退出前身份及恢复前身份的原始值和校验和。归档有数据时向下迁移会拒绝删除，必须先安排保留证据的结构恢复方案；普通回滚不得自动重新授予权限。

对于已明确选择、与历史医生绑定清单无关的退出人员，使用 `operator-retire selected-preflight|selected-apply|selected-verify`。`OPERATOR_RETIRE_LAYOUT=final`，`OPERATOR_RETIRE_INPUT` 指向包含 `OrgID`、`ActorID`、`OperatorID`、`ExpectedVersion`、`RequestID`、`Reason` 的受限 JSON；报告仍使用 `OPERATOR_RETIRE_REPORT`。Apply 要求预演指纹及 `OPERATOR_RETIRE_MAINTENANCE=true`，调用统一退出用例，失败保持停用并按原任务恢复。Verify 同时检查当前 IAM 后台角色已清空、本地身份已停用并软删除，不能仅凭任务历史完成判定通过。每人独立保存退出任务及原始事实，不改写此前医生退役清单，也不修改医生、门店、受试者或二维码。

对于任何公司都没有 Operator 记录、但仍有后台授权的用户，使用 `operator-retire orphan-preflight|orphan-apply|orphan-verify`。输入改为 `OrgID`、`ActorID`、`UserID`、`RequestID`、`Reason`，文件须为私有权限；公司用于校验总部操作人，目标用户必须在所有公司都没有 Operator，包括软删除历史。工具仅允许已确认的运营员、结果评估员和计划管理员标准角色，保留 `user` 自服务角色；发现其他权限或人员关联即停止。

孤立授权的 Apply 同样要求真实写入暂停和预演指纹。指纹包含完整角色事实及策略版本，后续重新赋权不能重放旧计划。工具通过 IAM 正常管理接口撤权，不制造 Operator 或退出任务。报告保留撤销前角色及提交的策略版本；`already_unprivileged` 仅说明当前没有后台角色，不能当作历史执行归属或业务验收证明。失败、响应不确定或后续事实变化时，先用只读命令核对，不自动重新授权。

### Statistics 开展门店升级（未发布）

MySQL 80 保存 Survey 不可变开始记录，81 为 Assessment 增加开展上下文，82 增加开展事实与日投影。MongoDB 34 对非空开始引用建立部分唯一索引。现有答卷不补写历史门店；旧客户端缺少开始引用时保持旧受理语义，并在统计中进入 `legacy_start_not_captured` 未知桶。

必须先升级全部 QS Writer 和 Worker，再发布采集端；新开始事实启用后只能回退到保留新字段的兼容版本。80/81 有业务事实时禁止直接向下清理。统计投影失败可停止新视图发布，不影响答卷受理。

运营统计使用 `qs:statistics:collection:operations read`；使用 IAM `statistics-operations preflight/apply --report <受限路径>` 独立配置，在维护窗口携带预演指纹和管理操作人执行。该工具不新增 Assignment，也不扩大 Scope。按现有 Statistics repair/validate/publish 流程补建查询窗口；旧发布批次未包含新投影时接口返回未就绪，不返回伪零。

## 兼容链退役发布 A

MySQL `96` 在既有 `evaluation_outcome` 增加原提交事件引用、JSON 证据及唯一约束；`97` 在既有 `ai_messaging_operations` 增加退休 ID 事实、证据、时间、现用统计索引与约束；`98` 在既有审计检查点增加真实 BSON Outbox cursor/upper-bound。MongoDB `37` 为 Generation 和 Run 的非空原事件引用增加部分唯一索引。迁移不回填旧行、不产生业务命令、不删除四个旧对象。

发布 B 的 MySQL `100`／MongoDB `39` 收尾迁移与联合缺失保护已实现，并随 PR #346 上线。当前源码退出额外历史证据集合的读取和核验器，以及无调用的维护窗口接口；标准事件证据、当前 MQ 和退休 ID 防护继续保留。已发布的历史迁移及其 nullable 字段和索引保留原状，本轮没有数据库结构变更。正常启动仍校验两库身份、完整 clean head 和四个旧对象缺失。
