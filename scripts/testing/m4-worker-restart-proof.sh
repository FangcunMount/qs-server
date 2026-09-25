#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
project="qs-m4-worker-restart-$(date +%s)-$$-${RANDOM}"
compose=(docker compose --project-name "$project" --file "$repo/scripts/testing/m4-worker-restart-compose.yaml")
context=$(docker context show)
endpoint=$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')
if [[ -n ${DOCKER_HOST:-} || "$endpoint" != unix://* ]]; then
  echo "Worker restart proof requires a local Unix-socket Docker context without DOCKER_HOST" >&2
  exit 1
fi
docker info >/dev/null

cleanup() {
  result=$?
  trap - EXIT
  if ((result != 0)); then "${compose[@]}" logs --no-color --tail 20 || true; fi
  if ! "${compose[@]}" down --volumes --remove-orphans --timeout 10; then result=1; fi
  if [[ -n ${build_dir:-} ]]; then rm -rf -- "$build_dir"; fi
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${compose[@]}" up -d --wait --wait-timeout 180
architecture=$(docker info --format '{{.Architecture}}')
case "$architecture" in
  aarch64|arm64) goarch=arm64 ;;
  x86_64|amd64) goarch=amd64 ;;
  *) echo "Unsupported Docker architecture: $architecture" >&2; exit 1 ;;
esac
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/$project-build.XXXXXX")
(cd "$repo" && GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags=integration -o "$build_dir/m4-worker-restart.test" ./internal/pkg/eventing/transport)
"${compose[@]}" cp "$build_dir/m4-worker-restart.test" mysql:/tmp/m4-worker-restart.test
"${compose[@]}" exec -T \
  -e MESSAGING_INTEGRATION=1 \
  -e MYSQL_DSN='root@tcp(mysql:3306)/?parseTime=true&loc=UTC' \
  -e NSQD_ADDR='nsqd:4150' \
  -e NSQD_HTTP_ADDR='nsqd:4151' \
  -e NSQ_LOOKUPD_ADDR='nsqlookupd:4161' \
  mysql /tmp/m4-worker-restart.test \
    -test.run '^(TestWorkerUnknownEventRecoversAfterSubscriberRestart|TestWorkerUnknownEventRecoversAfterProcessKill)$' \
    -test.count=1 -test.timeout=3m -test.v
