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

start_api "$current_image" current_first
stop_api
start_api "$fallback_image" fallback
stop_api
start_api "$current_image" current_return
stop_api
printf 'API image handoff: current -> fallback -> current processes healthy on one disposable schema 90/36\n'
