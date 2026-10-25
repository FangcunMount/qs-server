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
        MESSAGING_PROVIDER=rocketmq ROCKETMQ_ENDPOINT=127.0.0.1:8081 \
        ROCKETMQ_INSTANCE_ID=qs-apiserver-1 \
        ROCKETMQ_MYSQL_STORE_ID=Original-MySQL ROCKETMQ_MYSQL_WRITER_EPOCH=Original-SQL-Epoch \
        ROCKETMQ_MONGO_STORE_ID=Original-Mongo ROCKETMQ_MONGO_WRITER_EPOCH=Original-Mongo-Epoch \
        ROCKETMQ_PUBLISH_ENABLED=false \
        DEPLOY_PACKAGE_DIR="$package_dir" \
        DEPLOY_PACKAGE="$package_archive" \
        MONGODB_HOST=mongodb MONGODB_PORT=27017 \
        MONGODB_USERNAME=user MONGODB_PASSWORD=password MONGODB_DBNAME=qs \
        MYSQL_TLS_MODE=VERIFY_IDENTITY MYSQL_HOST=mysql MYSQL_PORT=3306 \
        MYSQL_USERNAME=user MYSQL_PASSWORD=password MYSQL_DATABASE=qs \
        REDIS_HOST=redis REDIS_PORT=6379 \
        JWT_SECRET=jwt-secret DELEGATED_SUBJECT_CURRENT_KEY=delegated-key \
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
    worker)
      env \
        SERVICE=worker \
        DEPLOY_PACKAGE_DIR="$package_dir" \
        DEPLOY_PACKAGE="$package_archive" \
        MONGODB_HOST=mongodb MONGODB_PORT=27017 \
        MONGODB_USERNAME=user MONGODB_PASSWORD=password MONGODB_DBNAME=qs \
        MYSQL_TLS_MODE=VERIFY_IDENTITY MYSQL_HOST=mysql MYSQL_PORT=3306 \
        MYSQL_USERNAME=user MYSQL_PASSWORD=password MYSQL_DATABASE=qs \
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
  test -f "$package_dir/scripts/cd/image-retention.py"
  test -f "$package_dir/scripts/cd/image-retention.sh"
  test -f "$package_dir/scripts/cd/sudo-selection.sh"
  test -f "$package_dir/scripts/cd/runtime-evidence.py"
  test -f "$package_dir/scripts/dbops/receipt-transport.py"
  cmp "$REPO_ROOT/scripts/cd/runtime-evidence.py" "$package_dir/scripts/cd/runtime-evidence.py"
  cmp "$REPO_ROOT/scripts/dbops/receipt-transport.py" "$package_dir/scripts/dbops/receipt-transport.py"
  test -x "$package_dir/scripts/cd/wait-worker-readiness.sh"
  test -x "$package_dir/scripts/cd/verify-worker-dependencies.sh"
  assert_absent "$env_file" "_REDIS_DB="
  if [ "${MESSAGING_PROVIDER:-nsq}" = rocketmq ] && [ "$service" != collection ]; then
    local prefix=QS_WORKER
    if [ "$service" = apiserver ]; then prefix=QS_APISERVER; fi
    assert_line "$env_file" "${prefix}_MESSAGING_PROVIDER=rocketmq"
    assert_line "$env_file" "${prefix}_MESSAGING_ROCKETMQ_ENDPOINT=127.0.0.1:8081"
    local mq_root=/app/configs/rocketmq
    if [ "$service" = worker ]; then mq_root=/etc/qs-worker/rocketmq; fi
    assert_line "$env_file" "${prefix}_MESSAGING_ROCKETMQ_CREDENTIALS_FILE=${mq_root}/credentials.json"
    assert_line "$env_file" "${prefix}_MESSAGING_ROCKETMQ_PUBLISH_ENABLED=false"
    if [ "$service" = apiserver ]; then
      assert_line "$env_file" "QS_APISERVER_IAM_AUTHZ_SYNC_PROVIDER=rocketmq"
      assert_line "$env_file" "QS_APISERVER_IAM_AUTHZ_SYNC_EPHEMERAL_NSQ=false"
    fi
  fi

  case "$service" in
    apiserver)
      assert_line "$env_file" "QS_APISERVER_MONGODB_MIN_POOL_SIZE=16"
      assert_line "$env_file" "QS_APISERVER_MONGODB_MAX_POOL_SIZE=64"
      assert_line "$env_file" "QS_APISERVER_MONGODB_MAX_CONNECTING=8"
      assert_line "$env_file" "QS_APISERVER_MONGODB_MAX_CONN_IDLE_TIME=10m"
      test -s "$package_dir/configs/cache/apiserver.prod.yaml"
      assert_line "$env_file" "QS_APISERVER_REDIS_DATABASE=0"
      assert_absent "$env_file" "NSQ_"
      assert_absent "$output_file" "deepseek-test-secret-value"
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
      assert_absent "$env_file" "NSQ_"
      assert_line "$env_file" "ROCKETMQ_WORKER_BINDING_REVISION=proof-v1"
      ;;
  esac
}

