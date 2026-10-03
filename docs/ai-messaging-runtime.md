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
