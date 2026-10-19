#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/image-metadata.sh"

require_env() {
  local missing=0
  local var
  for var in "$@"; do
    if [ -z "${!var:-}" ]; then
      echo "Missing required env: $var" >&2
      missing=1
    fi
  done
  if [ "$missing" -ne 0 ]; then
    exit 1
  fi
}

default_redis_ssl() {
  REDIS_DB="${REDIS_DB:-0}"
  if [ -z "${REDIS_USE_SSL:-}" ]; then
    if [ "${REDIS_PORT:-}" = "6380" ]; then
      REDIS_USE_SSL=true
    else
      REDIS_USE_SSL=false
    fi
  fi
  REDIS_SSL_INSECURE_SKIP_VERIFY="${REDIS_SSL_INSECURE_SKIP_VERIFY:-false}"
}

redact_env_file() {
  sed -E \
    -e 's/^([^=]*(PASSWORD|SECRET|API_KEY|ACCESS_KEY_ID|SESSION_TOKEN|CURRENT_KEY|PREVIOUS_KEY)[^=]*)=.*/\1=***REDACTED***/' \
    "$1"
}

PACKAGE_DIR="${DEPLOY_PACKAGE_DIR:-deploy-package}"
ENV_FILE="${PACKAGE_DIR}/configs/env/config.prod.env"

rm -rf "$PACKAGE_DIR" "$DEPLOY_PACKAGE"
mkdir -p "${PACKAGE_DIR}/configs/env" "${PACKAGE_DIR}/scripts/cd" "${PACKAGE_DIR}/scripts/dbops"
cp -r configs "$PACKAGE_DIR/"
cp build/docker/docker-compose.prod.yml "${PACKAGE_DIR}/docker-compose.prod.yml"
cp \
  scripts/cd/image-metadata.sh \
  scripts/cd/image-retention.py \
  scripts/cd/image-retention.sh \
  scripts/cd/ai-messaging-release.py \
  scripts/cd/sudo-selection.sh \
  scripts/cd/remote-deploy.sh \
  scripts/cd/runtime-evidence.py \
  scripts/cd/wait-worker-readiness.sh \
  scripts/cd/verify-worker-dependencies.sh \
  scripts/cd/verify-collection-nginx.sh \
  scripts/cd/verify-observability-nginx.sh \
  scripts/cd/verify-worker-governance.sh \
  "${PACKAGE_DIR}/scripts/cd/"
cp scripts/dbops/receipt-transport.py "${PACKAGE_DIR}/scripts/dbops/"

if [ "${MESSAGING_PROVIDER_REQUIRED:-false}" = true ] && [ "$SERVICE" != collection ]; then
  require_env MESSAGING_PROVIDER
fi
MESSAGING_PROVIDER="${MESSAGING_PROVIDER:-nsq}"
case "$MESSAGING_PROVIDER" in nsq|rocketmq) ;; *) echo 'Unsupported broker provider' >&2; exit 1 ;; esac
ROCKETMQ_PUBLISH_ENABLED="${ROCKETMQ_PUBLISH_ENABLED:-true}"
case "$ROCKETMQ_PUBLISH_ENABLED" in true|false) ;; *) echo 'Invalid publish gate' >&2; exit 1 ;; esac
NSQ_NSQD_HOST="${NSQ_NSQD_HOST:-}"
NSQ_LOOKUPD_HOST="${NSQ_LOOKUPD_HOST:-}"
if [ "$SERVICE" != collection ]; then
  if [ "$MESSAGING_PROVIDER" = rocketmq ]; then
    require_env ROCKETMQ_ENDPOINT
    if [ "$SERVICE" = apiserver ]; then require_env ROCKETMQ_INSTANCE_ID; fi
  else
    require_env NSQ_NSQD_HOST
    if [ "$SERVICE" = worker ]; then require_env NSQ_LOOKUPD_HOST; fi
  fi
fi

if [ "${SERVICE:-apiserver}" != collection ]; then
  require_env MYSQL_TLS_MODE
  if [ "$MYSQL_TLS_MODE" != VERIFY_IDENTITY ]; then
    echo 'Production requires MySQL VERIFY_IDENTITY' >&2
    exit 1
  fi
