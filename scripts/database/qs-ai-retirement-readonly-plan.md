# 八个旧 START 的 qs-ai 只读责任核验

本方案可用真实数据库逐页读取全部保留账本，替代有条数上限的运行详情。当前观察器已经实现固定上界、完整源行摘要、两次有界扫描和部分责任诊断；**尚不能签发旧命令全责已关闭的结论**。没有连接生产，没有修改 qs-ai，没有发送消息、回填证据或删除数据。

分析基于本机 `/Users/yangshujie/workspace/python/src/github.com/fangcun-mount/qs-ai` 的源码。以下路径、行号均相对该仓库；部署 SHA、真实生产库名、生产头版本、生产行和冻结状态仍须独立绑定，不能由本机代码推断。

## 真实所有者和入口

- `src/qs_ai/bootstrap/providers/core.py:51`：应用拥有 `Database`。`src/qs_ai/bootstrap/messaging.py:99` 把同一个 Database 交给 MQ，参与者执行与 MQ 使用同一 MySQL 库和连接池。
- `src/qs_ai/infrastructure/persistence/mysql/database.py:49,93,123`：Database 管理引擎；Transactions 管理连接与根事务；BorrowedTransactions 校验真实原事务。观察器只接受宿主已开启的 `AsyncSession` 根事务，不 Begin/Commit/Rollback、不关闭借来的连接或池；只关闭自己消费的查询结果。
- `.github/workflows/deploy.yml:100,122` 的 production 已有 Secret 名称为 `MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_DATABASE`、`MYSQL_DBNAME`、`MYSQL_USERNAME`、`MYSQL_PASSWORD`。`scripts/cd/deploy.py:50` 要求 DATABASE/DBNAME 同时存在时相同，将其编码成 `QS_AI_DATABASE_URL`；`src/qs_ai/config.py:195,203` 使用 SecretStr。此处没有读取这些值，也没有证明 qs-server 的连接就是 qs-ai 的连接。
- `src/qs_ai/maintenance/messaging_audit.py:187` 已有维护环境变量 `QS_AI_MESSAGING_AUDIT_DATABASE_URL`；这是源码入口名称，不代表已经存在相应 GitHub Secret。后续宿主可用 qs-ai 自己的受控工作流注入经过独立批准的只读连接；不能让操作者从日志复制 URL，不能用管理 UI 的摘要冒充数据库凭据或身份。
- `src/qs_ai/infrastructure/persistence/mysql/runtime.py:49,117` 使用 RR 快照，但详情截断 Run/结果为 100、诊断里程碑为 300，且明确 history_complete=false。本方案直接读取真实账本，不调用这些摘要作为完整性证明。

## 固定读取集合与关联

观察器 `qs-ai-retirement-readonly-observer.py` 的 `SPECS` 是可执行的闭合集合：完整列名、真实主键和取数模板都在此处，拒绝多列、少列、PK/类型/排序变化、非 InnoDB。扫描时不加组织过滤，避免将错组织或孤儿排除成零。

| 表 | 实际 PK | 本批责任与需要核验的关联 |
|---|---|---|
| external_requests | request_id | 原 request → 唯一 session；START 原 command_id 必须等于 request_id |
| interpretation_sessions | id | org/subject/testee/assessment、终态、当前 run/version、evidence/config 归属 |
| idempotency_requests | scope_hash,key | 原请求哈希和原首个 Receipt；空 response 不能当成功 |
| interpretation_runs | id | 此 session 全部历史 run，不能只取 active_run |
| execution_jobs | id | run 唯一 job、session、queued/leased/done/dead/cancelled、fence、原 answer/question |
| model_calls | run_id | 唯一 invocation、原 frozen request、response、failed/dispatched/unknown、fence |
| clarifications | id | session/question_seq、原问题与答复、answered_by/at、后续 job.question_id |
| evidence_sets | id | session 唯一冻结证据、schema、fingerprint、全部 assessment/testee/report/source_version |
| interpretation_artifacts | id | session/run 唯一 Artifact、原 evidence/invocation/provider result/content fingerprint |
| execution_configurations | session_id | 原接受 publication/sha/pointer/evidence，而不是当前 latest 配置 |
| participant_capacity_reservations | run_id | session/org/subject/assessment、active/acquired/released、原 quota_snapshot |
| participant_retries | organization_id,command_id | 原 retry/request/session/source_run→new_run、expected_version、operator、原 receipt、冻结请求与明确风险批准 |
| execution_leases | thread_id | thread=session；当前 fence/expiry 只是执行租约，不是 provider 终态证明 |
| result_outbox | event_id | 原 event/session/version 全部状态结果、delivered/mq_owned/attempts；禁止生成替代 ID |
| ai_messaging_outbox | producer,destination,message_id | 原 body/wire/hash、kind/org/aggregate/sequence、stage/attempts/confirmed_at |
| ai_messaging_inbox | producer,message_id | 原 command/body hash、processing/accepted/rejected/held、首个 receipt_id |
| ai_messaging_quarantine | wire_sha256 | 物理失败 wire、可信 logical identity（可能 NULL）、累计本地失败预算；不能按 org JOIN 忽略 |
| runtime_milestones | session_id,dedupe_key | 保留诊断、run/invocation 反向关联；有 TTL，不能证明全历史缺失或关闭 |

