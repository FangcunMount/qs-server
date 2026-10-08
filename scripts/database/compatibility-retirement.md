# Exact compatibility retirement: A-stage execution barrier

This is the foundation for the accepted four-object operation, not an enabled
production cleanup. `prepare` has real read-only identity discovery and a bounded
source inventory adapter; both exit 42 with diagnostic receipts. Discovery is
never automatic approval of an observed database identity.
`apply`, `verify`, `recover` and `purge` remain unavailable and exit 42.
Neither a JSON boolean nor
a workflow input can enable a missing backend. Do not dispatch this foundation
as proof that a production preparation/deletion/recovery/purge was performed.

The exact scope is MySQL `domain_event_outbox`, `ai_bridge_commands`,
`ai_messaging_legacy_commands`, and MongoDB `domain_event_outbox`. The prior
22 CBPT objects, their private archives, normal Mongo backups and whole-db
restores are outside this scope. No new DROP migration is included in A.

## Implemented and tested

- CD, all persistent/operational db-ops, the production AuthZ provisioner and this workflow share
  `production-deploy`, with cancellation of an active run disabled. Only the two
  exact metadata profiles and bounded metadata `inventory` retain their separate
  group. This does not retroactively change old workflow revisions.
- The new workflow rejects unknown fields/operations, another database scope,
  another ref, a stale main SHA and another manifest hash before its production
  job. Only exact `prepare` receives existing `MYSQL_METADATA_ADMIN_USERNAME`,
  `MYSQL_METADATA_ADMIN_PASSWORD`, `MYSQL_HOST/PORT/DATABASE` (with existing
  `MYSQL_DBNAME` fallback) and `MONGODB_HOST/PORT/USERNAME/PASSWORD/DBNAME`
  production Secrets. No server vault variable is guessed. It deploys nothing.
  A future B deployment
  must reuse prepared release scripts inside the same run, never dispatch CD
  and wait while occupying its lock.
- A private 0700 operation directory under
  `/opt/backups/qs-server/compatibility-retirement/<operation-id>` owns its
  immutable 0600 `inventory-request.json` first, then a separately approved
  `manifest.json` after real preparation. IDs use numeric `run-attempt` format.
  Manifest/evidence reads reject symlinks, hard links, duplicate JSON keys,
  malformed types, unknown fields, source/operation/target mismatches, dirty
  heads, metadata estimates and expired evidence. No secrets or raw records are
  accepted in the manifest protocol or emitted in receipts.
- Evidence references bind separate inventory, history, production fence,
  backup/restore and prepared-release proofs by filename and SHA256. Production
  acceptance is a separate proof required by the future purge adapter. Imported
  summaries are structurally validated but are not treated as trusted live
  retirement verification. The separate inventory adapter does not remove the
  frozen-manifest/history/fence/backup/release capability barrier.
- History distinguishes verified live, verified retired and unverifiable closed
  history. The last class is allowed only through the future business verifier
  proving terminal ownership with no unsettled/replay responsibility. Unknown
  execution, ambiguity, hash conflict, incomplete retirement references and
  unexplained high findings block preparation.
- A fsync-before-DDL journal binds all four targets and the immutable manifest.
  An interrupted `intent`/`unknown` cannot start another DROP. Only a new live
  read may reconcile it to `dropped`; incomplete/mixed/restored journals do not
  permit purge. The journal requires an operation lock and is not called by the
  current mutation CLI. Preparation writes only operation-owned private source
  inventory files; it issues no INSERT/UPDATE/DELETE/DDL/migration commands.
- Deadline helpers stop forward work at 20 minutes and all recovery work at
  30 minutes. These helpers alone do not guarantee recovery time; readiness
  requires a measured and independently verified rollback of at most 10 minutes.

## Required implementation before execution can become ready

1. The implemented inventory adapter must be extended by an actual retirement
   verifier that compares current facts to the frozen manifest and independently
   resolves dependency/business findings. The existing metadata inventory alone
   cannot prove emptiness. The new source-byte inventory is still not DROP-ready.
