# Reviewed legacy AI command handoff

This maintenance executable never starts a relay, publishes, calls AI, installs
schema or opens admission. `qs-ai-messaging-audit` remains permanently read-only.

Before apply, close the persistent command gate and verify **all old and new
delivery claimers have stopped**. The explicit CLI confirmation records operator
attestation; it cannot observe processes or prove that they have stopped.
Do not derive original command ordering from retry time. Any unresolved
multi-command legacy aggregate prevents apply, even outside the selected batch.

Bind `QS_AI_MESSAGING_HANDOFF_DSN` to the intended host database. Keep credentials
out of CLI arguments/output. The database must have a clean schema head at least
95, the closed admission row and existing messaging tables.

1. Write an explicit JSON array of **1–20 canonical command UUIDs** to `ids.json`.
   `-action dry-run -input ids.json` collects one database-enforced READ ONLY
   snapshot. It emits `manifest` and its canonical `manifest_sha256`, without
   source bodies or keys. Copy only the `manifest` object to `manifest.json`.
2. Review database/head/gate revision, every original identity, both business
   hashes and byte digests, delivered status, attempts and original times. Missing original time
   stays null. Resolve any `unknown_order_aggregates` before apply.
3. Supply a locally reviewed JSON `AIWorkflowMessagingOptions` document through
   `-key-options keys.json`. It names the same fixed local JOSE key files and
   trust mappings as the host; never include key contents in review material.
4. Run `-action apply -input manifest.json -reviewed-sha256 DIGEST
   -key-options keys.json -confirm-relays-stopped` only after the above checks.

Apply rechecks the database, schema head, closed gate revision, global ambiguous
aggregate count and exact source under locks **for each independent original
transaction**. It delegates transfer to `MessagingLegacyHandoff.StageSingle`;
original source rows, identities and budgets are retained. Source drift stops
the batch immediately. Earlier rows may already have committed: retain the
partial JSON report and resolve `commit_unknown` by querying/reapplying the
**same** manifest. Repeated transfer reuses persisted wire. Delivered historical
rows are retained without resetting them. The tool does not infer later rows
were applied merely because an earlier row committed.

This tool handles legacy QS `ai_bridge_commands`. AI `result_outbox` transfer,
unknown model state inventory, two-end migration/fault evidence and production
cutover remain separate host requirements; this tool does not establish them.