schema 定义在 `src/qs_ai/infrastructure/persistence/mysql/schema.py:10,62,73,91,117,128,138,146,371,500,520,656`；MQ 实际 DDL 在 `migrations/versions/0037_workflow_messaging.py:25`。当下本机支持的 Alembic 头为 `0038_messaging_observations`，原型拒绝其他头；生产若不同，先绑定并审查对应版本，不能把配置写成当前头以绕过校验。

18 表不是整个数据库的完整业务目录。完整上线核验还须读取全库物理目录和全部列/PK/索引元数据，绑定目录 SHA 并逐个分类；未知新增账本阻断。当前 `schema.py` 声明 48 张业务表，另有 MQ 4 张和 `ai_messaging_observations`（0038），以及 Alembic 版本表；这些源码数字不冒充生产实体清单。

参与者 START 的执行 claim 就在 execution_jobs + execution_leases；模型 unknown 就在 model_calls，没有另一张“Participant unknown 表”。Report 图在 `src/qs_ai/infrastructure/workflows/report.py:59` 使用 `graph.compile()`，没有接入独立 LangGraph checkpoint 数据库，不能凭 checkpoint_ref 假设另一套已审计账本。

评测是一条独立责任链：`evaluation_runs`（definition/progress 中包含创建 receipt、取消/恢复/人工决策）、`evaluation_checkpoints`、`evaluation_run_policies`、`evaluation_dispatches`、`evaluation_generation_completions`、`evaluation_semantic_completions`、`evaluation_slot_claims`、`evaluation_response_receipts`、`evaluation_capacity_reservations` 与 `ai_messaging_evaluation_sequences`。定义为 `schema.py:228–331,477,716`、0037 DDL；unknown 和人工 resolution 存于完成证据及 progress，不是独立 unknown 表（`evaluation_resolution_evidence.py:45`、`evaluation_unknowns.py:23`）。它们不能被 JOIN 到 interpretation_runs 或套用 Participant 终态。全局 MQ 出现评测类型时，必须按这条真实链证明其属于另一个已知责任域；关联到本批 START 原 ID、错组织、缺运行记录或未能分类时阻断。原型暂未实现该域的完整语义验证，回执明确保留缺口。

## 执行查询与两阶段批准

宿主使用专门受控只读账户，在真实 session 上设置 REPEATABLE READ 并执行：

```sql
START TRANSACTION WITH CONSISTENT SNAPSHOT, READ ONLY;
SELECT CAST(@@server_uuid AS BINARY), CAST(DATABASE() AS BINARY),
       VERSION(), @@transaction_isolation;
SELECT version_num FROM alembic_version ORDER BY version_num;
SELECT TABLE_NAME, ENGINE, TABLE_TYPE
FROM information_schema.tables WHERE table_schema=DATABASE() ORDER BY TABLE_NAME;
```

数据库身份按与 qs-server inventory 一致的 `mysql_database_identity_v1`/server_uuid/selected namespace 三项 NULL-aware 长度框架 SHA 绑定。真实名称、UUID、原业务 ID 留在私有批准链；公开只输出身份摘要、预期源码 SHA 和 schema 头。`information_schema` 完整可见和 InnoDB 是前提。宿主同时绑定实际运行镜像/源码；原型只核对调用者批准的 source_sha，**未独立观察 qs-ai 运行版本**。

每张表先读取六项列元数据、真实 PK 和 SHOW CREATE。对应实际模板如下（`:table` 仅用于元数据；取数标识符只能来自闭合常量）：