2. Actual business history producers must save minimum retirement references and
   conclusions to existing business records, without old bodies/permanent legacy
   tables. Evidence coverage must be complete and validate closed unverifiable
   records independently; no generic imported count authorizes deletion.
3. A live production-fence verifier must snapshot and validate original workflow
   state, pause variable including unset, environment ref/protection rules and
   all queued/running/waiting jobs. Disabling a workflow or allowing main alone
   does not prove that old historical main runs cannot rerun before credentials.
   An unproved historical-rerun boundary denies the maintenance window.
4. A dedicated backup/isolated-restore backend must dump only these four objects
   into this operation's private assets, perform real restores in owned isolated
   instances with no production network/ports, compare complete contents and
   schema/index/options, and register every raw copy/restore resource. It must
   never call the CBPT 22-target tool or whole-db Mongo `--drop db.*` restore.
5. The actual DDL/recover backend must recheck frozen identities/digests before
   each exact statement, durably write intent first, and stop on unknown results.
   Recovery restores only missing targets and never reactivates a legacy writer,
   rewinds migration heads or overwrites a current business database.
6. Prepared B and safe rollback artifacts/configs must be fully built, verified
   and preloaded before the window. B's runtime logic remains A's; B migrations
   and the exact terminal Mongo adapter are a separately coordinated batch.
   Existing `migration.enabled=false` emergency configuration is needed for a
   dirty-head stop. Image construction cannot occur inside the window.
7. The live acceptance/purge adapters must recheck B deployment, exact target
   absence, clean heads, non-target schema, retirement references and standard
   behavior. Only then remove registered assets/restore resources of this batch
   and any temporary key, verify zero leftovers and retain safe technical
   receipts. No older CBPT/ordinary backup or unrelated Docker asset is pruned.

The current capability barrier must be replaced only alongside real adapters
and independent integration tests. Removing the false capability bits, adding
an `--allow` switch, accepting an externally supplied `complete: true`, or
returning success for an unsupported stage is not a valid implementation.

## Real read-only preparation contract

`prepare_mode=identity` requires only `identity_request_sha256`;
`prepare_mode=inventory` requires only `inventory_request_sha256`. Both require an
empty `manifest_sha256`. Other stages require the immutable manifest hash and
empty request hashes. Unknown/mixed classes, another operation,
another SHA or another target set are rejected before connections are opened.

Identity bootstrap does not require the operator to read a GitHub SSH Secret or
locally connect to production. After reviewing the approved main tooling SHA and
numeric operation ID, the operator computes the fixed request bytes with
`identity_request_bytes(operation_id, source_sha)` and SHA256 locally. The
request is sorted compact ASCII JSON followed by one newline, kind
`readonly_identity_discovery_request`, the exact target/scope, protocols
`mysql_database_identity_v1` and `mongodb_database_identity_v1`, and fixed limits
`query_seconds=15`, `total_seconds=90`. It contains no expected identity, migration
head, credential, database name or observed value. In the pinned production
Action, identity mode checks that independently provided hash before creating
anything, atomically creates the SSH-user-owned 0700 operation directory and
0600 `identity-request.json`, and verifies identical contents before connecting.
Existing files are never overwritten; a mismatch fails. Only missing fixed
ancestors under `/opt/backups/qs-server/compatibility-retirement` may be created
via noninteractive sudo, then assigned to the current SSH user. Existing owners,
permissions or inaccessible ancestors are not repaired. Such a host path issue
requires an independently controlled setup, not weaker path validation.

Discovery reads actual identity snapshots, MySQL 8/Mongo 7 versions, actual migration
head/dirty flags and necessary narrow metadata visibility. Its receipt is
`diagnostic_only:true`, `drop_ready:false`, `complete:false` at the orchestrator
level, even when `identity_discovery_complete:true`. Root must independently bind
the receipt to exact source/run/attempt/job, pinned host, image ID and network,
review actual clean heads and identity protocol, then separately approve the
inventory identity/head request. No discovery-to-inventory builder or automatic
DROP authorization exists.
`metadata_permissions_sufficient` in discovery is explicitly scoped by
`permission_scope=identity_and_migration_head`. Mongo identity/head/connectionStatus
success does not prove full-catalog/system.profile/listIndexes visibility; the
separate source inventory must actually perform those reads. Runtime image/network
receipt fields record inspected image ID and selected fixed network configuration,
not a container ID. Timeout leaves actual container ownership unknown until live
read-only inspection binds ID, labels, image and mounts before any removal/retry.

