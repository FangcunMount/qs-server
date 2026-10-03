#!/usr/bin/env bash
set -Eeuo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo"
runtime=e4406fd2fc6bb82ea5b871a021faf2659dc61a06
git merge-base --is-ancestor "$runtime" HEAD
allow=(
 .github/workflows/m4-confirm-success-native.yml
 scripts/testing/m4-confirm-success-native.sh
 internal/apiserver/container/internal/transaction/standard_evaluation_confirm_success_m4_integration_test.go
 .github/workflows/m4-confirm-pressure-native.yml
 scripts/testing/m4-confirm-pressure-native.sh
 scripts/testing/m4-confirm-pressure-sample.sh
 scripts/testing/m4-confirm-pressure-verify.py
 scripts/testing/m4-confirm-pressure-contract.json
 internal/apiserver/container/internal/transaction/standard_evaluation_confirm_pressure_m4_integration_test.go
)
excludes=(); for name in "${allow[@]}"; do excludes+=(":!$name"); done
git diff --exit-code "$runtime" -- . "${excludes[@]}"
mkdir -p acceptance-artifact/source acceptance-artifact/metrics
for name in "${allow[@]}"; do mkdir -p "acceptance-artifact/source/$(dirname "$name")"; cp "$name" "acceptance-artifact/source/$name"; done
mkdir -p acceptance-artifact/runtime-source
cp internal/apiserver/infra/mysql/checkpoint/{repository.go,first_claim_integration_test.go} acceptance-artifact/runtime-source/
git rev-parse HEAD > acceptance-artifact/harness-head.txt
printf '%s\n' "$runtime" > acceptance-artifact/runtime-head.txt
go list -m github.com/FangcunMount/reliable-messaging > acceptance-artifact/runtime-sdk.txt
grep -qx 'github.com/FangcunMount/reliable-messaging v0.3.0-m6.4' acceptance-artifact/runtime-sdk.txt
cp scripts/testing/m4-confirm-pressure-contract.json acceptance-artifact/contract.json
project="qs-m4-confirm-pressure-${GITHUB_RUN_ID:-local}-$$"
python3 - <<'PY' > acceptance-artifact/nsq-observation-override.json
import json
print(json.dumps({"services": {"nsqd": {"command": [
 "/nsqd", "--data-path=/data", "--broadcast-address=nsqd",
 "--lookupd-tcp-address=nsqlookupd:4160", "--mem-queue-size=3000",
 "--e2e-processing-latency-percentile=0.5,0.95,0.99"]}}}))
PY
compose=(docker compose --project-name "$project" --file scripts/testing/m4-evaluation-duplicate-compose.yaml --file scripts/testing/m5-mysql-outcome-business-compose.yaml --file acceptance-artifact/nsq-observation-override.json)
sampler_pid=''
cleanup() {
 result=$?; trap - EXIT
 touch acceptance-artifact/metrics/stop
 if [[ -n "$sampler_pid" ]]; then wait "$sampler_pid" || result=1; fi
 for driver_name in "${project}-first-claim-1" "${project}-driver-1"; do
  if docker inspect "$driver_name" > /dev/null 2>&1; then
   docker rm -f "$driver_name" >> acceptance-artifact/cleanup.log 2>&1 || result=1
  fi
 done
 "${compose[@]}" down --volumes --remove-orphans --timeout 10 >> acceptance-artifact/cleanup.log 2>&1 || result=1
 for kind in containers volumes networks; do
  case "$kind" in
   containers) docker ps -a --filter "label=com.docker.compose.project=$project" --format '{{.Names}}' > "acceptance-artifact/remaining-$kind.txt";;
   volumes) docker volume ls --filter "label=com.docker.compose.project=$project" --format '{{.Name}}' > "acceptance-artifact/remaining-$kind.txt";;
   networks) docker network ls --filter "label=com.docker.compose.project=$project" --format '{{.Name}}' > "acceptance-artifact/remaining-$kind.txt";;
  esac
  [[ ! -s "acceptance-artifact/remaining-$kind.txt" ]] || result=1
 done
 git diff --exit-code "$runtime" -- . "${excludes[@]}" > acceptance-artifact/runtime-unchanged.txt || result=1
 printf '{"test_resource_and_cleanup_exit":%d}\n' "$result" > acceptance-artifact/terminal.json
 find acceptance-artifact -type f ! -name '*.test' ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > acceptance-artifact/SHA256SUMS
 rm -f acceptance-artifact/transaction.test acceptance-artifact/first-claim.test
 exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
