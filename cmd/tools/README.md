# Seeder Utilities

This directory contains standalone Go commands that seed reference data into
the IAM Contracts database. Each command expects a MySQL DSN supplied via the
`--dsn` flag or the `IAM_SEEDER_DSN` environment variable and can be executed
with `go run`.

## Available commands

1. `seeddata` &ndash; unified seeding tool that supports multiple steps:
   - `tenants` &ndash; inserts base tenant records
   - `user` &ndash; creates system administrators, sample users, children, and guardianship links
   - `authn` &ndash; configures authentication accounts along with operation credentials and WeChat bindings
   - `resources` &ndash; registers authorization resources with their action sets
   - `assignments` &ndash; applies default role memberships to users
   - `casbin` &ndash; loads core Casbin policies and role inheritance rules
   - `jwks` &ndash; seeds JWKS key material for JWT validation
   - `wechatapp` &ndash; creates WeChat application configurations (Mini Programs & Official Accounts)
2. `backfill-pending-assessments` &ndash; scans `assessment.status='pending'`, submits matching rows in batches, and stages `assessment.submitted` events into MySQL outbox so the normal worker evaluation pipeline can continue from there.

## Example usage

### Using the unified seeddata tool

```bash
# Seed all data (recommended)
go run ./cmd/tools/seeddata \
  --dsn "user:pass@tcp(127.0.0.1:3306)/iam_contracts?parseTime=true&loc=Local" \
  --config configs/seeddata.yaml

# Seed specific steps only
go run ./cmd/tools/seeddata \
  --dsn "user:pass@tcp(127.0.0.1:3306)/iam_contracts?parseTime=true&loc=Local" \
  --config configs/seeddata.yaml \
  --steps "tenants,user,wechatapp"

# Seed WeChat apps only
go run ./cmd/tools/seeddata \
  --dsn "user:pass@tcp(127.0.0.1:3306)/iam_contracts?parseTime=true&loc=Local" \
  --config configs/seeddata.yaml \
  --steps "wechatapp"
```

### Legacy individual commands (deprecated)

```bash
export IAM_SEEDER_DSN='user:pass@tcp(127.0.0.1:3306)/iam_contracts?parseTime=true&loc=Local'
go run ./cmd/tools/seed-tenants
go run ./cmd/tools/seed-user-center
go run ./cmd/tools/seed-auth-accounts
go run ./cmd/tools/seed-authz-resources
go run ./cmd/tools/seed-role-assignments
go run ./cmd/tools/seed-casbin
go run ./cmd/tools/seed-jwks
```

### Backfill pending assessments

```bash
# Dry-run: count pending assessments that still need downstream evaluation
go run ./cmd/tools/backfill-pending-assessments \
  --dsn "user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true&loc=Local" \
  --events-config configs/events.yaml

# Apply in controlled batches for one org and one historical window
go run ./cmd/tools/backfill-pending-assessments \
  --dsn "user:pass@tcp(127.0.0.1:3306)/qs?parseTime=true&loc=Local" \
  --events-config configs/events.yaml \
  --apply \
  --org-id 1 \
  --created-before "2026-04-16T00:00:00+08:00" \
  --batch-size 100 \
  --sleep 200ms
```

Run the commands in the above order after creating an empty schema to rebuild
the baseline data set.