cd "$REPO_ROOT"
assert_package_contract apiserver
assert_package_contract collection
MESSAGING_PROVIDER_REQUIRED=true MESSAGING_PROVIDER=rocketmq ROCKETMQ_ENDPOINT=127.0.0.1:8081 ROCKETMQ_INSTANCE_ID=qs-apiserver-1 ROCKETMQ_PUBLISH_ENABLED=false assert_package_contract apiserver
MESSAGING_PROVIDER_REQUIRED=true MESSAGING_PROVIDER=rocketmq ROCKETMQ_ENDPOINT=127.0.0.1:8081 ROCKETMQ_PUBLISH_ENABLED=false ROCKETMQ_WORKER_BINDING_REVISION=proof-v1 assert_package_contract worker

# Tencent public Redis must fail closed rather than infer TLS from its port.
redis_tls_dir="$TEST_ROOT/tencent-redis"
if env SERVICE=collection DEPLOY_PACKAGE_DIR="$redis_tls_dir" DEPLOY_PACKAGE="$TEST_ROOT/tencent-redis.tar.gz" \
  REDIS_HOST=bj-crs-3dli07u2.sql.tencentcdb.com REDIS_PORT=23136 REDIS_USE_SSL=false \
  JWT_SECRET=test DELEGATED_SUBJECT_CURRENT_KEY=test \
  "$SCRIPT_DIR/prepare-package.sh" >"$TEST_ROOT/tencent-redis-rejected.log" 2>&1; then
  echo 'Tencent Redis plaintext was unexpectedly accepted' >&2
  exit 1
fi
env SERVICE=collection DEPLOY_PACKAGE_DIR="$redis_tls_dir" DEPLOY_PACKAGE="$TEST_ROOT/tencent-redis.tar.gz" \
  REDIS_HOST=bj-crs-3dli07u2.sql.tencentcdb.com REDIS_PORT=23136 REDIS_USE_SSL=true \
  REDIS_PASSWORD='REDIS_APP_PASSWORD@synthetic-secret' JWT_SECRET=test DELEGATED_SUBJECT_CURRENT_KEY=test \
  "$SCRIPT_DIR/prepare-package.sh" >"$TEST_ROOT/tencent-redis-valid.log"
assert_line "$redis_tls_dir/configs/env/config.prod.env" 'SSL_CERT_DIR=/etc/ssl/certs:/etc/pki/tls/certs:/etc/qlume/redis'
assert_line "$redis_tls_dir/configs/env/config.prod.env" 'COLLECTION_SERVER_REDIS_PASSWORD=REDIS_APP_PASSWORD@synthetic-secret'
if grep -q 'synthetic-secret' "$TEST_ROOT/tencent-redis-valid.log"; then
  echo 'Redis credential leaked in packaging log' >&2
  exit 1
fi
if env SERVICE=collection DEPLOY_PACKAGE_DIR="$TEST_ROOT/tencent-insecure" DEPLOY_PACKAGE="$TEST_ROOT/tencent-insecure.tar.gz" \
  REDIS_HOST=bj-crs-3dli07u2.sql.tencentcdb.com REDIS_PORT=23136 REDIS_USE_SSL=true REDIS_SSL_INSECURE_SKIP_VERIFY=true \
  JWT_SECRET=test DELEGATED_SUBJECT_CURRENT_KEY=test \
  "$SCRIPT_DIR/prepare-package.sh" >"$TEST_ROOT/tencent-insecure.log" 2>&1; then
  echo 'Tencent Redis SkipVerify was unexpectedly accepted' >&2
  exit 1
fi
