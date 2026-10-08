# Exact compatibility retirement: preparation and B startup barriers

This is the foundation for the accepted four-object operation, not an enabled
production cleanup. `prepare` has real read-only identity discovery, separately approved boundary
discovery and a fixed-upper-bound paged source inventory adapter. These diagnostic
stages exit 42 with receipts even when the individual read-only producer succeeds. Discovery is
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
  immutable 0600 requests (`identity-request.json`, `boundary-request.json`,
  `inventory-request.json` as applicable), then a separately approved
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
`prepare_mode=bounds` and `prepare_mode=inventory` require only
`inventory_request_sha256`. All three preparation classes require an
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
referenced schema/name matching, under proven global visibility. Its narrowly
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
inventory retention; inventory payload copies are immutably registered by operation/run/request,
namespace, approved boundary and file SHA256 for later batch-specific purge
alongside the temporary backups. This temporary source register is not a
permanent database archive or a business-retirement verdict. A timed-out container is
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


## B migration and startup contract

B adds SQL99 and Mongo38 without rewriting any historical migration. Business
logic remains A's. Installed databases must have all four targets absent,
including empty-present objects, before either provider can create or mutate
migration metadata. Joint live preflight verifies both selected logical databases,
complete required catalog visibility, stable identity and clean expected heads;
missing/dirty/unknown heads, permission/network failures and mixed pristine pairs
fail before migration writes. The Mongo selection must equal the database actually
used by the business connection; migration overrides cannot point elsewhere.

SQL advances first (`98 -> 99`) and Mongo follows (`37 -> 38`). A fresh complete
joint proof can continue clean `99/37`; reverse `98/38` fails. The run owns only
its acquired SQL connection and releases it on every result, retaining the host's
borrowed pool and Mongo client. Public low-level Force/Down calls cannot downgrade
a B head or clear its dirty state without the successful exact terminal run.

The Mongo wrapper pins B's exact version/direction/resource bytes and single
terminal drop command. Installed absence is a no-op for that terminal command,
never a collection recreation. Other commands and permission/network/namespace
errors remain ordinary failures. Only an independently approved, truly pristine
pair may remove an empty old collection created by full historical bootstrap;
namespace type, UUID, empty content and identity are rechecked immediately.

Cold bootstrap needs complete empty catalogs (including routine/event/profiling
state), plus an identity/resource/source-bound private one-use authorization.
The two formal keys are `migration.retirement-bootstrap-authorization-file` and
`migration.retirement-bootstrap-authorization-sha256`. No boolean, empty target,
self-reported source SHA or observed state automatically approves cold bootstrap.
The application compares the approval source with its built GitCommit for this
cold path, rejects an unknown build SHA, and fsyncs the consumed receipt before
SQL creates metadata. Provisioning, exclusive writers and the actual image digest
still require independent external proof.

Recovery uses a preverified image and batch-bound configuration with
`migration.enabled=false`. It never performs Up/Down/Force or clears dirty state;
only the exact batch targets may be restored, retaining new evidence, retired IDs
and current MQ facts. The original A image is a possible candidate when separately
verified with migration disabled; B-checkout disabled-branch tests alone do not
attest that image or business acceptance. A restoration startup must not be
mistaken for successful B migration or production cleanup.

`python3 -B scripts/database/compatibility-retirement-b-integration.py` exercises
these paired contracts in owned authenticated loopback-only disposable MySQL8 /
Mongo7 fixtures. Source inventory scale verification is separate. Neither local
suite enables the currently unavailable deletion/recovery/acceptance/purge
backends or substitutes for the real measured ten-minute restoration budget.

Dispatch validation normalizes only declared optional defaults before the strict
stage/source/request checks. Required fields and unknown keys still fail before
production credentials. The contract test executes the actual github-script
validator for omitted empty defaults, complete inputs, unknown/mixed requests,
missing requirements, wrong ref/runtime SHA and an advanced main.