Discovery also counts the complete four targets by SQL DCE event_type/status,
AI bridge kind/delivered, AI legacy source_kind, and Mongo DCE event_type/status.
These are full GROUP BY/$group counts with 15 seconds per query and at most 128
distinct buckets per target, never samples or payload/ID scans. Missing targets
are explicit absence; timeout, wrong namespace/schema, missing fields or denied
permission remain incomplete/unknown. Known labels use exact source-grounded
enum strings (current nine, retired fourteen, historical five, six pre-delivery
guard diagnostic candidates, plus bridge kinds); unknown label values are only
hashes. This classification does not prove historical business ownership or
terminal/replay responsibility. Public receipts allow at most 128 total buckets,
flattened into four pages of at most 32; excess keeps headers incomplete and
retains only private count/hash diagnostics. It does not authorize cleanup.

The pre-provisioned private request has `format_version: 1`,
`kind: readonly_inventory_request`, approved `operation_id`/`source_sha`,
`target_hash` of the ordered four-object whitelist, `database_scope:
mysql-and-mongodb`, exact `identity_hashes.mysql/mongodb`,
`expected_migrations.mysql/mongodb`, and these immutable limits:
`query_seconds=15`, `total_seconds=180`, `max_records=100000`,
`max_bytes=134217728`. Record/byte limits are per target, so total source bytes
are bounded by four times that byte limit. Exceeding a bound produces incomplete
inventory; there is no automatic pagination/resume or limit override.
The prior metadata estimate of 651,765 SQL DCE rows is neither fresh nor exact,
but exceeds this source-inventory cap. This adapter must report incomplete for
such a dataset. It has no request-bound fixed SQL ID/BSON upper bound or paged
production traversal. Equal whole scans and a successful diagnostic histogram
do not satisfy those missing production preparation requirements.

Identity protocol `mysql_database_identity_v1` hashes actual `@@server_uuid`
and selected `DATABASE()` with unambiguous length framing. Protocol
`mongodb_database_identity_v1` hashes actual hello `setName/hosts/me`, the selected
database name and that database's `schema_migrations` UUID. A standalone server
therefore still requires an observed migration-collection UUID; hostname alone is never accepted.
The pinned Mongo migration driver drops and recreates `schema_migrations` when
setting a version, so this UUID is an A-head snapshot anchor, not an immutable
Mongo database identity across B migrations. Record and bind the pre/post
migration UUIDs and heads separately. A genuinely new empty database has no such
UUID and needs an explicit pristine-catalog bootstrap proof before migration;
discovery cannot authorize that bootstrap or weaken identity checks.
The embedded binary SHA must equal the approved source before DB credentials are
written to a private temporary environment file. Secrets never enter argv,
stdout, public artifacts or the raw source inventory.

The adapter owns and closes its connections. MySQL uses a repeatable-read,
read-only transaction, exact primary key (`id` or `command_id`), ordered full
columns expressed as `CAST(column AS BINARY)`, typed null/length frames and exact
source-byte SHA256. This is MySQL's selected binary column representation,
including its JSON representation, rather than a claim to physical disk bytes.
Raw columns are stored as base64 in private NDJSON. Mongo stores the original
server BSON documents, ordered by `_id`, as length-framed binary. It performs
two complete equal content scans and re-observes schema/UUID/index/options.
That Mongo procedure detects observed changes; it does not claim an atomic
cross-database snapshot or prove that writers are fenced.