fi

case "$SERVICE" in
  apiserver)
    REQUIRED_CACHE_POLICY="${PACKAGE_DIR}/configs/cache/apiserver.prod.yaml"
    ;;
  collection)
    REQUIRED_CACHE_POLICY="${PACKAGE_DIR}/configs/cache/collection-server.prod.yaml"
    ;;
  worker)
    REQUIRED_CACHE_POLICY=""
    ;;
  *)
    echo "Unsupported SERVICE: $SERVICE" >&2
    exit 1
    ;;
esac
if [ -n "$REQUIRED_CACHE_POLICY" ] && [ ! -r "$REQUIRED_CACHE_POLICY" ]; then
  echo "Missing required cache policy: $REQUIRED_CACHE_POLICY" >&2
  exit 1
fi

case "$SERVICE" in
  apiserver)
    MONGODB_PORT="${MONGODB_PORT:-27017}"
    MYSQL_PORT="${MYSQL_PORT:-3306}"
    REDIS_PORT="${REDIS_PORT:-6379}"
    NSQ_NSQD_PORT="${NSQ_NSQD_PORT:-4150}"
    default_redis_ssl
    require_env \
      MONGODB_HOST MONGODB_PORT MONGODB_USERNAME MONGODB_PASSWORD MONGODB_DBNAME \
      MYSQL_HOST MYSQL_PORT MYSQL_USERNAME MYSQL_PASSWORD MYSQL_DATABASE \
      REDIS_HOST REDIS_PORT JWT_SECRET \
      OSS_ACCESS_KEY_ID OSS_ACCESS_KEY_SECRET DELEGATED_SUBJECT_CURRENT_KEY

    cat > "$ENV_FILE" <<EOF
MYSQL_TLS_CA_REF=${MYSQL_TLS_CA_REF:-}
# Auto-generated production environment configuration for QS API Server
# MongoDB split parameter configuration.
QS_APISERVER_MONGODB_HOST=${MONGODB_HOST}:${MONGODB_PORT}
QS_APISERVER_MONGODB_USERNAME=${MONGODB_USERNAME}
QS_APISERVER_MONGODB_PASSWORD=${MONGODB_PASSWORD}
QS_APISERVER_MONGODB_DATABASE=${MONGODB_DBNAME}
QS_APISERVER_MONGODB_MIN_POOL_SIZE=16
QS_APISERVER_MONGODB_MAX_POOL_SIZE=64
QS_APISERVER_MONGODB_MAX_CONNECTING=8
QS_APISERVER_MONGODB_MAX_CONN_IDLE_TIME=10m

QS_APISERVER_MYSQL_HOST=${MYSQL_HOST}:${MYSQL_PORT}
QS_APISERVER_MYSQL_TLS_MODE=${MYSQL_TLS_MODE}
QS_APISERVER_MYSQL_TLS_CA_FILE=/etc/qlume/mysql/ca-chain.pem
QS_APISERVER_MYSQL_USERNAME=${MYSQL_USERNAME}
QS_APISERVER_MYSQL_PASSWORD=${MYSQL_PASSWORD}
QS_APISERVER_MYSQL_DATABASE=${MYSQL_DATABASE}

# Redis single instance.
QS_APISERVER_REDIS_HOST=${REDIS_HOST}
QS_APISERVER_REDIS_PORT=${REDIS_PORT}
QS_APISERVER_REDIS_USERNAME=${REDIS_USERNAME:-}
QS_APISERVER_REDIS_PASSWORD=${REDIS_PASSWORD:-}
QS_APISERVER_REDIS_DATABASE=${REDIS_DB}
QS_APISERVER_REDIS_USE_SSL=${REDIS_USE_SSL}
QS_APISERVER_REDIS_SSL_INSECURE_SKIP_VERIFY=${REDIS_SSL_INSECURE_SKIP_VERIFY}

QS_APISERVER_JWT_SECRET=${JWT_SECRET}

QS_APISERVER_DELEGATED_SUBJECT_CURRENT_KEY=${DELEGATED_SUBJECT_CURRENT_KEY}
QS_APISERVER_DELEGATED_SUBJECT_PREVIOUS_KEY=${DELEGATED_SUBJECT_PREVIOUS_KEY:-}

