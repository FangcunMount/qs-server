#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
project="qs-m5-best-effort-$(date +%s)-$$-${RANDOM}"
compose=(docker compose --project-name "$project" --file "$repo/scripts/testing/m5-best-effort-nsq-compose.yaml")
context=$(docker context show)
endpoint=$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')
if [[ -n ${DOCKER_HOST:-} || "$endpoint" != unix://* ]]; then
  echo "Best-effort NSQ proof requires a local Unix-socket Docker context without DOCKER_HOST" >&2
  exit 1
fi
docker info >/dev/null

cleanup() {
  result=$?
  trap - EXIT
  if ((result != 0)); then "${compose[@]}" logs --no-color --tail 30 || true; fi
  if ! "${compose[@]}" down --volumes --remove-orphans --timeout 10; then result=1; fi
  if [[ -n ${build_dir:-} ]]; then rm -rf -- "$build_dir"; fi
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${compose[@]}" up -d --wait --wait-timeout 120
architecture=$(docker info --format '{{.Architecture}}')
case "$architecture" in
  aarch64|arm64) goarch=arm64 ;;
  x86_64|amd64) goarch=amd64 ;;
  *) echo "Unsupported Docker architecture: $architecture" >&2; exit 1 ;;
esac
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/$project-build.XXXXXX")
(cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags='integration reliable_messaging_m5' -o "$build_dir/m5-best-effort-nsq.test" ./internal/worker/integration/eventing)
"${compose[@]}" cp "$build_dir/m5-best-effort-nsq.test" nsqd:/tmp/m5-best-effort-nsq.test
"${compose[@]}" cp "$repo/configs/events.yaml" nsqd:/tmp/events.yaml
"${compose[@]}" exec -T \
  -e QS_M5_BEST_EFFORT_NSQ=1 \
  -e QS_M5_EVENTS_CONFIG=/tmp/events.yaml \
  nsqd /tmp/m5-best-effort-nsq.test \
    -test.run '^TestBestEffortExternalFailureFinishesRealNSQDelivery$' \
    -test.count=1 -test.timeout=1m -test.v
