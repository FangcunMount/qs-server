#!/usr/bin/env bash
set -Eeuo pipefail

# Start the actual published API images in both directions on disposable
# MySQL/Mongo/Redis/NSQ. This checks process and schema compatibility only;
# no business event is submitted by this harness.
harness=${1:?Usage: handoff.sh HARNESS CURRENT_IMAGE FALLBACK_IMAGE}
current_image=${2:?Usage: handoff.sh HARNESS CURRENT_IMAGE FALLBACK_IMAGE}
fallback_image=${3:?Usage: handoff.sh HARNESS CURRENT_IMAGE FALLBACK_IMAGE}
[[ "$harness" = /* && -f "$harness/scripts/testing/m6-qs-api-image-schema90.yaml" ]]

project="qs-m6-api-image-${GITHUB_RUN_ID:-local}-$$"
api="${project}-api"
compose=(docker compose --project-name "$project" --file "$harness/scripts/testing/m6-qs-worker-image-compose.yaml")
config="$harness/scripts/testing/m6-qs-api-image-schema90.yaml"
network="${project}_default"

cleanup() {
  result=$?
  trap - EXIT
  if ((result != 0)); then
    docker logs --tail 30 "$api" 2>/dev/null | grep -E 'migration|Migration|Failed to prepare|FATAL|ERROR' || true
    "${compose[@]}" logs --tail 15 --no-color mysql mongo redis nsqd nsqlookupd 2>/dev/null || true
  fi
  docker rm -f "$api" >/dev/null 2>&1 || true
  "${compose[@]}" down --volumes --remove-orphans --timeout 10 >/dev/null || result=1
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${compose[@]}" up -d --wait --wait-timeout 180
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE qs'

assert_schema() {
  local mysql_version mongo_version
  mysql_version=$("${compose[@]}" exec -T mysql mysql -uroot --batch --skip-column-names \
    -e 'SELECT version, dirty FROM qs.schema_migrations')
  [[ "$mysql_version" == $'90\t0' ]] || {
    echo "MySQL schema is not clean version 90: $mysql_version" >&2
    return 1
  }
  mongo_version=$("${compose[@]}" exec -T mongo mongosh --quiet qs \
    --eval 'const row=db.schema_migrations.findOne(); print(row.version+" "+row.dirty)')
  [[ "$mongo_version" == '36 false' ]] || {
    echo "MongoDB schema is not clean version 36: $mongo_version" >&2
    return 1
  }
}

start_api() {
  local image=$1 phase=$2 state
  [[ $(docker image inspect "$image" --format '{{.Os}}/{{.Architecture}} {{.Config.User}}') == 'linux/amd64 www' ]]
  docker run -d --name "$api" --network "$network" \
    -v "$config:/tmp/apiserver.isolation.yaml:ro" \
    "$image" --config /tmp/apiserver.isolation.yaml \
    --cache.policy-file=/app/configs/cache/apiserver.dev.yaml >/dev/null
  for _ in {1..90}; do
    state=$(docker inspect "$api" --format '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}')
    if [[ "$state" == 'running healthy' ]]; then
      docker exec "$api" wget -qO- http://127.0.0.1:8080/healthz | grep -Fq '"status":"ok"'
      assert_schema
      printf '%s: image=%s mysql=90/clean mongo=36/clean health=ok\n' "$phase" "$image"
      return 0
    fi
    [[ "$state" != exited* && "$state" != 'running unhealthy' ]] || {
      echo "$phase API exited or became unhealthy: $state" >&2
      return 1
    }
    sleep 1
  done
  echo "$phase API did not become healthy" >&2
  return 1
}

stop_api() {
  docker stop --time 15 "$api" >/dev/null
  docker rm "$api" >/dev/null
}

topic=m6.api.handoff
nsqd_container=$("${compose[@]}" ps -q nsqd)
[[ -n "$nsqd_container" ]]
docker exec "$nsqd_container" wget -qO- --post-data '' \
  "http://127.0.0.1:4151/topic/create?topic=${topic}" >/dev/null
docker exec "$nsqd_container" wget -qO- --post-data '' \
  "http://127.0.0.1:4151/channel/create?topic=${topic}&channel=m6-api-proof" >/dev/null

seed_pending() {
  local id=$1
  M6_PROBE_ID="$id" python3 - <<'PY' | "${compose[@]}" exec -T mysql mysql -uroot qs
import datetime
import hashlib
import json
import os
import struct

event_id = os.environ["M6_PROBE_ID"]
occurred = datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00", "Z")
payload = json.dumps({"probe_id": event_id}, separators=(",", ":")).encode()
fields = ["qs-server", event_id, "m6.api.handoff", "m6.api.handoff", "v1", "org:1", "application/json", occurred]
parts = [field.encode() for field in fields] + [payload]
digest = hashlib.sha256(b"rm-fingerprint-draft-v1\x00")
for part in parts:
    digest.update(struct.pack(">Q", len(part)))
    digest.update(part)
values = ["X'" + part.hex() + "'" for part in parts]
values.append("X'" + digest.hexdigest() + "'")
values.append("UTC_TIMESTAMP(6)")
print("INSERT INTO rm_outbox ")
print("(producer,message_id,destination,event_type,schema_version,scope,content_type,occurred_at,payload,fingerprint,next_attempt_at)")
print("VALUES (" + ",".join(values) + ");")
PY
}

topic_count() {
  docker exec "$nsqd_container" wget -qO- 'http://127.0.0.1:4151/stats?format=json' |
    python3 -c 'import json,sys; data=json.load(sys.stdin); print(next((t["message_count"] for t in data["topics"] if t["topic_name"]=="m6.api.handoff"), -1))'
}

# Keep only metadata and a recomputed immutable-content comparison in CI logs.
# The SQL row remains in the disposable database until cleanup; never print
# its payload or any connection configuration.
diagnose_row() {
  local id=$1
  "${compose[@]}" exec -T mysql mysql -uroot qs --batch --raw --skip-column-names \
    -e "SELECT state,last_error_code,attempt_count,failure_count,HEX(producer),HEX(message_id),HEX(destination),HEX(event_type),HEX(schema_version),HEX(scope),HEX(content_type),HEX(occurred_at),HEX(payload),HEX(fingerprint) FROM rm_outbox WHERE message_id='$id'" |
    python3 -c '
import hashlib, struct, sys
line = sys.stdin.read().rstrip("\n")
values = line.split("\t")
if len(values) != 14:
    raise SystemExit("probe diagnostic row shape mismatch")
state, code, attempts, failures = values[:4]
parts = [bytes.fromhex(value) for value in values[4:13]]
stored = bytes.fromhex(values[13])
digest = hashlib.sha256(b"rm-fingerprint-draft-v1\x00")
for part in parts:
    digest.update(struct.pack(">Q", len(part)))
    digest.update(part)
print("probe diagnostic: state=%s code=%s attempts=%s failures=%s fingerprint_match=%s field_lengths=%s" %
      (state, code or "<empty>", attempts, failures, digest.digest() == stored,
       ",".join(str(len(part)) for part in parts)))
'
}

wait_confirmed() {
  local id=$1 expected_count=$2 row count
  for _ in {1..60}; do
    row=$("${compose[@]}" exec -T mysql mysql -uroot --batch --skip-column-names \
      -e "SELECT state,attempt_count FROM qs.rm_outbox WHERE message_id='$id'" 2>/dev/null || true)
    count=$(topic_count 2>/dev/null || true)
    if [[ "$row" == $'published\t1' && "$count" == "$expected_count" ]]; then
      printf 'original=%s state=published attempts=1 nsq_topic_messages=%s\n' "$id" "$count"
      return 0
    fi
    [[ "$row" != quarantined* ]] || { echo "Quarantined $id" >&2; diagnose_row "$id"; return 1; }
    sleep 1
  done
  echo "No single confirmed delivery for $id: row=$row topic_count=$count" >&2
  diagnose_row "$id"
  return 1
}

id_prefix="m6-api-${GITHUB_RUN_ID:-local}-$$"
start_api "$current_image" current_first
stop_api
seed_pending "${id_prefix}-fallback"
start_api "$fallback_image" fallback
wait_confirmed "${id_prefix}-fallback" 1
stop_api
seed_pending "${id_prefix}-current"
start_api "$current_image" current_return
wait_confirmed "${id_prefix}-current" 2
original=$("${compose[@]}" exec -T mysql mysql -uroot --batch --skip-column-names \
  -e "SELECT state,attempt_count FROM qs.rm_outbox WHERE message_id='${id_prefix}-fallback'")
[[ "$original" == $'published\t1' ]]
stop_api
printf 'API image handoff: current -> fallback -> current healthy; two original MySQL IDs, two NSQ publishes, no repeat\n'
