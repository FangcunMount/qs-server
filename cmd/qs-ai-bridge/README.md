# QS AI messaging runtime and participant operation reads

Runtime commands and events use one MQ Outbox, event subscriber and relay at process startup. The legacy command scanner, staging/retry/settlement SQL and direct runtime gRPC write adapters are retired. The normal module does not export the legacy Results receiver; an explicitly registered compatibility receiver still authenticates mTLS and permanently returns FailedPrecondition without persistence or acknowledgement. Query/governance gRPC and read-only payload retrieval remain available. Disabling MQ cannot reactivate any old writer.

## Assembly and lifecycle

`ai_workflow.messaging.enabled` selects the host `MessagingCommandStore`. Management evaluation Start/Cancel and participant Retry use the same store. The normal bootstrap calls `StartAIWorkflowRelay`; MQ startup validates required database tables and all three business and failure Topic/Channel pairs before connecting. Key loading is explicit local bootstrap work. Constructors do not connect, migrate, or schedule.

`ai_workflow.messaging.nsqd` maps fixed TCP addresses to their corresponding fixed HTTP stats endpoints. Signing and encryption are separate P-256 JWK files. `signing_key_file` and `ai_recipient_key_file` select outbound keys; `decrypt_key_files` and `ai_signer_files` are local kid-to-file trust maps. Old decryption keys remain configured while historical messages may need them. No remote key retrieval or caller-provided endpoint is accepted. Runtime topology preflight is read-only; provision channels before startup.

The host owns one 1-second scan loop, batch 20 and concurrency 1 per subscriber. Broker publication does not decide a command or confirm a result. Shutdown stops relay admission and consumer RDY, waits for committed active handlers, then drains publisher resources before closing the shared governance connection. The module uses a 20-second stop budget. A failed drain reports an error and does not close borrowed host resources or permit another runtime start.

## Durable receiving and technical failure

Authentication precedes body reads and business writes. Inbox, original projection/command decision and final confirmation Outbox commit in one host transaction; only commit permits FIN. Duplicate events rearm the original immutable confirmation. Original raw wire hashes are retained; SDK failure reencoding is used only to authenticate the signed message, never as the original wire hash.

Migration 092 adds the host-owned logical failure ledger and Inbox outcome. The eight-attempt technical budget survives republishing and process/object recreation. A valid event whose budget is exhausted may commit a TECHNICALLY_HELD confirmation without applying a business projection. Physical NSQ attempts and failed-handoff claims cannot invent that logical budget or apply a business effect. Malformed/untrusted inputs enter raw-wire quarantine; failed storage leaves physical delivery unconfirmed. Oversized quarantine inputs retain the full hash with an empty stored wire, not a truncated hash.

## Public operation authorization

Public Start returns 202 `submitted`, original request/operation/command IDs and a fixed local status URL. AI acceptance/rejection is a later durable decision. The public GET `/api/v1/interpretation/ai-workflow/operations/{command_id}` requires `testee_id`, `assessment_id` and `request_id`.

The collection service uses the existing delegated read-only `GetAIWorkflow` RPC with additive `command_id` and `operation` fields; original field numbers and legacy read behavior remain intact. This reuses the established collection workload ACL. The API rechecks the current ProfileLink and assessment, the original request actor/org/testee/assessment, and the command's original request aggregate. Internal OrgAdmin operation queries are not exposed to participants. Closing new intake retains reads and idempotent queries of already committed requests.

## Validation limits and production gate

Local tests cover real MySQL 8.0.36/8.4 and NSQ 1.3.0 at the 262144-byte limit, original-wire retention, duplicate confirmation, lifecycle recreation, cancellation, transaction rollback, persistent technical budget and participant scope. Lifecycle recreation is not a process-kill proof. Original Go M0–M6 acceptance fixtures are not changed or executed by this batch.

Existing isolated evidence covers separate-process command/receipt/final-ACK loss after PUB, Broker and both-process kills, a 128 KiB protected reference through real mTLS, and real browser original-intent behavior against a synthetic HTTP backend. The process helper proves the original transport runtime, not a full QS host. Required remainder: full normal QS host health and each command family's accepted/refused/duplicate chain, reviewed historical inventory and apply tool, genuinely compatible MQ version rollback, final assets and production natural business samples. See the independent R1–R7 execution ledger; none of these tests is production acceptance.

The production review must explicitly include the read-only `/qsai.workflow.v1.MessagePayloads/Get` permission for the qs-ai workload, host key/network/topology mounts and schema ownership. Actual production ACL/configuration readiness must be verified independently; local mTLS service tests do not prove it. Host main merge/automatic deployment requires separate review. SDK prerelease approval is not host production authorization.

