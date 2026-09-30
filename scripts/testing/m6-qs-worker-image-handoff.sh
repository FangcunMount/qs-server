#!/usr/bin/env bash
set -Eeuo pipefail

# Real Worker processes, built from exact production/fallback source commits,
# exchange one durable NSQ channel on disposable databases. The probe is an
# unknown event: both versions must persist its original identity before ACK.
harness=${1:?Usage: handoff.sh HARNESS CURRENT_SOURCE CURRENT_IMAGE FALLBACK_IMAGE}
current_source=${2:?Usage: handoff.sh HARNESS CURRENT_SOURCE CURRENT_IMAGE FALLBACK_IMAGE}
current_image=${3:?Usage: handoff.sh HARNESS CURRENT_SOURCE CURRENT_IMAGE FALLBACK_IMAGE}
fallback_image=${4:?Usage: handoff.sh HARNESS CURRENT_SOURCE CURRENT_IMAGE FALLBACK_IMAGE}
current_sha=${5:-7ca541afc24d13187c5b894a249126eabc009ae2}
fallback_sha=${6:-7c0c919e621d3cc93cb82dca668bd6077f7a1446}
[[ "$harness" = /* && -f "$harness/scripts/testing/m6-qs-worker-image-compose.yaml" ]]
[[ "$current_source" = /* && -f "$current_source/internal/pkg/migration/migrations/mysql/000049_add_retry_governance.up.sql" ]]
verify_image() {
  local image=$1 expected_sha=$2 identity
  identity=$(docker image inspect "$image" --format '{{.Os}}/{{.Architecture}} {{.Config.User}} {{index .Config.Labels "org.opencontainers.image.revision"}}')
  if [[ "$expected_sha" == - ]]; then
    # Published images have no revision label; the caller verifies RepoDigest.
    [[ "$identity" == 'linux/amd64 www ' ]]
  else
    [[ "$identity" == "linux/amd64 www $expected_sha" ]]
  fi
}
verify_image "$current_image" "$current_sha"
verify_image "$fallback_image" "$fallback_sha"

project="qs-m6-image-${GITHUB_RUN_ID:-local}-$$"
worker="${project}-worker"
compose=(docker compose --project-name "$project" --file "$harness/scripts/testing/m6-qs-worker-image-compose.yaml")
cleanup() {
  result=$?
  trap - EXIT
  if ((result != 0)); then
    docker logs --tail 80 "$worker" 2>/dev/null || true
    "${compose[@]}" logs --tail 30 --no-color mysql mongo redis nsqd nsqlookupd 2>/dev/null || true
  fi
  docker rm -f "$worker" >/dev/null 2>&1 || true
  "${compose[@]}" down --volumes --remove-orphans --timeout 10 >/dev/null || result=1
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${compose[@]}" up -d --wait --wait-timeout 180
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE qs'
# Only the two Worker transport ledgers are needed. Read the real migration
# definitions, instead of keeping a second hand-maintained test schema.
awk '/^CREATE TABLE `event_delivery_dead_letter` / { copy=1 } copy { print }' \
  "$current_source/internal/pkg/migration/migrations/mysql/000049_add_retry_governance.up.sql" |
  "${compose[@]}" exec -T mysql mysql -uroot qs
"${compose[@]}" exec -T mysql mysql -uroot qs \
  < "$current_source/internal/pkg/migration/migrations/mysql/000050_add_retry_event_hold.up.sql"
"${compose[@]}" exec -T mysql mysql -uroot qs \
  < "$current_source/internal/pkg/migration/migrations/mysql/000086_dead_letter_transport_identity.up.sql"

network="${project}_default"
nsqd_container=$("${compose[@]}" ps -q nsqd)
[[ -n "$nsqd_container" ]]
channel_clients() {
  docker exec "$nsqd_container" wget -qO- 'http://127.0.0.1:4151/stats?format=json' |
    python3 -c 'import json,sys; s=json.load(sys.stdin); print(sum(len(c.get("clients", [])) for t in s.get("topics", []) if t.get("topic_name") == "qs.plan.task" for c in t.get("channels", []) if c.get("channel_name") == "qs-worker"))'
}
start_worker() {
  local image=$1
  docker run -d --name "$worker" --network "$network" \
    -e QS_WORKER_MYSQL_HOST=mysql:3306 \
    -e QS_WORKER_MYSQL_USERNAME=root \
    -e QS_WORKER_MONGODB_URL=mongodb://mongo:27017/questionnaire_scale \
    -e QS_WORKER_REDIS_HOST=redis \
    -e QS_WORKER_GRPC_INSECURE=true \
    -e QS_WORKER_WORKER_CONCURRENCY=1 \
    "$image" >/dev/null
  for _ in {1..90}; do
    if [[ $(docker inspect "$worker" --format '{{.State.Running}}' 2>/dev/null) != true ]]; then
      echo "Worker image exited before readiness" >&2
      return 1
    fi
    if [[ $(channel_clients 2>/dev/null || true) == 1 ]]; then
      docker exec "$worker" wget -qO- http://127.0.0.1:9092/healthz >/dev/null
      return 0
    fi
    sleep 1
  done
  echo "Worker image did not reach its subscription lifecycle" >&2
  return 1
}
stop_worker() {
  docker stop --time 15 "$worker" >/dev/null
  docker rm "$worker" >/dev/null
}

publish_unknown() {
  local id=$1 wire response
  wire=$(MESSAGE_ID="$id" python3 - <<'PY'
import base64, json, os
event = {"id": os.environ["MESSAGE_ID"], "eventType": "m6.image.unknown", "data": {}}
wire = {
    "type": "component-base.messaging.message.v1",
    "uuid": os.environ["MESSAGE_ID"],
    "metadata": {"event_type": "m6.image.unknown"},
    "payload": base64.b64encode(json.dumps(event, separators=(",", ":")).encode()).decode(),
}
print(json.dumps(wire, separators=(",", ":")))
PY
)
  response=$(docker exec "$nsqd_container" wget -qO- --post-data "$wire" \
    'http://127.0.0.1:4151/pub?topic=qs.plan.task')
  [[ "$response" == OK ]]
}
wait_audit() {
  local id=$1 row
  for _ in {1..60}; do
    row=$("${compose[@]}" exec -T mysql mysql -uroot qs --batch --skip-column-names \
      -e "SELECT transport_message_id,last_error,retry_disposition,delivery_attempts FROM event_delivery_dead_letter WHERE message_id='$id'" 2>/dev/null || true)
    if [[ -n "$row" ]]; then
      IFS=$'\t' read -r physical_id cause disposition attempts <<< "$row"
      [[ -n "$physical_id" && "$cause" == 'unknown event type: m6.image.unknown' && "$disposition" == manual_required && "$attempts" == 1 ]] || {
        echo "Invalid durable unknown-event audit for $id: $row" >&2
        return 1
      }
      printf 'audited %s physical=%s\n' "$id" "$physical_id"
      return 0
    fi
    sleep 1
  done
  echo "No durable audit for $id" >&2
  return 1
}

id_prefix="m6-image-${GITHUB_RUN_ID:-local}-$$"
start_worker "$fallback_image"
publish_unknown "${id_prefix}-fallback-live"
wait_audit "${id_prefix}-fallback-live"
stop_worker

# Message arrives with no consumer, so the pre-existing durable channel must
# retain it for the newly started version. Repeat in reverse after stopping it.
publish_unknown "${id_prefix}-current-handoff"
start_worker "$current_image"
wait_audit "${id_prefix}-current-handoff"
stop_worker

publish_unknown "${id_prefix}-fallback-return"
start_worker "$fallback_image"
wait_audit "${id_prefix}-fallback-return"
stop_worker

total=$("${compose[@]}" exec -T mysql mysql -uroot qs --batch --skip-column-names \
  -e "SELECT COUNT(*) FROM event_delivery_dead_letter WHERE message_id LIKE '${id_prefix}-%'")
[[ "$total" == 3 ]]
printf 'Worker image handoff: three original IDs, three durable audits, no duplicate rows\n'
