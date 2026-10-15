#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
container="qs-m5-open-cas-$(date +%s)-$$"
database="qs_m5_task_open_test_$(date +%s)_$$"

run_task_open_tests() {
  local result_file
  result_file=$(mktemp "${TMPDIR:-/tmp}/qs-plan-open-proof.XXXXXX")
  (
    trap 'rm -f "$result_file"' EXIT
    cd "$repo_root"
    QS_M5_TASK_OPEN_MYSQL_REQUIRED=1 go test -json -tags reliable_messaging_m4 \
      ./internal/apiserver/infra/mysql/plan \
      -run '^(TestConcurrentTaskOpenHasOnePersistedEntryAndOneEventMySQL|TestTaskOpeningAndReminderIntentCommitTogetherMySQL)$' \
      -count=1 | tee "$result_file"
    python3 - "$result_file" <<'PY'
import json
import sys

expected = {
    "TestConcurrentTaskOpenHasOnePersistedEntryAndOneEventMySQL",
    "TestTaskOpeningAndReminderIntentCommitTogetherMySQL",
}
started, passed = set(), set()
with open(sys.argv[1]) as stream:
    for line in stream:
        event = json.loads(line)
        name = event.get("Test")
        action = event.get("Action")
        if action in {"skip", "fail"}:
            raise SystemExit("required Plan MySQL proof skipped or failed")
        if name in expected:
            if action == "run":
                started.add(name)
            elif action == "pass":
                passed.add(name)
if started != expected or passed != expected:
    raise SystemExit("required Plan MySQL selectors did not all RUN and PASS")
print("Plan MySQL proof: both required selectors RUN/PASS, no SKIP")
PY
  )
}

# CI provides an exclusive schema on its service MySQL. The tests themselves
# reject databases outside the qs_m5_task_open_test_* namespace.
if [[ -n "${QS_M5_TASK_OPEN_MYSQL_DSN:-}" ]]; then
  run_task_open_tests
  exit 0
fi

if [[ "${QS_M5_USE_LOCAL_INFRA_MYSQL:-}" == 1 ]]; then
  if [[ "$(docker inspect mysql --format '{{index .Config.Labels "com.docker.compose.project"}}' 2>/dev/null)" != infra ]] ||
    [[ "$(docker port mysql 3306/tcp 2>/dev/null)" != '127.0.0.1:3306' ]]; then
    echo "expected the local infra MySQL bound to 127.0.0.1:3306" >&2
    exit 1
  fi
  test_user="m5cas_$(date +%s)_$$"
  test_password=isolated-m5-open-cas
  cleanup_local() {
    docker exec -i mysql sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot' <<SQL
DROP DATABASE IF EXISTS \`$database\`;
DROP USER IF EXISTS '$test_user'@'%';
SQL
  }
  trap cleanup_local EXIT
  docker exec -i mysql sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot' <<SQL
CREATE DATABASE \`$database\`;
CREATE USER '$test_user'@'%' IDENTIFIED BY '$test_password';
GRANT ALL PRIVILEGES ON \`$database\`.* TO '$test_user'@'%';
SQL
  QS_M5_TASK_OPEN_MYSQL_DSN="$test_user:$test_password@tcp(127.0.0.1:3306)/$database?parseTime=true&loc=UTC" \
    run_task_open_tests
  exit 0
fi

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run -d \
  --name "$container" \
  -e MYSQL_ROOT_PASSWORD=isolated-m5-open-cas \
  -e MYSQL_ROOT_HOST='%' \
  -e MYSQL_DATABASE="$database" \
  -p 127.0.0.1::3306 \
  mysql:8.0 --default-time-zone=+08:00 >/dev/null

for attempt in $(seq 1 90); do
  if docker exec "$container" mysqladmin ping -uroot -pisolated-m5-open-cas --silent >/dev/null 2>&1; then
    break
  fi
  if [[ "$(docker inspect "$container" --format '{{.State.Running}}')" != true ]]; then
    docker logs "$container" >&2
    echo "disposable MySQL stopped during startup" >&2
    exit 1
  fi
  if [[ "$attempt" == 90 ]]; then
    docker logs "$container" >&2
    echo "disposable MySQL did not become ready" >&2
    exit 1
  fi
  sleep 1
done

port=$(docker port "$container" 3306/tcp | sed -n 's/^127\.0\.0\.1:\([0-9][0-9]*\)$/\1/p')
if [[ -z "$port" ]]; then
  echo "disposable MySQL port is unavailable" >&2
  exit 1
fi

QS_M5_TASK_OPEN_MYSQL_DSN="root:isolated-m5-open-cas@tcp(127.0.0.1:$port)/$database?parseTime=true&loc=UTC" \
  run_task_open_tests
