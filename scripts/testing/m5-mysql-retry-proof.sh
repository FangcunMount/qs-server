#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
project="qs-m5-mysql-proof-$(date +%s)-$$-${RANDOM}"
compose=(docker compose --project-name "$project" --file "$repo/scripts/testing/m4-standard-process-compose.yaml")
context=$(docker context show)
endpoint=$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')
if [[ -n ${DOCKER_HOST:-} || "$endpoint" != unix://* ]]; then
  echo "M5 proof requires a local Unix-socket Docker context without DOCKER_HOST" >&2
  exit 1
fi
docker info >/dev/null
echo "QS source: $(git -C "$repo" rev-parse HEAD)"
echo "SDK dependency: $(cd "$repo" && GOPROXY=https://proxy.golang.org,direct go list -m github.com/FangcunMount/reliable-messaging)"

cleanup() {
  result=$?
  trap - EXIT
  if ((result != 0)); then "${compose[@]}" logs --no-color --tail 35 mysql || true; fi
  if ! "${compose[@]}" down --volumes --remove-orphans --timeout 10; then result=1; fi
  if [[ -n ${build_dir:-} ]]; then rm -rf -- "$build_dir"; fi
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${compose[@]}" up -d --wait --wait-timeout 180 mysql
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m5_qs_retry'
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m5_qs_outcome'

architecture=$(docker info --format '{{.Architecture}}')
case "$architecture" in
  aarch64|arm64) goarch=arm64 ;;
  x86_64|amd64) goarch=amd64 ;;
  *) echo "Unsupported Docker architecture: $architecture" >&2; exit 1 ;;
esac
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/$project-build.XXXXXX")
(cd "$repo" && GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags='integration,reliable_messaging,reliable_messaging_m4,reliable_messaging_m5' \
  -o "$build_dir/m5-mysql-retry.test" ./internal/apiserver/container/internal/transaction)
"${compose[@]}" cp "$build_dir/m5-mysql-retry.test" mysql:/tmp/m5-mysql-retry.test
"${compose[@]}" exec -T \
  -e RM_QS_M5_MYSQL_DSN='root@tcp(mysql:3306)/m5_qs_retry?parseTime=true&loc=UTC' \
  mysql /tmp/m5-mysql-retry.test \
    -test.run '^TestM5StandardEvaluationFailureAndScheduledRetryTransaction$' -test.count=1 -test.timeout=2m -test.v
"${compose[@]}" exec -T \
  -e RM_QS_M5_OUTCOME_DSN='root@tcp(mysql:3306)/m5_qs_outcome?parseTime=true&loc=UTC' \
  mysql /tmp/m5-mysql-retry.test \
    -test.run '^TestM5StandardEvaluationOutcomeOriginalTransaction$' -test.count=1 -test.timeout=2m -test.v