## Historical command handoff boundary

Migration 093 adds immutable source evidence. `MessagingLegacyHandoff.StageSingle` borrows an active maintenance transaction, after both relays and new intake have stopped. It validates the first legacy typed payload/hash and original request binding, stages one new immutable wire and operation, and records original JSON bytes/business hash/retry state in that same transaction. It never changes legacy `delivered`, request version/status, frozen evidence or source payload. Nullable perimeter indexes may be filled only from the verified immutable request; unknown creation time remains unknown.

Old Start identity uses request_id; old answer/cancel identities use command_id. Known UTC request creation time is represented in UTC+8 with original precision. A Change has no stored original submission time, so its envelope retains an empty value instead of using retry available_at or the migration clock. Previous attempts and retry availability are carried forward; an exhausted historical budget is technically held and does not receive new retries. Already delivered history is not requeued. Duplicate handoff reuses first wire with no resealing.

An aggregate with multiple unowned pending commands is refused: the old schema does not retain sufficient immutable commit ordering. Such rows stay untouched in the maintenance inventory until an explicit ordering/evidence disposition is reviewed. This API therefore does not claim universal historical migration completion. The maintenance transaction must roll back on any error; the API does not commit, create a transaction or close resources. The runtime no longer contains the old scanner; switching off MQ is not a rollback to old gRPC writes.

## 切换前只读数据库清单

`cmd/qs-ai-messaging-audit` 是维护核查工具，不是消息服务。仅借用显式 DSN 建立宿主拥有的连接，用 MySQL READ ONLY / REPEATABLE READ 事务执行固定 SELECT；不加载密钥、不连接 NSQ、不记录正文、不执行迁移/移交/重投/清理或模型调用。二进制由独立 AI bridge CI 构建并记录源码 SHA。

```sh
# 凭据在操作环境中安全设置，不把实际 DSN 写进材料或命令参数。
qs-ai-messaging-audit -limit 1000 > mq-database-inventory.json
```

连接从 `QS_AI_MESSAGING_AUDIT_DSN` 获取；构建时通过 `-ldflags '-X main.sourceSHA=<精确提交>'` 固定来源。输出含原 command/request ID、已存正文 hash、原重试预算/UTC available_at、移交关联、新 Outbox 状态统计、Inbox/隔离/失败账本数量。available_at 与展示排序不充当业务提交顺序。多个未移交待投命令的同聚合标为 `unknown_commit_order_requires_review`；单条也只标为待原事务验证，不自动宣称可安全移交。

移交后旧 command.delivered 不复用为归属标志；核查区分旧来源与新 MQ 记录。关联须匹配首次 source hash/kind/request 与新正文 hash，否则保持未验证。delivered 历史不重置。样本截断明确为 incomplete，必须补齐清单后审核。缺 MQ 表显示为未安装；表存在不能证明迁移 head、Broker 拓扑、密钥、权限或业务就绪。所有这些仍需独立现场证据。

真实 MySQL 门禁验证数据库强制拒绝只读事务中的 UPDATE、原记录/预算不变、宿主池仍可用、历史与未知顺序如实呈现、移交 hash 冲突不冒充有效归属、缺 schema 与截断均不能当成完整清单。测试仅创建和删除带随机 ID 的专属一次性数据库。

## 统一运行类维护门禁

迁移094只新增宿主数据库单行 `ai_messaging_admission`，初始 **closed=true / revision=0**。MQ 正常启动要求表和行存在，但门禁关闭不停止已提交消息的 Relay、接收或 ACK。配置 `ai_workflow.enabled` 的旧参与者入口开关不能代替该统一维护门禁。

Start、Change、参与者 Retry、评测 Start/Cancel 在同一条 `MessagingCommandStore.stageCommand` 中，先借原提交事务取得门禁共享行锁，再核对不可变原操作。新操作仅在开放时继续原业务写入、操作与 Outbox；锁保持至原事务提交/回滚。关闭者持有同一行的排他锁，因此成功提交关闭后，先进入的提交已经结束，后来的新操作不能越过门禁。没有缓存、第二个调度器或独立服务。

关闭后的同身份/内容/组织/操作人/资源/聚合/消息族操作返回原操作，不执行今天的版本/状态验证或重封装；当前调用权限仍在应用入口重检。内容/主体冲突依然拒绝。查询不受门禁影响。新操作明确 HTTP429：`AI runtime command admission is closed for maintenance; operation was not submitted`，无操作或Outbox；该关闭原因与 AI 容量/版本业务拒绝不同。公共同步委托 RPC 保留原通道，将固定关闭原因映射 ResourceExhausted 再由 collection 返回429，无每日容量 Retry-After。DB不可用、缺表/行或锁等待取消都保持不确定错误，不能冒充确定未提交。

