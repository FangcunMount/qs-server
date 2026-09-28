#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
output_dir=${RM_QS04_EVIDENCE_DIR:-${TMPDIR:-/tmp}}
mkdir -p "$output_dir"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
mysql_name="rm-m6-qs04-mysql-${stamp}"
nsq_name="rm-m6-qs04-nsq-${stamp}"
nsq_volume="${nsq_name}-data"
output="${output_dir}/run-${stamp}"
password=$(openssl rand -hex 16)
nsq_image='nsqio/nsq@sha256:1a369c146af71bc95c25d54b375a2b98452478c1eaf4e85f8fcb01da20f2c78a'
audit_binary=$(mktemp -t rm-qs04-audit.XXXXXX)
created=false

cleanup() {
  python3 - "$audit_binary" <<'PY'
from pathlib import Path
import sys
Path(sys.argv[1]).unlink(missing_ok=True)
PY
  if [[ $created == true ]]; then
    docker container stop "$mysql_name" "$nsq_name" >/dev/null 2>&1 || true
    docker container rm "$mysql_name" "$nsq_name" >/dev/null 2>&1 || true
    docker volume rm "$nsq_volume" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

export MYSQL_ROOT_PASSWORD="$password"
export MYSQL_PWD="$password"
created=true
docker volume create "$nsq_volume" >/dev/null
docker run -d --name "$mysql_name" --tmpfs /var/lib/mysql:rw,size=768m \
  -e MYSQL_ROOT_PASSWORD -e MYSQL_DATABASE=m6_qs04_postconfirm \
  -p 127.0.0.1::3306 mysql:8.0.36 --default-time-zone='+08:00' >/dev/null
docker run -d --name "$nsq_name" \
  --mount "type=volume,source=${nsq_volume},target=/data" \
  -p 127.0.0.1::4150 -p 127.0.0.1::4151 \
  "$nsq_image" /nsqd --data-path=/data --broadcast-address=nsqd \
  --mem-queue-size=3000 --sync-every=2500 --sync-timeout=2s >/dev/null

for attempt in {1..90}; do
  if docker exec -e MYSQL_PWD "$mysql_name" mysqladmin ping -uroot -h 127.0.0.1 --silent >/dev/null 2>&1; then break; fi
  sleep 1
done
mysql_port="$(docker port "$mysql_name" 3306/tcp)"
mysql_port="${mysql_port##*:}"
export RM_QS04_MYSQL_DSN="root:${password}@tcp(127.0.0.1:${mysql_port})/m6_qs04_postconfirm?parseTime=true&loc=Asia%2FShanghai"
export RM_QS04_NSQ_TCP="$(docker port "$nsq_name" 4150/tcp)"
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
if len(topics)!=1: raise SystemExit(f'{stage}: expected one topic')
channels=[c for c in topics[0].get('channels',[]) if c.get('channel_name')=='rm-qs04-postconfirm']
if len(channels)!=1: raise SystemExit(f'{stage}: expected one channel')
c=channels[0]
out={'broker_start_time':data.get('start_time'),'topic_message_count':topics[0].get('message_count'),
     'channel_depth':c.get('depth'),'channel_backend_depth':c.get('backend_depth'),
     'channel_in_flight':c.get('in_flight_count')}
print(json.dumps(out,sort_keys=True))
if stage=='before_kill' and not (out['topic_message_count']==1 and out['channel_depth']==1 and out['channel_backend_depth']==0 and out['channel_in_flight']==0):
    raise SystemExit('confirmed message was not solely in channel memory')
if stage=='after_restart' and not (out['channel_depth']==0 and out['channel_backend_depth']==0 and out['channel_in_flight']==0):
    raise SystemExit('broker did not lose the in-memory event')
PY
  cat "${output}-${stage}-stats.json"
}

