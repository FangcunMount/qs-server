# QS AI messaging runtime and participant operation reads

The reviewed MQ mode chooses one Outbox, event subscriber and relay at process startup. It does not start the legacy command scanner or register the legacy Results receiver. Query/governance gRPC and read-only payload retrieval remain available. The mode is a cutover configuration, not an automatic failure fallback or authorization to run two writers.

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

Still required: additive migration/history handoff, separate-process dual-end fault/lost-receipt/lost-final-confirmation/Broker-loss tests, large referenced bodies through the real mTLS endpoints, real browser behavior, final normal images, compatible MQ rollback, and production business samples.

The production review must explicitly include the read-only `/qsai.workflow.v1.MessagePayloads/Get` permission for the qs-ai workload, host key/network/topology mounts and schema ownership. Existing production ACL/configuration has not been edited; local mTLS service tests do not prove production ACL readiness. Host main merge/automatic deployment requires separate review. SDK prerelease approval is not host production authorization.

## Historical command handoff boundary

Migration 093 adds immutable source evidence. `MessagingLegacyHandoff.StageSingle` borrows an active maintenance transaction, after both relays and new intake have stopped. It validates the first legacy typed payload/hash and original request binding, stages one new immutable wire and operation, and records original JSON bytes/business hash/retry state in that same transaction. It never changes legacy `delivered`, request version/status, frozen evidence or source payload. Nullable perimeter indexes may be filled only from the verified immutable request; unknown creation time remains unknown.

Old Start identity uses request_id; old answer/cancel identities use command_id. Known UTC request creation time is represented in UTC+8 with original precision. A Change has no stored original submission time, so its envelope retains an empty value instead of using retry available_at or the migration clock. Previous attempts and retry availability are carried forward; an exhausted historical budget is technically held and does not receive new retries. Already delivered history is not requeued. Duplicate handoff reuses first wire with no resealing.

An aggregate with multiple unowned pending commands is refused: the old schema does not retain sufficient immutable commit ordering. Such rows stay untouched in the maintenance inventory until an explicit ordering/evidence disposition is reviewed. This API therefore does not claim universal historical migration completion. The maintenance transaction must roll back on any error; the API does not commit, create a transaction or close resources. Compatible runtime ownership disables the old scanner; switching off MQ is not a rollback to old gRPC writes.

## 切换前只读数据库清单

`cmd/qs-ai-messaging-audit` 是维护核查工具，不是消息服务。仅借用显式 DSN 建立宿主拥有的连接，用 MySQL READ ONLY / REPEATABLE READ 事务执行固定 SELECT；不加载密钥、不连接 NSQ、不记录正文、不执行迁移/移交/重投/清理或模型调用。二进制由独立 AI bridge CI 构建并记录源码 SHA。

```sh
# 凭据在操作环境中安全设置，不把实际 DSN 写进材料或命令参数。
qs-ai-messaging-audit -limit 1000 > mq-database-inventory.json
```

连接从 `QS_AI_MESSAGING_AUDIT_DSN` 获取；构建时通过 `-ldflags '-X main.sourceSHA=<精确提交>'` 固定来源。输出含原 command/request ID、已存正文 hash、原重试预算/UTC available_at、移交关联、新 Outbox 状态统计、Inbox/隔离/失败账本数量。available_at 与展示排序不充当业务提交顺序。多个未移交待投命令的同聚合标为 `unknown_commit_order_requires_review`；单条也只标为待原事务验证，不自动宣称可安全移交。

移交后旧 command.delivered 不复用为归属标志；核查区分旧来源与新 MQ 记录。关联须匹配首次 source hash/kind/request 与新正文 hash，否则保持未验证。delivered 历史不重置。样本截断明确为 incomplete，必须补齐清单后审核。缺 MQ 表显示为未安装；表存在不能证明迁移 head、Broker 拓扑、密钥、权限或业务就绪。所有这些仍需独立现场证据。

真实 MySQL 门禁验证数据库强制拒绝只读事务中的 UPDATE、原记录/预算不变、宿主池仍可用、历史与未知顺序如实呈现、移交 hash 冲突不冒充有效归属、缺 schema 与截断均不能当成完整清单。测试仅创建和删除带随机 ID 的专属一次性数据库。
