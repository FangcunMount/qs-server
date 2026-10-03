#!/usr/bin/env bash
set -Eeuo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo"
# Runtime must remain the currently deployed source; only these test files differ.
git merge-base --is-ancestor 308a0d966529ffab80f3b600f464a90c8d2a3bf1 HEAD
git diff --exit-code 308a0d966529ffab80f3b600f464a90c8d2a3bf1 -- . ':!.github/workflows/m4-confirm-success-native.yml' ':!scripts/testing/m4-confirm-success-native.sh' ':!internal/apiserver/container/internal/transaction/standard_evaluation_confirm_success_m4_integration_test.go'
project="qs-m4-confirm-${GITHUB_RUN_ID:-local}-$$"
compose=(docker compose --project-name "$project" --file scripts/testing/m4-evaluation-duplicate-compose.yaml --file scripts/testing/m5-mysql-outcome-business-compose.yaml)
mkdir -p acceptance-artifact
cp internal/apiserver/container/internal/transaction/standard_evaluation_confirm_success_m4_integration_test.go acceptance-artifact/source.go.txt
cp scripts/testing/m4-confirm-success-native.sh acceptance-artifact/runner.sh.txt
{
 git rev-parse HEAD
 go version
 go list -m github.com/FangcunMount/reliable-messaging
} > acceptance-artifact/source-versions.txt
cleanup() {
 result=$?
 trap - EXIT
 if ((result != 0)); then "${compose[@]}" logs --no-color --tail 30 mysql nsqd nsqlookupd > acceptance-artifact/dependency-error.log 2>&1 || true; fi
 if ! "${compose[@]}" down --volumes --remove-orphans --timeout 10 > acceptance-artifact/cleanup.log 2>&1; then result=1; fi
 docker ps -a --filter "label=com.docker.compose.project=$project" --format '{{.Names}}' > acceptance-artifact/remaining-containers.txt
 docker volume ls --filter "label=com.docker.compose.project=$project" --format '{{.Name}}' > acceptance-artifact/remaining-volumes.txt
 if [[ -s acceptance-artifact/remaining-containers.txt || -s acceptance-artifact/remaining-volumes.txt ]]; then result=1; fi
 docker network ls --filter "label=com.docker.compose.project=$project" --format '{{.Name}}' > acceptance-artifact/remaining-networks.txt
 if [[ -s acceptance-artifact/remaining-networks.txt ]]; then result=1; fi
 printf '{"test_and_cleanup_exit":%d}\n' "$result" > acceptance-artifact/terminal.json
 (cd acceptance-artifact && sha256sum source.go.txt runner.sh.txt source-versions.txt execution.log terminal.json cleanup.log remaining-containers.txt remaining-volumes.txt remaining-networks.txt > SHA256SUMS)
 exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
"${compose[@]}" up -d --wait --wait-timeout 180 mysql nsqd nsqlookupd
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m4_qs_confirm'
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags='integration,reliable_messaging,reliable_messaging_m4,reliable_messaging_m5,reliable_messaging_m4_integration' \
 -o acceptance-artifact/transaction.test ./internal/apiserver/container/internal/transaction
"${compose[@]}" exec -T mysql mkdir -p /tmp/acceptance/internal/apiserver/container/internal/transaction /tmp/acceptance/internal/pkg/migration/migrations/mysql /usr/share/zoneinfo/Asia
"${compose[@]}" cp acceptance-artifact/transaction.test mysql:/tmp/acceptance/transaction.test
"${compose[@]}" cp /usr/share/zoneinfo/Asia/Shanghai mysql:/usr/share/zoneinfo/Asia/Shanghai
"${compose[@]}" cp internal/pkg/migration/migrations/mysql/000089_qs_evaluation_request_ref.up.sql mysql:/tmp/acceptance/internal/pkg/migration/migrations/mysql/000089_qs_evaluation_request_ref.up.sql
"${compose[@]}" exec -T -w /tmp/acceptance/internal/apiserver/container/internal/transaction \
 -e TZ=Asia/Shanghai \
 -e 'RM_M4_CONFIRM_DSN=root@tcp(mysql:3306)/m4_qs_confirm?parseTime=true&loc=Asia%2FShanghai&time_zone=%27%2B08%3A00%27' \
 -e RM_QS_M5_NSQ_TCP=nsqd:4150 -e RM_M4_CONFIRM_RESULT=/tmp/acceptance/business-result.json \
 mysql /tmp/acceptance/transaction.test -test.run '^TestM4ConfirmWritebackActualSQLAndSuccessfulOutcome$' -test.count=1 -test.timeout=3m -test.v 2>&1 | tee acceptance-artifact/execution.log
"${compose[@]}" cp mysql:/tmp/acceptance/business-result.json acceptance-artifact/business-result.json
sha256sum acceptance-artifact/business-result.json >> acceptance-artifact/business-result.sha256
rm acceptance-artifact/transaction.test
