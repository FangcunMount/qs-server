#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

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
    *)
      echo "unsupported test service: $service" >&2
      return 1
      ;;
  esac

  test -s "$env_file"
  test -s "$package_archive"
}

cd "$REPO_ROOT"
assert_package_contract apiserver
assert_package_contract collection