"${compose[@]}" up -d --wait --wait-timeout 180 mysql nsqd nsqlookupd
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m4_qs_confirm'
"${compose[@]}" exec -T mysql mysql -uroot -e 'CREATE DATABASE m4_qs_first_claim'
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags='integration,reliable_messaging' -o acceptance-artifact/first-claim.test ./internal/apiserver/infra/mysql/checkpoint
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags='integration,reliable_messaging,reliable_messaging_m4,reliable_messaging_m5,reliable_messaging_m4_integration' -o acceptance-artifact/transaction.test ./internal/apiserver/container/internal/transaction
sha256sum acceptance-artifact/first-claim.test acceptance-artifact/transaction.test > acceptance-artifact/test-binary-hashes.txt
mkdir -p acceptance-artifact/internal/apiserver/container/internal/transaction acceptance-artifact/internal/apiserver/infra/mysql/checkpoint acceptance-artifact/internal/pkg/migration/migrations/mysql
for migration in 000040_merge_runtime_checkpoint 000046_add_evaluation_run_claim_lease 000049_add_retry_governance 000089_qs_evaluation_request_ref; do
 cp "internal/pkg/migration/migrations/mysql/${migration}.up.sql" acceptance-artifact/internal/pkg/migration/migrations/mysql/
done
if docker run --rm --name "${project}-first-claim-1" --user "$(id -u):$(id -g)" \
 --label "com.docker.compose.project=$project" --network "${project}_default" \
 --entrypoint /tmp/acceptance/first-claim.test --workdir /tmp/acceptance/internal/apiserver/infra/mysql/checkpoint \
 --mount "type=bind,src=$repo/acceptance-artifact,dst=/tmp/acceptance" \
 --mount type=bind,src=/usr/share/zoneinfo/Asia/Shanghai,dst=/usr/share/zoneinfo/Asia/Shanghai,readonly \
 -e TZ=Asia/Shanghai -e 'RM_QS_FIRST_CLAIM_DSN=root@tcp(mysql:3306)/m4_qs_first_claim?parseTime=true&loc=Asia%2FShanghai&time_zone=%27%2B08%3A00%27' \
 nsqio/nsq@sha256:1a369c146af71bc95c25d54b375a2b98452478c1eaf4e85f8fcb01da20f2c78a \
 -test.run '^TestFirstRunClaimsWithoutMissingRowGapDeadlocks$' -test.count=1 -test.timeout=45s -test.v 2>&1 | tee acceptance-artifact/first-claim-test.log; then
 printf '{"exit":0}\n' > acceptance-artifact/first-claim-terminal.json
else
 claim_exit=$?
 printf '{"exit":%d}\n' "$claim_exit" > acceptance-artifact/first-claim-terminal.json
 exit "$claim_exit"
fi
docker create --name "${project}-driver-1" --user "$(id -u):$(id -g)" --label "com.docker.compose.project=$project" --network "${project}_default" \
 --entrypoint /tmp/acceptance/transaction.test --workdir /tmp/acceptance/internal/apiserver/container/internal/transaction \
 --mount "type=bind,src=$repo/acceptance-artifact,dst=/tmp/acceptance" \
 --mount type=bind,src=/usr/share/zoneinfo/Asia/Shanghai,dst=/usr/share/zoneinfo/Asia/Shanghai,readonly \
 -e TZ=Asia/Shanghai -e 'RM_M4_CONFIRM_DSN=root@tcp(mysql:3306)/m4_qs_confirm?parseTime=true&loc=Asia%2FShanghai&time_zone=%27%2B08%3A00%27' \
 -e RM_QS_M5_NSQ_TCP=nsqd:4150 -e RM_M4_CONFIRM_RESULT=/tmp/acceptance/business-result.json \
 nsqio/nsq@sha256:1a369c146af71bc95c25d54b375a2b98452478c1eaf4e85f8fcb01da20f2c78a \
 -test.run '^TestM4ConfirmSustainedSQLAndSuccessfulOutcomes$' -test.count=1 -test.timeout=10m -test.v > acceptance-artifact/driver-id.txt
bash scripts/testing/m4-confirm-pressure-sample.sh "$project" "$repo/acceptance-artifact/metrics" > acceptance-artifact/metrics/sampler.log 2>&1 & sampler_pid=$!
if docker start -a "${project}-driver-1" 2>&1 | tee acceptance-artifact/execution.log; then
 attach_exit=0
else
 attach_exit=$?
fi
driver_exit=$(docker inspect --format '{{.State.ExitCode}}' "${project}-driver-1")
"${compose[@]}" exec -T mysql mysql -uroot --batch --raw \
 -e "SELECT @@transaction_isolation; SHOW CREATE TABLE m4_qs_confirm_pressure.runtime_checkpoint; SHOW INDEX FROM m4_qs_confirm_pressure.runtime_checkpoint; SELECT status,COUNT(*) FROM m4_qs_confirm_pressure.runtime_checkpoint GROUP BY status; SELECT COUNT(*) FROM m4_qs_confirm_pressure.evaluation_outcome; SHOW ENGINE INNODB STATUS" > acceptance-artifact/runtime-sql-diagnostic.tsv
printf '{"attach_exit":%d,"driver_exit":%d}\n' "$attach_exit" "$driver_exit" > acceptance-artifact/driver-terminal.json
[[ "$attach_exit" == 0 && "$driver_exit" == 0 ]]
touch acceptance-artifact/metrics/stop
wait "$sampler_pid"; sampler_pid=''
python3 scripts/testing/m4-confirm-pressure-verify.py acceptance-artifact