新增宿主维护工具 `cmd/qs-ai-messaging-control`，显式环境 `QS_AI_MESSAGING_CONTROL_DSN`，动作 `-action inspect|close|open`；变更必须传 `-expected-revision <已审核当前版本>`。inspect 用只读事务；close/open 用宿主事务，30秒有界等待，成功输出仅在提交后。旧revision拒绝，失败输出不含DSN/底层错误；遇不确定提交结果先inspect，不能猜测关闭已完成或重复开门。工具只操作门禁，不安装schema、不迁移、不连Broker、不触发模型。audit继续永久只读，无公开维护HTTPAPI。

生产开门仍需完整审核和现场 prerequisites。本批候选开发不授权主线合并/自动部署/切换，不执行down迁移。真实风险测试覆盖五族关闭/原幂等wire和时间、当前投影变化后复用原Change、关闭等待MySQL实际共享锁后原提交/回滚、取消/缺行无孤儿记录、旧revision及宿主池继续可用。

## 正常宿主配置解码

正常启动使用 Viper，而隔离进程 helper 直接读取 JSON。Viper 会把 `127.0.0.1:4150` 和 `qs.encrypt.v1` 内的点拆成配置树；直接解码到字符串映射会失败。MQ 选项仅对 NSQD、解密信任和 AI 签名信任三种本地映射接收该树，在宿主 `Options.Complete` 中恢复完整字面键，再沿原 Validate/KeyID/目标检查。不改变通用配置加载器、SDK Go 接口或 wire，不读取密钥或连接网络；直接 JSON / 构造选项保持原映射形态。非字符串叶值、保留的空分支及歧义拒绝，不将数字或列表转为可信密钥路径。Viper丢弃的空映射保持空值，仅兼容禁用配置；启用时沿原完整性校验拒绝，不能产生可信端点。沿现有 Viper 规则使用一致的小写本地 kid，并与 JWK 的固定 kid 精确一致。

正常镜像启动与配置加载的回归必须独立于 helper 证据；配置失败、IAM 关闭的部分探针以及镜像静态检查都不能关闭完整宿主权限验收。

## 宿主连接字符集与原 Unicode 正文

当前 component-base 宿主池协商 `utf8`（MySQL 8 对应 utf8mb3），而合法结果含四字节 Unicode。原 JSON 参数写入会报转换错误，普通 JSON 查询还会把这类字符转换成问号；只用驱动默认 utf8mb4 的测试连接不能覆盖该风险。

消息外围对已有请求、历史命令和结果投影的 JSON 参数显式使用 `CONVERT(CAST(? AS BINARY) USING utf8mb4)`，对 JSON 查询、历史移交与索引回填使用二进制结果读取。原 JSON 的 UTF-8 字节在进入数据库 JSON 解释前不经过连接字符集转换，读取仍由原业务解码器处理。首次业务 hash、protobuf body、签名和 wire 不重建；JSON 数据库自身的表示规范化规则不改变。没有 SET NAMES、额外池、连接配置或 schema 修改。

必要存储回归必须显式协商宿主的三字节字符集并检查未被适配器更改，覆盖含 emoji/补充平面字符的原接单与历史回答、合法128KiB结果、原hash、重复复用ACK、ACK失败时投影与Inbox共同回滚。正常镜像中失败到技术挂起的旧隔离样本保留原身份、wire与预算；修复验证使用独立合成样本，不重置挂起样本或授予新模型调用。

## 持久状态观测

MQ运行Start成功时在原Prometheus注册器登记pull collector，完成Stop后注销；构造/Describe不读库、不注册、不连接网络，没有observer任务或第二个调度器。MQ关闭不注册此命名空间，不能把不存在当零积压。每次scrape借用宿主原池，2秒总context/每SQL1秒、REPEATABLE READ+READ ONLY，读取后rollback，不领取/提交/确认/关闭池。

固定qs_server_ai_mq_* gauges区分staged（含未来重试）、due（不是按序可领取）、awaiting_receipt（等待业务决定）、Outboxheld与Inboxheld；创建年龄clamp未来到0。四种quarantine保留分类记录为gauge，不把attempts的8次上限称终生counter。所有查询不读身份、正文、wire或密钥，注册器与renderer只认固定描述，无高基数标签；存储失败只给observation_available=0，不输出部分/旧值或空队列。

