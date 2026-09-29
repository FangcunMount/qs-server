#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
project="qs-m6-qs04-ledger-$(date +%s)-$$-${RANDOM}"
compose=(docker compose --project-name "$project" \
  --file "$repo/scripts/testing/m4-evaluation-duplicate-compose.yaml" \
  --file "$repo/scripts/testing/m5-mysql-outcome-business-compose.yaml")
context=$(docker context show)
endpoint=$(docker context inspect "$context" --format '{{.Endpoints.docker.Host}}')
if [[ -n ${DOCKER_HOST:-} || "$endpoint" != unix://* ]]; then
  echo "QS-04 ledger proof requires local Unix-socket Docker without DOCKER_HOST" >&2
  exit 1
fi
docker info >/dev/null
printf 'QS source: %s\n' "$(git -C "$repo" rev-parse HEAD)"
printf 'SDK dependency: %s\n' "$(cd "$repo" && GOPROXY=https://proxy.golang.org,direct go list -m github.com/FangcunMount/reliable-messaging)"

cleanup() {
  result=$?
  trap - EXIT
  if ((result != 0)); then "${compose[@]}" logs --no-color --tail 25 mysql || true; fi
  if ! "${compose[@]}" down --volumes --remove-orphans --timeout 10; then result=1; fi
  if [[ -n ${build_dir:-} ]]; then rm -rf -- "$build_dir"; fi
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

"${compose[@]}" up -d --wait --wait-timeout 180 mysql
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m4_qs_assessment'
architecture=$(docker info --format '{{.Architecture}}')
case "$architecture" in
  aarch64|arm64) goarch=arm64 ;;
  x86_64|amd64) goarch=amd64 ;;
  *) echo "Unsupported Docker architecture: $architecture" >&2; exit 1 ;;
esac
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/$project-build.XXXXXX")
(cd "$repo" && GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go test -c \
  -tags='integration,reliable_messaging,reliable_messaging_m4,reliable_messaging_m4_integration' \
  -o "$build_dir/transaction.test" ./internal/apiserver/container/internal/transaction)
"${compose[@]}" exec -T mysql mkdir -p /tmp/m6-qs04/internal/pkg/migration/migrations/mysql \
  /tmp/m6-qs04/internal/apiserver/container/internal/transaction
"${compose[@]}" cp "$repo/internal/pkg/migration/migrations/mysql/000089_qs_evaluation_request_ref.up.sql" \
  mysql:/tmp/m6-qs04/internal/pkg/migration/migrations/mysql/000089_qs_evaluation_request_ref.up.sql
"${compose[@]}" cp "$repo/internal/pkg/migration/migrations/mysql/000090_qs_gap_recovery_request.up.sql" \
  mysql:/tmp/m6-qs04/internal/pkg/migration/migrations/mysql/000090_qs_gap_recovery_request.up.sql
"${compose[@]}" cp "$build_dir/transaction.test" \
  mysql:/tmp/m6-qs04/internal/apiserver/container/internal/transaction/transaction.test
"${compose[@]}" exec -T -w /tmp/m6-qs04/internal/apiserver/container/internal/transaction \
  -e RM_QS_ASSESSMENT_DSN='root@tcp(mysql:3306)/m4_qs_assessment?parseTime=true&loc=Asia%2FShanghai' \
  mysql ./transaction.test -test.run '^TestStandardAssessmentOriginalTransaction$' -test.count=1 -test.timeout=2m -test.v
printf 'result=isolated_gap_recovery_ledger_governance resources=removed_on_exit\n'