```sql
SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME
FROM information_schema.columns
WHERE table_schema=DATABASE() AND table_name=:table ORDER BY ORDINAL_POSITION;
SELECT COLUMN_NAME FROM information_schema.statistics
WHERE table_schema=DATABASE() AND table_name=:table AND index_name='PRIMARY'
ORDER BY SEQ_IN_INDEX;
SHOW CREATE TABLE `external_requests`;
SELECT CAST(request_id AS BINARY) FROM external_requests ORDER BY request_id DESC LIMIT 1;
SELECT CAST(organization_id AS BINARY),CAST(command_id AS BINARY)
FROM participant_retries ORDER BY organization_id DESC,command_id DESC LIMIT 1;
```

Discovery 的私有 bound 包含 18 表全部元数据/DDL 摘要、带类型的 PK upper（present-empty 必须显式 NULL）、identity/head/source 与固定预算。独立审阅并批准 canonical bound 文件的原 SHA 后，另一次调用 `observe(..., approved_bounds_sha256=...)` 才能扫描。发现上界不是批准，也不能把 descriptor hash 当 bound hash。调用者不得任意更改预算：page=1000、每表最多 1,000,000 行/2 GiB 源字节、保留最小图编码总量最多 256 MiB、单查询 30 秒、整个扫描 1500 秒。图上限是最小事实 canonical 编码长度，不冒充 Python RSS 上限。超限、超时、缺表、未知 schema 均不输出 complete=true。

分页是完整 PK 的字典序，不是 OFFSET；SQL 绑定参数按实际列类型还原，BIGINT 9/10 用整数比较，UUID/text 用实际 binary collation。观察器由已批准 metadata 为 18 表生成**全部实际列**的 CAST AS BINARY，禁止 SELECT 投影代替完整源摘要。两张表的完整模板如下：

```sql
SELECT CAST(request_id AS BINARY),CAST(session_id AS BINARY)
FROM external_requests WHERE request_id<=:upper AND request_id>:last
ORDER BY request_id LIMIT :page;
-- 首页省略 last 条件；present-empty 也实际查询一次，非零即拒绝。
SELECT CAST(organization_id AS BINARY),CAST(command_id AS BINARY),
 CAST(session_id AS BINARY),CAST(request_id AS BINARY),CAST(source_run_id AS BINARY),
 CAST(run_id AS BINARY),CAST(operator_user_id AS BINARY),CAST(expected_version AS BINARY),
 CAST(reason AS BINARY),CAST(accepted_unknown_risk AS BINARY),
 CAST(source_failure_code AS BINARY),CAST(frozen_request_json AS BINARY),
 CAST(receipt AS BINARY),CAST(created_at AS BINARY)
FROM participant_retries
WHERE (organization_id,command_id)<=(:upper_org,:upper_id)
  AND (organization_id,command_id)>(:last_org,:last_id)
ORDER BY organization_id,command_id LIMIT :page;
```

每页在相同 RR 内先读原 PK + 所有列 `COALESCE(OCTET_LENGTH(CAST(column AS BINARY)),0)` 之和，先判剩余行/byte预算，再 fetch 完整 bytes；完整页的原 PK/实际 byte 长度必须与预检逐行一致。因此超大 MEDIUMBLOB 页在原文 fetch 前拒绝。所有完整列的 SELECT 返回原 bytes，NULL 与空串用存在位+8 字节大端长度区分，顺序加入 SHA；这是 `mysql_cast_binary_source_columns/v1` 源摘要，**不是**历史作者 JSON.Marshal/qs-ai fingerprint/SDK wire fingerprint。JSON 使用 MySQL SELECT 后确切 bytes；验证原作者算法时必须独立 strict decode + 已知 canonical 算法，不能拿 source SHA 顶替。DDL、列元数据、PK 类型和页计数都绑定，二扫后再核对 schema。正文只在受控内存里即时验证/摘要，不写入长期报告。

两次扫描必须在同一已批准 upper 内、同一个 RR 快照里完全相同。宿主结束原事务后，才允许借用另一个新快照检查各表 PK>upper（原 empty 表查任意行）。这会标记 next_cycle_required，不会继续旧文件或自动批准新 bound。它只发现上界之后的新行，无法证明较小 UUID 新增/原行修改不存在；最终还必须停写后对完整源 hash/业务记录重核。原型不替宿主 COMMIT/ROLLBACK，不自行续跑失败 checkpoint。

