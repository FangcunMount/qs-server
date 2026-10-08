package retirement

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"go.mongodb.org/mongo-driver/bson"
)

func sqlResolverFixture(t *testing.T, kind string) *DecodedSourceEvent {
	t.Helper()
	at := time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)
	v := &DecodedSourceEvent{EventID: "original-sql-event", EventType: kind, AggregateType: "Evaluation", AggregateID: "42", OrgID: 7, OccurredAt: at, BusinessAt: at, BusinessIDs: map[string]string{"assessment_id": "42", "testee_id": "21", "answersheet_id": "10042"}, Source: evidence.HistoricalSourceReferenceV1{Database: "mysql", Object: "domain_event_outbox", PrimaryKeyKind: "mysql_uint64", PrimaryKeySHA256: strings.Repeat("a", 64), Digest: evidence.SourceDigest(SQLRowDigestKind, []byte("private-source-row"))}, ContentDigest: evidence.SourceDigest(ContentDigestKind, []byte("private-body"))}
	switch kind {
	case "evaluation.requested", "evaluation.retry.requested":
		v.Requested = &eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, AnswerSheetID: "10042", QuestionnaireCode: "Q", QuestionnaireVer: "v1", ModelKind: "scale", ModelCode: "S", ModelVersion: "v1", RequestedAt: at}
		if kind == "evaluation.retry.requested" {
			v.Requested.ExpectedAttempt = 1
			v.Requested.Mode = "next_attempt"
			v.Requested.AttemptOrigin = "automatic"
		}
	case "evaluation.failed":
		v.Failed = &eventpayload.EvaluationFailedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, Reason: "closed historical failure", FailedAt: at}
	case "evaluation.outcome.committed":
		v.OutcomeCommitted = &eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "original-run", CommittedAt: at}
	default:
		t.Fatal("unsupported local fixture")
	}
	return v
}

func sqlFactsFixture() sqlevaluation.SQLHistoricalFactsSnapshot {
	at := time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)
	return sqlevaluation.SQLHistoricalFactsSnapshot{Owner: sqlevaluation.SQLHistoricalOwner{AssessmentID: 42, OrgID: 7, TesteeID: 21, AnswerSheetID: 10042, Status: "evaluated", QuestionnaireCode: "Q", QuestionnaireVersion: "v1", ModelKind: "scale", ModelCode: "S", ModelVersion: "v1", SubmittedAt: &at, EvaluatedAt: &at, ActualSubmittedAtDataType: "datetime", ActualSubmittedAtPrecision: 3, ActualEvaluatedAtDataType: "datetime", ActualEvaluatedAtPrecision: 3, ActualFailedAtDataType: "datetime", ActualFailedAtPrecision: 3, ClockComparisonRuleVersion: sqlevaluation.SQLHistoricalClockComparisonRuleVersion}, Runs: []sqlevaluation.SQLHistoricalRun{{ID: 1, AssessmentID: 42, ResourceID: "original-run", Scope: "evaluation_run", Attempt: 5, Status: "succeeded", FinishedAt: &at}}, Outcomes: []sqlevaluation.SQLHistoricalOutcome{{ID: 9001, AssessmentID: 42, OrgID: 7, TesteeID: 21, RunID: "original-run", EvaluatedAt: at}}}
}