QS_APISERVER_MESSAGING_NSQ_ADDR=${NSQ_NSQD_HOST}:${NSQ_NSQD_PORT}

QS_APISERVER_OSS_ACCESS_KEY_ID=${OSS_ACCESS_KEY_ID}
QS_APISERVER_OSS_ACCESS_KEY_SECRET=${OSS_ACCESS_KEY_SECRET}
QS_APISERVER_OSS_SESSION_TOKEN=${OSS_SESSION_TOKEN:-}

# The selected AI Provider credential is process-scoped and never stored in YAML.
EOF
    ;;
  collection)
    REDIS_PORT="${REDIS_PORT:-6379}"
    default_redis_ssl
    require_env \
      REDIS_HOST REDIS_PORT JWT_SECRET DELEGATED_SUBJECT_CURRENT_KEY

    cat > "$ENV_FILE" <<EOF
# Auto-generated production environment configuration for Collection Server
# Redis single instance.
COLLECTION_SERVER_REDIS_HOST=${REDIS_HOST}
COLLECTION_SERVER_REDIS_PORT=${REDIS_PORT}
COLLECTION_SERVER_REDIS_USERNAME=${REDIS_USERNAME:-}
COLLECTION_SERVER_REDIS_PASSWORD=${REDIS_PASSWORD:-}
COLLECTION_SERVER_REDIS_DATABASE=${REDIS_DB}
COLLECTION_SERVER_REDIS_USE_SSL=${REDIS_USE_SSL}
COLLECTION_SERVER_REDIS_SSL_INSECURE_SKIP_VERIFY=${REDIS_SSL_INSECURE_SKIP_VERIFY}

COLLECTION_SERVER_JWT_SECRET=${JWT_SECRET}

COLLECTION_SERVER_DELEGATED_SUBJECT_CURRENT_KEY=${DELEGATED_SUBJECT_CURRENT_KEY}
COLLECTION_SERVER_DELEGATED_SUBJECT_PREVIOUS_KEY=${DELEGATED_SUBJECT_PREVIOUS_KEY:-}
EOF
    ;;
  worker)
    MONGODB_PORT="${MONGODB_PORT:-27017}"
    MYSQL_PORT="${MYSQL_PORT:-3306}"
    NSQ_NSQD_PORT="${NSQ_NSQD_PORT:-4150}"
    NSQ_LOOKUPD_PORT="${NSQ_LOOKUPD_PORT:-4161}"
    GRPC_APISERVER_ADDR="${GRPC_APISERVER_ADDR:-qs-apiserver:9090}"
    REDIS_PORT="${REDIS_PORT:-6379}"
    default_redis_ssl
    require_env \
      MONGODB_HOST MONGODB_PORT MONGODB_USERNAME MONGODB_PASSWORD MONGODB_DBNAME \
      MYSQL_HOST MYSQL_PORT MYSQL_USERNAME MYSQL_PASSWORD MYSQL_DATABASE \
      REDIS_HOST REDIS_PORT

    cat > "$ENV_FILE" <<EOF
# Auto-generated production environment configuration for QS Worker
# MongoDB split parameter configuration.
QS_WORKER_MONGODB_HOST=${MONGODB_HOST}:${MONGODB_PORT}
QS_WORKER_MONGODB_USERNAME=${MONGODB_USERNAME}
QS_WORKER_MONGODB_PASSWORD=${MONGODB_PASSWORD}
QS_WORKER_MONGODB_DATABASE=${MONGODB_DBNAME}
QS_WORKER_MONGODB_MIN_POOL_SIZE=4
QS_WORKER_MONGODB_MAX_POOL_SIZE=32
QS_WORKER_MONGODB_MAX_CONNECTING=4
QS_WORKER_MONGODB_MAX_CONN_IDLE_TIME=10m

QS_WORKER_MYSQL_HOST=${MYSQL_HOST}:${MYSQL_PORT}
QS_WORKER_MYSQL_TLS_MODE=${MYSQL_TLS_MODE}
QS_WORKER_MYSQL_TLS_CA_FILE=/etc/qlume/mysql/ca-chain.pem
QS_WORKER_MYSQL_USERNAME=${MYSQL_USERNAME}
QS_WORKER_MYSQL_PASSWORD=${MYSQL_PASSWORD}
QS_WORKER_MYSQL_DATABASE=${MYSQL_DATABASE}

