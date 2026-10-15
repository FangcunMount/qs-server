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
- 当前目录末端版本为 MySQL `103`、MongoDB `40`；MySQL `86` 为传输死信增加物理投递身份，`87` 新建任务开放提醒逐收件账本，`88` 原子冻结一次开放事件的完整收件身份集合，`89` 在原业务事务内索引 Assessment 与其原始评估请求事件 ID（不回填或重放旧消息），`90` 增加单条缺口恢复的宿主审批结果账本（不自动重投）。`91`～`95` 为 AI MQ 增加持久消息、失败状态、旧命令归属、统一准入门禁和技术观测；原消息及未确认责任须保留。仓库目录版本未代表生产已执行或启用；生产实际版本以数据库只读查询和[当前版本定档验收台账](../../../docs/00-总览/09-当前版本定档验收台账.md)为准。`87`／`88` 一旦写入真实提醒责任，应用回退时须保留两张表；`89`／`90` 的事件身份与人工恢复证据亦须保留，不执行删除账本或关联的 down 迁移。

MySQL `99`、MongoDB `38` 原历史证据槽和索引的创建迁移保持原字节；SQL101和Mongo40分别提供M4-A两列与M4-B四索引的独立退役资源，SQL103不扩大这两批范围。历史回填、额外历史证据读取和核验器已退出源码。目录有退役资源不代表生产已执行；每批仍须独立完成实际对象盘点、保护恢复、授权和发布，目录升级成功不能代替生产数据或业务验收。

## 固定退役边界与后续版本

MySQL `100`／MongoDB `39` 是不可逆的兼容链退役边界，不是未来迁移的版本上限。对已安装库，启动必须证明 MySQL `domain_event_outbox`、`ai_bridge_commands`、`ai_messaging_legacy_commands` 和 MongoDB `domain_event_outbox` 均已缺失。身份、可见性、权限、网络或 dirty 结果未知均阻断；正常启动不代替受控流程删除现存旧对象。历史迁移字节及退役正文绑定保留。

`resources.go` 从嵌入文件形成实际版本清单和目标 head，并通过 `declaredSchemaPairs` 明确声明可运行的 clean 版本对。当前声明 `99/38`、`100/39` 的历史升级入口、独立 SQL 发布的 clean `101/39`、Mongo 发布的 clean `101/40` 、Actor 发布的 `102/40` 和本批完整目标 `103/40`。新增迁移必须同时声明已审核的完整版本对；不存在于资源清单的版本、未来版本和任意组合不能靠数值大于退役边界获得通过。

同一次启动先证明初始版本对，再执行 SQL 到该 release 的目标。只有迁移、cleanup、连接释放和最终 clean 回读成功，才允许 Mongo 从未变化的初始版本继续到自身目标。两个 backend 最终达到目标、身份不变且旧对象缺失后才能完成启动。同一 live pair 任一步失败或响应未知即失效。`100/38` 仅允许在同次已证明 SQL 成功的阶段存在，新进程不接管该部分状态。

专用备份 `Capture`、恢复摘要生成及 B 恢复核验器已退出当前源码。已完成批次的备份、恢复预算和生产回执保留在独立归档，当前通用迁移器不提供该历史恢复工具接口。

后续互相独立的 DDL 分别准备：SQL101 声明 `101/39`，Mongo40 声明 `101/40`。发布顺序必须先验收 SQL-only `101/39`，再以该 clean pair 为初始批准执行 Mongo-only `101/40`；资源文件和版本声明不授权生产数据库动作。只有被源码声明并实际回读为 clean 的完整状态可以重启；dirty、未声明中间状态和未知 DDL 结果必须保留现场，依据该版本独立演练的方案前向修复。

### M4-C：Actor 两列

SQL102 仅删除 `testee.tags` 与 `operators.effective_roles`；历史迁移1..101及Mongo1..40保持原字节。使用 `m4_actor_columns` 只授权 `101/40 → 102/40`，不能借历史JSON或索引保护范围；`102/39` 不声明为可重启版本对。

本候选必须先离线完成DDL，再启动新runtime；旧有效角色列无默认值，新PO省略它后不能在旧schema写入。C 发布时现代prepare/exit/recover限定cleanSQL102；当前 D 源码已显式重审到cleanSQL103，旧未执行指纹重做。旧raw archive和checksum不重写；旧镜像对SQL102未来head会拒绝。SQL102down明确拒绝，不提供Force、SetVersion、手清dirty或空列重建替代真实数据恢复。

