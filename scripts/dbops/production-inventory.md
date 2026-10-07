# Production database metadata inventory

`Database Operations` provides `operation=inventory` for `database=mysql`, `mongodb`, or `all`. `all` queries these two databases sequentially; it never runs Redis, backup, restore, migration, or business repair. Production execution requires the separately authorized database operation. The source SHA identifies the audited scripts, not the deployed application SHA.

A reviewed ref can be selected with:

```sh
gh workflow run db-ops.yml --ref <reviewed-ref> -f operation=inventory -f database=all
```

If GitHub rejects a choice absent from the default branch, merge the reviewed workflow through the normal checks before dispatching it. Do not substitute `status`: status runs broader business consistency queries.

The job uses the existing production Environment and serverA SSH host/user/key/fingerprint. It validates the binding before upload, transfers three scripts with an archive SHA256, runs them from a private temporary directory, and removes temporary files on exit. MySQL credentials use a private client defaults file; Mongo credentials use a private Docker env-file and authentication inside mongosh. Files are mode 0600 in mode 0700 directories and are removed after each client. Passwords and usernames are absent from client argv, public output, connection URIs and Docker command output.

The JSON between `QS_DB_INVENTORY_BEGIN` and `QS_DB_INVENTORY_END` contains source SHA, UTC observation time, selected scope, completeness and per-database metadata:

- MySQL: server version, table/view names, estimated rows and allocated bytes, column names/types, index columns/flags, and schema_migrations version/dirty.
- MongoDB: collection/view/time-series names, field names declared in a JSON Schema validator (no document sampling), metadata count/bytes, index keys/flags/TTL, and schema_migrations version/dirty. No validator values, index partial-filter values or view pipeline is emitted.

MongoDB reads the complete namespace catalog and validates the migration head before requesting storage/index details. Only a numeric MongoDB Unauthorized code `13` from `collStats` or `listIndexes` permits a partial report: the affected `storage` or `indexes` value is `null`, never zero or an empty list, and its `metadata_error` list contains the fixed token `collStats_unauthorized_code_13` or `listIndexes_unauthorized_code_13`. Other namespaces and any permitted details remain available. The database result has `metadata_complete=false`; the outer result has `complete=false`, and the job still exits nonzero. Missing catalog/head, authentication failure, nonnumeric/other error codes, timeout and limits remain failures without a usable catalog. No role or grant is added or relaxed.

A catalog retained in a partial report supports namespace comparison only. Unknown storage/index details cannot establish emptiness, missing indexes, TTL absence or cleanup eligibility. A complete report uses `metadata_complete=true` and an empty `metadata_error` list for every namespace. Fixed failure stages and numeric server/client codes do not include raw error messages, namespace error context, credentials or document contents.

Counts are metadata estimates. Zero rows/documents is not proof of emptiness and never authorizes DROP/delete. MySQL metadata visibility depends on the account's grants. A missing Mongo namespace can be lazily created. Compare actual names with the separately reviewed source inventory; preserve unexpected namespaces, views, backups and migration metadata until their provenance is established.

Every MySQL query has a 5-second max_execution_time inside an explicit READ ONLY transaction; metadata lock wait is 5 seconds. Mongo metadata commands have a 5-second limit; cursor continuation inherits the original command budget, with a 70-second local scan deadline. Each client has a 75-second process budget, 0.5 CPU, 512 MiB memory, 64 PIDs, read-only filesystem and dropped capabilities. Host stdout+stderr capture is limited to 4 MiB while reading. SSH timeout is 5 minutes, job timeout 10 minutes, and client images must already exist (`--pull=never`).

Each client gets a random ownership label. Cleanup queries only that label's exact container ID, removes only that ID, and confirms absence. A name collision is rejected; failed creation never removes another owner. A timeout terminates the isolated client process group and still checks owned-container cleanup. Unknown/missing metadata, output limit, authentication error, query timeout, duplicate/missing migration heads, or unconfirmed cleanup produces `complete=false` and a failed job, not a zero count. Raw client errors are not printed.

This entry uses the existing production secret fields and admin authentication source for MongoDB, as the maintained Database Operations clients do. It does not discover alternate databases or infer TLS/topology changes from service configuration. Confirm an alternate TLS/replica-set binding before extending it. The tool never initializes the application or its migration runner.

Offline checks:

```sh
bash scripts/dbops/test-production-inventory.sh
```

These validate metadata/output/credential/ownership/deadline contracts, not production connectivity, permissions, current data or cleanup eligibility.