QS_WORKER_MESSAGING_NSQ_ADDR=${NSQ_NSQD_HOST}:${NSQ_NSQD_PORT}
QS_WORKER_MESSAGING_NSQ_LOOKUPD_ADDR=${NSQ_LOOKUPD_HOST}:${NSQ_LOOKUPD_PORT}

QS_WORKER_GRPC_APISERVER_ADDR=${GRPC_APISERVER_ADDR}

# Redis single instance.
QS_WORKER_REDIS_HOST=${REDIS_HOST}
QS_WORKER_REDIS_PORT=${REDIS_PORT}
QS_WORKER_REDIS_USERNAME=${REDIS_USERNAME:-}
QS_WORKER_REDIS_PASSWORD=${REDIS_PASSWORD:-}
QS_WORKER_REDIS_DATABASE=${REDIS_DB}
QS_WORKER_REDIS_USE_SSL=${REDIS_USE_SSL}
QS_WORKER_REDIS_SSL_INSECURE_SKIP_VERIFY=${REDIS_SSL_INSECURE_SKIP_VERIFY}
EOF
    ;;
esac

if [ "$SERVICE" != collection ]; then
  case "$SERVICE" in apiserver) mq_prefix=QS_APISERVER ;; worker) mq_prefix=QS_WORKER ;; esac
  printf '%s_MESSAGING_PROVIDER=%s\n' "$mq_prefix" "$MESSAGING_PROVIDER" >> "$ENV_FILE"
  if [ "$MESSAGING_PROVIDER" = rocketmq ]; then
    for mq_entry in \
      "ENDPOINT=${ROCKETMQ_ENDPOINT}" \
      "CREDENTIALS_FILE=/app/configs/rocketmq/credentials.json" \
      "CA_FILE=/app/configs/rocketmq/ca.crt" \
      "ROUTES_FILE=/app/configs/rocketmq/routes.json" \
      "INSTANCE_ID=${ROCKETMQ_INSTANCE_ID:-}" \
      "PUBLISH_ENABLED=${ROCKETMQ_PUBLISH_ENABLED}"; do
      printf '%s_MESSAGING_ROCKETMQ_%s\n' "$mq_prefix" "$mq_entry" >> "$ENV_FILE"
      if [ "$SERVICE" = apiserver ]; then
        printf 'QS_APISERVER_IAM_AUTHZ_SYNC_ROCKETMQ_%s\n' "$mq_entry" >> "$ENV_FILE"
      fi
    done
    if [ "$SERVICE" = apiserver ]; then
      printf 'QS_APISERVER_IAM_AUTHZ_SYNC_PROVIDER=rocketmq\nQS_APISERVER_IAM_AUTHZ_SYNC_EPHEMERAL_NSQ=false\n' >> "$ENV_FILE"
    fi
  fi
fi
# Redis cloud TLS uses Go's additive system trust directories; retain stock roots.
if [ "${REDIS_HOST:-}" = bj-crs-3dli07u2.sql.tencentcdb.com ] && [ "${REDIS_USE_SSL:-false}" != true ]; then
  echo 'Tencent production Redis requires explicit TLS' >&2
  exit 1
fi
if [ "${REDIS_USE_SSL:-false}" = true ]; then
  if [ "${REDIS_SSL_INSECURE_SKIP_VERIFY:-false}" != false ]; then
    echo 'Production Redis TLS may not skip certificate verification' >&2
    exit 1
  fi
  printf '%s\n' 'SSL_CERT_DIR=/etc/ssl/certs:/etc/pki/tls/certs:/etc/qlume/redis' >> "$ENV_FILE"
fi
chmod 0600 "$ENV_FILE"
echo "Generated config.prod.env for ${SERVICE}:"
redact_env_file "$ENV_FILE"

tar -czf "$DEPLOY_PACKAGE" -C "$PACKAGE_DIR" .
echo "Created ${DEPLOY_PACKAGE}"