## 精确本地闭环验证

私有批准目标恰为八个 START，字段仅 command_id=request_id、session_id、organization_id、subject_id、testee_id、assessment_ids、可信 decoder 计算的 qs_ai_request_hash 和完整原源行 SHA；禁止 imported complete:true。原 IDs 必须 canonical UUID、业务 ID 为非零 uint64，八个原命令/session 均唯一。公开回执仅逐目标 reference_sha256/source_row_sha256。

1. **原接受事实**：external_requests、session 和 global scope=`fingerprint(["qs-server","external-start-v1"])` 的 idempotency 行恰各一条。request_hash 用 `service.py:178` 的真实 Python算法：UTF-8、sort_keys、compact JSON，对 `[asdict(actor),testee,assessment_tuple,goal]+asdict(evidence_items)` 求 SHA。原 Receipt 的 session/run/status/version 与可信开始事实对照；这里不能用 qs-server 的 payload_hash，也不能用后来生成的新 request。零行、跨 org/subject/testee、另一 session 或不支持的历史 writer 版本阻断。
2. **全部运行责任**：按 session 收集所有 interpretation_runs，按 run 双向匹配 job/call；还从全表反查无 Run 的 job/call、job.run 的 session 不同、无 session 的子行。没有 `LIMIT 100`。原开始 receipt.run、每次答题和 retry 链必须能解释每个非初始 run，拒绝未关联分支、环、重复 source/new_run 或跨组织。
3. **Provider责任**：`execution.py:73` 先提交 dispatched 再调用 HTTP，崩溃前后不可分辨，禁止用租约过期、job dead/cancelled、session cancelled、accepted_unknown_risk 把 dispatched/unknown 变成功。response_received 必须 strict `JSONModelCallCodec.decode_request/decode_response` 验证原 schema、invocation/fence/provider request。failed 必须已知确定失败，技术未知仍阻断。当前原型只验状态/响应存在；完整 codec/历史 schema 分支尚未实现。
4. **终态与原产物**：completed 要按 `execution.py:462` 验 session/run/evidence/invocation/provider request/content fingerprint；发布快照按 `execution_configurations.validate_generation` 与 `build_artifact` 精确重建，使用原冻结配置和响应，禁止最新配置/再次调用模型。cancelled 需证明没有活动执行或 unknown。原 admission blocked/no job/no call 只能在已知确定拒绝和 qs-server 对应原关闭回执成立后归类；普通 blocked 不自动等于全责关闭。答卷/clarification 要核原答复、answered_by/at 与 successor job.question_id，不能只看最新 session。
5. **预算和租约**：每个 reservation 完整匹配 org/subject/assessment/run；active=1 阻断。lease fence 与 job/call 原 token 对照，expiry 不是关闭结论。`participant_retries.py:156` 的原 command/org、source→new run、原 receipt/frozen_request、expected_version/人工批准逐条核对。承认 unknown risk 只是新调用授权，不能抹掉原 provider unknown。预算/attempt 保留真实发生值，不补造未发生 attempt。
6. **原结果**：result_outbox 全历史版本、StateEvent UUID/request/session/actor/testee/status/version 全部核对。completed event 的 artifact_json 必须等于已证原 Artifact；question/failure 的实际 payload 与对应业务事实匹配。delivered 是旧 gRPC/MQ 投递状态，不能证明 qs-server 完成接受和业务关闭。mq_owned 要与真实原 MQ wire/预算继承匹配；不得因 transferred/delivered 推断结束。

反向检查必须用无组织过滤的全量快照或下列 LEFT JOIN 条件，并保留固定 PK 分页；不能只拿已知 ID 正向 JOIN：

```sql
SELECT j.id FROM execution_jobs j
LEFT JOIN interpretation_runs r ON r.id=j.run_id
LEFT JOIN interpretation_sessions s ON s.id=j.session_id
WHERE r.id IS NULL OR s.id IS NULL OR r.session_id<>j.session_id;
SELECT c.run_id FROM model_calls c
LEFT JOIN interpretation_runs r ON r.id=c.run_id WHERE r.id IS NULL;
SELECT p.organization_id,p.command_id FROM participant_retries p
LEFT JOIN interpretation_sessions s ON s.id=p.session_id
LEFT JOIN interpretation_runs src ON src.id=p.source_run_id
LEFT JOIN interpretation_runs dst ON dst.id=p.run_id
LEFT JOIN external_requests er ON er.request_id=p.request_id
WHERE s.id IS NULL OR src.id IS NULL OR dst.id IS NULL OR er.request_id IS NULL
 OR s.org_id<>p.organization_id OR src.session_id<>p.session_id
 OR dst.session_id<>p.session_id OR er.session_id<>p.session_id;
```