Actor原生四主场景及两个范围拒绝子场景在源码 `7f131c9e` 的独占 MySQL8.0.36/MongoRS7.0.31 夹具上实际 RUN/PASS；现代工具及原历史隔离 SQL 九场景亦通过，均零 SKIP。该轮 schema/value 恢复的版本表保持 clean101/40，不作为 dirty 恢复证明。

后续源码 `c11faebf3c715f511e8b936f46f28bcc1f757150` 的 `TestActorColumnsNativeDirtyObjectBackupRestore` 在同补丁版本的另一套自有夹具上捕获首次 clean101/40 的原列定义、完整列值、保留事实和真实版本账本，并冻结原材料 SHA。真正第二条 ALTER 失败留下 dirty102 后，先恢复两列及原值、核对完整保留事实，最后显式事务回放完整原版本账本；成功后 fresh101/40 preflight 通过。错误备份、保留事实变化、列值恢复失败和账本重复主键失败均拒绝提前完成，回放失败保留 dirty102。该轮五个顶层、七个子场景共十二项 RUN/PASS、零 SKIP，耗时46.51秒；原生日志摘要 `d122b770ca09fa2fcdc05acf713c296f06efb5a1273c5fd37287e04b6ce194fe`，自有容器、网络和私有连接材料已释放。这是固定测试 seam，没有新增生产恢复入口、启动业务 runtime 或证明候选启动后覆盖旧备份合法。

合并后的源码仍须通过当前完整 CI，旧组件证明不签为新合并头的原生执行。生产值分布、定向备份、原/新完整服务启动、完整冻结与30/60分钟/RPO0仍待具名证明。五张历史维护表不在SQL102范围。

### M4-D：历史维护表与在线旧工具退出

SQL103 只删除 `testee_store_migration_reversions`、`testee_store_migration_items`、`testee_store_migration_manifests`、`clinician_operator_binding_archive` 与 `operator_retirement_manifests`，每张表独立 DROP；历史1..102与Mongo1..40资源保持原字节。`m4_legacy_maintenance_tables` 只允许 `102/40 → 103/40`，其他对象scope、跨阶段、no-op、Mongo变化或未来版本拒绝。已到103或其后已声明版本时，五旧对象复现也拒绝启动。

旧 `testee-store-migrate` CLI、Testee初始归属维护包和专属测试退出；现代 `testeestore` 仓库、事务、`testee_store_history`、门店历史与回答开始职责保留。Operator旧批量 manifest/archive/Report、`NewLegacyRepository` 和 staff装配退出，selected/orphan、prepare/recovery、`operator_retirement_tasks` 和 `operator_recovery_archives` 继续使用。旧四个CLI动作用原早拒入口明确拒绝，不能重新签历史清单。

现代维护工具只接受单行cleanSQL103，固定 `CurrentOperatorColumns` 不变，指纹版本为 `actor_maintenance_sql103`；SQL102或更早的未执行preflight须重做。历史归档正文先核原checksum再解码，原字节不重写。原工具、原manifest和摘要解释仅留在具名历史隔离恢复材料与Git历史中，不增加在线兼容分支。

源码 `d25313486c739b1584539e5fbf61e51eb7038848` 在唯一自有 MySQL8.0.36/MongoRS7.0.31 夹具上实际通过 D 五主场景与十子场景共15项、现代人员及Activity八项、门店六项与受试者归属七项；36项均 RUN/PASS、零SKIP/FAIL。D包括全原迁移102/40、真正第三DROP失败留下103dirty、完整五原表DDL/值与保留事实核对、完整真实原账本最后事务回放、1062冲突/中断/新runtime边界拒旧回放，以及旧头/Force/down/旧对象复现/错误scope拒绝。当前fixture的时间列以server CHAR冻结，保留NULL/微秒并绑定同一SQL会话时区，不把带客户端offset的时间文本重新解释为原值。

该轮原生阶段耗时57.96秒、现代12.98秒、门店归属6.56秒。私有receipt摘要 `e5f522eea776cf976243d7594c8242415dfa44d5e02aed0416fcd08bd48a8866`；D原始JSONL `9dd1c622cdae50aa0666d9e3bf7cbdcf53bfae057fa588f71f82ce6a68dc26ef`，现代 `45cd096b7f4424a5043954017aa174d1a1ae1a3e1e80dad75b889bc5180508a4`，门店归属 `9e8839b9b451255768ca19c899d7753ea31be45542009123548131b431242a22`。28个源文件摘要前后一致，13个删除源仍缺失；两自有容器、网络和连接材料已释放，原IDs/images/volumes保留，既有NSQ runtime变化因此不签全部环境稳定。原75b首轮15RUN/11PASS及5602第二轮D15PASS但现代8RUN/4PASS失败均保留；未发布103down文案长度和测试时间值/当前版本资源库存分别独立修正，不改旧迁移。