func TestSQLLocalOwnerClockUsesActualPrecisionWithoutNearestMatch(t *testing.T) {
	source := sqlResolverFixture(t, "evaluation.requested")
	facts := sqlFactsFixture()
	seconds := source.BusinessAt.Truncate(time.Second)
	facts.Owner.ActualSubmittedAtPrecision, facts.Owner.SubmittedAt = 0, &seconds
	v, err := resolveSQLLocalFacts(source, facts)
	if err != nil || !v.OwnerLocalTerminal || !containsString(v.Gaps, "storage_precision_gap") {
		t.Fatal("same-second historical precision gap rejected or hidden", err)
	}
	source.Requested.RequestedAt, source.BusinessAt = seconds, seconds
	v, err = resolveSQLLocalFacts(source, facts)
	if err != nil || containsString(v.Gaps, "storage_precision_gap") {
		t.Fatal("exact-second original invented precision gap", err)
	}
	for _, scenario := range []struct {
		name   string
		mutate func()
	}{
		{"cross second", func() { at := seconds.Add(time.Second); facts.Owner.SubmittedAt = &at }},
		{"unknown precision", func() { facts.Owner.ActualSubmittedAtPrecision = 2 }},
		{"unknown type", func() { facts.Owner.ActualSubmittedAtDataType = "timestamp" }},
		{"missing clock", func() { facts.Owner.SubmittedAt = nil }},
		{"unknown rule", func() { facts.Owner.ClockComparisonRuleVersion = "unknown" }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			facts = sqlFactsFixture()
			facts.Owner.SubmittedAt, facts.Owner.ActualSubmittedAtPrecision = &seconds, 0
			scenario.mutate()
			if _, err := resolveSQLLocalFacts(source, facts); err == nil {
				t.Fatal("unknown or cross-second original clock accepted")
			}
		})
	}
	facts = sqlFactsFixture()
	facts.Owner.ActualEvaluatedAtPrecision, facts.Owner.EvaluatedAt = 0, &seconds
	v, err = resolveSQLLocalFacts(sqlResolverFixture(t, "evaluation.outcome.committed"), facts)
	if err != nil || !v.OwnerLocalTerminal || !containsString(v.Gaps, "storage_precision_gap") {
		t.Fatal("owner second precision promoted to exact milliseconds", err)
	}
	facts.Runs[0].FinishedAt = &seconds
	v, err = resolveSQLLocalFacts(sqlResolverFixture(t, "evaluation.outcome.committed"), facts)
	if err != nil || v.OwnerLocalTerminal || !containsString(v.BlockingReasons, "evaluated_owner_original_success_unproven") {
		t.Fatal("Assessment precision rule weakened Run/Outcome exact binding", err)
	}
}

func TestSQLLocalResolutionNeverApprovesSourceOrRetirement(t *testing.T) {
	for _, kind := range []string{"evaluation.requested", "evaluation.failed", "evaluation.outcome.committed"} {
		t.Run(kind, func(t *testing.T) {
			v, err := resolveSQLLocalFacts(sqlResolverFixture(t, kind), sqlFactsFixture())
			if err != nil {
				t.Fatal(err)
			}
			if !v.OwnerLocalTerminal || !v.SourceAuthenticationRequired || !v.MongoDBResponsibilityRequired || !v.GlobalUnboundResponsibilityCoverageRequired {
				t.Fatal("local facts promoted to retirement approval")
			}
			if kind != "evaluation.outcome.committed" && (v.OriginalRun != nil || !containsString(v.Gaps, "missing_original_run_identity")) {
				t.Fatal("missing original Run inferred")
			}
			if kind == "evaluation.requested" && !containsString(v.Gaps, "original_model_binding_requires_full_chain_verification") {
				t.Fatal("local intake model promoted to original cross-store proof")
			}
			if kind == "evaluation.outcome.committed" && (v.OriginalRun == nil || v.OriginalRun.RunID != "original-run" || v.OriginalRun.Attempt != 5) {
				t.Fatal("actual original attempt replaced with a default")
			}
			if _, err := json.Marshal(v); !errors.Is(err, ErrSourceSerialization) {
				t.Fatal("private local resolution serialized")
			}
			if _, err := bson.Marshal(v); !errors.Is(err, ErrSourceSerialization) {
				t.Fatal("private local resolution BSON serialized")
			}
		})
	}
}