原发布快照依赖的 `configuration_publications`、其 evaluated Run、`evaluation_suites`、profile/prompt/route/schema/semantic assets 和原 quota JSON 必须通过固定 PK 点读+source hash 验证（`execution_configurations.py:74`、`asset_snapshot.py`）。不会因为 18 表扫描完成宣称这些依赖也已验证；这也是原型的明确缺口。

## MQ、反向孤儿和跨服务闭环

真实 routes/types 为 START、CHANGE、PARTICIPANT_RETRY、EVALUATION_START、EVALUATION_CANCEL、COMMAND_RECEIPT、INTERPRETATION_STATE、EVALUATION_STATE、EVENT_ACKNOWLEDGEMENT（`infrastructure/workflow_transport/messaging.py:23,57`）。每个保留 Outbox/Inbox/body/wire 都应验证两层 wire + 原 PB：secure metadata、schema、producer/destination/topic、original message UUID、aggregate、org、body_length/SHA、物理 wire SHA；真实历史 keys/可信 signer 由宿主只读提供。原型只能 PB+存储 SHA 校验，未解密/验签，不能冒充完整消息校验。

- 以原 START/CHANGE/retry ID、request、session 及 nested receipt/event refs 的并集关联，不能只按 aggregate 或 organization_id 过滤。每个 command Inbox 必须有唯一 first receipt Outbox，receipt 原 command_id/body SHA/decision/WorkflowReceipt 完全匹配；processing、held、缺原 receipt、冲突或外部 unknown 均阻断。
- 每个 interpretation result/event 必须有相同原 event UUID 的 Outbox，并证明原 body 与 result_outbox typed StateEvent 一致；original attempt 继承由 `state_events.py:60` 的真实 handoff 链判定，不统一置零、不补发。ordered/sequence/requires_receipt 按实际 AI 合约核，不套 qs-server 标准 domain rm_outbox 格式。
- 所有 MQ 行再反向映射到可信本地/peer 业务根；受支持但其它域必须证明独立归属，不允许用 JOIN 缺行当 out-of-scope。未解释跨组织、错误 request、重复 body、孤儿 original ID 阻断。ai_messaging_quarantine 对 NULL logical identity 不可能安全按组织过滤，必须先验证原/failure wire，再证明是本批闭合重复或属于另一个可信责任域；不能拿 failure attempts 当业务决策。
- `mq_receiver.py:209` 的 ACK 消费 authenticate→confirm_event→commit **没有保存 ACK Inbox/原 wire**；`mysql/messaging.py:298` 用原 ACK eventID/kind/bodySHA/aggregate 才确认。必须点读 qs-server 的原接收 Inbox/bodySHA、原业务 receipt/operation/projection、原 ACK Outbox/body+wire，核对 STORED 与同一 original event。qs-ai confirmed 和 qs-server Broker published 都不能单独替代这条 peer 链。没有原 ACK 的直接可重验材料时，记录“仅现有核验事实/原认证材料缺失”，不得声称可重验原 ACK 全文。
- DB 扫描也不证明 NSQ command/ACK 及其既有失败 Topic/Channel 无待处理。`bootstrap/messaging.py:133` 使用既有 FailureTopology、订阅本地预算 max_attempts=8。后续受控只读 GET 每个已冻结 nsqd HTTP endpoint 的 `/stats?format=json`，严格核既有 Topic/Channel、depth/backend_depth/in_flight/deferred、failure 责任；绝不创建订阅、读取/FIN/REQ 消息或清队列。非零无法用 stats 获得原 ID，必须保持 unknown，不能新开消费者“验证”。只有在全局零待处理且所有已知 durable logical责任闭合，或已有可信逐原ID peer事实证明安全重复时，协调层才可判断退出。

最终核验需短维护窗口内同时停止本批相关 writer/worker/relay/Inbox admission/handoff/legacy source 入口，等待正在执行的事务结束，再取得两服务精确只读快照、全 source/业务 binding 重核与 broker 已冻结拓扑观察。两个库不能天然同一事务；不冻结跨服务写入时，分别 RR 读到的一致行不足以证明原命令全责结束。观察器不实施 fence，也不会自动把变动的边界更新为批准值。

