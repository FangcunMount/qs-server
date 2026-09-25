#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
project="qs-m4-proof-$(date +%s)-$$-${RANDOM}"
compose=(docker compose --project-name "$project" --file "$repo/scripts/testing/m4-standard-process-compose.yaml")
context=$(docker context show)
endpoint=$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')
if [[ -n ${DOCKER_HOST:-} || "$endpoint" != unix://* ]]; then
  echo "M4 proof requires a local Unix-socket Docker context without DOCKER_HOST" >&2
  exit 1
fi
docker info >/dev/null
echo "QS source: $(git -C "$repo" rev-parse HEAD)"
echo "SDK dependency: $(cd "$repo" && GOPROXY=https://proxy.golang.org,direct go list -m github.com/FangcunMount/reliable-messaging)"

cleanup() {
  result=$?
  trap - EXIT
  if ((result != 0)); then "${compose[@]}" logs --no-color --tail 35 || true; fi
  if ! "${compose[@]}" down --volumes --remove-orphans --timeout 10; then result=1; fi
  if [[ -n ${build_dir:-} ]]; then rm -rf -- "$build_dir"; fi
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${compose[@]}" up -d --wait --wait-timeout 180
"${compose[@]}" exec -T mongo mongosh --quiet --file /dev/stdin <<'JS'
const result = rs.initiate({_id: 'rm-test', members: [{_id: 0, host: 'mongo:27017'}]});
if (result.ok !== 1) throw new Error('replica set initiation failed');
let primary = false;
for (let i = 0; i < 60; i++) {
  if (db.hello().isWritablePrimary) { primary = true; break; }
  sleep(500);
}
if (!primary) throw new Error('replica set did not become primary');
print('PASS isolated Mongo replica set');
JS
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m4_qs_bootstrap'
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m4_qs_chain'
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m5_qs_mongo_only'

architecture=$(docker info --format '{{.Architecture}}')
case "$architecture" in
  aarch64|arm64) goarch=arm64 ;;
  x86_64|amd64) goarch=amd64 ;;
  *) echo "Unsupported Docker architecture: $architecture" >&2; exit 1 ;;
esac
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/$project-build.XXXXXX")
(cd "$repo" && GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags='reliable_messaging_m4,reliable_messaging_m4_integration' \
  -o "$build_dir/m4-process.test" ./internal/apiserver/process)
(cd "$repo" && GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags='integration,reliable_messaging,reliable_messaging_m4,reliable_messaging_m4_integration' \
  -o "$build_dir/m4-answer-chain.test" ./internal/apiserver/container/internal/transaction)

"${compose[@]}" exec -T mysql mkdir -p /tmp/m4-qs-bootstrap/configs /tmp/m4-qs-bootstrap/mysql
"${compose[@]}" cp "$build_dir/m4-process.test" mysql:/tmp/m4-qs-bootstrap/m4-process.test
"${compose[@]}" cp "$build_dir/m4-answer-chain.test" mysql:/tmp/m4-qs-bootstrap/m4-answer-chain.test
"${compose[@]}" cp "$repo/configs/events.yaml" mysql:/tmp/m4-qs-bootstrap/configs/events.yaml
"${compose[@]}" cp "$repo/configs/grpc-acl.prod.yaml" mysql:/tmp/m4-qs-bootstrap/configs/grpc-acl.prod.yaml
"${compose[@]}" cp "$repo/internal/pkg/migration/migrations/mysql/000084_standard_reliable_outbox.up.sql" mysql:/tmp/m4-qs-bootstrap/mysql/000084_standard_reliable_outbox.up.sql
"${compose[@]}" cp "$repo/internal/pkg/migration/migrations/mysql/000048_add_system_governance_action_runs.up.sql" mysql:/tmp/m4-qs-bootstrap/mysql/000048_add_system_governance_action_runs.up.sql
"${compose[@]}" cp "$repo/internal/pkg/migration/migrations/mysql/000085_system_governance_pending_replay_index.up.sql" mysql:/tmp/m4-qs-bootstrap/mysql/000085_system_governance_pending_replay_index.up.sql

# The business-chain test subscribes to the production-shaped topic. Run it
# before the sender-only fault proof, whose topic would otherwise hold messages
# that the later subscription could mistake for the chain's own event.
"${compose[@]}" exec -T \
  -e RM_QS_MONGO_URI='mongodb://mongo:27017/?replicaSet=rm-test' \
  -e RM_QS_GRPC_ACL_CONFIG='/tmp/m4-qs-bootstrap/configs/grpc-acl.prod.yaml' \
  -e RM_QS_ASSESSMENT_DSN='root@tcp(mysql:3306)/m4_qs_chain?parseTime=true&loc=UTC' \
  -e RM_QS_NSQ_TCP='nsqd:4150' \
  mysql /tmp/m4-qs-bootstrap/m4-answer-chain.test \
    -test.run '^(TestStandardAnswerSheetToAssessmentAcrossNSQ|TestM5CollectionAdmissionReceiptFollowsStandardMongoCommit)$' -test.count=1 -test.timeout=2m -test.v

"${compose[@]}" exec -T \
  -e RM_QS_BOOTSTRAP_MYSQL_DSN='root@tcp(mysql:3306)/m4_qs_bootstrap?parseTime=true&loc=UTC' \
  -e RM_QS_BOOTSTRAP_MONGO_URI='mongodb://mongo:27017/?replicaSet=rm-test' \
  -e RM_QS_BOOTSTRAP_NSQ_ADDR='nsqd:4150' \
  -e RM_QS_BOOTSTRAP_CATALOG='/tmp/m4-qs-bootstrap/configs/events.yaml' \
  -e RM_QS_BOOTSTRAP_MYSQL_MIGRATION='/tmp/m4-qs-bootstrap/mysql/000084_standard_reliable_outbox.up.sql' \
  -e RM_QS_BOOTSTRAP_AUDIT_MIGRATION='/tmp/m4-qs-bootstrap/mysql/000048_add_system_governance_action_runs.up.sql' \
  -e RM_QS_BOOTSTRAP_AUDIT_INDEX_MIGRATION='/tmp/m4-qs-bootstrap/mysql/000085_system_governance_pending_replay_index.up.sql' \
  mysql /tmp/m4-qs-bootstrap/m4-process.test \
    -test.run '^TestM4ProcessBootstrapRunsSelectedStandardProfiles$' -test.count=1 -test.timeout=4m -test.v

"${compose[@]}" exec -T \
  -e RM_QS_MONGO_ONLY_MYSQL_DSN='root@tcp(mysql:3306)/m5_qs_mongo_only?parseTime=true&loc=UTC' \
  -e RM_QS_BOOTSTRAP_MONGO_URI='mongodb://mongo:27017/?replicaSet=rm-test' \
  -e RM_QS_BOOTSTRAP_NSQ_ADDR='nsqd:4150' \
  -e RM_QS_BOOTSTRAP_CATALOG='/tmp/m4-qs-bootstrap/configs/events.yaml' \
  -e RM_QS_BOOTSTRAP_AUDIT_MIGRATION='/tmp/m4-qs-bootstrap/mysql/000048_add_system_governance_action_runs.up.sql' \
  mysql /tmp/m4-qs-bootstrap/m4-process.test \
    -test.run '^TestM5MongoOnlyProcessKeepsLegacyMySQLAndHotRankSubscription$' -test.count=1 -test.timeout=2m -test.v