func TestSQLLocalResolutionRetrySeparatesAuthorizationFromExecution(t *testing.T) {
	source := sqlResolverFixture(t, "evaluation.retry.requested")
	facts := sqlFactsFixture()
	at := source.BusinessAt
	facts.Runs = []sqlevaluation.SQLHistoricalRun{{ID: 1, AssessmentID: 42, ResourceID: "actual-original-failure", Scope: "evaluation_run", Attempt: 1, Status: "failed", FinishedAt: &at, Retryable: true, RetryDisposition: "automatic", RetryEventID: source.EventID}, {ID: 2, AssessmentID: 42, ResourceID: "actual-following-run", Scope: "evaluation_run", Attempt: 2, Status: "succeeded", FinishedAt: &at, Origin: "automatic"}}
	facts.Outcomes[0].RunID = "actual-following-run"
	v, err := resolveSQLLocalFacts(source, facts)
	if err != nil || !v.OwnerLocalTerminal || v.OriginalRun != nil || v.AuthorizationRun == nil || v.AuthorizationRun.Attempt != 1 || v.ExecutionRun == nil || v.ExecutionRun.Attempt != 2 {
		t.Fatal("authorization treated as executed identity", err)
	}
	source.EventID = "different-retry-id"
	v, err = resolveSQLLocalFacts(source, facts)
	if err != nil || !containsString(v.BlockingReasons, "retry_original_authorization_missing_or_ambiguous") {
		t.Fatal("unlinked authorization accepted", err)
	}
	source.EventID = facts.Runs[0].RetryEventID
	facts.Runs[1].ActionRequestID = "different-action"
	v, err = resolveSQLLocalFacts(source, facts)
	if err != nil || !containsString(v.BlockingReasons, "retry_authorized_execution_link_unproven") {
		t.Fatal("wrong actual successor accepted", err)
	}
}

func TestSQLLocalResolutionBlocksAmbiguityResponsibilityAndOwnerConflicts(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*sqlevaluation.SQLHistoricalFactsSnapshot)
		reason string
	}{
		{"runtime unknown", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) { f.Runs[0].Status = "unknown" }, "runtime_execution_unfinished_or_unknown"},
		{"runtime lease", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) { f.Runs[0].LeaseExpiresAt = f.Runs[0].FinishedAt }, "runtime_finished_or_lease_conflict"},
		{"runtime duplicate", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) { f.Runs = append(f.Runs, f.Runs[0]) }, "runtime_attempt_ambiguous"},
		{"outcome duplicate", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) { f.Outcomes = append(f.Outcomes, f.Outcomes[0]) }, "canonical_outcome_ambiguous"},
		{"outcome tenant", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) { f.Outcomes[0].OrgID = 8 }, "canonical_outcome_owner_or_run_conflict"},
		{"owner commit time", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) {
			at := f.Owner.EvaluatedAt.Add(time.Second)
			f.Owner.EvaluatedAt = &at
		}, "evaluated_owner_commit_time_conflict"},
		{"run commit time", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) {
			at := f.Runs[0].FinishedAt.Add(time.Second)
			f.Runs[0].FinishedAt = &at
		}, "evaluated_owner_original_success_unproven"},
		{"outbox pending", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) {
			f.Responsibilities = []sqlevaluation.SQLHistoricalResponsibility{{Store: "rm_outbox", OrgID: 7, AssessmentID: 42, Unfinished: true}}
		}, "current_sql_message_or_replay_responsibility_unfinished"},
		{"outbox wrong tenant", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) {
			f.Responsibilities = []sqlevaluation.SQLHistoricalResponsibility{{Store: "rm_outbox", OrgID: 8, AssessmentID: 42}}
		}, "current_message_or_replay_identity_conflict"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := sqlFactsFixture()
			scenario.mutate(&f)
			v, err := resolveSQLLocalFacts(sqlResolverFixture(t, "evaluation.outcome.committed"), f)
			if err != nil || v.OwnerLocalTerminal || !containsString(v.BlockingReasons, scenario.reason) {
				t.Fatal("local conflict suppressed", err)
			}
		})
	}
	facts := sqlFactsFixture()
	facts.Owner.OrgID = 8
	if _, err := resolveSQLLocalFacts(sqlResolverFixture(t, "evaluation.requested"), facts); !errors.Is(err, ErrSourceOrganization) {
		t.Fatal("source/owner tenant conflict accepted")
	}
}