Exact four namespace types, clean expected migration heads, grants/privileges,
catalog, non-target schema definitions and indexes are recorded. Missing targets
are explicit `present:false` observations, so partial/all absence is supported.
MySQL requires direct complete global SELECT/SHOW VIEW/TRIGGER/EVENT visibility
without partial revocations; role names or restricted metadata estimates do not
substitute for that proof. Full Mongo listCollections/listIndexes/privilege reads
must succeed. An inaccessible `system.profile` index therefore keeps metadata
incomplete; the tool never treats permission-denied as absence. Dependency text
review remains required even after a successful metadata read; dynamic SQL or
external/manual writers cannot be proved absent by this adapter.
The current SQL foreign-key count covers selected-schema outbound metadata only
and compares referenced schema as well as target name. `dependency_scope` says so
and `dependency_coverage_complete:false` remains explicit: cross-schema inbound
keys and dynamic/external dependencies require a later exact verifier. A complete
catalog is never a complete dependency/business deletion proof.

Each producer run owns a new 0700 `inventory-<run-attempt>` below its private
operation directory. Existing outputs are never overwritten. Source files and
metadata/report JSON are 0600. The armored receipt contains only technical
counts, hashes, observed head/identity flags and `inventory_complete`; it always
keeps `complete:false`, `execution_allowed:false`. It binds the exact private
report file hash and does not infer business terminal status from published or
delivered transport flags. Temporary credentials are removed independently of
inventory retention; inventory payload copies must be registered for the later
batch-specific purge alongside the temporary backups. A timed-out container is
left with exact operation/run/source/request labels for read-only reconciliation,
not blindly removed or retried.

`compatibility-retirement-entrypoints.json` is the reviewed **source-only**
12-workflow inventory for the later fence verifier. Besides CD/db-ops it includes
the writing AuthZ provisioner, approved read-only diagnostics with privileged
SSH, and isolated registry-backed image tests. M5 preflight/postcheck and Ping
Runner jobs have no production environment: environment ref policy alone cannot
fence their repository credential path. Current ordinary Nginx verification is
read-only; its shared script's install/reload mode is separately called by CD.
Host cron/manual tasks, other repositories and obsolete source reruns remain
unproven. The pre-A independent metadata snapshot reported 27 remote workflows
against 18 then-current source workflows, with nine active historical workflows absent
from that source baseline listed explicitly as unproved. These counts are dated
observations; adding this workflow or merging A requires a fresh live inventory. Organization Secrets coverage,
historical rerun credential denial and production ref protection are not inferred
from that metadata or the source catalog. The final fence must deny all historical credential paths before the
maintenance window, not simply cancel current CD/db-ops runs.

Local validation commands are `python3 -B scripts/database/test-compatibility-retirement.py`,
`go test ./cmd/qs-compatibility-retirement`, and
`python3 -B scripts/database/compatibility-retirement-integration.py`. The last
command creates only uniquely labeled, loopback-only disposable MySQL 8/Mongo 7
containers, refuses remote test endpoints, and removes only its own test containers
and anonymous volumes. It tests all-present/partial-absence/all-absence and actual
dirty-head, wrong-identity, wrong-kind and partial-permission refusal, identity
bootstrap protocol equality, dirty SQL/Mongo head and absent Mongo UUID refusal,
exact full diagnostic counts, private unknown labels and excess bucket refusal. It is never
a production preparation receipt.

Historical label evidence: retired footprint eight at
`66b00e2e1617f1187065a2a2b1f291ca366d02a7:configs/events.yaml:95-151`, retired AI
six at `04ac40d3ab87b8c3e53f816c5d8dc64bc99ed247:configs/events.yaml:107-147`,
historical five at `3de8761108c02b613f9641847b24f89585d85125:configs/events.yaml:55-87`,
and pre-delivery-guard six at `ea123c7f88b521a68f9fa2de2e7dbe8b44223883:configs/events.yaml:28-154`.
Their presence in historical config is diagnostic vocabulary, not proof of real
production row existence or business verification.

Dispatch validation normalizes only declared optional defaults before the strict
stage/source/request checks. Required fields and unknown keys still fail before
production credentials. The contract test executes the actual github-script
validator for omitted empty defaults, complete inputs, unknown/mixed requests,
missing requirements, wrong ref/runtime SHA and an advanced main.