迁移095增加固定技术事实账和记录起点。重复接收事实随原Inbox/ACK事务提交，正文读取失败沿独立有界技术记录事务提交；均不改变业务投影或重试授权。observations_available仅在完整记录设施存在时为1，缺表或旧设施保持不可用，读取错误不冒充零。历史覆盖标志与记录起点分别展示，不从新计数推断升级前历史。正常metrics与生命周期注册/注销必须分别取证；collector存在不等于业务已确认。

定向真实MySQL测试使用随机独立测试schema，仅创建同结构状态表，不启动消息消费者/模型。验证未提交不可见、提交/rollback、writer持锁时reader不等待/不结算、未来时间clamp、技术保留记录是gauge、后续查询缺表不泄漏已读部分值和宿主池可用。原实际NSQ运行/停止/借用池回归保护新增注册生命周期，不重复GoM0～M6套件。

## 隔离 CLI 与正式入口

`qs-ai-bridge` 仅保留原历史投影读取与 runtime-index-backfill 维护，不提供用户认证。stage-start、stage-change、relay、receive 在借用数据库或传输资源前永久拒绝；隔离消息故障夹具由独立 testing 工具承载。正常消息生命周期由 qs-apiserver 宿主装配；迁移完成后不能把旧 CLI relay 当作生产降级写路径。原业务契约继续来自 `api/grpc/proto/aiworkflow/workflow.proto`，新增消息契约来自 `messaging.proto`。维护清单、门禁与移交分别由同镜像中的 `qs-ai-messaging-audit`、`qs-ai-messaging-control`、`qs-ai-messaging-handoff` 执行，默认服务入口不改变。

## MQ 发布绑定与离线预检

正常 qs-apiserver 镜像现在包含 audit/control/handoff/preflight 四个维护入口，默认入口仍为 qs-apiserver。`qs-ai-messaging-preflight` 显式运行：`--source-sha` 仅输出完整构建提交；`--binding` 校验固定 `/run/qs-server-jose` 角色、kid、P-256、公私钥、用途及权限；可同时指定 `--base-config` 和 `--output-config`，只替换 ai_workflow.messaging，拒绝覆盖已存在文件，不读取环境凭证，不连接数据库、Broker 或模型。

首发由受控操作准备 `/data/infra/qs-server-messaging/current.json` 的非秘密绑定描述及 `versions/<binding_revision>/` 下的独立密钥。密钥和描述不进入仓库、镜像或工作流变量。原角色 kid 可无版本后缀，轮换保留旧解密／验签映射，不重封装历史 wire。

发布包携带 `scripts/cd/ai-messaging-release.py`；仅 apiserver 目标在绑定存在或已建立 MQ required 标记时使用。在共享部署锁内先拉取固定镜像、以该镜像非 root 用户离线预检，再同步原配置和停止旧容器。配置、原绑定、公开指纹、精确镜像 ID 及 Compose 覆盖文件保留在 `/opt/qs-server/qs-apiserver/ai-mq-releases/<完整源码SHA>/`；覆盖文件固定镜像 ID、MQ 配置文件和逐文件只读密钥挂载，其他服务及原 TLS／凭证装配沿用宿主原配置。

相同版本或回退复用其冻结绑定，当前指针不覆盖已有版本。原源码配置、镜像、冻结文件或密钥指纹漂移拒绝；缺密钥或旧不兼容镜像在停止旧实例前失败。建立 required 标记后不恢复旧 gRPC 写路径。保留的 MQ 版本镜像通过既有 `--protect-image-id` 机制保护，未增加清理策略。维护窗口、schema、关闭准入、数据移交和真实业务对账由独立切换流程完成，离线预检及容器健康均不能代替它们。

## 最终 ACK 失败预算

当前固定 SDK `v0.3.0-mq-ai.2`（源码 `8633ba585b5a1c8f95fb16b19469fbd0097611c7`）。只对无需业务回执的最终 ACK，成功 PUB 不再消耗技术失败预算；Unknown 仍计入原持久预算，重复事件不能解除已有 held 状态。接收与 Relay 存储回归覆盖两次未知后连续16次成功重发、首次 wire/正文/时间及单次业务投影保留，以及8次失败挂起后的重复接收不能解挂。

旧 ACK 计数可能包含成功发布，补丁无法推断历史失败次数，不自动归零、解挂或重封装。需要按原事实治理历史挂起；技术重发不会授权模型调用。既有 Go subscriber、relay、transport/wire、接口和黄金样例与 alpha1 相同（只读源码差异核对）；不代签 M0～M6 业务验收。宿主正常镜像、兼容回退与生产对账单独取证。
