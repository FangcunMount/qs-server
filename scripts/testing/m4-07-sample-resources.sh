#!/usr/bin/env bash
set -euo pipefail

project=${1:?compose project required}
compose_file=${2:?compose file required}
output_dir=${3:?absolute output directory required}
profile=${4:?old or new profile required}
stream=${5:?answer or assessment stream required}
[[ "$output_dir" == /* ]] || { echo 'resource output directory must be absolute' >&2; exit 1; }
[[ "$profile" == old || "$profile" == new ]] || { echo 'unknown M4-07 profile' >&2; exit 1; }
[[ "$stream" == answer || "$stream" == assessment || "$stream" == both ]] || { echo 'unknown M4-07 stream' >&2; exit 1; }
command -v jq >/dev/null || { echo 'jq is required for resource sampling' >&2; exit 1; }
mkdir -p "$output_dir"
compose=(docker compose --project-name "$project" --file "$compose_file")
mysql_id=$("${compose[@]}" ps -q mysql)
mongo_id=$("${compose[@]}" ps -q mongo)
nsqd_id=$("${compose[@]}" ps -q nsqd)
[[ -n "$mysql_id" && -n "$mongo_id" && -n "$nsqd_id" ]] || {
  echo 'all three disposable services must be running' >&2
  exit 1
}

docker info --format '{{json .}}' | jq -c '{architecture:.Architecture,cpus:.NCPU,memory_bytes:.MemTotal,server_version:.ServerVersion}' > "$output_dir/host.json"
docker inspect --format '{{json .}}' "$mysql_id" "$mongo_id" "$nsqd_id" |
  jq -c '{name:.Name,image:.Image,nano_cpus:.HostConfig.NanoCpus,memory_limit_bytes:.HostConfig.Memory}' > "$output_dir/containers.jsonl"

capture() {
  local result
  for attempt in 1 2 3 4 5; do
    if result=$("$@" 2>> "$output_dir/retry-errors.log"); then
      printf '%s\n' "$result"
      return 0
    fi
    sleep 1
  done
  echo "resource sample failed after five attempts: $1" >&2
  return 1
}

while :; do
  at=$(TZ=Asia/Shanghai date '+%Y-%m-%dT%H:%M:%S%z')
  driver="${project}-driver-1"
  if driver_id=$(docker ps -q --filter "name=^/${driver}$") && [[ -n "$driver_id" ]]; then
    if [[ ! -s "$output_dir/driver-container.json" ]]; then
      docker inspect --format '{{json .}}' "$driver_id" |
        jq -c '{name:.Name,image:.Image,nano_cpus:.HostConfig.NanoCpus,memory_limit_bytes:.HostConfig.Memory}' > "$output_dir/driver-container.json"
    fi
    capture docker stats --no-stream --format '{{json .}}' "$driver_id" |
      while IFS= read -r row; do printf '%s\t%s\n' "$at" "$row"; done >> "$output_dir/driver-stats.jsonl"
  fi
  capture docker stats --no-stream --format '{{json .}}' "$mysql_id" "$mongo_id" "$nsqd_id" |
    while IFS= read -r row; do printf '%s\t%s\n' "$at" "$row"; done >> "$output_dir/docker-stats.jsonl"
  capture "${compose[@]}" exec -T mysql mysql -uroot --batch --raw --skip-column-names \
    -e "SHOW GLOBAL STATUS WHERE Variable_name IN ('Threads_connected','Threads_running','Innodb_row_lock_waits','Innodb_row_lock_time','Innodb_rows_read','Innodb_rows_inserted','Innodb_rows_updated','Innodb_buffer_pool_reads','Com_select','Com_insert','Com_update','Com_commit','Com_rollback','Questions')" |
    while IFS= read -r row; do printf '%s\t%s\n' "$at" "$row"; done >> "$output_dir/mysql-status.tsv"
  capture "${compose[@]}" exec -T mongo mongosh --quiet --eval \
    'const s=db.getSiblingDB("admin").serverStatus(); print(JSON.stringify({connections:s.connections,globalLock:s.globalLock,opLatencies:s.opLatencies,opcounters:s.opcounters,wiredTigerTransactions:s.wiredTiger?.concurrentTransactions}))' |
    while IFS= read -r row; do printf '%s\t%s\n' "$at" "$row"; done >> "$output_dir/mongo-status.jsonl"
  capture "${compose[@]}" exec -T nsqd wget -qO- 'http://127.0.0.1:4151/stats?format=json' |
    jq -c '{topics:[.topics[] | {name:.topic_name,message_count,depth,channels:[.channels[] | {name:.channel_name,message_count,depth,in_flight_count,deferred_count,requeue_count,timeout_count,clients:(.clients|length)}]}]}' |
    while IFS= read -r row; do printf '%s\t%s\n' "$at" "$row"; done >> "$output_dir/nsqd-status.jsonl"
  if [[ "$stream" == assessment ]]; then
    mysql_db="m4_qs_${profile}_assessment"
    if [[ "$profile" == old ]]; then mysql_table=domain_event_outbox; mysql_state=status
    else mysql_table=rm_outbox; mysql_state=state; fi
    exists=$(capture "${compose[@]}" exec -T mysql mysql -uroot --batch --raw --skip-column-names \
      -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$mysql_db' AND table_name='$mysql_table'")
    if [[ "$exists" == 1 ]]; then
      snapshot=$(capture "${compose[@]}" exec -T mysql mysql -uroot --batch --raw --skip-column-names --database="$mysql_db" \
        -e "SELECT UTC_TIMESTAMP(6), COUNT(*), COALESCE(SUM($mysql_state='published'),0), COALESCE(SUM($mysql_state<>'published'),0), MIN(IF($mysql_state<>'published',created_at,NULL)) FROM $mysql_table")
      printf '%s\t%s\n' "$at" "$snapshot" >> "$output_dir/mysql-outbox.tsv"
    else
      printf '%s\tabsent\n' "$at" >> "$output_dir/mysql-outbox.tsv"
    fi
  elif [[ "$stream" == answer ]]; then
    mongo_db="m4_07_${profile}_answer_chain"
    if [[ "$profile" == old ]]; then mongo_coll=domain_event_outbox; mongo_state=status
    else mongo_coll=rm_outbox; mongo_state=state; fi
    capture "${compose[@]}" exec -T mongo mongosh --quiet --eval \
      "const c=db.getSiblingDB('$mongo_db').getCollection('$mongo_coll'); const field='$mongo_state'; const pending=c.aggregate([{'\$match':{[field]:{'\$ne':'published'}}},{'\$group':{_id:null,count:{'\$sum':1},oldest:{'\$min':'\$created_at'}}}]).toArray(); print(JSON.stringify({sampled_at:new Date(),total:c.countDocuments({}),unfinished:pending[0]?.count??0,oldest_unfinished:pending[0]?.oldest??null}))" |
      while IFS= read -r row; do printf '%s\t%s\n' "$at" "$row"; done >> "$output_dir/mongo-outbox.jsonl"
  fi
  sleep 10
done
