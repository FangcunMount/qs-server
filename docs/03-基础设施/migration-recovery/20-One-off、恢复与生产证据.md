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

本次兼容链退役经负责人于 2026-10-13 明确批准采用 `backup_integrity_only_v1` 准备方式，取消历史兼容和历史回填；完整恢复演练未完成，已按用户要求取消；随后进一步授权直接删除四对象，再通过最终 R4 普通三服务部署完成发布 B 的双库迁移。例外仅限 MySQL 的 `domain_event_outbox`、`ai_bridge_commands`、`ai_messaging_legacy_commands` 和 MongoDB 的 `domain_event_outbox`，不改变本页其他 one-off 的恢复要求。四对象现已删除，最终 R4 `627fefa44c22c36ef722a3d3d82cfd107b5f72b2` 已随[CD `38323013758`](https://github.com/FangcunMount/qs-server/actions/runs/38323013758) 上线三个服务；实际回读确认 MySQL `100`／MongoDB `39` clean、四对象不存在，六实例版本一致且 ready 通过。

本次两个精确备份已在删后验收通过后销毁，ServerA 54 个固定材料范围已清零：删除 1,064 个文件、63 个目录，11 个范围原已缺失，最终剩余文件为 0。普通备份、此前清理档案和共享父目录排除。源码删除、CI／CD、六实例健康、数据库回读和材料退出分别绑定实际证据；完整恢复演练未完成，已按用户要求取消，不签发恢复成功证明。失败的旧退役运行 `38316933488-1` 不作为成功证明，也不补造旧原生回执；54 范围文件退出不等于所有主机、容器资源或本地临时材料全局清零。临时入口及路由变量已退出，普通部署暂停变量保持原 true；18 个仍有源码入口的工作流和本次 Mac mini 临时设置已恢复，12 个已无源码入口的旧工作流不重建。证据摘要与确切覆盖范围见[退役工具说明](../../../scripts/database/compatibility-retirement.md)。

`cleanup_orphaned_assessment_documents` 当前必须是 blocked：AnswerSheet 与 `answersheet.submitted` Outbox 在同一 Mongo 事务持久化，Assessment 由 worker 异步确保；
“暂时没有 MySQL Assessment”不能证明 AnswerSheet 是孤儿。该工具还未把 durable Outbox 纳入候选保护/备份，apply 会制造新的反向不一致。

这里的 blocked 目前是文档、变更流程和运维授权层面的阻断，不是二进制 fail-closed：源码仍解析并执行 `--apply`、`--hard-delete` 与 `--skip-backup`。在代码级拒绝开关或移除可执行分发完成前，必须继续限制工具制品/生产凭证访问；
任何人都不能把本页或门禁通过解读为“写删路径已在技术上禁用”。

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

Statistics coordinator 等长任务若留下 stale `running` ledger，不能仅靠“不要启动新批次”作为恢复方案；恢复必须定义 stale 判定、owner、重入/标失败规则和缓存/投影复验。
请求 context 已取消时，失败结算还需要独立可用的清理 context，这一代码缺口在修复前保持 gap。

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

## 7. M4 离线退役维护窗口

采用“全部业务进程停止 → 独立迁移 → 独立只读结构检查 → 明确提交 → 首个业务runtime启动”。不为本清理建立长期全局maintenance框架。正常窗口30分钟从入口关闭到重启实际核验；恢复60分钟从恢复决策时刻开始。RPO目标0和时限都必须有实际冻结、恢复和启动演练支持，源码/CI不能签发时间承诺。

普通API初始化会创建Mongo索引和补齐缺失的Bootstrap模板，开放HTTP/gRPC前已启动EventSubsystem、AI MQ relay、Scheduler。Worker初始化启动attention reconciler，订阅消息也不受automatic-retry开关完整控制。关闭DDL、入口代理或部分scheduler不是完整冻结。现有ready主要报告Redis治理/Collection控制同步，不能冒充maintenance检查。

### 7.1 逐对象批次与责任

| 批次 | 目标与保留边界 | 发布前冻结与验收 |
| --- | --- | --- |
| SQL-only101/39 | assessment.historical_lifecycle_evidence、chk_assessment_historical_lifecycle；evaluation_outcome.historical_committed_evidence、chk_outcome_historical_committed | 原SHOW CREATE/CHECK、PK与全量原列值/NULL分布；按列保护manifest和隔离恢复；两对象确实缺失，当前事件证据、Outcome及schema3冻结ReportInput保留 |
| Mongo-only101/40 | answersheets的uk_answersheet_legacy_submission_event_id、idx_answersheet_legacy_submission_audit；report_generations的uk_generation_historical_generated_event_id、idx_generation_historical_generated_audit | 四个实际index spec，包括key顺序/unique/partial/collation；具名索引保护manifest和隔离恢复；其余索引和业务文档不变；不执行legacy_submission_evidence/historical_generated_evidence的全库unset |
| Actor后续SQL | testee.tags、operators.effective_roles | 外部读取退出，按PK保留原值/roles；PO/Mapper/ReadModel/Domain及现代人员恢复写入完整退出，公开EffectiveRoles继续派生，IAM授权事实保留；NOT NULL旧列不能先停写再留结构 |
| Testee维护三表 | testee_store_migration_manifests/items/reversions | manifest/state/hash、candidate/deferred与当前ownership/version/history及补偿义务逐项结清；未结默认保留，空reversions不是删除授权 |
| Operator维护两表 | operator_retirement_manifests、clinician_operator_binding_archive | payload/hash、原绑定、现代退出完成和当前IAM权限/回退义务；未结默认保留。operator_retirement_tasks、operator_recovery_archives和正常历史表继续使用 |
| AI索引回填入口 | qs-ai-bridge runtime-index-backfill、BackfillRuntimeIndexes | 全部受支持环境organization_id NULL为0，Subject/Testee/assessment关联与现用MQ责任完整；退出入口，不删除ai_bridge_requests及正常payload/MQ账本 |
| 临时验收claims | evaluation_acceptance_failure_claims、interpretation_acceptance_failure_claims、qrcode_acceptance_failure_claims | 探针源码及全部运行实例退出、真实失败/重试/重启验收完成；冻结三集合实际文档/spec/身份后独立清除，不删除正常Run/Generation/Outbox责任 |

legacy operatorretire会对assessment/testee等整行算business hash，删除全NULL历史列或空tags也会改变历史投影。先保存原schema、原manifest/AfterHash及对应源码，区分schema改变与业务漂移；不得重写原manifest为“已验证”。历史工具不直接复制到新schema运行，恢复义务只能在具名完整备份与原schema语境验收。Definition派生字段的真实MBTI差异和未结head属于独立M5，默认保留原发布快照、stored assets及冻结报告。

### 7.2 按批准范围保护与隔离恢复

离线批准明确 `protection_scope`、`protection_manifest_sha256` 和 `protection_location`。M4-A 的 `m4_sql_historical_columns` 只允许 `100/39 → 101/39`，M4-B 的 `m4_mongo_historical_indexes` 只允许 `101/39 → 101/40`；各自保护原值/结构或完整索引定义，并完成对应隔离恢复，不把完整双库备份作为两批统一前置。`full_database_pair` 仍保留完整双库保护责任，以下完整备份命令和全库恢复要求适用于该 scope。缺失/未知 scope 或对象 scope 的其他版本组合拒绝；没有旧字段兼容。

所有 scope 仍绑定双库身份/clean版本、完整资源摘要、停写范围、责任、真实恢复和启动回执、30天保留、30分钟维护/60分钟恢复及首个 runtime 启动边界。对象保护不自动授权源库恢复，也不能 force、down 或清 dirty；具体失败恢复步骤必须在批次批准中独立可审查，执行器没有恢复 API。

先确定具名data/recovery owner、受限存放位置、源/目标身份、工具版本、完整业务namespace范围、备份hash/数量、恢复点和30天保留期。服务部署脚本的配置tar包不是数据库备份。当前项目清理不能凭历史空值计数或合成对象重建签发“生产完整恢复成功”；实际备份位置、责任或恢复receipt缺失，写批次保持blocked。

窗口前在独立环境恢复真实生产备份，验证源metadata、所有应用表/集合、DDL/CHECK/索引spec、文档与NULL分布、跨库引用和冻结报告。记录完整恢复耗时不超过3600秒，并正常启动三个服务验证全部构造/初始化路径；使用隔离NSQ/Redis/IAM及受限外发端点。隔离目标身份可以不同，但不得连接生产目标或任意共享库；目标scope逐项确认后才允许写。

下列命令只示范已授权备份/隔离恢复的单库机制，**不是可直接在生产执行的清理脚本**。MySQL采用匹配服务器版本的工具，私有defaults文件作为第一个选项；不带`--databases`以免恢复文件把USE/CREATE DATABASE写回源库。Mongo使用私有`--config`保存敏感连接，固定已审核版本；单库dump不与`--oplog`组合，本方案依靠双库整个窗口已停写建立同一恢复边界。参数依据 [MySQL工具文档](https://dev.mysql.com/doc/refman/8.0/en/mysqldump.html)、[mongodump](https://www.mongodb.com/docs/database-tools/mongodump/)和[mongorestore](https://www.mongodb.com/docs/database-tools/mongorestore/)，实际版本/权限和所有namespace覆盖仍须演练。

```bash
# 仅在完整停写、身份和namespace范围已核实后执行已批准备份。
mysqldump --defaults-extra-file=/restricted/mysql-source.cnf \
  --single-transaction --quick --routines --triggers --events --hex-blob \
  --no-tablespaces --set-gtid-purged=OFF qs > /restricted/batch/mysql.sql
mongodump --config=/restricted/mongo-source.yaml --db=qs \
  --archive=/restricted/batch/mongo.archive.gz --gzip

# 仅恢复到task-owned隔离目标；两个target配置必须已验证不指向生产。
mysql --defaults-extra-file=/restricted/mysql-isolated-target.cnf \
  --database=qs_m4_restore_test < /restricted/batch/mysql.sql
mongorestore --config=/restricted/mongo-isolated-target.yaml \
  --archive=/restricted/batch/mongo.archive.gz --gzip \
  --nsInclude='qs.*' --nsFrom='qs.*' --nsTo='qs_m4_restore_test.*'
```

隔离MySQL目标数据库须预先由该任务分配且为空，隔离Mongo目标namespace也须为空；拒绝静默覆盖，不能用drop清理共享目标。若备份中的view/routine含源库限定名，必须由恢复演练显式核验目标绑定，不能机械替换文本。工具失败、权限缺口、partial archive或未覆盖namespace均阻断。system.profile等非业务诊断对象如需排除，要在manifest具名批准；不放宽生产权限。恢复环境的event scheduler、TTL、队列消费者和外发保持隔离/受控，不让恢复自带的任务抢先写事实。

备份前独立核实导出账号对全部 qs 对象的权限与可见性。MySQL 8.0 的 `--routines` 要求 global SELECT，view/trigger/event 另有对应权限；只读取 information_schema 可见数量、当前直接 grants 或业务读写成功，不能签发完整对象范围。未展开角色或未核查 partial revocations 时，范围继续 unknown；Mongo 也须核对全部 namespace 的 list/read/export 覆盖。缺失权限时使用已审定的备份连接或阻断，不临时扩大生产权限、不以忽略错误生成“完整备份”。

每次实际发布在T0重新冻结：入口关闭并排空在途，全部API/Collection/Worker及维护直接写入者已停止、不会自动重启。另外核查数据库自主任务，包含Mongo TTL及当前或窗口内将到期的文档；进程全停不能暂停TTL。未证明该范围无事实变化即停止，不自行关TTL。保留消息、hold、死信、命令及Outbox责任；不清队列。确认无写入后按已批准 scope 获取最新同窗口保护材料，核对原schema_migrations与源身份，形成新hash/manifest；完整双库 scope 取得完整备份对，对象 scope 取得其具名完整保护范围。旧演练材料不能直接当最新恢复点。

### 7.3 执行、提交与失败边界

使用当前exact-source的 [`qs-migrate-offline`](../../../cmd/qs-migrate-offline/README.md)。只连接所选双库，固定AutoSeed=false，既有Preflight后按SQL→Mongo→VerifyComplete执行；不走普通数据库初始化/业务Container，不自动reconcile额外索引，不启动任何发布者、消费者或调度。执行器默认只读；apply批准独立绑定source、完整资源digest、初始clean pair、身份、目标以及冻结/具名保护/对应scope恢复/隔离启动/责任证据。对缺失位置、owner、receipt、过期窗口或已启动runtime拒绝，批准一次消费。

迁移完成后另行只读检查实际目标对象、完整保留范围和双库身份/版本/dirty，释放执行器连接；全程业务runtime仍停止。正常30分钟包含启动核验，批准预留基于隔离演练的startup时间；到T0+30分钟减预留仍未通过即在提交前作恢复决定，不把DDL用满30分钟后再额外重启。

首个新业务runtime启动命令发出前记录唯一明确commit点。此前失败/超时/dirty：继续停写，确认server侧DDL/事务/锁已结束，保留原始批准和consumed marker，再按已批准 scope 的恢复路径处理：完整双库 scope 恢复完整备份对及对应镜像，对象 scope 使用独立审定的对象恢复或目标schema兼容向前恢复步骤。不得Force、down、改版本或清dirty；仅恢复列或索引不能据此宣称迁移元数据已经恢复clean。SQL第一ALTER成功第二失败、Mongo若干drop完成后失败均可能留下partial状态，退出码或ctx取消不能证明“无改动/服务器已停止”。

首个runtime启动命令发出后，即使没有ready或启动失败，也禁止旧全量备份覆盖：构造器、relay、consumer和外部发送可能已产生事实。停止继续扩散并保留新增事实，用相同目标schema兼容镜像/补丁向前恢复。完整备份仅作取证或受控提取，不回滚覆盖当前事实；该阶段的60分钟恢复目标须有实际向前恢复演练。

SQL-only目标101/39仅增加明确的101/39版本对；生产验收后才进入Mongo-only101/40，增加101/40，floor100/39与原资源字节不变。100/38、新进程未知/dirty/任意版本组合不接管；关闭DDL仍要求达到该镜像完整目标。SQL批次旧备份100/39配M3镜像，Mongo批次旧备份101/39配SQL101镜像；旧M3不能新启动连接101/39，SQL101不能新启动连接101/40，旧进程健康不是回滚兼容证明。

每批生产验收后至少保留30天材料，未结责任延期；到期独立人工复核销毁，不新增自动清理。source/tests/CI、备份恢复、部署六实例、结构回读、业务接受及材料退出分别记入现有版本/生产台账；本节流程和隔离测试不改变当前生产未完成状态。
