#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

assert_line() {
  local file="$1"
  local expected="$2"
  grep -Fqx -- "$expected" "$file" || {
    echo "missing expected line in $file: $expected" >&2
    return 1
  }
}

assert_absent() {
  local file="$1"
  local forbidden="$2"
  if grep -Fq -- "$forbidden" "$file"; then
    echo "forbidden deployment environment key in $file: $forbidden" >&2
    return 1
  fi
}

assert_package_contract() {
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
        DEEPSEEK_API_KEY=deepseek-test-secret-value \
        "$SCRIPT_DIR/prepare-package.sh" >"$output_file"
      ;;
    collection)
      env \
        SERVICE=collection \
        DEPLOY_PACKAGE_DIR="$package_dir" \
        DEPLOY_PACKAGE="$package_archive" \
        REDIS_HOST=redis REDIS_PORT=6379 \
        JWT_SECRET=jwt-secret DELEGATED_SUBJECT_CURRENT_KEY=delegated-key \
        "$SCRIPT_DIR/prepare-package.sh" >"$output_file"
      ;;
    worker)
      env \
        SERVICE=worker \
        DEPLOY_PACKAGE_DIR="$package_dir" \
        DEPLOY_PACKAGE="$package_archive" \
        MONGODB_HOST=mongodb MONGODB_PORT=27017 \
        MONGODB_USERNAME=user MONGODB_PASSWORD=password MONGODB_DBNAME=qs \
        MYSQL_HOST=mysql MYSQL_PORT=3306 \
        MYSQL_USERNAME=user MYSQL_PASSWORD=password MYSQL_DATABASE=qs \
        NSQ_NSQD_HOST=nsqd NSQ_NSQD_PORT=4150 \
        NSQ_LOOKUPD_HOST=nsqlookupd NSQ_LOOKUPD_PORT=4161 \
        REDIS_HOST=redis REDIS_PORT=6379 \
        "$SCRIPT_DIR/prepare-package.sh" >"$output_file"
      ;;
    *)
      echo "unsupported test service: $service" >&2
      return 1
      ;;
  esac

  test -s "$env_file"
  test -s "$package_archive"
  test -x "$package_dir/scripts/cd/wait-worker-readiness.sh"
  test -x "$package_dir/scripts/cd/verify-worker-dependencies.sh"
  assert_absent "$env_file" "_REDIS_DB="

  case "$service" in
    apiserver)
      assert_line "$env_file" "QS_APISERVER_MONGODB_MIN_POOL_SIZE=16"
      assert_line "$env_file" "QS_APISERVER_MONGODB_MAX_POOL_SIZE=64"
      assert_line "$env_file" "QS_APISERVER_MONGODB_MAX_CONNECTING=8"
      assert_line "$env_file" "QS_APISERVER_MONGODB_MAX_CONN_IDLE_TIME=10m"
      test -s "$package_dir/configs/cache/apiserver.prod.yaml"
      assert_line "$env_file" "QS_APISERVER_REDIS_DATABASE=0"
      assert_line "$env_file" "QS_APISERVER_MESSAGING_NSQ_ADDR=nsqd:4150"
      assert_line "$env_file" "DEEPSEEK_API_KEY=deepseek-test-secret-value"
      assert_absent "$output_file" "deepseek-test-secret-value"
      assert_line "$output_file" "DEEPSEEK_API_KEY=***REDACTED***"
      assert_absent "$env_file" "QS_APISERVER_NSQ_NSQD_"
      ;;
    collection)
      test -s "$package_dir/configs/cache/collection-server.prod.yaml"
      assert_line "$env_file" "COLLECTION_SERVER_REDIS_DATABASE=0"
      ;;
    worker)
      assert_line "$env_file" "QS_WORKER_MONGODB_MIN_POOL_SIZE=4"
      assert_line "$env_file" "QS_WORKER_MONGODB_MAX_POOL_SIZE=32"
      assert_line "$env_file" "QS_WORKER_MONGODB_MAX_CONNECTING=4"
      assert_line "$env_file" "QS_WORKER_MONGODB_MAX_CONN_IDLE_TIME=10m"
      assert_line "$env_file" "QS_WORKER_MYSQL_HOST=mysql:3306"
      assert_line "$env_file" "QS_WORKER_REDIS_DATABASE=0"
      assert_line "$env_file" "QS_WORKER_MESSAGING_NSQ_ADDR=nsqd:4150"
      assert_line "$env_file" "QS_WORKER_MESSAGING_NSQ_LOOKUPD_ADDR=nsqlookupd:4161"
      ;;
  esac
}

cd "$REPO_ROOT"
assert_package_contract apiserver
assert_package_contract collection
assert_package_contract worker
