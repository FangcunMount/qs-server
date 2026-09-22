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
