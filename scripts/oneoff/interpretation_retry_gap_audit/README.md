# Interpretation retry gap audit

This one-off, read-only command scans a **fixed Run ID range** for due automatic report retries. It reports whether the original retry intent was confirmed and whether a later Run exists. `suspected_post_confirm_gap` is a question for per-ID investigation, **not** permission to replay the event or call a model. Reads span several Mongo collections without a snapshot transaction; an operator must recheck the current Generation, Run, Outbox, Worker, and external-result state before any separate recovery action.

Build and run with a Mongo read-only identity. Pass a fixed upper **Run** ID captured before the scan and a cutoff at least ten minutes old. A page with `complete=false` exits 3; continue from `next_after_id` with the same upper ID and cutoff. A complete page with `suspected_post_confirm_gap`, `relay_pending_or_unknown`, or `manual_required` exits 2; other complete pages exit 0. `--max-rows` is limited to 1000 and the overall timeout to five minutes.

```sh
go run ./scripts/oneoff/interpretation_retry_gap_audit \
  --connections-stdin --after-id 0 --upper-id "$RUN_UPPER_ID" \
  --due-before 2026-09-30T20:00:00+08:00 --max-rows 500 \
  < private-connection-json
```

`private-connection-json` has `mongo_uri` and `mongo_db` keys. Alternatively, set `MONGO_URI`/`MONGO_DB`. A QS API runtime environment can provide `QS_APISERVER_MONGODB_HOST`, `QS_APISERVER_MONGODB_USERNAME`, `QS_APISERVER_MONGODB_PASSWORD`, and `QS_APISERVER_MONGODB_DATABASE`; the URI is then built in process, without putting it on the command line. Never post connection values or `--include-ids` output to public CI logs. Public output contains only counts, cutoff and completion state.

The query requires the existing Run ID and `(generation_id,attempt)` indexes, the Generation ID index, and a leading `message_id` Outbox index. It does not write to Mongo, publish NSQ messages, change service configuration, or start an InterpretationRun. A zero-finding result is meaningful only for the exact completed ID range and cutoff supplied.
