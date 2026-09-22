# M2 original qs-server Mongo Runner proof

Run `scripts/testing/reliable-messaging-proof.sh /absolute/path/to/reliable-messaging` with local Docker. The runner builds a tagged test in a temporary Go workspace, starts only its isolated pinned Mongo replica set, runs the proof and removes its resources. It accepts no production URI and changes no module files.

Tested against SDK 42fa4c3, qs-server baseline 5573735ae, Mongo 7.0.37. `TestReliableMessagingOriginalMongoRunner` invokes the actual NewMongoRunner and historical eventoutbox.Store. It writes a synthetic business record, original historical event and SDK comparison intent under the original session. The committed case forces a transient callback retry: two callbacks leave exactly one record in each collection; the outer admission slot is acquired/released once. Host rollback and SDK content-conflict cases leave no extra records.

The test injects an unknown top-level extension into historical payload_json, reads ID/type/time without reconstructing the body, rejects ID/type mismatch, and copies original bytes into the SDK intent. SDK claim/confirmation keeps that fingerprint. This proves the transaction and raw-byte compatibility path, not complete AnswerSheet business acceptance, production dual writing, historical claim/mark replacement or token migration.

Only a tagged proof and isolated runner are added. No execution/recovery file, production wiring or dependencies are changed. SDK APIs remain provisional. Original qs-ai candidate/model acceptance remains separately owned.