exec > >(tee "${output}.txt") 2>&1
printf 'candidate_commit=%s candidate_tree=%s\n' "$(git -C "$repo" rev-parse HEAD)" "$(git -C "$repo" rev-parse HEAD^{tree})"
printf 'mysql_image=mysql:8.0.36 nsq_image=%s\n' "$nsq_image"
curl -fsS -X POST "${nsq_http}/topic/create?topic=qs.evaluation.lifecycle" >/dev/null
curl -fsS -X POST "${nsq_http}/channel/create?topic=qs.evaluation.lifecycle&channel=rm-qs04-postconfirm" >/dev/null
cd "$repo"
go test -tags 'integration reliable_messaging reliable_messaging_m4 reliable_messaging_m4_integration' \
  ./internal/apiserver/container/internal/transaction -run '^TestQS04PostConfirmSeed$' -count=1 -v
capture before_kill
docker kill --signal KILL "$nsq_name" >/dev/null
exit_code=$(docker inspect -f '{{.State.ExitCode}}' "$nsq_name")
printf 'broker_forced_exit=%s\n' "$exit_code"
[[ $exit_code == 137 ]]
docker start "$nsq_name" >/dev/null
nsq_http="http://$(docker port "$nsq_name" 4151/tcp)"
for attempt in {1..30}; do
  if [[ $(curl -fsS --connect-timeout 1 --max-time 2 "${nsq_http}/ping" 2>/dev/null) == OK ]]; then break; fi
  sleep 1
done
[[ $(curl -fsS --connect-timeout 1 --max-time 2 "${nsq_http}/ping") == OK ]]
capture after_restart

assessment_id=$(docker exec -e MYSQL_PWD "$mysql_name" mysql -uroot -N -B -D m6_qs04_postconfirm \
  -e "SELECT id FROM assessment WHERE answer_sheet_id=90020077 AND deleted_at IS NULL")
[[ "$assessment_id" =~ ^[0-9]+$ ]]
docker exec -e MYSQL_PWD "$mysql_name" mysql -uroot -D m6_qs04_postconfirm \
  -e "UPDATE assessment SET submitted_at=DATE_SUB(NOW(), INTERVAL 20 MINUTE) WHERE id=${assessment_id} AND status='submitted'"
cutoff=$(python3 -c 'from datetime import datetime,timedelta,timezone; print((datetime.now(timezone(timedelta(hours=8)))-timedelta(minutes=10)).isoformat(timespec="seconds"))')
go build -o "$audit_binary" ./scripts/oneoff/assessment_run_gap_audit
audit_args=(--after-id="$((assessment_id-1))" --upper-id="$assessment_id" --submitted-before="$cutoff" \
  --max-rows=10 --include-ids --timeout=10s)
export MYSQL_DSN="$RM_QS04_MYSQL_DSN"
if "$audit_binary" "${audit_args[@]}" > "${output}-gap-audit.json"; then audit_exit=0; else audit_exit=$?; fi
printf 'gap_audit_exit=%s\n' "$audit_exit"
[[ $audit_exit == 2 ]]
python3 - "${output}-gap-audit.json" "$assessment_id" <<'PY'
import json,sys
report=json.load(open(sys.argv[1]))
expected=sys.argv[2]
if not (report['complete'] and report['scanned']==1 and
        report['counts']=={'candidate_never_claimed':1} and
        report['candidate_ids']==[expected]):
    raise SystemExit(f'unexpected QS-04 audit result: {report}')
print(json.dumps({'complete':report['complete'],'scanned':report['scanned'],
                  'counts':report['counts'],'original_id_matched':True},sort_keys=True))
PY
final_state=$(docker exec -e MYSQL_PWD "$mysql_name" mysql -uroot -N -B -D m6_qs04_postconfirm \
  -e "SELECT a.status, (SELECT COUNT(*) FROM runtime_checkpoint rc WHERE rc.scope='evaluation_run' AND rc.assessment_id=a.id), (SELECT COUNT(*) FROM rm_outbox o WHERE o.event_type='evaluation.requested' AND o.state='published' AND o.attempt_count=1) FROM assessment a WHERE a.id=${assessment_id}")
printf 'business_outbox_run_state=%s\n' "$final_state"
[[ "$final_state" == $'submitted\t0\t1' ]]
printf 'result=isolated_postconfirm_never_claimed_detected resources=removed_on_exit\n'
