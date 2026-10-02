#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
baseline=${RM_QS_M407_RUNTIME_BASE:?frozen runtime base required}
project="qs-m4-07-new-$(date +%s)-$$-${RANDOM}"
compose=(docker compose --project-name "$project" --file "$repo/scripts/testing/m4-07-new-chain-compose.yaml")
stream=${RM_QS_M407_STREAM:-both}
mysql_command_dir=${RM_QS_M407_MYSQL_COMMAND_DIR:-}
if [[ -n "$mysql_command_dir" ]]; then
  [[ "$stream" == assessment && "$mysql_command_dir" == /* && ! -e "$mysql_command_dir" ]] || {
    echo 'RM_QS_M407_MYSQL_COMMAND_DIR requires assessment stream and a new absolute directory' >&2
    exit 1
  }
fi
case "$stream" in
  answer) test_pattern='^TestM407NewAnswerSheetBatchThroughStandardProfile$' ;;
  assessment) test_pattern='^TestM407NewAssessmentBatchThroughStandardProfile$' ;;
  both) test_pattern='^TestM407New(AnswerSheet|Assessment)BatchThroughStandardProfile$' ;;
  *) echo 'RM_QS_M407_STREAM must be answer, assessment, or both' >&2; exit 1 ;;
esac

if ! git -C "$repo" merge-base --is-ancestor "$baseline" HEAD; then
  echo "M4-07 standard-chain proof requires the frozen current recovery candidate baseline" >&2
  exit 1
fi
if ! git -C "$repo" diff --quiet "$baseline" -- ':!internal/apiserver/container/internal/transaction/m4_new_answer_chain_integration_test.go' ':!internal/apiserver/container/internal/transaction/m4_07_mongo_command_metrics_test.go' ':!internal/apiserver/container/internal/transaction/m4_new_assessment_chain_integration_test.go' ':!internal/apiserver/container/internal/transaction/m4_07_mysql_command_metrics_test.go' ':!internal/apiserver/container/internal/transaction/m4_07_mysql_commit_observer_test.go' ':!internal/apiserver/container/internal/transaction/m4_07_nsq_fault_proxy_test.go' ':!internal/apiserver/domain/survey/answersheet/m4_07_event_id_proof.go' ':!scripts/testing/m4-07-new-chain-compose.yaml' ':!scripts/testing/m4-07-new-chain-proof.sh' ':!scripts/testing/m4-07-sample-resources.sh'; then
  echo "Current business runtime differs from frozen recovery candidate" >&2
  exit 1
fi
context=$(docker context show)
endpoint=$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')
if [[ -n ${DOCKER_HOST:-} || "$endpoint" != unix://* ]]; then
  echo "M4-07 proof requires a local Unix-socket Docker context" >&2
  exit 1
fi

cleanup() {
  result=$?
  trap - EXIT
  if [[ -n ${sampler_pid:-} ]]; then
    if kill -0 "$sampler_pid" 2>/dev/null; then
      kill "$sampler_pid" 2>/dev/null || true
      wait "$sampler_pid" || true
    elif ! wait "$sampler_pid"; then
      result=1
    fi
    for file in docker-stats.jsonl mysql-status.tsv mongo-status.jsonl nsqd-status.jsonl; do
      if [[ ! -s "$metrics_dir/$file" ]]; then result=1; fi
    done
    if [[ "$stream" == assessment ]] && ! grep -qv 'absent$' "$metrics_dir/mysql-outbox.tsv" 2>/dev/null; then result=1; fi
    if [[ "$stream" == answer && ! -s "$metrics_dir/mongo-outbox.jsonl" ]]; then result=1; fi
  fi
  if ! "${compose[@]}" down --volumes --remove-orphans --timeout 10; then result=1; fi
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
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m4_qs_new_chain'
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m4_qs_new_assessment'
# The current host stages original evaluation identity in its transaction.
# Apply the exact current production migration to each disposable database;
# missing this table rolls back every request before the pressure phase.
for database in m4_qs_new_chain m4_qs_new_assessment; do
  "${compose[@]}" exec -T mysql mysql -uroot "$database" < \
    "$repo/internal/pkg/migration/migrations/mysql/000089_qs_evaluation_request_ref.up.sql"
done
"${compose[@]}" exec -T mysql mkdir -p /tmp/m4-07/configs
"${compose[@]}" cp "$repo/configs/events.yaml" mysql:/tmp/m4-07/configs/events.yaml

architecture=$(docker info --format '{{.Architecture}}')
case "$architecture" in
  aarch64|arm64) goarch=arm64 ;;
  x86_64|amd64) goarch=amd64 ;;
  *) echo "Unsupported Docker architecture: $architecture" >&2; exit 1 ;;
esac
binary=/tmp/qs-m4-07-new-chain.test
(cd "$repo" && GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags='integration,reliable_messaging,reliable_messaging_m4,reliable_messaging_m4_integration,m4_07_new_chain' \
  -o "$binary" ./internal/apiserver/container/internal/transaction)
"${compose[@]}" cp "$binary" mysql:/tmp/m4-07-new-chain.test
metrics_dir=${RM_QS_M407_METRICS_DIR:-}
if [[ -n "$metrics_dir" ]]; then
  [[ "$metrics_dir" == /* && ! -e "$metrics_dir" ]] || {
    echo 'RM_QS_M407_METRICS_DIR must be a new absolute directory' >&2
    exit 1
  }
  mkdir -p "$(dirname "$metrics_dir")"
  bash "$repo/scripts/testing/m4-07-sample-resources.sh" "$project" "$repo/scripts/testing/m4-07-new-chain-compose.yaml" "$metrics_dir" new "$stream" 2>"$metrics_dir.sampler.err" &
  sampler_pid=$!
fi
profile_dir=${RM_QS_M407_MONGO_PROFILE_DIR:-}
keep_profile=0
if [[ -n "$profile_dir" ]]; then
  [[ "$stream" == answer && "$profile_dir" == /* && ! -e "$profile_dir" ]] || {
    echo 'RM_QS_M407_MONGO_PROFILE_DIR requires answer stream and a new absolute directory' >&2
    exit 1
  }
  mkdir -p "$profile_dir"
  keep_profile=1
  "${compose[@]}" exec -T mongo mongosh --quiet --file /dev/stdin <<'JS'
const target = db.getSiblingDB('m4_07_new_answer_chain');
target.createCollection('system.profile', {capped: true, size: 64 * 1024 * 1024});
const result = target.setProfilingLevel(2, {slowms: 0});
if (result.ok !== 1) throw new Error('new Mongo profiler did not start');
JS
fi
command_dir=${RM_QS_M407_COMMAND_DIR:-}
command_path=
if [[ -n "$command_dir" ]]; then
  [[ "$stream" == answer && "$command_dir" == /* && ! -e "$command_dir" ]] || {
    echo 'RM_QS_M407_COMMAND_DIR requires answer stream and a new absolute directory' >&2
    exit 1
  }
  mkdir -p "$command_dir"
  command_path=/tmp/m4-07/outbox-commands.jsonl
fi
mysql_command_path=
if [[ -n "$mysql_command_dir" ]]; then
  mkdir -p "$mysql_command_dir"
  mysql_command_path=/tmp/m4-07/mysql-outbox-commands.jsonl
fi
"${compose[@]}" exec -T \
  -e RM_QS_MONGO_URI='mongodb://mongo:27017/?replicaSet=rm-test' \
  -e RM_QS_ASSESSMENT_DSN='root@tcp(mysql:3306)/m4_qs_new_chain?parseTime=true&loc=UTC' \
  -e RM_QS_NSQ_TCP='nsqd:4150' \
  -e RM_QS_CATALOG='/tmp/m4-07/configs/events.yaml' \
  -e RM_QS_M407_BATCH_COUNT="${RM_QS_M407_BATCH_COUNT:-16}" \
  -e RM_QS_M407_NSQ_OUTAGE_AFTER="${RM_QS_M407_NSQ_OUTAGE_AFTER:-0}" \
  -e RM_QS_M407_MYSQL_OUTAGE_AFTER="${RM_QS_M407_MYSQL_OUTAGE_AFTER:-0}" \
  -e RM_QS_M407_MONGO_OUTAGE_AFTER="${RM_QS_M407_MONGO_OUTAGE_AFTER:-0}" \
  -e RM_QS_M407_LOST_CONFIRM="${RM_QS_M407_LOST_CONFIRM:-0}" \
  -e RM_QS_M407_SPACING_MS="${RM_QS_M407_SPACING_MS:-0}" \
  -e RM_QS_M407_KEEP_MONGO_PROFILE="$keep_profile" \
  -e RM_QS_M407_COMMAND_METRICS="$command_path" \
  -e RM_QS_M407_MYSQL_COMMAND_METRICS="$mysql_command_path" \
  mysql /tmp/m4-07-new-chain.test \
    -test.run "$test_pattern" -test.count=1 -test.timeout=30m -test.v
if [[ -n "$mysql_command_dir" ]]; then
  "${compose[@]}" cp mysql:/tmp/m4-07/mysql-outbox-commands.jsonl "$mysql_command_dir/outbox-commands.jsonl"
  [[ -s "$mysql_command_dir/outbox-commands.jsonl" ]] || {
    echo 'new MySQL command monitor returned no samples' >&2
    exit 1
  }
fi
if [[ -n "$command_dir" ]]; then
  "${compose[@]}" cp mysql:/tmp/m4-07/outbox-commands.jsonl "$command_dir/outbox-commands.jsonl"
  [[ -s "$command_dir/outbox-commands.jsonl" ]] || {
    echo 'new Mongo command monitor returned no samples' >&2
    exit 1
  }
fi
if [[ -n "$profile_dir" ]]; then
  "${compose[@]}" exec -T mongo mongosh --quiet --file /dev/stdin > "$profile_dir/mongo-outbox-profile.jsonl" <<'JS'
db.getSiblingDB('m4_07_new_answer_chain').system.profile.find({ns: 'm4_07_new_answer_chain.rm_outbox'}).forEach(doc => print(EJSON.stringify(doc)));
JS
  [[ -s "$profile_dir/mongo-outbox-profile.jsonl" ]] || {
    echo 'new Mongo Outbox profiler returned no commands' >&2
    exit 1
  }
fi
if [[ -n "$metrics_dir" ]]; then
  "${compose[@]}" exec -T mysql mysql -uroot --batch --raw \
    -e "SELECT SCHEMA_NAME, DIGEST_TEXT, COUNT_STAR, SUM_TIMER_WAIT, MAX_TIMER_WAIT, SUM_LOCK_TIME FROM performance_schema.events_statements_summary_by_digest WHERE SCHEMA_NAME LIKE 'm4_qs_%' AND COUNT_STAR > 0 ORDER BY SUM_TIMER_WAIT DESC LIMIT 50" \
    > "$metrics_dir/mysql-statements.tsv"
  "${compose[@]}" exec -T mysql mysql -uroot --batch --raw \
    -e "SELECT EVENT_NAME, COUNT_STAR, SUM_TIMER_WAIT, MAX_TIMER_WAIT, SUM_LOCK_TIME FROM performance_schema.events_statements_summary_global_by_event_name WHERE EVENT_NAME IN ('statement/com/Execute','statement/sql/select','statement/sql/insert','statement/sql/update','statement/sql/commit','statement/sql/rollback')" \
    > "$metrics_dir/mysql-statement-types.tsv"
fi
