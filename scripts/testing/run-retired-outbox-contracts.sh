#!/usr/bin/env bash
# Historical contracts run against their immutable source, never the new API.
set -euo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
source_commit=96fc4df42f6af9f6ce22e2184394b1a07df2ffdd
kind=${1:-unit}
case "$kind" in
 transport-unit|unit|original-mongo|original-mysql|fin-loss|profile-handoff|mongo-drain|mongo-only|runtime-closure|mysql-outcome-script|qs04-worker-script|evaluation-duplicate-script) ;;
 *) echo 'Unknown historical contract kind; use one of the fixed cases listed in this script.' >&2; exit 2;;
esac
case "$kind" in
 runtime-closure|mysql-outcome-script|qs04-worker-script|evaluation-duplicate-script)
  source_commit=f77bd0feb3a3559c0a1bfc5493451ae0fba25c5d ;;
esac
if ! git -C "$repo" cat-file -e "$source_commit^{commit}" 2>/dev/null; then
  git -C "$repo" fetch --no-tags origin "$source_commit"
fi
[[ $(git -C "$repo" rev-parse "$source_commit^{commit}") == "$source_commit" ]] || exit 1
snapshot=$(mktemp -d "${TMPDIR:-/tmp}/qs-retired-outbox-test.XXXXXX")
historical_worktree=false
cleanup_snapshot() {
 if [[ $historical_worktree == true ]]; then
  git -C "$repo" worktree remove --force "$snapshot"
 else
  rm -rf -- "$snapshot"
 fi
}
trap cleanup_snapshot EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
case "$kind" in
 *-script)
  git -C "$repo" worktree add --detach "$snapshot" "$source_commit"
  historical_worktree=true ;;
 *) git -C "$repo" archive "$source_commit" | tar -x -C "$snapshot" ;;
esac
echo "Historical source: $source_commit; case: $kind"
cd "$snapshot"
export GOWORK=off
case "$kind" in
 runtime-closure)
  go test -tags=integration -ldflags "-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$source_commit" \
   -run '^TestCurrentRuntimeClosure$' -count=1 -timeout=15m -v ./internal/apiserver/integration/runtimeclosure ;;
 mysql-outcome-script) bash scripts/testing/m5-mysql-outcome-business-proof.sh ;;
 qs04-worker-script) bash scripts/testing/m6-qs04-postconfirm-worker-proof.sh ;;
 evaluation-duplicate-script) bash scripts/testing/m4-evaluation-duplicate-proof.sh ;;
 transport-unit)
  go test -count=1 -run '^Test(FailedMessageHandlerPreservesTransportEvidence|UnknownEventRecorderPreservesPayloadAndRejectsMissingAudit)$' -v ./internal/pkg/eventing/transport ;;
 unit)
  go test -count=1 ./internal/apiserver/outboxcore ./internal/apiserver/application/eventing ./internal/apiserver/infra/mysql/eventoutbox ./internal/apiserver/infra/mongo/eventoutbox ./internal/apiserver/infra/redis/outboxready ;;
 original-mongo)
  : "${RM_QS_MONGO_URI:?Disposable rm-test replica set required}"
  go test -tags=reliable_messaging -count=1 -run '^TestReliableMessagingOriginalMongoRunner$' -v ./internal/apiserver/container/internal/transaction ;;
 original-mysql)
  : "${RM_QS_ASSESSMENT_DSN:?Disposable assessment MySQL required}"
  go test -tags=reliable_messaging -count=1 -run '^TestReliableMessagingAssessmentPersistence$' -v ./internal/apiserver/container/internal/transaction ;;
 fin-loss)
  : "${RM_QS_REDELIVERY_DSN:?Disposable MySQL required}"
  : "${RM_QS_NSQ_TCP:?Disposable NSQ required}"
  : "${RM_QS_NSQ_HTTP:?Disposable NSQ HTTP required}"
  go test -tags=reliable_messaging -count=1 -run '^TestReliableMessagingAnswerSheetFINLoss$' -v ./internal/apiserver/container/internal/transaction ;;
 profile-handoff)
  : "${RM_QS_BOOTSTRAP_MYSQL_DSN:?Disposable m4_qs_bootstrap MySQL required}"
  : "${RM_QS_BOOTSTRAP_MONGO_URI:?Disposable rm-test replica set required}"
  go test -tags='reliable_messaging_m4,reliable_messaging_m4_integration' -count=1 -run '^TestM4ProcessBootstrapRunsSelectedStandardProfiles$' -v ./internal/apiserver/process ;;
 mongo-drain)
  : "${RM_QS_ATTENTION_MONGO_URI:?Disposable rm-test Mongo required}"
  : "${RM_QS_NSQ_TCP:?Disposable NSQ required}"
  go test -tags='integration,reliable_messaging,reliable_messaging_m4,reliable_messaging_m4_integration' -count=1 -run '^TestM5MongoRollbackHoldsUntilStandardIntentsDrain$' -v ./internal/apiserver/container/internal/transaction ;;
 mongo-only)
  : "${RM_QS_MONGO_ONLY_MYSQL_DSN:?Disposable m5_qs_mongo_only MySQL required}"
  go test -tags='reliable_messaging_m4,reliable_messaging_m4_integration' -count=1 -run '^TestM5MongoOnlyProcessKeepsLegacyMySQLAndHotRankSubscription$' -v ./internal/apiserver/process ;;
esac