## 最小回执、临时材料和原型边界

公开回执只允许 protocol/operation/run/source、DB identity hash/schema head、目标 source/reference hash、每个固定表 rows/bytes/pages/source SHA/upper-token SHA、实际核验项版本和固定结果类别。完整未来回执还应绑定 peer receipt SHA、broker observation SHA、writer fence SHA、fresh recheck SHA 和全目录 SHA。不得包含库名、原 IDs、正文、配置、模型输入/输出、URL、wire 或任意异常文本。未知或矛盾保持 blocked/unknown；不接受外部 JSON 里的 complete/drop_ready 字段。

原型暴露 `discover_bounds(session,...)`、`observe(session,bounds,approved_bounds_sha256=...)`、`fresh_after_upper(new_session,observation)`；`PrivateObservation.facts(exact_eight_targets)` 只返回 partial diagnostic。公开始终 `business_retirement_proven=false`、`drop_ready=false`、`peer_receipts_verified=false`、`fence_verified=false`。RR 隔离和真实根事务可以检查；`READ ONLY / WITH CONSISTENT SNAPSHOT` 的实际设置由宿主负责，原型明确未独立观察该服务端模式。早期观察器没有 SDK JOSE 验签能力；新增完整核验器复用真实 SDK，历史 wire 的可信 keys 仍须由宿主独立绑定。

原型不保存 source 正文文件，只有受控内存中的当前页和最小图引用/摘要。未来受控宿主若必须生成 bound/checkpoint/源临时副本，应先注册 owned 私有路径，用 O_EXCL/NOFOLLOW/0600+fsync，不覆盖既存不同文件；只允许 hash 引用和明确审批链，不从日志导回正文。故障时保留真实 checkpoint 供核对，不自行续写旧 source。按本批已批准策略，临时回滚材料在最终删除验收通过后销毁；这不让缺少原证明的 unknown 自动变为成功，也不保留长期正文归档。

本地验证使用 qs-ai 实际 SQLAlchemy metadata + 0037 原 DDL，在根授权且先核验 CID/image/labels/volume/127.0.0.1:34306 的 owned MySQL 中创建独立随机 schema，最终清理自己的 schema。30 个离线测试通过（另5个 native 默认跳过），5 个显式 owned-native 测试通过。离线覆盖固定集、NULL source digest、Python 原 fingerprint、PB/hash/内外 kind、八目标批准、孤儿/跨 org/unknown/held、分页、empty、预检 row/byte/图预算、隐私和 host ownership；原生覆盖真实18表、RR read-only/根事务、1001行分页、BIGINT 9/10 复合排序、同 RR 并发新增不可见、结束后 fresh above-upper、schema/head/identity/nested、非事务引擎/非 binary PK、deadline/query timeout拒绝。通过不等于生产观察。

完成 typed 产物/配置/历史 writer、全反向 MQ/retry/独立评测归属、peer 原接受/ACK、真实 frozen broker、最终 fence/recheck 之前，**此原型的通过只证明有限扫描机制，不计为八个旧命令已核验退出**。

新增的 `qs-ai-retirement-readonly-verifier.py` 已实现 53 个 qs-ai 账本与 14 个 qs-server 协作表的全主键、固定上界扫描，核对原请求、执行版本、模型调用、产物、原接受和 MQ/ACK 事实，并在宿主结束旧快照后，使用独立新事务重读完整 source 与业务摘要。原命令范围来自完整旧表扫描，未硬编码为八个 ID；旧 gRPC 历史缺少原 wire 时记录缺口，不补造当前协议消息。观察器仍仅承担扫描基础，不得单独作为业务闭环证明。

当前本地独立回归为 61 项规则／真实 SDK JOSE 测试及 14 项 owned MySQL 原生测试通过；包括完整脚本及其观察器依赖。核验器不创建连接或接管宿主事务，不发送、修复、回填或退休命令。生产 qs-ai 库身份／schema／部署来源、历史可信 keys、NSQ 冻结拓扑、跨服务写入隔离与最终批准尚未绑定；公开结果保持 `production_proof=false`、`retirement_proven=false`、`drop_ready=false`。这些本地结果不证明生产八条旧命令已经核验或退役。
