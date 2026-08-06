#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/settle-statistics-stale-runs.sh"

for required in \
  'SETTLE-6-STATISTICS-RUNS' \
  "TTL 'cache:lock:statistics:1'" \
  'rebuild[-_]?statistics' \
  'START TRANSACTION' \
  'FOR UPDATE' \
  'SET @eligible_count' \
  'AND @eligible_count = 6' \
  'SET @affected_rows = ROW_COUNT()' \
  'SET @verified_rows' \
  'SETTLEMENT_RESULT|6|6|6|' \
  'POSTCHECK|6|6|6|6|6' \
  'COMMIT'; do
  if ! grep -Fq -- "$required" "$TARGET"; then
    echo "Statistics settlement contract is missing: $required" >&2
    exit 1
  fi
done

for id in \
  631012088902332974 \
  631034496552022574 \
  631349010061341230 \
  631362645156442670 \
  631363406154183214 \
  631444782798877230; do
  if ! grep -Fq -- "$id" "$TARGET"; then
    echo "Statistics settlement contract is missing target: $id" >&2
    exit 1
  fi
done

if grep -Eq 'DELETE[[:space:]]+FROM|DROP[[:space:]]+TABLE|TRUNCATE[[:space:]]+TABLE' "$TARGET"; then
  echo "Statistics settlement must not delete facts or schema" >&2
  exit 1
fi

echo "statistics stale-run settlement contract passed"
