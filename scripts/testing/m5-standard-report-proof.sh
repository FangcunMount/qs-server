#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
project="qs-m5-report-proof-$(date +%s)-$$-${RANDOM}"
compose=(docker compose --project-name "$project" --file "$repo/scripts/testing/m4-standard-process-compose.yaml")
context=$(docker context show)
endpoint=$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')
if [[ -n ${DOCKER_HOST:-} || "$endpoint" != unix://* ]]; then
  echo "M5 proof requires a local Unix-socket Docker context without DOCKER_HOST" >&2
  exit 1
fi
docker info >/dev/null

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

"${compose[@]}" up -d --wait --wait-timeout 180 mongo nsqd mysql
"${compose[@]}" exec -T mongo mongosh --quiet --file /dev/stdin <<'JS'
const result = rs.initiate({_id: 'rm-test', members: [{_id: 0, host: 'mongo:27017'}]});
if (result.ok !== 1) throw new Error('replica set initiation failed');
let primary = false;
for (let i = 0; i < 60; i++) {
  if (db.hello().isWritablePrimary) { primary = true; break; }
  sleep(500);
}
if (!primary) throw new Error('replica set did not become primary');
JS

architecture=$(docker info --format '{{.Architecture}}')
case "$architecture" in
  aarch64|arm64) goarch=arm64 ;;
  x86_64|amd64) goarch=amd64 ;;
  *) echo "Unsupported Docker architecture: $architecture" >&2; exit 1 ;;
esac
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/$project-build.XXXXXX")
(cd "$repo" && GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags='integration,reliable_messaging_m4' \
  -o "$build_dir/m5-report.test" ./internal/apiserver/infra/mongo/interpretation)
"${compose[@]}" cp "$build_dir/m5-report.test" mysql:/tmp/m5-report.test
"${compose[@]}" exec -T mysql mkdir -p /configs
"${compose[@]}" cp "$repo/configs/events.yaml" mysql:/configs/events.yaml
"${compose[@]}" exec -T \
  -e QS_SERVER_TEST_MONGO_URI='mongodb://mongo:27017/?replicaSet=rm-test' \
  -e QS_SERVER_TEST_MONGO_DB_PREFIX='m5_report_contract' \
  -e RM_QS_NSQ_TCP='nsqd:4150' \
  mysql /tmp/m5-report.test \
    -test.run '^TestInterpretationReportEventsReachWorkerThroughStandardMongoAndNSQ$' \
    -test.count=1 -test.timeout=90s -test.v
