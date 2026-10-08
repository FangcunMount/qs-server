#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
project="qs-m6-qs04-worker-$(date +%s)-$$-${RANDOM}"
compose=(docker compose --project-name "$project" \
  --file "$repo/scripts/testing/m4-evaluation-duplicate-compose.yaml" \
  --file "$repo/scripts/testing/m5-mysql-outcome-business-compose.yaml")
context=$(docker context show)
endpoint=$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')
if [[ -n ${DOCKER_HOST:-} || "$endpoint" != unix://* ]]; then
  echo "QS-04 proof requires local Unix-socket Docker without DOCKER_HOST" >&2
  exit 1
fi
docker info >/dev/null
source_sha=$(git -C "$repo" rev-parse HEAD)
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || { echo "Exact QS source SHA is required" >&2; exit 1; }
printf 'QS source: %s\n' "$source_sha"
printf 'SDK dependency: %s\n' "$(cd "$repo" && GOPROXY=https://proxy.golang.org,direct go list -m github.com/FangcunMount/reliable-messaging)"

cleanup() {
  result=$?
  trap - EXIT
  if [[ -n ${test_pid:-} ]] && kill -0 "$test_pid" 2>/dev/null; then
    kill "$test_pid" 2>/dev/null || true
    wait "$test_pid" 2>/dev/null || true
  fi
  if ((result != 0)); then
    if [[ -f ${build_dir:-}/test.log ]]; then cat "$build_dir/test.log"; fi
    "${compose[@]}" logs --no-color --tail 35 mysql mongo nsqd || true
  fi
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
JS

architecture=$(docker info --format '{{.Architecture}}')
case "$architecture" in
  aarch64|arm64) goarch=arm64 ;;
  x86_64|amd64) goarch=amd64 ;;
  *) echo "Unsupported Docker architecture: $architecture" >&2; exit 1 ;;
esac
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/$project-build.XXXXXX")
(cd "$repo" && GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags='integration,reliable_messaging_m4,reliable_messaging_m5' \
  -ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$source_sha" \
  -o "$build_dir/runtimeclosure.test" ./internal/apiserver/integration/runtimeclosure)
[[ $(git -C "$repo" rev-parse HEAD) == "$source_sha" ]] || { echo "QS source changed during test build" >&2; exit 1; }
"${compose[@]}" exec -T mysql mkdir -p /tmp/m6-qs04/configs /tmp/m6-qs04/internal/apiserver/integration/runtimeclosure
"${compose[@]}" cp "$repo/configs/events.yaml" mysql:/tmp/m6-qs04/configs/events.yaml
"${compose[@]}" cp "$repo/configs/grpc-acl.prod.yaml" mysql:/tmp/m6-qs04/configs/grpc-acl.prod.yaml
"${compose[@]}" cp "$build_dir/runtimeclosure.test" mysql:/tmp/m6-qs04/internal/apiserver/integration/runtimeclosure/runtimeclosure.test

"${compose[@]}" exec -T -w /tmp/m6-qs04/internal/apiserver/integration/runtimeclosure \
  -e QS_RUNTIME_CLOSURE_APPROVED_SOURCE_SHA="$source_sha" \
  -e MYSQL_DSN='root@tcp(mysql:3306)/mysql?parseTime=true&multiStatements=true&loc=Asia%2FShanghai' \
  -e QS_SERVER_TEST_MONGO_URI='mongodb://mongo:27017/?replicaSet=rm-test' \
  -e QS_SERVER_TEST_MONGO_DB_PREFIX='qs_m6_qs04_worker_test' \
  -e QS_SERVER_TEST_REDIS_URL='redis://redis:6379/15' \
  -e RM_QS_M5_NSQ_TCP='nsqd:4150' \
  -e RM_QS04_COORDINATED='1' \
  mysql ./runtimeclosure.test -test.run '^TestM6QS04RecoveredOriginalHasOneWorkerEffect$' -test.count=1 -test.timeout=5m -test.v >"$build_dir/test.log" 2>&1 &
test_pid=$!

ready=false
for attempt in {1..120}; do
  if "${compose[@]}" exec -T mysql test -f /tmp/m6-qs04-broker-kill-ready 2>/dev/null; then
    ready=true
    break
  fi
  if ! kill -0 "$test_pid" 2>/dev/null; then
    wait "$test_pid"
    cat "$build_dir/test.log"
    exit 1
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  echo "Worker test did not reach confirmed Outbox publish before timeout" >&2
  exit 1
fi

capture() {
  local stage=$1
  "${compose[@]}" exec -T nsqd wget -qO- 'http://127.0.0.1:4151/stats?format=json&topic=qs.evaluation.lifecycle' |
    python3 -c '
import json,sys
stage=sys.argv[1]
data=json.load(sys.stdin)
topics=[t for t in data.get("topics",[]) if t.get("topic_name")=="qs.evaluation.lifecycle"]
if len(topics)!=1: raise SystemExit(f"{stage}: expected one topic")
channels=[c for c in topics[0].get("channels",[]) if c.get("channel_name")=="rm-m6-qs04-business-recovery"]
if len(channels)!=1: raise SystemExit(f"{stage}: expected one recovery channel")
c=channels[0]
out={"stage":stage,"topic_messages":topics[0].get("message_count"),
     "depth":c.get("depth"),"backend_depth":c.get("backend_depth"),
     "in_flight":c.get("in_flight_count")}
print(json.dumps(out,sort_keys=True))
if stage=="before_kill" and not (out["topic_messages"]==1 and out["depth"]==1 and out["backend_depth"]==0 and out["in_flight"]==0):
    raise SystemExit("confirmed original message was not solely in channel memory")
if stage=="after_restart" and not (out["depth"]==0 and out["backend_depth"]==0 and out["in_flight"]==0):
    raise SystemExit("broker did not lose in-memory original event")
' "$stage"
}

capture before_kill
broker_id=$("${compose[@]}" ps -q nsqd)
[[ -n "$broker_id" ]]
"${compose[@]}" kill -s KILL nsqd
broker_exit=$(docker inspect -f '{{.State.ExitCode}}' "$broker_id")
printf 'broker_forced_exit=%s\n' "$broker_exit"
[[ "$broker_exit" == 137 ]]
"${compose[@]}" start nsqd
healthy=false
for attempt in {1..60}; do
  if [[ $("${compose[@]}" exec -T nsqd wget -qO- http://127.0.0.1:4151/ping 2>/dev/null) == OK ]]; then
    healthy=true
    break
  fi
  sleep 1
done
[[ "$healthy" == true ]]
capture after_restart
"${compose[@]}" exec -T mysql touch /tmp/m6-qs04-broker-restarted
if ! wait "$test_pid"; then
  cat "$build_dir/test.log"
  exit 1
fi
test_pid=
cat "$build_dir/test.log"
printf 'result=isolated_same_fault_original_recovered_through_worker resources=removed_on_exit\n'