func TestSQLLocalFailedOwnerRequiresActualCurrentTerminalRun(t *testing.T) {
	source := sqlResolverFixture(t, "evaluation.failed")
	facts := sqlFactsFixture()
	facts.Owner.Status, facts.Owner.EvaluatedAt, facts.Owner.FailedAt = "failed", nil, &source.BusinessAt
	facts.Owner.FailureReasonSHA256 = evidence.SourceDigest("sql-owner-failure-reason/v1", []byte(source.Failed.Reason)).SHA256
	facts.Outcomes = nil
	v, err := resolveSQLLocalFacts(source, facts)
	if err != nil || v.OwnerLocalTerminal || !containsString(v.BlockingReasons, "failed_owner_actual_terminal_run_unproven") {
		t.Fatal("owner failed without actual terminal failure accepted", err)
	}
	facts.Runs[0].Status, facts.Runs[0].RetryDisposition = "failed", "terminal"
	v, err = resolveSQLLocalFacts(source, facts)
	if err != nil || !v.OwnerLocalTerminal || v.OriginalRun != nil || !containsString(v.Gaps, "missing_original_run_identity") {
		t.Fatal("current terminal check invented original source Run", err)
	}
}

func TestSQLLocalFailedDoesNotUseExactTimeToInventRun(t *testing.T) {
	source := sqlResolverFixture(t, "evaluation.failed")
	facts := sqlFactsFixture()
	at := source.BusinessAt
	facts.Runs = append(facts.Runs, sqlevaluation.SQLHistoricalRun{ID: 2, AssessmentID: 42, ResourceID: "failed-a", Scope: "evaluation_run", Attempt: 1, Status: "failed", FinishedAt: &at, RetryDisposition: "terminal"})
	v, err := resolveSQLLocalFacts(source, facts)
	if err != nil || v.OriginalRun != nil || !containsString(v.Gaps, "missing_original_run_identity") {
		t.Fatal("clock candidate promoted to original Run", err)
	}
	facts.Runs = append(facts.Runs, sqlevaluation.SQLHistoricalRun{ID: 3, AssessmentID: 42, ResourceID: "failed-b", Scope: "evaluation_run", Attempt: 2, Status: "failed", FinishedAt: &at, RetryDisposition: "terminal"})
	v, err = resolveSQLLocalFacts(source, facts)
	if err != nil || !containsString(v.BlockingReasons, "original_failure_execution_ambiguous") {
		t.Fatal("ambiguous original failure accepted", err)
	}
}

func TestSQLLocalSnapshotCopiesAndUntrustedDTOMismatch(t *testing.T) {
	local, err := resolveSQLLocalFacts(sqlResolverFixture(t, "evaluation.outcome.committed"), sqlFactsFixture())
	if err != nil {
		t.Fatal(err)
	}
	r := &SQLOwnerResolution{local: local}
	v := r.Local()
	v.OriginalRun.RunID = "edited"
	v.Gaps = append(v.Gaps, "forged approval")
	if r.Local().OriginalRun.RunID == "edited" || containsString(r.Local().Gaps, "forged approval") {
		t.Fatal("editable accessor changed internal resolution")
	}
	source := sqlResolverFixture(t, "evaluation.requested")
	source.Requested.AssessmentID = 43
	if _, err := sqlSourceAssessment(source); err == nil {
		t.Fatal("contradictory source DTO accepted")
	}
	source = sqlResolverFixture(t, "evaluation.requested")
	source.BusinessIDs["testee_id"] = strconv.Itoa(22)
	if _, err := sqlSourceAssessment(source); err == nil {
		t.Fatal("contradictory source ID accepted")
	}
	source = sqlResolverFixture(t, "evaluation.requested")
	source.Submitted = &eventpayload.AnswerSheetSubmittedData{}
	if _, err := sqlSourceAssessment(source); err == nil {
		t.Fatal("contradictory source type accepted")
	}
	source = sqlResolverFixture(t, "evaluation.requested")
	facts := sqlFactsFixture()
	facts.Owner.ModelCode = "different-intake-identity"
	if _, err := resolveSQLLocalFacts(source, facts); !errors.Is(err, ErrSourceIdentity) {
		t.Fatal("fixed intake model identity conflict accepted")
	}
}
