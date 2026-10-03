"""Fail closed on empty, incomplete, failed or skipped required MQ test reports."""

import json
import sys
from pathlib import Path

required = {
    "TestMQParticipantClosedIntakeRetainsReadAndOriginalDuplicate",
    "TestMQReceiverBudgetSurvivesRepublishAndRestartWithoutProjection",
    "TestMQPhysicalFailureCannotApplyEventOrInventLogicalBudget",
    "TestMQReceiverCommitAndCancellationCannotBeBrokerAcknowledged",
    "TestMQRuntimeNSQCommitDuplicateRearmAndStopBorrowedPool",
    "TestMQKeyRolesRotationAndLocalTrustMapping",
    "TestMQTopologyPreflightReadsAllBusinessAndFailureChannels",
    "TestMQModuleReplacesLegacyRelayAndResultIngressWhileIntakeClosed",
    "TestMQOptionsRequireFixedEndpointsAndLocalTrust",
    "TestMQParticipantOperationRequiresCurrentAccessAndOriginalRequest",
    "TestMQDelegatedOperationUsesCurrentParticipantAuthorizationWithoutNewACL",
    "TestMQPublic202ReturnsOriginalIdentityAndReadOnlyStatusURL",
    "TestMQParticipantOperationCannotCrossAggregateOrActor",

    "TestMQOperationOriginalTransactionIdentityAndSingleSeal",
    "TestMQPendingRequiresDurableDecisionAndHeldBlocksNext",
    "TestMQReceiptLostAckDuplicateAndLateReceiptDoNotRegress",
    "TestMQProjectionInboxAndAckRollbackTogether",
    "TestMQReceiptMismatchNeverConfirmsCommand",
    "TestMQEvaluationEventsDeduplicateBeforeMonotonicProjection",
    "TestMQPayloadMTLSTrustAndExactStoredReference",
    "TestMQRelayBrokerAcceptanceRetainsOriginalWire",
    "TestMQRelayCancellationAndOverlapPreservePending",
    "TestMQRelayPersistentBudgetAndBoundedUnknown",
    "TestMessagingPayloadReferenceAndExactBody",
    "TestMQSubmissionSharesOriginalRequestAndAvoidsLegacyDelivery",
    "TestMQSubmissionSealFailureRollsBackRequestAndOperation",
    "TestMQManagementFamiliesUseOriginalAggregateAndSingleWire",
    "TestMQHTTP202MeansQSCommitAndUsesOriginalCommandIdentity",
    "TestMQOperationRouteRequiresCurrentPermissionAndSeparatesPublication",
}
rows = [json.loads(line) for line in Path(sys.argv[1]).read_text().splitlines() if line]
if not rows:
    raise SystemExit("empty MQ acceptance report")
started = {(r["Package"], r["Test"]) for r in rows if r["Action"] == "run" and "Test" in r}
passed = {(r["Package"], r["Test"]) for r in rows if r["Action"] == "pass" and "Test" in r}
bad = [r for r in rows if r["Action"] in ("fail", "build-fail") or (r["Action"] == "skip" and "Test" in r)]
if bad or started != passed or not required.issubset({name for _, name in passed}):
    raise SystemExit("required MQ acceptance is failed, skipped or incomplete")
print(json.dumps({"passed_cases": len(passed), "required_mq_cases": len(required), "failed": 0, "skipped_cases": 0}))
