#!/usr/bin/env bash
# Historical contracts run against their immutable source, never the new API.
set -euo pipefail
repo=$(git rev-parse --show-toplevel)
source_commit=96fc4df42f6af9f6ce22e2184394b1a07df2ffdd
[[ $(git -C "$repo" rev-parse "$source_commit^{commit}") == "$source_commit" ]] || exit 1
kind=${1:-unit}
case "$kind" in
 transport-unit|unit|original-mongo|original-mysql|fin-loss|profile-handoff|mongo-drain|mongo-only) ;;
 *) echo 'Usage: run-retired-outbox-contracts.sh transport-unit|unit|original-mongo|original-mysql|fin-loss|profile-handoff|mongo-drain|mongo-only' >&2; exit 2;;
esac
snapshot=$(mktemp -d "${TMPDIR:-/tmp}/qs-retired-outbox-test.XXXXXX")
trap 'rm -rf -- "$snapshot"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
git -C "$repo" archive "$source_commit" | tar -x -C "$snapshot"
echo "Historical source: $source_commit; case: $kind"
cd "$snapshot"
export GOWORK=off
case "$kind" in
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