实际argv及全量case明细以原harness/JSONL为准；CI源码 `f562abfa057405a34f718ae635ead3c77b8c5153` 新增D15项逐一RUN/PASS且任何SKIP失败的门禁，不代表本地在这个后续head重复运行native。没有三服务启动、生产对象/权限/冻结、30/60分钟或RPO0证明，未用模拟runtime标识冒充实际进程启动。

生产只读盘点 `80f422101ccfd83b4c4eb5bd165272145afe7afd` / run `38628472662-1` 在cleanSQL100、MySQL8.0.36的一致快照中确认：Testee manifests/items/reversions为1/83797/0，Operator manifests/binding archive为1/119；Testee manifest applied=1、item candidate=55577/deferred=28220，Operator applied_unchanged=1，三个历史引用gap均0。私有回执摘要 `73bdf2677701f454cb7f64cdce950a319328c5fc87e96ea505353fbd55fc0125` 绑定原目标和所有连接/事务释放，`freeze_verified=false`。用户已确认五表没有遗留任务，原材料按隔离恢复职责保留；这些时点与责任事实不代替D执行时clean102/40的实际全量原值/DDL、完整原账本保护、持续停写和迁移/恢复资格。

定向保护、生产迁移/恢复及30/60分钟/RPO0仍未验收。固定测试seam不提供生产备份/恢复入口；首个新runtime命令后只允许保留新事实的向前恢复，不能回放旧表/账本覆盖新事实。SQL103down只明确拒绝，不用Force、SetVersion或手清dirty替代恢复。

D 定向只读工具 `scripts/oneoff/m4_maintenance_table_protection` 已准备源码，状态为 `read_only_prepared / no_native / restore_not_implemented`。固定执行源 `c335b2a1a34dd69830261e2d8e11b16da3f8befb`，原 SQL1..103/Mongo1..40 的286个 up/down 资源按 `[backend,path,rawSHA256]` 顺序编码后的摘要为 `20e0096ffa50a50a7818bfd09741fa48ce8d6e0dc909bf0cf49e0292ba254c4d`；这与独立编译的工具 SHA 分别登记，不用后续 main 的资源替换。工具未调用迁移 provider、DDL、恢复、批准或 journal。

默认 `inspect` 只检查 clean102/40、五张原 InnoDB BASE TABLE 的列/主键形状和原 DDL。`capture` 在同一 SQL 只读 RR 中完整保存五表原 DDL、固定列与完整主键、逐行服务端 `CAST AS BINARY` 原值及 SQL NULL 标记、行摘要、完整原 SQL 账本及其 DDL；`ParseTime=false`、固定 UTC/结果字符集/SQL mode，DATETIME(6) 微秒、LONGTEXT 空白/键顺序、已有摘要均不经 Go 时间或 JSON map 重编码、不重签。字符串主键按完整 binary bytes 排序；复合主键再按 uint64 数值排序，边界使用 `DECIMAL(20,0)`，保留空字符串、0 与 Uint64MAX，每表额外读取空 EOF 并在同 RR 执行完整 COUNT 守恒。

保护边界是单值8MiB、单行原值12MiB、每表行 JSON 完整编码128MiB、总材料256MiB、每表250000行/总500000行，分页500，单个 DDL 上限64KiB。超限整次拒绝，不截断、不写成功材料；这些是工具限额，不推断生产实际内容一定能容纳。普通元数据/分页请求各5秒；只有末端完整 COUNT 使用有限 action 剩余绝对期限及正值服务端 MAX_EXECUTION_TIME，action 最长20分钟，不能据此签30分钟窗口/60分钟恢复预算。

`capture` 需要调用方提供独立 SQL/Mongo 选库身份及 Mongo 账本 UUID 摘要，连接仅来自 `QS_M4_D_OBJECT_MYSQL_DSN` / `QS_M4_D_OBJECT_MONGO_URI` 环境；不在 argv、回执或材料保存连接凭据。SQL 同 RR 与 Mongo 原40账本严格三字段 BSON/UUID 的前后独立观测分别记录，不声称双库原子 RR；Mongo options/index 未保存或核验。新材料必须在当前 UID 的0700叶目录以 O_EXCL/0600 写入、fsync、重开核对 SHA；写前后同一个真实目录/UID和总期限通过、所有事务/连接释放后才输出成功摘要。旧文件、未知/重复 schema 成员、身份/原材料 SHA 或首录绝对路径不符均拒绝。

