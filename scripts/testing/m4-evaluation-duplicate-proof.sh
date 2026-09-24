#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
project="qs-m4-evaluation-duplicate-$(date +%s)-$$-${RANDOM}"
compose=(docker compose --project-name "$project" --file "$repo/scripts/testing/m4-evaluation-duplicate-compose.yaml")

cleanup() {
  result=$?
  trap - EXIT
  if ! "${compose[@]}" down --volumes --remove-orphans --timeout 10; then result=1; fi
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
JS

architecture=$(docker info --format '{{.Architecture}}')
case "$architecture" in
  aarch64|arm64) goarch=arm64 ;;
  x86_64|amd64) goarch=amd64 ;;
  *) echo "Unsupported Docker architecture: $architecture" >&2; exit 1 ;;
esac
binary=/tmp/qs-m4-evaluation-duplicate.test
(cd "$repo" && GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags=integration -o "$binary" ./internal/apiserver/integration/runtimeclosure)
"${compose[@]}" exec -T mysql mkdir -p /tmp/m4-duplicate/configs /tmp/m4-duplicate/internal/apiserver/integration/runtimeclosure
"${compose[@]}" cp "$repo/configs/events.yaml" mysql:/tmp/m4-duplicate/configs/events.yaml
"${compose[@]}" cp "$binary" mysql:/tmp/m4-duplicate/internal/apiserver/integration/runtimeclosure/runtimeclosure.test
"${compose[@]}" exec -T -w /tmp/m4-duplicate/internal/apiserver/integration/runtimeclosure \
  -e MYSQL_DSN='root@tcp(mysql:3306)/mysql?parseTime=true&multiStatements=true&loc=Asia%2FShanghai' \
  -e QS_SERVER_TEST_MONGO_URI='mongodb://mongo:27017/?replicaSet=rm-test' \
  -e QS_SERVER_TEST_MONGO_DB_PREFIX='qs_m4_duplicate_test' \
  -e QS_SERVER_TEST_REDIS_URL='redis://redis:6379/15' \
  mysql ./runtimeclosure.test -test.run '^TestCurrentRuntimeClosure$' -test.count=1 -test.timeout=15m -test.v
