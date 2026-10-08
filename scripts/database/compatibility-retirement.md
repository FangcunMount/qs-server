# Exact compatibility retirement: A-stage execution barrier

This is the foundation for the accepted four-object operation, not an enabled
production cleanup. `prepare` has real read-only identity discovery, separately approved private
request bootstrap, fixed-upper-bound discovery, paged source inventory and a
separately approved read-only history host. Identity, boundaries, inventory and
their existing bootstrap modes exit 42 with diagnostic receipts. Separately
approved physical-file metadata observation and parent registration may exit 0
only after their complete read-only checks and durable registration; neither
grants execution or CAS authority. The history host may exit 0 only when its
actual child completes two independent epochs,
the private and stdout readiness bytes agree, and the diagnostic receipt is
successfully armored. Its `complete`, `execution_allowed` and `drop_ready`
remain false. A successful diagnostic Action does not authorize retirement.
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
  job. Only database-backed `prepare` modes receive existing `MYSQL_METADATA_ADMIN_USERNAME`,
  `MYSQL_METADATA_ADMIN_PASSWORD`, `MYSQL_HOST/PORT/DATABASE` (with existing
  `MYSQL_DBNAME` fallback) and `MONGODB_HOST/PORT/DBNAME` production Secrets.
  For MongoDB it selects the dedicated pair `MONGODB_METADATA_ADMIN_USERNAME`
  and `MONGODB_METADATA_ADMIN_PASSWORD` together when both are configured;
  an incomplete pair fails before connecting. If neither is configured it uses
  the existing `MONGODB_USERNAME/PASSWORD` pair. The selected credentials enter
  only the temporary private inventory environment; service credentials are not
  changed. Authentication uses the existing `admin` auth source. Read-only
  identity discovery requires business-database reads and the cluster
  [`replSetGetConfig`](https://www.mongodb.com/docs/v7.0/reference/privilege-actions/#replsetgetconfig)
  privilege on the cluster resource; a real code-13 denial is reported as
  `mongo_replica_anchor_not_authorized`, without a weaker identity fallback.
  This discovery does not establish future write or DROP permission.
  No server vault variable is guessed. It deploys nothing.
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
- Backup private-file reads use `NOFOLLOW` and `NONBLOCK`, then require a
  regular file and preserve the existing owner, mode and size checks. A FIFO
  without a writer is rejected before reading. This protects file acquisition;
  it does not bound regular-file I/O or prove the combined 10-minute recovery.
- Deadline helpers stop forward work at 20 minutes and all recovery work at
  30 minutes. These helpers alone do not guarantee recovery time; readiness
  requires a measured and independently verified rollback of at most 10 minutes.

`cmd/qs-compatibility-history` implements a strictly read-only host for the
actual inventory outputs. It binds the compiled source, operation/run, approved
input bytes and all four private source files, authenticates clean EOF, and
checks actual original source rows against current SQL and Mongo business facts.
The host owns and ends each SQL repeatable-read read-only transaction and Mongo
snapshot before starting a different actual epoch. Full global unknown/orphan
coverage remains visible when the old objects contain no candidate rows.
Database and input close outcomes are settled before the final private readiness
file is written. Readiness counts and hashes cannot recreate a qualifier or
authorize evidence writes, message sends or deletion.

Production qualification still requires actual whole-epoch transaction lifetime,
peak process memory, related-owner and evidence-slot capacity measurements.
The exact qs-ai 0040 physical-schema adapter is implemented locally; production
still requires its actual source, database and original execution/MQ closure,
independent approvals, writer fencing, evidence CAS and
fresh readback. The local CLI and synthetic source-scale tests establish none
of those production gates.

The pre-fix combined five-package race/coverage regression at source
`2a89d5577ddfb6598b54eda6f1ec22670da1deb2` recorded 710 passing test actions
and one failed concurrent-challenge category test. A separate actual Linux
euid-0 call rejected the public challenge-store root anchor `/` at protected-path
validation. Prior ordinary-user seams, synthetic fixtures and documentation
checks cannot replace successful current-source public-root and complete
concurrency verification. Those failures grant no production fence or execution
authority; source regression, local root-entry proof and production installation
remain separate gates.

The subsequent source `b3c546f0bdd5d79b4d69d42130a813f6937e1b9d` passes the
combined five-package race/coverage regression: 713 test actions and five package
results pass, with no failure or skip. Each of three targeted challenge tests
also passes 20 repetitions (60 test actions). These are local regression results.
A separate Linux arm64 euid-0 public-API test passes with one parent test, no
subtest, failure or skip, without race or coverage instrumentation. Its
`2a89` plus exact two-file library overlay is independently equal to all 4,026
tracked files at `b3c546`; the extra private native fixture is declared separately.
It verifies genuine signature checks against two complete synthetic local API
snapshots, durable public challenge consumption and unsafe-directory, symlink,
replay and wrong-source rejection. The safe receipt
`qs-fence-linux-root-public-api-native-20261009.json` has SHA256
`afab6c0c43b158aaa406409b215a0192bc0417738af003c0d9cbc68d17a16cd6`.
This local root-entry result establishes no live GitHub origin, production
installation, executor, whole-writer fence, CAS or DROP.

Source `d252d11fd80bf42ea23d9eb5c5d2a92f8eb57cc2` adds the root-directory
maintenance-window budget component. Its immutable binding fixes the four
legacy targets, source, operation, manifest and original run. Public Linux
entry points require actual euid 0 and a root-owned 0700 directory with protected
ancestors. The original start and first recovery record use `CLOCK_BOOTTIME` and
`/proc/sys/kernel/random/boot_id`, exclusive creation, primary/commit seals,
file/directory fsync and a real directory flock. Forward work has at most 1,200
seconds; the shared window has 1,800 seconds; recovery expires at
`min(original_start + 1800s, first_recovery_start + 600s)`. Reopening cannot reset
those records. The 25ms clock monitor is cooperative, not a hard OS timeout.
Zero-value Close is a no-op; a copied receiver cannot acquire budget or release
the original lease. Other platforms reject the public production entry.

The separate actual Linux arm64 root test
`TestMaintenanceWindowActualLinuxRootLease` passes once: one parent test,
no subtest, failure or skip, without race or coverage instrumentation. Its
`b7e8b905` archive plus the exact four window files is byte-identical to the
4,030 tracked files at `d252d11`; a metadata-only private harness is declared
separately. The native test uses public root options, real kernel time/procfs,
0700 tmpfs and flock, observes five root-owned 0600 single-link files, verifies
start/first-recovery seals and preserves the original deadlines after Close/Open.
It checks propagation of the returned forward cancel function; it does not test
parent-context cancellation natively. The safe receipt
`qs-maintenance-window-linux-root-native-20261009.json` has SHA256
`22f06e68fa71c41e54ca110c13311c634c51f13524db5890c3aaf74fb3b67b8e`.
This seconds-scale local entry/lease test is `BudgetOnly`: it proves neither a
complete 600-second restoration nor independent production approval, writer
fencing, installed executor, CAS, DROP, restore or deployment permission.
Production capabilities remain false.

The main-line lint follow-up preserves six local deprecated-call exceptions:
five deliberately disconnected `mongo.NewClient` fixtures must prove rejection
before storage I/O, and PlanEntry keeps its deprecated token argument while the
real resolver ignores it and Collection enforces the IAM User/Testee relationship.
Cursor and transaction cleanup now explicitly ignore best-effort cleanup errors;
read, business and commit outcomes, including cursor release before UPDATE,
retain their existing order. These exceptions do not retire another public
contract or establish production cleanup readiness.

The runtime observer preserves full role-inventory and container-state checks.
Only the order of strictly validated unique mount-destination strings is
canonicalized, with exact path spelling and members retained in the state hash.
A real identity, image, restart, start-time or mount-member change still fails.
On `runtime_changed`, an additional fixed diagnostic exposes only approved
field names or the `instance_set_changed` category; it exposes no values,
container IDs, process output or credentials. Successful receipt v2 and its
business-acceptance boundary are unchanged. Local mounted-container and actual
restart tests do not identify the cause of a past production failure.

## Read-only history Action

`prepare_mode=bootstrap-history` binds an independently reviewed canonical
approval to the exact source, operation, original parent request/run, inventory
request/report and four whole-file hashes and encoded sizes. The original
`history-request.json` is read without rewriting. A new unique current-run
directory receives an immutable registration and a derived request whose only
changed field is `run_id`; the compiled history binary must report that exact
source in its strict JSON `--source-sha` response.

The host preserves the original absolute input paths in read-only mounts and
allows writes only to its own readiness directory. It validates the inspected
image ID, inherited labels, exact container ID, user, mounts, network and fixed
3 GiB / two CPU / no additional swap profile. The image's declared MySQL volume
is overridden by an explicitly owned empty read-only bind, so this diagnostic
container does not create an anonymous data volume. Creation or attach outcomes
that are unknown are inspected and retained for reconciliation; another run is
not allowed to erase the uncertainty by creating a replacement.

The first epoch releases its SQL/Mongo row graphs only after validating and
sealing the original live-scope anchor. The host then closes its own transactions
and starts genuinely different second transactions. The anchor retains original
identities, source receipts and digests; it does not extend their lifetime.
Candidate storage retains stable page values through private pointers instead
of repeatedly copying a growing value slice. A coordinator-private pool shares
only identical ordered lists for original-run gaps, historical gaps, blockers
and required adapters. The 4,096-entry / 8 MiB accounted limit bounds this
optional cache: saturation retains the complete original list and continues.
A used cache conflict fails before consuming a page. Public range reads remain
deep copies; nil/empty distinctions and the original ordered private hash remain
unchanged. After each epoch has independently authenticated all four complete
copies, only the coordinator-owned event consumption readers retain a compact
original-ID set instead of a second complete event reference. Every decoded
row still matches the sealed complete typed-fact digest; public readers, initial
authentication, origin/index reads, strict ordering, clean EOF, per-page keys
and original defensive-clone checks retain their existing behavior. This removes
duplicate retention; it does not establish a measured production memory budget.

The private decoded-fact digest includes the time-zone name and offset as part
of the complete DTO. The derived diagnostic `whole_source_index_sha256` is
therefore a process-environment-dependent observation, compared only between
two epochs of the same CLI. It is not portable across time zones and is not
persistent business evidence. Original source bytes, content digests, SDK
fingerprints and business bindings remain independently verified. The capacity
golden test uses explicit UTC input; other historical fixtures retain their
original inputs. Source indexes reserve capacity only after authenticated bounded
counts; these changes do not establish the complete production resource budget.

`prepare_mode=bootstrap-history-metadata` has a separate independently hashed
`readonly_history_metadata_approval`. It binds the final tooling SHA, operation,
original inventory run/request/report and exact limits. The Python-only four-file
package uses no database credentials or Go binary. It rechecks the original
private identity/bounds/inventory/sidecars, rejects prior unknown handles, and
reads each exact source file to EOF twice. Regular single-link private files,
full encoded byte counts, physical SHA256 and before/after inode/stat baselines
must agree. The physical file SHA/size are distinct from the inventory semantic
`DataHash` and source bytes. The 900-second limit is cooperative, not proof that
all operating-system I/O is bounded. A fresh private run saves the approved
metadata and an immutable parent proposal, without registering an executable
history request or claiming semantic source verification.

`prepare_mode=bootstrap-history-parent` requires another independently hashed
`readonly_history_parent_registration_approval` binding the actual metadata
run/report, proposal SHA, original inventory run/report and all four exact
physical assets. Its Python-only five-file package also sends no database
credentials. It repeats the complete read-only file checks and exclusively
publishes `history-request-bootstrap.json` and `history-request.json`; an
interrupted one-sided publication blocks rather than adopting or overwriting
it. The parent keeps the original inventory run; metadata and registration runs
are separate. An identical registration is idempotent and preserves its first
creator. Both modes require armored diagnostic receipts and keep
`complete`, `execution_allowed`, `drop_ready`, CAS and process-budget proof false.

Production must run the whole identity/bounds/inventory/metadata/parent/history
chain on the same final main tooling SHA, with independently bound approvals.
The local filesystem, Action package and native fixture tests do not prove that
production chain, full-sized scan budgets or database historical acceptance.
Read-only completion counts retain local, AI and global blockers. External AI
closure, evidence CAS, writer fencing, production-sized exact restore, release B,
DROP, acceptance and batch-owned purge remain incomplete; execution backends
remain disabled.

## Required implementation before execution can become ready

The target recovery library now provides `PrepareTargetRecovery` and
`RecoverTargets` for this batch's exact four objects. It borrows the host's
existing SQL connection and Mongo database, authenticates the original archive,
heads, identities, target contents and non-target schema projection, and uses
an opaque native DROP result with durably persisted intent. Missing objects by
themselves do not grant restore authority. It restores missing targets only;
existing exact targets remain unchanged, and a content conflict, unknown DDL
result, changed owner or failed restoration blocks continuation. It preserves
current MQ facts, business evidence, retired command identities and migration
heads; a restored Mongo collection receives a newly observed UUID.

Actual isolated nonempty native tests cover partial and complete target loss,
ordered BSON/SQL restoration, repeated read-only verification and an unresolved
foreign-key failure. They establish a local recovery primitive, not a production
600-second full recovery budget, complete writer fencing, independent execution
authorization or a production Action backend. Fresh production qualification,
privilege-negative checks and deployment rollback still need their own proofs.

The native DROP producer remains package-private and is currently exercised
only by integration-tag recovery tests. Its narrow unused-linter exception
does not provide a production mutation entry; the reviewed fixed host
executor and its independent authority remain required.

The private decoded-fact fingerprint now streams text through bounded,
instance-owned scratch storage. Its original frame bytes, protocol, NULL/type
semantics and byte/node limits remain unchanged. Independent byte vectors,
source-authentication/joint-scan regression and full package race checks pass.
This implementation change does not establish the full-size scan memory or
lifetime budget; the earlier full-size OOM remains a failed observation until
a new independently bounded scan completes both physical EOF epochs.

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
separate source inventory must actually perform those reads. Each database state
also carries its producer's `error_category` through the armored diagnostic receipt.
Only reviewed, fixed categories from that database's allowlist are accepted;
unknown categories and raw exception/connection text are rejected before transport.
The `replSetGetConfig` anchor read classifies only actual MongoDB driver
`CommandError` codes: 13 produces `mongo_replica_anchor_not_authorized`, and
76 produces `mongo_replica_anchor_replication_not_enabled`. Network failures,
other codes and nil errors retain `mongo_replica_anchor_permission_or_read_failed`.
Error text is never used to infer these diagnoses. These categories do not
change permissions, accept a standalone server, or substitute another anchor.
An incomplete identity receipt remains diagnostic (`execution_allowed=false`,
`drop_ready=false`), including when another database's head was observed clean.
Runtime image/network
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

Request pre-provisioning is implemented as two separate diagnostic modes.
`bootstrap-bounds` receives independently approved canonical
`bootstrap_approval_json` plus its SHA256 including the final newline. It binds
the exact main/source/operation/target, identity producer run and private report
hash, clean heads and the fixed V2 profile; it verifies that original private
report and creates `boundary-request.json` without opening a database.
`bootstrap-inventory` additionally verifies the separately approved original
boundary producer run/report before creating `inventory-request.json`; the true
BSON tokens stay private. Neither mode chooses an observed identity for the
reviewer, executes a scan or permits deletion. Both return exit 42 and
`request_bootstrap_complete:true` with `derived_request_sha256`. That derived
hash must be reviewed and separately supplied to the subsequent `bounds` or
`inventory` run.

The request and bootstrap binding are exact 0600, no-follow, exclusive-created
files with fsync under the operation lock. An identical completed pair is
verified; changed bytes, a one-file partial pair, symlink/hardlink or an
interrupted `.bootstrap.partial` refuses continuation. No overwrite, automatic
repair or discovery-to-approval shortcut exists.

Production boundary and inventory requests require `format_version: 2`. V1's
100,000-record / 128 MiB / 180-second helpers remain only for historical fixture
coverage; both live Python and Go entrypoints reject V1 before connecting.
V2 retains the exact scope, separately approved source/operation, identity hashes
and expected clean heads. Its immutable per-target profile is
`query_seconds=30`, `total_seconds=1500`, `max_records=1000000`,
`max_bytes=2147483648`, `page_size=1000`, `max_pages=1001`.
The source-byte total and the actual encoded private source file each have the
2 GiB bound; framing/base64 overhead cannot bypass the on-disk bound. Exceeding
any record, byte, page or deadline cap remains incomplete and denies readiness.

`bounds` reads exact namespace/schema and the true primary-key upper token into
an immutable `boundary.private.json` owned by that run. Missing and present-empty
objects have distinct representations. This observation does not create an
inventory approval. A reviewer must independently bind and approve that file's
raw SHA256 and all four boundaries in the separate inventory request using
`boundary_run_id`, `boundary_report_hash`, and `approved_boundaries`; there is no
automatic discovery-to-approval path or limit override.

SQL pages compare numeric bigint IDs numerically; AI command IDs require the
actual supported single ASCII binary-collation primary key. Mongo retains true
one-field BSON `_id` tokens and proves a homogeneous supported `_id` type across
the collection before paging: string, ObjectID, int32 or int64. Mixed/unsupported
types and non-simple collection collation fail explicitly. Converting `_id` to
JSON text or allowing type-bracket queries to silently skip records is forbidden.

Two complete passes use the approved fixed upper bound. SQL passes share one
read-only repeatable-read snapshot, then close it before a fresh read observes
post-upper records and rechecks identity/head/catalog. Mongo also rechecks its
actual catalog/head/UUID after scanning. New records above the approved upper
set `next_cycle_required`; they are not silently folded into the old approval.
The two equal passes prove content within the scanned range, not a production
writer fence, global business-history coverage or an atomic dual-store snapshot.
The final maintenance fence still requires a fresh final difference check and
fixed-bound scan before deletion.

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
The additive tooling also records `database_anchor_hash` independently from
`migration_generation_hash`. MySQL uses the existing server UUID/selected database
identity and an empty migration generation. Mongo requires actual
`replSetGetConfig` visibility and hashes the observed nonzero
`settings.replicaSetId`, matching replica-set name and selected database; its
migration generation separately hashes the actual migration-collection UUID.
Primary/hosts/`me` and migration UUID do not enter the stable Mongo anchor.
Permission errors, unsupported topology, ambiguous BSON and mismatched set
identity fail explicitly. Existing V1 identity/head request bytes and approvals
remain unchanged; the new anchor does not replace full catalog or non-target
business-baseline checks. Across a migration, compare the separately bound
stable anchor and expected catalog/facts, while recording the changed generation;
do not silently accept a changed V1 aggregate.
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
The SQL catalog reads cross-schema inbound foreign-key metadata with exact
referenced schema/name matching under proven global visibility. Its narrowly
scoped `inbound_foreign_key_coverage_complete` does not establish complete
dependency coverage: selected-schema SQL text still requires review, and dynamic
SQL, external/manual writers and other repositories remain unproved. A complete
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
