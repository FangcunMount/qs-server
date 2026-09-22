# M2 qs-server transaction and consumer-boundary proof

Run `scripts/testing/reliable-messaging-proof.sh /absolute/path/to/reliable-messaging` with local Docker. The runner builds a tagged test in a temporary Go workspace, starts its isolated pinned Mongo replica set and MySQL, runs the proof and removes its resources. It accepts no production URI and changes no module files.

Tested against SDK 42fa4c3, qs-server baseline 5573735ae, Mongo 7.0.37. `TestReliableMessagingOriginalMongoRunner` invokes the actual NewMongoRunner and historical eventoutbox.Store. It writes a synthetic business record, original historical event and SDK comparison intent under the original session. The committed case forces a transient callback retry: two callbacks leave exactly one record in each collection; the outer admission slot is acquired/released once. Host rollback and SDK content-conflict cases leave no extra records.

The test injects an unknown top-level extension into historical payload_json, reads ID/type/time without reconstructing the body, rejects ID/type mismatch, and copies original bytes into the SDK intent. SDK claim/confirmation keeps that fingerprint. This proves the transaction and raw-byte compatibility path, not complete AnswerSheet business acceptance, production dual writing, historical claim/mark replacement or token migration.

The original bridge was test-only. The follow-up below adds a host Assessment submission concurrency fix for review; no production deployment, SDK wiring, dependencies or qs-ai execution/recovery files are changed. SDK APIs remain provisional. Original qs-ai candidate/model acceptance remains separately owned.

## Durable failure-hold compatibility

The runner now also starts isolated MySQL and executes `TestReliableMessagingDurableHold` using the exact existing 000050 migration. It exercises the real dispatch settlement handler and mysqlRetryEventHoldStore, with dispatcher pause and ACK failure deliberately injected at their boundaries. ACK observes the committed hold first; redelivery of the same broker message does not reset manual_required, replay count, frozen replay request or original bytes. A real MySQL trigger rejects the next hold: the handler returns an error/NACK instead of acknowledging an unpersisted message. Local proof passed against SDK d13ee10. This is failure-hold compatibility, not Assessment business idempotency or a network-level ACK-loss test.

## Assessment persistence characterization: open consumer gate

`TestReliableMessagingAssessmentPersistence` uses real MySQL 8.0.44, the original Assessment repository, intake service, NewMySQLRunner and historical Outbox. Model validation and current-PO schema construction are explicit fixtures; this is not a full Worker/gRPC/journey, production migration, or SDK consumer acceptance test.

The unique answer-sheet constraint rejects a second creation and lookup retains the original Assessment ID. A real server-side trigger rejecting the event insert rolls back the pending-to-submitted status change. A deterministic barrier after two real pending reads then reproduces a baseline gap: both submissions succeed and persist two different `evaluation.requested` IDs for one Assessment. A later sequential submission is rejected without another event. The passing test characterizes this defect; it does not certify idempotency.

M2 consumer gate remains open. Before QS cutover, protect the business transition with a host-owned conditional write/lock in the same transaction as the event, and prove loser behavior, durable event count, crash recovery and downstream handling. Message-ID deduplication alone cannot collapse two distinct events generated for the same transition. Do not add a generic SDK policy that silently discards legitimate business transitions. The original evaluation executor and qs-ai recovery files remain untouched.

## Follow-up: host atomic submission candidate

The candidate adds a narrow PendingSubmissionRepository capability. The intake finalizer requires it and fails closed if a decorator drops it. MySQL requires the original active transaction, updates only submission status/time and normal update audit fields with `id + pending + not deleted`, and requires exactly one affected row. The same transaction then stages the historical event. A losing competitor returns conflict, and its event is never staged. Existing ordinary Save and other transitions are unchanged. The existing cache decorator explicitly forwards the capability.

The same real-MySQL test now asserts one winner, one conflict, one Assessment and one evaluation.requested. The actual Journey is re-entered twice after submission and returns the original Assessment without another transition/event; its AnswerSheet reader remains an explicit fixture. The trigger-induced event failure still leaves pending and no event, permitting later recovery. The initial reproduction is retained in commit 84a7d02ff, not as an accepted behavior assertion in the fixed test.

This closes the demonstrated submission race in isolation, not the entire M2 consumer gate: full persisted AnswerSheet/Worker/gRPC/NSQ replay, downstream attempt accounting and production acceptance remain outstanding. The change is a host business correction required for reliable delivery, not an SDK-wide deduplication policy. Final approval and deployment remain separate.

## Original durable execution claims

`TestReliableMessagingExecutionClaims` exercises the original runtime_checkpoint repository against real MySQL with a current-PO schema fixture. Six concurrent duplicates cannot acquire an active attempt. Advancing the repository's explicit clock input reclaims the same run/attempt and preserves the frozen input reference; an old token cannot save after ownership transfer. A succeeded run cannot be claimed again. Ordinary or wrong-event replays of a failed run do not create another attempt; only the stored retry event and expected attempt authorize attempt 2, whose replay is again suppressed.

This proves persisted claim and retry-authorization behavior, not model-call idempotency, natural elapsed-time recovery, clock-skew safety or full Worker/gRPC/NSQ replay. No model is called and no execution/recovery implementation is changed. A reclaimed run alone must not be interpreted as permission to resubmit an unknown external operation.

## Broker FIN loss through the original AnswerSheet consumer

`TestReliableMessagingAnswerSheetFINLoss` connects real NSQ through a TCP proxy that drops the first FIN. The SDK NSQ publisher sends the fixed original-style envelope; the actual Worker registry/answersheet handler uses a generated client over loopback gRPC to the actual AssessmentIntakeService, Journey, intake, MySQL repository and historical Outbox. Both broker deliveries reach the service (two observed RPCs), share the same broker message ID and have increasing delivery attempts. Before each successful handler return, real SQL observes exactly one Assessment and one evaluation.requested row. Final status is submitted and the frozen model identity/version remain unchanged.

The test intentionally has no Redis lock, exercising the original degraded-open path so a lock skip cannot masquerade as durable idempotency. The queue/channel is created before publishing to avoid a first-subscription race. The consumer, proxy, gRPC server, producer and isolated containers are drained/removed.

Boundaries remain explicit: the AnswerSheet reader and model validator are fixtures; the wire event is a fixture rather than a new Mongo business submission. Loopback gRPC omits production TLS/auth middleware, and NSQ settlement uses go-nsq directly around the original handler rather than the full production dispatcher. This is a real ACK-loss-to-business-persistence proof, not end-to-end AnswerSheet intake, model execution, unknown provider outcome or production deployment acceptance. Earlier durable-hold tests cover that distinct failure-settlement boundary.
