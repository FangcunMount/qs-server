# 离线双库迁移执行器

源码位于 `scripts/maintenance/qs-migrate-offline/`，可由 `make build-maintenance` 或下文的独立命令构建。

`qs-migrate-offline` 默认只拥有两条数据库连接和一次已批准的嵌入式 up 执行。默认只读；不启动 API、Collection、Worker、业务 Container、Scheduler、relay、NSQ、索引自动 reconcile 或种子数据。独立显式的`--owner-drain`模式仅注册暂停新发送的原生事务Checker，可能持久化正式SDK回查/恢复审计，不能标为只读。它不是业务服务、备份工具、恢复工具或全局维护框架。

完整停写、按批准范围的保护与恢复、逐对象责任和首个 runtime 启动边界由 [One-off、恢复与生产证据](../../../docs/03-基础设施/migration-recovery/20-One-off、恢复与生产证据.md#7-m4-离线退役维护窗口)维护。本文只定义执行器输入与输出。

## 构建与只读检查

从已审核的 clean exact SHA 构建，保留源码 SHA、二进制摘要和实际平台。`pkg/version.GitCommit` 必须为构建时的40位小写SHA；没有 `--source-sha` 覆盖参数。

```bash
go build -ldflags "-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$(git rev-parse HEAD)" \
  -o /private/task-owned/qs-migrate-offline ./scripts/maintenance/qs-migrate-offline
sha256sum /private/task-owned/qs-migrate-offline
```

凭据通过受限环境输入 `QS_OFFLINE_MYSQL_DSN`、`QS_OFFLINE_MONGO_URI`；不放进CLI、命令历史或文档。MySQL DSN必须选择显式指定的数据库；Mongo集群必须提供稳定replica-set身份。仅创建所选MySQL pool和Mongo client，不加载普通服务配置或业务资源；information_schema与admin hello仅用于只读身份/迁移元数据验证。

```bash
/private/task-owned/qs-migrate-offline --mysql-database=qs --mongo-database=qs
```

只读JSON输出包含编译SHA、完整up/down资源摘要、实际选中库身份摘要、初始clean版本对和实际嵌入目标head。不会生成批准、查询业务正文、消费批准或证明业务已经冻结。未知、dirty、未来、未声明、低于100/39、pristine或旧对象复现均拒绝。

实际发布准备也可显式传入 `--application-env`，复用所选 QS API 容器的 `QS_APISERVER_*` 数据库与 TLS 环境；此模式要求数据库名与参数一致、VERIFY_IDENTITY/CA 明确，拒绝同时传入独立离线连接串或冲突 TLS 选项。默认仍只读，批准文件及 apply 规则不变。既有 `release-snapshot.yml` 只读盘点入口固定构建并校验此执行器 SHA/digest，以应用 UID 运行无 apply/approval 参数的只读预览，保存真实初始/目标配对、资源和身份摘要；它不签发冻结或写执行许可。

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
  "protection_scope": "full_database_pair",
  "protection_manifest_sha256": "<reviewed same-window protection manifest digest>",
  "restore_receipt_sha256": "<actual restore rehearsal receipt for the declared protection scope>",
  "startup_receipt_sha256": "<isolated complete runtime startup rehearsal digest>",
  "responsibility_sha256": "<per-object exit responsibility digest>",
  "protection_location": "<actual restricted protection-material reference, no credentials>",
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

两个目标版本必须来自当前二进制的只读输出，不能照抄示例数字。本候选资源目标为111/44，仅 SQL111 退出空 `ai_messaging_outbox`；原110/44的事务和答卷幂等结构保留，生产执行仍须独立审查；105/40的Task旧token退出仍是独立上线前置；生产实际版本仍须只读核实。SQL-only101/39、Mongo-only101/40、Actor-only102/40及维护表-only103/40分别需要独立迁移资源、已审核初始版本对和各自批准，旧迁移字节保持不变。

`protection_scope` 必须显式声明，范围与版本转换不可互换：

| scope | 唯一对象转换或原有责任 |
| --- | --- |
| `m5_qs_empty_ai_publisher` | 仅 `110/44 → 111/44`；保护 QS 原发布表的完整原 DDL、零行及结构恢复证明；其余对象不删除。原冻结、具名责任、恢复/启动回执和材料保留要求不变 |
| `full_database_pair` | 保留完整同窗口双库备份、完整恢复和启动责任；版本仍受嵌入资源与 M3 已声明版本对限制 |
| `m4_sql_historical_columns` | 仅 `100/39 → 101/39`；保护两列历史值、主键、原 schema/CHECK 和具名恢复材料 |
| `m4_mongo_historical_indexes` | 仅 `101/39 → 101/40`；保护四个完整有序索引定义、集合身份与保留对象核对材料 |
| `m4_actor_columns` | 仅 `101/40 → 102/40`；保护 tags/effective_roles 的完整原列定义、主键和值及真实版本表材料，不包含五张历史维护表 |
| `m4_legacy_maintenance_tables` | 仅 `102/40 → 103/40`；保护五表完整原DDL/值及真实完整原版本表，保留现代任务、恢复归档和业务历史 |

| `m6_task_entry_token` | 仅 `104/40 → 105/40`；保护Task旧token列完整定义、主键、原值及真实版本表材料，保留entry_url与现用业务事实 |

| `m6_task_entry_token_with_transport_evidence` | 仅 `103/40 → 105/40`；保护Task旧列，记录传输证据表原先不存在，并执行main原SQL104建表；不切换Broker |

七个对象 scope 均要求相应真实隔离恢复与启动 receipt；不要求用完整双库备份替代具名对象保护。所有 scope 继续要求完整源码/资源摘要、双库身份、冻结、runtime 未启动、单次文件消费、30天材料保留以及30分钟/60分钟预算。`backup_retention_days` 在此表示已批准保护材料的保留期。对象 scope 不授予恢复 API、任意 schema 回退或 dirty 清理权限。

本入口不提供旧格式兼容：旧 `backup_pair_manifest_sha256` / `backup_pair_location` 字段、缺失 scope、未知 scope、对象 scope 的其他版本组合直接拒绝。

执行器验证绑定、时效和字段完整性，不会自动证实外部存储可恢复或进程已停止。证据摘要、保护位置和负责人由批准者核实；缺失、unknown/pending、零摘要、恢复耗时缺失或超过3600秒、保留不足30天、已启动runtime均拒绝。此前四旧对象的恢复例外不能用于本批准。

批准在开始DDL前再次绑定实际初始版本对、身份和原始文件，一次消费生成同目录 `.consumed` 并fsync文件及目录。失败、不确定或无DDL的apply也消费；不能删除marker重试。只读检查不会消费。已完成阶段的fresh restart必须有新批准，绑定当前真实clean版本对；dirty绝不接管。

## 执行与结果

执行只经过 `PreflightCompatibilityPair → SQL Migrator → Mongo Migrator → VerifyComplete`。AutoSeed固定false，不能传force、down、任意版本、任意migration源或dirty清理参数。SQL完成并实际回读后才允许Mongo阶段。

正常30分钟从入口关闭开始，包含重启与实际核验；批准必须保留至少30秒、少于30分钟的`startup_reserve_seconds`，具体值依据完整隔离启动演练。默认总连接/执行deadline15分钟，`--timeout`最多30分钟，并受T0+30分钟减启动预留和expires_at较早者约束。offline的post-floor SQL/Mongo命令使用该context；连接、provider元数据和断开采用有界transport超时。超时或取消不证明服务器DDL已停止：继续保持业务停机，确认实际server活动和metadata，再作恢复决定。

JSON结果报告stage、批准是否已消费、已完成后端的changed值及最终verified。失败后端的changed值省略，并标记`migration_outcome_unknown`；非零退出要求保留原始批准/marker和另行只读盘点，不能把false或进程退出解读为“没有写入”。`verified=true`只证明资源目标的clean版本对，逐对象结构、完整数据恢复、业务ready和生产业务验收仍独立执行。

所有业务进程在执行和只读验收期间继续停止。发出第一个新业务runtime启动命令就是不可用旧全量备份覆盖的提交点，即使启动失败或尚未ready也一样。该工具不启动runtime、不签发生产ready、不解除冻结。

## 验证

普通参数/文件批准测试：

```bash
go test -count=1 ./scripts/maintenance/qs-migrate-offline ./internal/pkg/migration
```

真实隔离数据库测试：

```bash
go test -race -tags=integration -count=1 -timeout=8m -v \
  -run '^TestOfflineNative' ./internal/pkg/migration
```

要求 `QS_M3_TEST_MYSQL_DSN`、`QS_SERVER_TEST_MONGO_URI` 指向隔离MySQL和MongoRS，Mongo数据库prefix含test/contract；缺失依赖即失败。测试只使用独立SQL schema和随机Mongo数据库，测试FS未来资源不进入生产embed。合成批准仅用于逻辑测试，不是生产备份/恢复证明。

### 当前 protection-scope 源码验收

源码 `c334d9b3f2655e7485cc9b817567904c3336dd3d` 在文档 head `09906ae76118f629030b46b7aca9e9cfdb8f753c` 上执行真实隔离 race 门禁。MySQL8.0.36、Mongo replica-set7.0.25 下，现有7个 required Offline 顶层测试及6个子场景全部实际 RUN/PASS，共13项，0 SKIP、exit0；四个scope拒绝场景（缺失、未知、SQL/Mongo对象scope与实际版本转换不符）均在 claim/DDL 前拒绝。3个受影响Go文件前后摘要一致，两自有容器和bridge已释放；现有CI的7个required顶层selector已覆盖，无需改CI。

参数等价的可重放命令（`-p=2`、`-run=^TestOfflineNative`）仅供复现参考，本次文档任务没有调用它，closure不将它登记为passed command。在该源码上它与harness的具名7项selector等价，实际原始argv与执行结果保留在私有harness/receipt，不改写原始证据。

私有原始JSONL摘要 `ed4cbaf137c82aa96a281bf8ea9e08d34d45a74fba8dfff3ab5dc5db586c4cd8`、receipt摘要 `70a655cc7e05ebbe6ed6924e83e9a1e079fb36373dc03103f1ab9a64296ea78e`，harness摘要 `6268a020d08771ad4a5e56c5d38251b4472525515e48c074cde1a7dfded4ff0b`。这些是当前源码的隔离逻辑验收，不是生产对象恢复、RPO、30/60分钟预算或三个服务完整启动的证明。

测试期间既有nsqd发生重启。原容器IDs/images保留，但原服务runtime元数据并非全部稳定，不能据此宣称所有原服务未变，也不推断重启原因。生产、正式部署和业务验收状态仍未提升。

RocketMQ M1 的完整资源目标为104/40，仅增加MySQL传输审计表；原离线批准、真实库身份、保护范围和维护窗口要求继续生效。非空传输证据不允许通过down删除。

## Public MySQL TLS

Production Actions use organization `MYSQL_PUBLIC_HOST`, `MYSQL_PUBLIC_PORT`,
`MYSQL_TLS_MODE=VERIFY_IDENTITY` and immutable `MYSQL_TLS_CA_REF`. The host verifies
the pinned infra CA before deployment or maintenance. Containers receive a read-only
CA mount; Go clients share component-base v0.8.2 verification. Credentials, query
budgets, timezone, transactions, schema and existing approval boundaries are preserved.
This connection change does not authorize a migration or business write. Local and
disposable test databases retain explicit `DISABLED`. Production cutover and business
acceptance require separate runtime evidence.

M3 T24-D 候选110/43显式安装宿主原生事务超限保护账本。索引身份与首次私有审计摘要分离；仅封闭新事务准入，不改写业务、SDK witness或Broker确认。构造和启动不执行DDL，未安装/损坏/错误绑定均不放行。该候选不提供解除、删除或自动清理入口，不代表生产已安装。只声明完整110/43，不接受110/42或109/43。

## M3：初始事务 owner 绑定（候选）

同一维护二进制新增`--binding-action preview|apply|verify`，与原DDL action/批准严格分开；每次只处理`qs.mysql`或`qs.mongo`一个原库根事务，绝不宣称MySQL和Mongo分布式原子提交。必须先显式安装完整、clean的当前资源对，两个owner分别验证完成前，全部业务runtime保持停止。原迁移、双库保护/恢复和首次启动边界不变。

- 公共参数继续显式选择原MySQL/Mongo库，凭据及TLS继续使用`QS_OFFLINE_MYSQL_DSN`/`QS_OFFLINE_MONGO_URI`。绑定action必须额外指定`--timeout 30s`等至多2分钟的总预算，不使用普通离线DDL默认的15分钟。
- `preview`要求`--binding-plan <新私有文件>`、`--binding-operation-id`、`--binding-owner`、`--binding-store-id`及`--binding-writer-epoch`。原文件独占0600创建，冻结当前完整资源摘要、真实双库身份、所选store的实际DDL/collection与index规格摘要、owner及一小时有效期；不推断绑定、不创建元数据。
- `apply`只接收原`--binding-plan`、独立`--binding-reviewed-sha256`及`--binding-writers-excluded`。重新核对原身份/clean head/实际对象摘要、空SDK责任及空原生notice；SQL在SERIALIZABLE根内重查并持锁，Mongo在原session根内重查并唯一INSERT绑定。Mongo所选库使用Primary/majority；不在transaction callback里做外部调用，不执行消息/模型或SDK发送。
- `verify`在新只读边界核对精确原绑定、当前双库与资源。已有绑定或任一新责任不允许再次apply，不upsert、覆盖或删除。SQL/Mongo分别返回`not_attempted`（未尝试commit）、`unknown`或`committed`以及独立`verified`；一个owner成功不能覆盖另一个失败或未知，准入继续关闭。提交未知先保留材料并只读盘点，不自动重试、补偿或伪造witness。

这些新SDK表/集合和notice须为空，旧业务、Outbox、重试计划及历史责任不会被删除，也不由绑定入口签为排空。冻结声明不是真实停写证明；所有结果始终`cutover_authorized=false`，受审文件/命令仍由infra单一协调目录持久记录。构造与运行都不执行DDL、不启动MQ/API/worker、不改变worker配额。`TestM3InitialBindingCLINative`通过实际独立CLI验证原库安装、并发唯一赢家、新责任拒绝及版本账本保持；本地仅编译收集，真实结果须隔离CI，本候选的原生结果以infra执行总表为准。

T25-C3 的受控原库 fixture 新增已处置 notice 的重导入验证：保留首次 evidence/receipt，周期读取只见未处置责任，新物理索引继续封闭。fixture 直接设置 resolved 仅用于状态读取/约束测试，不是生产处置入口或真实 Broker/RPC 恢复证明；独立绑定、原版本账本和停写/保护/恢复规则保持。

Mongo44将原运行期`uk_answersheet_submit_intent`索引安装移至受批准离线迁移。现有同定义索引保留；缺失时原生安装并验证writer/key唯一性及无key记录的兼容性，冲突不自动改写。原105/40前置不变，完整目标需从当前二进制回读为110/44；110/43不是新runtime的完成版本。

### 2026-10-31 宿主退役 M4 复核

本批未新增或执行数据库迁移、修复、重投或删除。当前发布构建检查三个实际进程的依赖图，禁止 NSQ、发布型 Relay 及历史测试夹具进入生产；旧 Relay runtime closure 明确在固定历史源码运行。重试交接夹具改用中立 `eventmessage` 编码与 Reference 校验，原身份、wire、计划/receipt 格式、原事务原子性与人工恢复权限保持。历史命令和生产证据保留原提交归属，不由本次源码复核补为当前验收。

SQL111 的直接资源执行会拒绝；只有精确配对驱动在目标表 WRITE 锁内确认零行才设置会话绑定。非空（含 confirmed/NULL/未知）、缺表、权限/锁/读取未知均停止。失败可能留下 dirty 111；禁止自动 force 或重新启用旧发布器。普通 down 拒绝，须按审查的原 DDL、实际配对状态和兼容镜像边界恢复。自有夹具的原 DDL/结构恢复报告只是隔离证据；与现网原结构/零行、恢复和启动材料绑定后，才进入具体执行批准，不能倒签为已备份现网。

本批窄范围退出还要求完整元数据可见性，并在执行前及目标锁内复核 schema 例程/触发器/事件及指向目标的外键依赖均无未审查对象、外键检查开启。只读离线检查与空流材料生成也执行该依赖检查；失败保留未知，不删除附属对象或按表空推断无依赖。

## M5 事务凭证只读观察

显式使用 `--application-env --mysql-database=qs --mongo-database=qs --protocol-observation --timeout=30s`。与 apply、批准文件和初始 binding 操作互斥，仅接受已安装110/44或111/44，不建表、不修改 binding、不调用Broker，不启动发送器、Checker或业务进程。沿用原数据库TLS/身份，外层deadline最多1分钟，协议观察内部预算15秒。

依次读取原 `qs.mysql`、`qs.mongo` binding，复用正式SDK `ObserveProtocol` 和宿主native-limit reader。只输出计数、数据库采样时间和binding摘要，排除计划、Reference、消息身份与原文。两库样本不是跨库原子快照。保留reserved/preparing、确认审计缺口、无时间缺口、未跟踪abort、未观察commit、冲突和年龄；不能解释为Broker积压。非零保持原计数；读取失败整体available和no_observed_unresolved为false，尚未取得的owner对象为null；仅保留固定错误阶段，不输出provider原始错误。

`no_observed_unresolved` 仅说明本次原主库元数据与native-limit未观察到未决状态，不证明Broker、业务ACK、写者排除或owner可停机。报告固定 `runtime_frozen=false`、`broker_pending_known=false`，不能独立消费为批准。最后owner的在途发送/回查、排空/退出及同窗口材料仍须独立核实。不会自动reconcile、重投或清理凭证。

原 `release-snapshot.yml` 在 `messaging_retirement_inventory=true` 时可显式选 `protocol_observation=true`，通过同一已校验维护容器读取，保存独立 `messaging-retirement-protocol.json`。默认不执行协议观察，原schema预览和清单语义不变。

## M5 独立维护 owner

显式`--owner-drain`与apply、binding、protocol-observation互斥。另要求`--owner-drain-config`为原应用JSON绝对路径、`--owner-drain-config-sha256`为其原字节摘要、`--owner-drain-operation=<digits-attempt>`为新操作，以及`--owner-drain-mysql-identity`/`--owner-drain-mongo-identity`为canonical已核双库身份。生产沿用`--application-env --mysql-database=qs --mongo-database=qs --timeout=30m`；隔离测试仅使用已分配库的显式连接与实际身份。拒绝非110/44、原binding不符或同operation复用，不建表/修改binding。

读取原`messaging.rocketmq`，沿既有Viper时长和literal owner解码保留原routes、凭据和每实例预算，仅在内存将新发送准入设false。分别复用正式SDK的原SQL/Mongo store、native-limit与事务Runtime，无NORMAL发布者、Reserve/Execute入口、业务消费者、HTTP/gRPC、模型、调度或业务Container。它是临时第二组Checker，重叠期间不宣称全主机回查worker数量与原来相同；编排须记录旧/维护两组实例、资源/并发上界，保持原每实例预算和Broker回查参数，并尽快完成旧实例排空。

原业务停止前先验证维护Checker实际注册、原库观测可用新鲜，保留原owner到交接成立。停止原API后调度/业务生产均退出，维护Checker仍可依据原凭证回复Broker并持久化正向恢复审计。没有新业务执行或凭证删除，但SDK正常维护可能转移协议状态；因此同窗口静态保护在维护owner最终退出后捕获。

状态写入新0700目录`$TMPDIR/qs-owner-drain-<operation>/status.json`，0600原子替换；包含实际源码、配置摘要、时间、两owner协议/SDK/保护状态，默认没有停写或批准结论。仅在同目录新0600单链接普通文件`stop.json`声明精确operation、独立receipt摘要，且`writers_stopped`与`replacement_owner_ready`恰一个为true时，才审查退出。声明本身不是写者排除证据；操作者须绑定实际全写者退出或原owner恢复就绪回执。

最终退出还要求新鲜可用的原主库样本、所有未决项/年龄为零、native-limit=false及SDK实际active_sends/running_checks/queued_checks为零。停止后状态明确stopped及实际SDK/sampler退出，独立canonical观察仍须再读。SIGTERM、操作者断线或deadline单独不会关闭仍有责任的最后Checker；expiry标记使本次执行资格失效，保持资源到受审移交/退出。关闭不完整继续保留借用池和其他owner，不把进程超时解释为已排空。状态陈旧、丢失或错误皆停止资格，不复用旧JSON。

本批具体生产编排、额外维护资源、原六实例退出、同窗口保护/恢复及首业务启动点由infra执行包审查。新增原生CLI用真实隔离Broker/MySQL/Mongo验证，不能用包级测试或导入成功代替生产交接。

## M5 同窗口完整保护与原状恢复

`--retained-baseline --timeout=2m`只读枚举所选QS的全部SQL表/视图和Mongo集合/视图/index。逐行覆盖SQL全部列（服务端BINARY/HEX保留NULL、字节、微秒和十进制差异），逐文档覆盖原BSON；对完整行摘要多重集排序再摘要，保留重复数量，不导出业务内容。每对象默认上限100万行，显式`--baseline-max-rows`只可收紧；超过预算、权限、超时或非空Mongo TTL集合均失败且`complete=false`，从不把前缀或未知当作零。system.profile仅保留诊断定义，不签为业务内容保护。跨库快照不原子，实际写者排除及两次稳定基线由协调窗口独立证明。

`--capture-original-operation=<新操作> --timeout=2m`独立只读导出逐字原目标DDL、完整单行110/clean账本及上述全量基线；输出重定向到新0600私有文件并保全原字节。只接受原110/44、受核原目标/账本DDL摘要、空目标和当前完整迁移资源；不是备份整库或停写声明。

`--restore-original=<原私有文件> --restore-original-sha256=<原字节摘要> --timeout=2m`默认只检查实际中断状态，接受111 dirty（DDL前或后）与111 clean首次启动前，以及完整原110；绕过clean-head入口仅用于此严格原材料检查。重新核对canonical库身份、完整账本恰一行、Mongo44、所有保留对象定义和完整内容，并输出实际state摘要。任一新增/缺失/改变的保留资产拒绝，已有目标必须逐项等于原空对象，不覆盖非空表。

实际恢复另加`--restore-apply --restore-approval-file=<独立0600批准> --restore-approval-sha256=<独立原字节摘要>`；与原SQL111批准互斥，不存在Force/down入口。批准格式为`M5RestoreApproval`，kind固定`qs_m5_original_empty_restore`，绑定原材料、实际state、精确源码、停写/首次启动边界/执行审查回执、具名数据与恢复责任人及UTC一小时恢复预算；`runtime_frozen`和`runtime_not_started`只是经外层真实回执支持的声明。材料最多1MiB、批准保持32KiB，拒绝重复/未知JSON字段、symlink、共享/可写路径或复用consumed凭证。

输入和完整主库状态在消费批准前再次实读。只在目标缺失时执行原逐字DDL，完整复核目标和保留资产后，单个SERIALIZABLE根内锁定完整实际111账本，精确DELETE一行并INSERT捕获的完整原110/clean行；事务失败回滚，commit未知保留未知，不重试。最后独立复核全量原对象和110/44。DDL已成功而账本失败时保留实际111 dirty/clean，重新只读检查并另签下一次独立恢复，不手工清dirty。工具不证明全主机排他或首次启动尚未发生；外层必须通过infra `pre-start-repair`及持久首次启动边界调用。任何业务runtime首次启动尝试后禁用旧材料回放，只准111兼容前向修复。

必需真实隔离CI `TestSchemaPairNativeM5RetainedFullBaseline`与`TestSchemaPairNativeM5OriginalRestoreCLI`覆盖完整原字节差异、预算/TTL拒绝、实际独立CLI三种中断、首次启动/停写声明及变化事实拒绝、实际ledger INSERT权限故障后的事务回滚及一次性批准。测试报告严格属于自有数据库，不构成生产恢复验收；实际CI结论按精确提交记录。

原生CLI还必须先与两原SDK同时保持ready，随后确认原SDK正常停止、维护SDK仍活跃，再释放Broker回查并取得正向恢复审计与实际进程退出；该重叠交接是第9个显式必需回执字段，不用“旧owner先死、维护进程再启动”的夹具冒充生产顺序。原消费者13项先独立运行，避免CLI已提交夹具消息污染其pre-commit观察。


R2首次业务启动后的维护交接必须显式加`--owner-drain-forward`，只接受clean111/44；默认`--owner-drain`仍仅接受原clean110/44，不自动猜版本，也不接受dirty111。二者沿同一原配置/身份/binding及暂停新发送的SDK路径，保持原每实例预算，不执行DDL或账本修复。状态的`forward_only`和`registered_mysql_version`标明模式及注册时验证的SQL head，不代替当前schema复读。新业务首次启动后，协调器继续持原首次启动屏障，先证明111维护Checker真实ready，再停止/替换旧新实例；只用已固定111兼容镜像前向修复，不回放110材料。最后交还ready的业务owner并安全停止维护owner。

原生CLI在原110模式的实际恢复/保留断言之后，由自有SDK夹具显式模拟111头，验证默认入口拒绝、显式forward真实双owner注册/ready/安全退出，且原业务事实保持单次、没有新half或版本改写；此模拟头不计为真实SQL111执行或完整schema-pair验收。完整SQL111与三中断原状恢复由独立真实schema-pair门禁覆盖，不能互相替代。
