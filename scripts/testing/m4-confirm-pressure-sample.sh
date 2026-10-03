#!/usr/bin/env bash
set -Eeuo pipefail
project=${1:?project}
out=${2:?absolute metrics directory}
[[ "$out" == /* ]]
mkdir -p "$out"
mysql_id=$(docker ps -q --filter "label=com.docker.compose.project=$project" --filter label=com.docker.compose.service=mysql)
nsqd_id=$(docker ps -q --filter "label=com.docker.compose.project=$project" --filter label=com.docker.compose.service=nsqd)
lookup_id=$(docker ps -q --filter "label=com.docker.compose.project=$project" --filter label=com.docker.compose.service=nsqlookupd)
driver_id=$(docker ps -aq --filter "name=^/${project}-driver-1$")
[[ -n "$mysql_id" && -n "$nsqd_id" && -n "$lookup_id" && -n "$driver_id" ]]
docker info --format '{{json .}}' > "$out/host.json"
docker inspect --format '{{json .}}' "$mysql_id" "$nsqd_id" "$lookup_id" "$driver_id" > "$out/containers.jsonl"
while [[ ! -f "$out/stop" ]]; do
  at=$(TZ=Asia/Shanghai date '+%Y-%m-%dT%H:%M:%S%z')
  if [[ $(docker inspect --format '{{.State.Running}}' "$driver_id") == true ]]; then
    docker stats --no-stream --format '{{json .}}' "$mysql_id" "$nsqd_id" "$lookup_id" "$driver_id" |
      while IFS= read -r row; do printf '%s\t%s\n' "$at" "$row"; done >> "$out/docker-stats.jsonl"
    docker exec "$mysql_id" mysql -uroot --batch --raw --skip-column-names \
      -e "SHOW GLOBAL STATUS WHERE Variable_name IN ('Threads_connected','Threads_running','Innodb_row_lock_waits','Innodb_row_lock_time','Com_select','Com_insert','Com_update','Com_commit','Com_rollback')" |
      while IFS= read -r row; do printf '%s\t%s\n' "$at" "$row"; done >> "$out/mysql-status.tsv"
    docker exec "$nsqd_id" wget -qO- 'http://127.0.0.1:4151/stats?format=json' |
      jq -c . | while IFS= read -r row; do printf '%s\t%s\n' "$at" "$row"; done >> "$out/nsqd-status.jsonl"
  fi
  sleep 10
done
