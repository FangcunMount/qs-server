#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
if [[ -n $(git -C "$repo" status --porcelain) ]]; then
  printf 'QS candidate worktree must be clean for exact-head evidence\n' >&2
  exit 1
fi
here=${1:?an external evidence output directory is required}
mkdir -p "$here"
stamp="$(date -u +%Y%m%dT%H%M%SZ)-$$-${RANDOM}"
owner="rm-qs03-original-${stamp}"
context=$(docker context show)
endpoint=$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')
if [[ -n ${DOCKER_HOST:-} || "$endpoint" != unix://* ]]; then
  echo "QS-03 proof requires local Unix-socket Docker" >&2; exit 1
fi
mongo_name="rm-m6-qs03-mongo-${stamp}"
mysql_name="rm-m6-qs03-mysql-${stamp}"
nsq_name="rm-m6-qs03-nsq-${stamp}"
nsq_volume="${nsq_name}-data"
output="${here}/run-${stamp}"
password=$(openssl rand -hex 16)
nsq_image='nsqio/nsq@sha256:1a369c146af71bc95c25d54b375a2b98452478c1eaf4e85f8fcb01da20f2c78a'
audit_binary=$(mktemp -t rm-qs03-audit.XXXXXX)
qs03_sheet_id=90010077
created=false

cleanup() {
  result=$?
  trap - EXIT
  rm -f "$audit_binary"
  if [[ $created == true ]]; then
    for name in "$mongo_name" "$mysql_name" "$nsq_name"; do
      if docker inspect "$name" >/dev/null 2>&1; then
        label=$(docker inspect -f '{{index .Config.Labels "rm.proof.owner"}}' "$name")
        if [[ "$label" != "$owner" ]]; then result=1; continue; fi
        if ! docker rm -f "$name" >/dev/null; then result=1; fi
      fi
    done
    if docker volume inspect "$nsq_volume" >/dev/null 2>&1; then
      label=$(docker volume inspect -f '{{index .Labels "rm.proof.owner"}}' "$nsq_volume")
      if [[ "$label" != "$owner" ]] || ! docker volume rm "$nsq_volume" >/dev/null; then result=1; fi
    fi
    remaining=$(docker ps -aq --filter "label=rm.proof.owner=$owner")
    volumes=$(docker volume ls -q --filter "label=rm.proof.owner=$owner")
    if [[ -n "$remaining$volumes" ]]; then result=1; fi
    printf 'cleanup_result=%s owned_containers_remaining=%s owned_volumes_remaining=%s\n' "$result" "${remaining:-none}" "${volumes:-none}"
  fi
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

created=true
docker volume create --label "rm.proof.owner=$owner" "$nsq_volume" >/dev/null
docker run -d --label "rm.proof.owner=$owner" --name "$mongo_name" --tmpfs /data/db:rw,size=512m \
  -p 127.0.0.1::27017 mongo:7.0 --replSet rm-gap --bind_ip_all >/dev/null
docker run -d --label "rm.proof.owner=$owner" --name "$mysql_name" --tmpfs /var/lib/mysql:rw,size=768m \
  -e "MYSQL_ROOT_PASSWORD=$password" -e MYSQL_DATABASE=m6_qs03_postconfirm \
  -p 127.0.0.1::3306 mysql:8.0.36 --default-time-zone='+08:00' >/dev/null
docker run -d --label "rm.proof.owner=$owner" --name "$nsq_name" \
  --mount "type=volume,source=${nsq_volume},target=/data" \
  -p 127.0.0.1::4150 -p 127.0.0.1::4151 \
  "$nsq_image" /nsqd --data-path=/data --broadcast-address=nsqd \
  --mem-queue-size=3000 --sync-every=2500 --sync-timeout=2s >/dev/null

for attempt in {1..60}; do
  if docker exec "$mongo_name" mongosh --quiet --eval 'db.hello().ok' >/dev/null 2>&1; then break; fi
  sleep 1
done
docker exec "$mongo_name" mongosh --quiet --eval \
  'rs.initiate({_id:"rm-gap",members:[{_id:0,host:"localhost:27017"}]})' >/dev/null
for attempt in {1..60}; do
  if docker exec "$mongo_name" mongosh --quiet --eval 'if (!db.hello().isWritablePrimary) quit(1)' >/dev/null 2>&1; then break; fi
  sleep 1
done
for attempt in {1..90}; do
  if docker exec -e "MYSQL_PWD=$password" "$mysql_name" mysqladmin ping -uroot -h 127.0.0.1 --silent >/dev/null 2>&1; then break; fi
  sleep 1
done

mongo_port="$(docker port "$mongo_name" 27017/tcp)"
mongo_port="${mongo_port##*:}"
mysql_port="$(docker port "$mysql_name" 3306/tcp)"
mysql_port="${mysql_port##*:}"
export RM_QS03_MONGO_URI="mongodb://127.0.0.1:${mongo_port}/?replicaSet=rm-gap&directConnection=true"
export RM_QS03_MYSQL_DSN="root:${password}@tcp(127.0.0.1:${mysql_port})/m6_qs03_postconfirm?parseTime=true&loc=Asia%2FShanghai"
export RM_QS03_NSQ_TCP="$(docker port "$nsq_name" 4150/tcp)"
nsq_http="http://$(docker port "$nsq_name" 4151/tcp)"
for attempt in {1..30}; do
  if [[ $(curl -fsS --connect-timeout 1 --max-time 2 "${nsq_http}/ping" 2>/dev/null) == OK ]]; then break; fi
  sleep 1
done
[[ $(curl -fsS --connect-timeout 1 --max-time 2 "${nsq_http}/ping") == OK ]]

capture() {
  local stage=$1
  python3 - "$nsq_http" "$stage" > "${output}-${stage}-stats.json" <<'PY'
import json,sys,urllib.request
endpoint,stage=sys.argv[1:]
with urllib.request.urlopen(endpoint+'/stats?format=json&topic=qs.evaluation.lifecycle',timeout=3) as response:
 data=json.load(response)
topics=[t for t in data.get('topics',[]) if t.get('topic_name')=='qs.evaluation.lifecycle']
if len(topics)!=1: raise SystemExit(f'{stage}: expected exactly one topic')
channels=[c for c in topics[0].get('channels',[]) if c.get('channel_name')=='rm-qs03-postconfirm']
if len(channels)!=1: raise SystemExit(f'{stage}: expected exactly one durable channel')
c=channels[0]
out={'broker_start_time':data.get('start_time'),'topic_message_count':topics[0].get('message_count'),
 'topic_depth':topics[0].get('depth'),'channel_depth':c.get('depth'),
 'channel_backend_depth':c.get('backend_depth'),'channel_in_flight':c.get('in_flight_count')}
print(json.dumps(out,sort_keys=True))
if stage=='before_kill' and not (out['topic_message_count']==1 and out['topic_depth']==0 and out['channel_depth']==1 and out['channel_backend_depth']==0 and out['channel_in_flight']==0):
 raise SystemExit('published answer sheet event is not solely in channel memory')
if stage=='after_recovery' and not (out['topic_message_count']==2 and out['channel_depth']==0 and out['channel_in_flight']==0 and c.get('requeue_count')==0):
 raise SystemExit('late original deliveries did not settle without additional PUB/requeue')
if stage=='after_restart' and not (out['channel_depth']==0 and out['channel_backend_depth']==0 and out['channel_in_flight']==0):
 raise SystemExit('broker did not lose the in-memory event; this run is not the target fault')
PY
  cat "${output}-${stage}-stats.json"
}

exec > >(tee "${output}.txt") 2>&1
printf 'candidate_commit=%s candidate_tree=%s\n' "$(git -C "$repo" rev-parse HEAD)" "$(git -C "$repo" rev-parse HEAD^{tree})"
printf 'nsq_image=%s mongo_image=mongo:7.0 mysql_image=mysql:8.0.36\n' "$nsq_image"
curl -fsS -X POST "${nsq_http}/topic/create?topic=qs.evaluation.lifecycle" >/dev/null
curl -fsS -X POST "${nsq_http}/channel/create?topic=qs.evaluation.lifecycle&channel=rm-qs03-postconfirm" >/dev/null
cd "$repo"
go build -o "$audit_binary" ./scripts/oneoff/answersheet_gap_recover
export RM_QS03_RECOVERY_BINARY="$audit_binary"
go test -tags 'integration reliable_messaging reliable_messaging_m4 reliable_messaging_m4_integration' \
  ./internal/apiserver/container/internal/transaction -run '^TestQS03PostConfirmSeed$' -count=1 -v
capture before_kill
docker kill --signal KILL "$nsq_name" >/dev/null
exit_code=$(docker inspect -f '{{.State.ExitCode}}' "$nsq_name")
printf 'broker_forced_exit=%s\n' "$exit_code"
[[ $exit_code == 137 ]]
docker start "$nsq_name" >/dev/null
export RM_QS03_NSQ_TCP="$(docker port "$nsq_name" 4150/tcp)"
nsq_http="http://$(docker port "$nsq_name" 4151/tcp)"
for attempt in {1..30}; do
  if [[ $(curl -fsS --connect-timeout 1 --max-time 2 "${nsq_http}/ping" 2>/dev/null) == OK ]]; then break; fi
  sleep 1
done
[[ $(curl -fsS --connect-timeout 1 --max-time 2 "${nsq_http}/ping") == OK ]]
capture after_restart
export RM_QS03_NSQ_HTTP="$nsq_http"
go test -tags 'integration reliable_messaging reliable_messaging_m4 reliable_messaging_m4_integration' \
  ./internal/apiserver/container/internal/transaction -run '^TestQS03OperatorRecoveryAfterBrokerLoss$' -count=1 -timeout=3m -v
capture after_recovery
printf 'result=isolated_original_effect_recovered recovery_pub=0 late_originals=2\n'