`verify-target` 只接受独立冻结的完整材料及外部 SHA，确认 clean103/40、五表全部缺失、原账本 DDL和双库身份/Mongo 原 BSON 不变；它不保存或证明五表之外的业务行值不变，不把 ready/账本状态替代业务验收。`restore` 在读取环境/连接前明确拒绝，尚无批准消费或恢复入口。本轮源码 `f571d36d7e755d7ae843d7e02c7c7245acf1e7b2` 的普通/race、编译/vet/lint与固定资源/NULL/原字节/复合键/EOF/COUNT分类/私有文件边界测试只证明本地代码；D 新工具真实 MySQL/Mongo 夹具、生产102/40全量材料、五表各 DROP 前缀及恢复、窗口与业务验收仍未完成。原 d253 固定 seam 的15项证明不移签给此工具。D 工作完成且恢复责任结清后按 M7 退役专用代码，原材料按批准保留期与责任保留。


## 新空库的批准

现有库的停机DDL使用独立 [`qs-migrate-offline`](../../../cmd/qs-migrate-offline/README.md)，默认只读，写执行需要精确绑定source/资源/初始pair/身份/目标和具名保护恢复证据的私有文件批准。`protection_scope` 显式选择完整双库、M4历史两列、M4四索引、Actor两列或五张历史维护表；四个对象scope分别只允许 `100/39 → 101/39`、`101/39 → 101/40`、`101/40 → 102/40` 和 `102/40 → 103/40`，使用 `protection_manifest_sha256` / `protection_location`，旧backup-pair字段拒绝。完整双库scope保留原完整责任，所有scope保留双库身份、冻结、一次消费、恢复/启动回执和预算保护。它不启动业务runtime、不AutoSeed、不自动reconcile索引；与本节的新空库v2批准互不通用。完整维护窗口与提交边界见 [恢复Runbook](../../../docs/03-基础设施/migration-recovery/20-One-off、恢复与生产证据.md#7-m4-离线退役维护窗口)。

新空双库继续使用 `migration.retirement-bootstrap-authorization-file` 和 `migration.retirement-bootstrap-authorization-sha256`，不新增 CLI。文件须为真实私有绝对路径、`0600` 单链接文件，摘要绑定已批准的原始字节；目录为空不构成批准。

当前批准格式为 `format_version: 2`、`kind: qs_schema_pair_pristine_bootstrap`。它绑定实际编译 SHA、选定 MySQL 库与 Mongo 集群身份、该二进制的两个目标 head、完整嵌入迁移资源摘要和有效期。资源摘要是按 MySQL、MongoDB 目录及文件名顺序，将全部 up/down 文件记录为 `[backend, path, sha256]` 的 JSON 数组后再计算 SHA256。原 v1 仅绑定退役末尾资源，不能授权当前格式。

批准文件的生成方可通过只读的 `EmbeddedMigrationResources()` 读取实际编译目标 head 与完整资源摘要，避免复制摘要算法或固定目标版本。运行闭环集成测试使用这个接口生成 v2 夹具；该只读接口不创建批准文件，也不绕过真实库身份和空库预检。

批准在 SQL 开始前复核并一次消费；失败或部分冷启动不重新使用该文件。只有批准的新空库才允许完整历史升级后删除新生成的空旧对象。正常恢复不能使用冷启动批准，不能借关闭 DDL、down、force 或直接清除 dirty 绕过保护。

本批真实数据库测试覆盖当前 `101/40`、独立 SQL-only `101/39`、从嵌入头计算且仅在测试 FS 中的未来版本、独立阶段重启和部分 DDL 失败。源码和测试不是生产发布证明；本重构的实际发布及后续数据退出须独立验收，四对象此前的执行事实仍见[退役工具说明](../../../scripts/database/compatibility-retirement.md)。

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

支持的操作和约束以 `driver_mongo.go` 及 [MongoDB migration 说明](migrations/mongodb/README.md)为准，不要凭借 Mongo Shell 语法推断 JSON driver 一定支持。

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
  -run '^(TestHistoricalEvidenceNative|TestSchemaPairNative|TestDisabledDDLNative)' -v \
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

- `preflight`：仅在当前cleanSQL103读取身份、退出记录和 IAM 权限，输出确定性事实指纹，不写授权或身份数据。仓库保留的clean78缺归档历史读取边界，不是当前维护工具的执行许可。
- `apply`：要求当前cleanSQL103、已保留79版恢复归档结构、暂停人员及授权写入，并提交本次预演指纹。事务内保存原退出审计、恢复空角色身份、移除已归档退出的阻断记录。
- `status`：校验恢复归档并报告 `pending` 或 `historical_completed`，同时输出当前 Operator 事实。历史完成不表示当前事实未变化，也不表示业务验收通过；重复 Apply 不覆盖后续变化。

输入使用 `OPERATOR_RECOVERY_INPUT` 指向受限 JSON 文件，包含 `OrgID`、`ActorID`、`OperatorID`、`ExpectedVersion`、`ExpectedPolicyVersion`、`RequestID`、`Reason`。目标版本是退出完成后的版本；策略版本必须来自当次核实。报告路径由 `OPERATOR_RECOVERY_REPORT` 指定，必须是尚不存在的新文件，工具以 `0600` 创建。输入不接受未知字段或多个 JSON 对象。

Apply 另需 `OPERATOR_RECOVERY_FINGERPRINT` 与 `OPERATOR_RECOVERY_MAINTENANCE=true`。维护标识是外部写入已暂停的确认，不会自动暂停服务，也不能将 QS 与 IAM 变成一个事务。运行日志只输出状态摘要；人员事实、指纹与错误详情保存到受限报告。

79 版新增 `operator_recovery_archives`。归档包含恢复记录、原退出任务、退出前身份及恢复前身份的原始值和校验和。归档有数据时向下迁移会拒绝删除，必须先安排保留证据的结构恢复方案；普通回滚不得自动重新授予权限。

历史医生批次 `operator-retire preflight|apply|status|verify` 在 Options/App、配置读取、报告创建及 DB/IAM 连接之前明确拒绝；旧library/manifest/archive消费者已随M4-D退出。旧工具对assessment/testee等整行算摘要，旧schema、原manifest/AfterHash及解释源码仍按具名隔离恢复材料保留，不能在新schema重新签发旧摘要。五表实际退役及定向数据保护仍待独立生产批准，不以本次源码删除代替责任结清。现代任务、恢复归档、门店和受试者历史继续保留。

对于已明确选择、与历史医生绑定清单无关的退出人员，使用 `operator-retire selected-preflight|selected-apply|selected-verify`。`OPERATOR_RETIRE_INPUT` 指向包含 `OrgID`、`ActorID`、`OperatorID`、`ExpectedVersion`、`RequestID`、`Reason` 的受限 JSON；报告仍使用 `OPERATOR_RETIRE_REPORT`。Apply 要求预演指纹及 `OPERATOR_RETIRE_MAINTENANCE=true`，调用统一退出用例，失败保持停用并按原任务恢复。Verify 同时检查当前 IAM 后台角色已清空、本地身份已停用并软删除，不能仅凭任务历史完成判定通过。每人独立保存退出任务及原始事实，不改写此前医生退役清单，也不修改医生、门店、受试者或二维码。

对于任何公司都没有 Operator 记录、但仍有后台授权的用户，使用 `operator-retire orphan-preflight|orphan-apply|orphan-verify`。输入改为 `OrgID`、`ActorID`、`UserID`、`RequestID`、`Reason`，文件须为私有权限；公司用于校验总部操作人，目标用户必须在所有公司都没有 Operator，包括软删除历史。工具仅允许已确认的运营员、结果评估员和计划管理员标准角色，保留 `user` 自服务角色；发现其他权限或人员关联即停止。

孤立授权的 Apply 同样要求真实写入暂停和预演指纹。指纹包含完整角色事实及策略版本，后续重新赋权不能重放旧计划。工具通过 IAM 正常管理接口撤权，不制造 Operator 或退出任务。报告保留撤销前角色及提交的策略版本；`already_unprivileged` 仅说明当前没有后台角色，不能当作历史执行归属或业务验收证明。失败、响应不确定或后续事实变化时，先用只读命令核对，不自动重新授权。

### Statistics 开展门店升级（未发布）

MySQL 80 保存 Survey 不可变开始记录，81 为 Assessment 增加开展上下文，82 增加开展事实与日投影。MongoDB 34 对非空开始引用建立部分唯一索引。现有答卷不补写历史门店；旧客户端缺少开始引用时保持旧受理语义，并在统计中进入 `legacy_start_not_captured` 未知桶。

必须先升级全部 QS Writer 和 Worker，再发布采集端；新开始事实启用后只能回退到保留新字段的兼容版本。80/81 有业务事实时禁止直接向下清理。统计投影失败可停止新视图发布，不影响答卷受理。

运营统计使用 `qs:statistics:collection:operations read`；使用 IAM `statistics-operations preflight/apply --report <受限路径>` 独立配置，在维护窗口携带预演指纹和管理操作人执行。该工具不新增 Assignment，也不扩大 Scope。按现有 Statistics repair/validate/publish 流程补建查询窗口；旧发布批次未包含新投影时接口返回未就绪，不返回伪零。

## 兼容链退役发布 A

MySQL `96` 在既有 `evaluation_outcome` 增加原提交事件引用、JSON 证据及唯一约束；`97` 在既有 `ai_messaging_operations` 增加退休 ID 事实、证据、时间、现用统计索引与约束；`98` 在既有审计检查点增加真实 BSON Outbox cursor/upper-bound。MongoDB `37` 为 Generation 和 Run 的非空原事件引用增加部分唯一索引。迁移不回填旧行、不产生业务命令、不删除四个旧对象。

发布 B 的 MySQL `100`／MongoDB `39` 收尾迁移与联合缺失保护已实现，并随 PR #346 上线。当前源码退出额外历史证据集合的读取和核验器，以及无调用的维护窗口接口；标准事件证据、当前 MQ 和退休 ID 防护继续保留。已发布的历史迁移及其 nullable 字段和索引保留原状，本轮没有数据库结构变更。正常启动仍校验两库身份、完整 clean head 和四个旧对象缺失。

## M4-01 历史 JSON 对象退役（源码准备，生产待验收）

新增 MySQL `101` 只删除 `assessment.historical_lifecycle_evidence`、`evaluation_outcome.historical_committed_evidence` 及对应的两个 CHECK，声明完整版本对 `101/39`。SQL-only 发布批次保持 MongoDB `39`；下一批 Mongo40 独立退出四个历史索引，见下节。所有历史迁移字节保留。现用提交事件证据、canonical Outcome、冻结的 schema 3 ReportInput、报告历史以及历史归档/明细表继续保留。

生产部署暂停仍有效。生产配置启用了自动迁移，因此在启动这版二进制前，必须完成以下部署 gate：消费者与历史读取者退出证据、当前事件与命令责任核查、已批准逐对象范围的实际备份与恢复核验、维护窗口及具名恢复责任人。完整双库备份不再作为 M4-A／M4-B 的统一前置；各批备份必须绑定实际源身份、原结构／值、完整原版本账本和资源指纹。只读盘点中两个列全部 NULL，只能支持结构及零值分布证据；不能证明备份或恢复通过。101 的源码和测试通过也不授权执行生产 DDL。

维护与恢复门槛须通过各批实际逐对象恢复及完整启动演练核验：正常窗口 30 分钟，从关闭入口到恢复服务并完成启动核验计时；恢复预算 60 分钟，从作出恢复决定开始计时。所有业务进程、直接写入者及数据库自主写入任务的完整冻结证据缺失时阻断。独立准备的 [离线执行器 PR #360](https://github.com/FangcunMount/qs-server/pull/360) 获批并合并后，先完成 DDL、版本与目标结构只读检查，再明确提交并启动业务 runtime。首个业务 runtime 启动命令发出之前，失败恢复须保持停写、确认 server 侧 DDL／事务／锁已结束，恢复获批对象及原值并核对保留事实后，最后回放实际捕获的完整原版本账本；该命令一旦发出，即使启动失败或尚未 ready，也禁止旧备份覆盖新增事实，只能保留新增事实后前向恢复。当前隔离测试未证明原镜像完整服务启动、生产补丁版本上的恢复或 30／60 分钟及 RPO=0 门槛；不新增生产恢复工具，不以 force 或凭空修改版本元数据代替真实备份恢复。

两条 ALTER 各自提交，不能保证跨表原子性。第二条失败时可能出现第一列已删除、第二列仍存在的 `101 dirty/39 clean`；现有启动保护会拒绝继续，Mongo 不推进。保留失败结构、元数据和日志，由另行审查的向前修复协议处理，或按获批逐对象方案恢复目标结构／原值、核对保留事实，再最后回放完整原版本账本到已声明的 clean pair。禁止 `force`、清 dirty、向下迁移或先 DROP 再 force100 制造成功状态。新增 down 文件以 SQLSTATE `45000` 明确拒绝，不重建空历史列。

`TestHistoricalEvidenceNativeSQLOnlyUpgradeAndRestart` 从真实执行历史资源所得的 clean100/39 升级，核对列/CHECK消失、Mongo元数据不变、业务/冻结报告/现用事件内容不变及无DDL重启。部分DDL测试故障注入第二条ALTER，核对dirty保留、Mongo停止、freshpreflight及force/down拒绝。对象恢复测试在隔离数据库重建原99版对象，恢复合法synthetic非NULL与NULL样本，比较完整SHOW CREATE TABLE、JSON值和CHECK拒绝语义。该演练只证明两个结构对象及synthetic值恢复，未验证生产完整backup pair恢复。

`TestHistoricalEvidenceNativeDirtyObjectBackupRestore` 在真正第二条 ALTER 失败所得的 `101 dirty/39 clean` 上，使用此前捕获的对象结构、完整列值和真实 `schema_migrations` 结构／行备份：对象与保留事实核对通过后，最后在显式事务中回放原账本。错误备份、保留事实变化、真实 CHECK 3819 及账本重复主键 1062 均拒绝提前完成，账本写失败回滚后保留原 dirty 状态；成功后 fresh `100/39` preflight 通过。这是固定测试 seam，未增加生产恢复入口。源码 `e249641f14d48499e211677e6867709a98fbb18b` 在 MySQL 8.0.36／MongoDB 7.0.25 隔离 replica-set 上实际执行四个顶层、五个子场景，共九项 RUN／PASS、零 SKIP；完整证据按 `sql-final-result.json` 摘要 `8c2682c8b8b00a574fca9a9e0b8a1fd477a9239d133c1fa9a34ee5ca6b75e4ae` 登记在既有版本验收台账。该测试没有启动三个业务服务，不能签发生产 7.0.31、完整业务或恢复时间预算验收。

未来版本夹具从嵌入头计算下一个版本，真实101不会与假101相撞。CI要求上述四项 SQL 真实测试和启动保护实际 PASS，禁止 SKIP 或空 selector 通过。SQL-only release 自动 DDL 关闭时必须已经安装其完整101/39。进入本批 Mongo-only release 后完整目标为101/40；声明的旧clean pair只是可升级起点，不能绕过当前完整head校验。


## M4-02 四个历史 Mongo 索引退役（源码准备，生产阻断）

新增 MongoDB `40` 只执行以下四个具名 `dropIndexes`。SQL101 资源在本批不变；SQL101 的 PR #358 独立审查和发布，本批 PR 暂以其分支为 base，须在 SQL 合入实际 main 后重新 retarget、整合并验证最终资源/source digest。当前已整合实际 main `ab998bdcae7c5d15ac518872580ff6eddaafed15` 和 SQL-A 精确 head `73dbb045d47a28873ea209c67feb48ce8fca9567`；已发布的 M2 SHA 为 `4a8642b0`，SQL-A 的 `d1fa6ade` 旧批次人员 CLI 入口退役保持；现代 selected/orphan 入口保留，旧四动作在配置和连接创建前明确拒绝。

| 集合 | 精确索引名 |
| --- | --- |
| `answersheets` | `uk_answersheet_legacy_submission_event_id` |
| `answersheets` | `idx_answersheet_legacy_submission_audit` |
| `report_generations` | `uk_generation_historical_generated_event_id` |
| `report_generations` | `idx_generation_historical_generated_audit` |

本批没有删除集合、历史 BSON 字段、业务文档、现用 `durable_acceptance`／`generated_event_id` 索引或冻结的 schema 3 ReportInput。历史1～39迁移字节不改，down使用明确不受支持的命令拒绝自动重建。索引删除不替代历史责任归档批准：即使历史字段当前不存在，消费者退出、归档 receipt、业务 writer 与自主 TTL 完整冻结、获批的四个完整 live index spec 和原 `schema_migrations` 文档／options／索引备份，以及逐对象恢复仍需独立证据。证据位置或责任人缺失时保持阻断，生产暂停状态不变。

M4-A 先把 `100/39` 升到 `101/39`，独立只读验证后按已批准的 runtime commit 边界启动。M4-B 另开冻结窗口，批准必须绑定实际 clean `101/39`、实际身份、完整资源与 source digest，SQL migrator 回读101且不执行DDL，Mongo只推进40；完成 `101/40` clean 和结构/内容验收后才可启动新业务 runtime。源码接受100/39的历史入口不授权跳过这两批的独立批准。100/40、任意未知/future版本、dirty和未声明组合均拒绝。旧 SQL-only 101/39 镜像会拒绝40库；当前101/40镜像在DDL关闭时也会拒绝尚处101/39的库。

四条索引DDL逐项提交。第三条失败可留下前两索引已删、后两仍在及 `101 clean/40 dirty`；权限拒绝也可能发生在元数据已写dirty之后。保留实际索引清单、元数据和错误证据，不以force、清dirty、down或重建空字段掩盖中断。首个业务 runtime 启动命令尚未发出且停写事实保持时，按获批逐对象方案恢复四个完整索引 spec，核对业务文档、业务集合 UUID／options 和其他索引后，最后回放真实原版本集合备份；也可执行另行审查的向前修复。首个业务 runtime 启动命令一旦发出，禁止旧备份覆盖新增事实。恢复和30/60分钟门槛见[生产恢复与证据](../../../docs/03-基础设施/migration-recovery/20-One-off、恢复与生产证据.md)。独立离线执行器的 PR #360 不混入本批diff；实际执行必须用已合入的执行器和本批资源编译同一批准SHA。

真实隔离测试覆盖 Mongo-only升级/无DDL重启、捕获完整 live index spec 并逐项恢复、unique冲突和partial缺失/非string键约束、第三条失败保留dirty、未知元数据拒绝，以及自有认证replica set中实际Unauthorized13。比较其他索引完整spec、所有business namespace UUID/options、业务BSON与SQL版本元数据；provider自身重建schema_migrations集合的UUID变化通过head/dirty和VerifyComplete另行验证，不冒充业务集合变化。恢复夹具包含 synthetic 非空历史字段和冻结报告，属于对象恢复证明，不是生产完整服务启动或业务验收。新增 `TestHistoricalIndexNativeDirtyObjectBackupRestore` 捕获实际 clean39 的完整原 BSON（含 `_id`、类型及字段顺序）、options／索引与四个 live spec；真正 MongoMigrator40 的固定第三条失败 seam 产生部分 drop 和 dirty40。先核对并恢复索引／保留事实，再在显式 replica-set 事务中替换版本文档；中断或真实重复 `_id` 写失败均保留原 dirty40 BSON，成功后 fresh `101/39` preflight 通过。provider 在39→40过程中已重建版本集合；恢复事务不额外 Drop，保持恢复前后的版本集合 UUID。这不是 force、SetVersion 或凭空清 dirty，也没有新增生产恢复入口。CI要求六项顶层实际 PASS，无 auth 配置、缺依赖或 SKIP 均失败。

2026-10-14 重新整合后的源码 `ffee9817` 在任务独占的 MySQL 8.0.36、MongoDB 7.0.25 plain/auth replica set 上，以 race 执行索引五项、SQL 历史字段三项、版本对四项和关闭 DDL 两项，共十四个顶层用例全部 PASS、无 SKIP；任务自有三容器、网络和认证配置已精确释放。此前已有的本机 NSQ 容器在执行期间继续重启，额外环境稳定性总标志未通过；该观察与迁移用例通过、自有资源释放分别保留，不能声明所有已有容器稳定或 NSQ 验收通过。新版 PR 的 CI、生产逐对象备份恢复、冻结窗口和生产发布继续独立验收。

源码 `94d683c12acca78bc2564c810eae767ed7274674` 的最终事务恢复证明在 MySQL 8.0.36／MongoDB 7.0.25 隔离 replica-set 上实际执行四个顶层及三个恢复子场景，共七项 RUN／PASS、零 SKIP，耗时 32.84 秒；完整原生 JSON 日志摘要为 `735bd63a1aa8775dd6175f7555d2d401f2eb554d2c1ff97a08a48a9212dd7b6e`，结果 `mongo-result-v2.json` 摘要为 `cd7138588ed6be37670b46f8dacba9798e669247f447a67337c90147369a2dc8`，按既有版本验收台账保留。唯一 fixture owner 已精确释放两容器和网络（清理回执摘要 `8c1ee58bd273d1d2d9088697060637727210c1f541b9498950da289382252b89`）；已有容器的一项运行元数据变化原因本轮未诊断，不声明整个旧环境稳定。这轮 7.0.25 证明不单独覆盖 7.0.31；后续同补丁版本证明见下段。原镜像完整启动、生产权限／容量、30／60 分钟或 RPO=0 仍未验收。

随后在 checkout `b7e6e5cff5977abc7c7b85d4269fb428e5d34a91` 上，源码任务仍为 `94d683c12acca78bc2564c810eae767ed7274674`、测试文件前后摘要仍为 `c4c40fac1f276ba0cbef1bdd798e573913ef1fa1574c3f2ad775a94c4e95015f`，使用与生产相同补丁版本的 MongoDB 7.0.31 replica-set 及 MySQL 8.0.36 自有隔离夹具执行同四个顶层／三个子场景：七项 RUN／PASS、零 SKIP、exit0。完整原生日志摘要为 `059bad5d06f4a0773369ea607564e2dca91fddc42372166bcb21186de7511968`，`mongo-result.json` 摘要为 `ed5fac1688f7c23e902190adbd37ab83a51848435e81fa9400743a3ccac2e7d1`，最终 fixture 回执摘要为 `cde6a661a8587ca5354a170099f8ab3c3ea61b8cc63c62b534e9b7f8576657f3`。该回执证明两自有容器／网络已精确释放、原 IDs／images 保留；旧容器运行元数据仍有变化，不签全部旧环境稳定。这补齐了同补丁版本上的 synthetic 对象恢复门禁，不是生产连接、生产规模恢复、原镜像三个业务服务完整启动或 30／60 分钟、RPO=0 验收。
