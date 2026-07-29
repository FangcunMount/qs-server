#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

readonly CONTRACT_SECRET='historical-secret-contract-value'

assert_secret_contract() {
  local service="$1"
  local package_dir="$TEST_ROOT/${service}-package"
  local package_archive="$TEST_ROOT/${service}.tar.gz"
  local output_file="$TEST_ROOT/${service}.log"
  local env_file="$package_dir/configs/env/config.prod.env"

  case "$service" in
    apiserver)
      env \
        SERVICE=apiserver \
        DEPLOY_PACKAGE_DIR="$package_dir" \
        DEPLOY_PACKAGE="$package_archive" \
        MONGODB_HOST=mongodb MONGODB_PORT=27017 \
        MONGODB_USERNAME=user MONGODB_PASSWORD=password MONGODB_DBNAME=qs \
        MYSQL_HOST=mysql MYSQL_PORT=3306 \
        MYSQL_USERNAME=user MYSQL_PASSWORD=password MYSQL_DATABASE=qs \
        REDIS_HOST=redis REDIS_PORT=6379 \
        JWT_SECRET=jwt-secret DELEGATED_SUBJECT_CURRENT_KEY=delegated-key \
        NSQ_NSQD_HOST=nsqd NSQ_NSQD_PORT=4150 \
        OSS_ACCESS_KEY_ID=access-key OSS_ACCESS_KEY_SECRET=access-secret \
        QS_HISTORICAL_CONTEXT_SECRET="$CONTRACT_SECRET" \
        "$SCRIPT_DIR/prepare-package.sh" >"$output_file"
      ;;
    collection)
      env \
        SERVICE=collection \
        DEPLOY_PACKAGE_DIR="$package_dir" \
        DEPLOY_PACKAGE="$package_archive" \
        REDIS_HOST=redis REDIS_PORT=6379 \
        JWT_SECRET=jwt-secret DELEGATED_SUBJECT_CURRENT_KEY=delegated-key \
        QS_HISTORICAL_CONTEXT_SECRET="$CONTRACT_SECRET" \
        "$SCRIPT_DIR/prepare-package.sh" >"$output_file"
      ;;
    *)
      echo "unsupported test service: $service" >&2
      return 1
      ;;
  esac

  grep -Fqx "QS_HISTORICAL_CONTEXT_SECRET=$CONTRACT_SECRET" "$env_file"
  if grep -Fq "$CONTRACT_SECRET" "$output_file"; then
    echo "prepare-package leaked the historical secret for $service" >&2
    return 1
  fi
}

assert_missing_secret_rejected() {
  local output_file="$TEST_ROOT/missing-secret.log"

  if env -u QS_HISTORICAL_CONTEXT_SECRET \
    SERVICE=collection \
    DEPLOY_PACKAGE_DIR="$TEST_ROOT/missing-secret-package" \
    DEPLOY_PACKAGE="$TEST_ROOT/missing-secret.tar.gz" \
    REDIS_HOST=redis REDIS_PORT=6379 \
    JWT_SECRET=jwt-secret DELEGATED_SUBJECT_CURRENT_KEY=delegated-key \
    "$SCRIPT_DIR/prepare-package.sh" >"$output_file" 2>&1; then
    echo "prepare-package accepted a missing historical secret" >&2
    return 1
  fi

  grep -Fq 'Missing required env: QS_HISTORICAL_CONTEXT_SECRET' "$output_file"
}

cd "$REPO_ROOT"
assert_secret_contract apiserver
assert_secret_contract collection
assert_missing_secret_rejected
