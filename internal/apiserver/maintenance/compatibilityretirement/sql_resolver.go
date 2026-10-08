package retirement

import (
	"context"
	"slices"
	"strconv"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

const ErrSQLOwnerResolution SourceError = "sql_historical_owner_resolution_rejected"

// SQLLocalResolution describes only local facts. No field is a final evidence
// classification, authenticated source, permission to append, or DROP receipt.
type SQLLocalResolution struct {
	EventID, EventType                                                                                       string
	AssessmentID, OrgID, TesteeID                                                                            uint64
	OwnerStatus                                                                                              string
	OwnerLocalTerminal                                                                                       bool
	OriginalRun, AuthorizationRun, ExecutionRun                                                              *evidence.HistoricalRunReferenceV1
	Gaps, BlockingReasons                                                                                    []string
	CurrentResponsibilityCount                                                                               int
	SourceAuthenticationRequired, MongoDBResponsibilityRequired, GlobalUnboundResponsibilityCoverageRequired bool
}

func (SQLLocalResolution) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (SQLLocalResolution) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (SQLLocalResolution) String() string {
	return "private SQL-local historical resolution; not retirement approval"
}
func (SQLLocalResolution) GoString() string {
	return "private SQL-local historical resolution; not retirement approval"
}

type SQLOwnerResolution struct {
	facts *sqlevaluation.SQLHistoricalOwnerFacts
	local SQLLocalResolution
}

// ResolveSQLOwner accepts an untrusted, editable decoded DTO, and returns only
// SQL-local facts. The coordinator MUST bind the exact original row digest from
// an independently approved complete-EOF private copy before producing proof.
// It borrows the host's actual transaction; it opens/closes/commits nothing.
func ResolveSQLOwner(ctx context.Context, expectedIdentityHash string, source *DecodedSourceEvent) (*SQLOwnerResolution, error) {
	assessment, err := sqlSourceAssessment(source)
	if err != nil {
		return nil, err
	}
	facts, err := sqlevaluation.PrepareSQLHistoricalOwnerFacts(ctx, assessment, source.EventID, expectedIdentityHash)
	if err != nil {
		return nil, err
	}
	local, err := resolveSQLLocalFacts(source, facts.Snapshot())
	if err != nil {
		return nil, err
	}
	return &SQLOwnerResolution{facts: facts, local: local}, nil
}

func (r *SQLOwnerResolution) Local() SQLLocalResolution {
	if r == nil {
		return SQLLocalResolution{BlockingReasons: []string{"resolution_absent"}, SourceAuthenticationRequired: true, MongoDBResponsibilityRequired: true, GlobalUnboundResponsibilityCoverageRequired: true}
	}
	v := r.local
	v.Gaps = append([]string(nil), v.Gaps...)
	v.BlockingReasons = append([]string(nil), v.BlockingReasons...)
	clone := func(p *evidence.HistoricalRunReferenceV1) *evidence.HistoricalRunReferenceV1 {
		if p == nil {
			return nil
		}
		v := *p
		return &v
	}
	v.OriginalRun, v.AuthorizationRun, v.ExecutionRun = clone(v.OriginalRun), clone(v.AuthorizationRun), clone(v.ExecutionRun)
	return v
}

// RecheckSQL cannot authenticate a caller's altered source DTO, Mongo facts,
// global orphan coverage, or a future maintenance window. Those are separate
// gates. The original internal snapshot itself cannot be edited by callers.
func (r *SQLOwnerResolution) RecheckSQL(ctx context.Context) error {
	if r == nil || r.facts == nil {
		return ErrSQLOwnerResolution
	}
	return r.facts.Recheck(ctx)
}

func sqlSourceAssessment(source *DecodedSourceEvent) (uint64, error) {
	if source == nil || source.Submitted != nil || source.Generated != nil || source.Source.Database != "mysql" || source.Source.Object != "domain_event_outbox" || source.Source.PrimaryKeyKind != "mysql_uint64" || !evidence.ValidSHA256(source.Source.PrimaryKeySHA256) || !evidence.ValidSHA256(source.Source.Digest.SHA256) || !evidence.ValidSHA256(source.ContentDigest.SHA256) || !identifier(source.EventID, 64) || source.AggregateType != "Evaluation" || source.OrgID == 0 || source.BusinessAt.IsZero() {
		return 0, ErrSQLOwnerResolution
	}
	var assessment int64
	var org int64
	var testee uint64
	switch source.EventType {
	case "evaluation.requested", "evaluation.retry.requested":
		if source.Requested == nil || source.Failed != nil || source.OutcomeCommitted != nil {
			return 0, ErrSQLOwnerResolution
		}
		assessment, org, testee = source.Requested.AssessmentID, source.Requested.OrgID, source.Requested.TesteeID
		if !BusinessTimeEqual(source.BusinessAt, source.Requested.RequestedAt) {
			return 0, ErrSQLOwnerResolution
		}
	case "evaluation.failed":
		if source.Failed == nil || source.Requested != nil || source.OutcomeCommitted != nil {
			return 0, ErrSQLOwnerResolution
		}
		assessment, org, testee = source.Failed.AssessmentID, source.Failed.OrgID, source.Failed.TesteeID
		if !BusinessTimeEqual(source.BusinessAt, source.Failed.FailedAt) {
			return 0, ErrSQLOwnerResolution
		}
	case "evaluation.outcome.committed":
		if source.OutcomeCommitted == nil || source.Requested != nil || source.Failed != nil {
			return 0, ErrSQLOwnerResolution
		}
		assessment, org, testee = source.OutcomeCommitted.AssessmentID, source.OutcomeCommitted.OrgID, source.OutcomeCommitted.TesteeID
		if !BusinessTimeEqual(source.BusinessAt, source.OutcomeCommitted.CommittedAt) {
			return 0, ErrSQLOwnerResolution
		}
	default:
		return 0, ErrSourceEventType
	}
	if assessment <= 0 || org <= 0 || uint64(org) != source.OrgID || testee == 0 || source.AggregateID != strconv.FormatInt(assessment, 10) || source.BusinessIDs["assessment_id"] != source.AggregateID || source.BusinessIDs["testee_id"] != strconv.FormatUint(testee, 10) || (source.OuterOrgID != nil && uint64(*source.OuterOrgID) != source.OrgID) {
		return 0, ErrSQLOwnerResolution
	}
	return uint64(assessment), nil
}

func resolveSQLLocalFacts(source *DecodedSourceEvent, facts sqlevaluation.SQLHistoricalFactsSnapshot) (SQLLocalResolution, error) {
	id, err := sqlSourceAssessment(source)
	if err != nil {
		return SQLLocalResolution{}, err
	}
	o := facts.Owner
	result := SQLLocalResolution{EventID: source.EventID, EventType: source.EventType, AssessmentID: id, OrgID: o.OrgID, TesteeID: o.TesteeID, OwnerStatus: o.Status, SourceAuthenticationRequired: true, MongoDBResponsibilityRequired: true, GlobalUnboundResponsibilityCoverageRequired: true}
	if o.AssessmentID != id || o.OrgID != source.OrgID || source.BusinessIDs["testee_id"] != strconv.FormatUint(o.TesteeID, 10) {
		return result, ErrSourceOrganization
	}
	if source.OuterOrgID == nil {
		result.Gaps = append(result.Gaps, "outer_organization_absent")
	}
	block := func(reason string) { result.BlockingReasons = append(result.BlockingReasons, reason) }
	ownerClock := func(actual *time.Time, original time.Time, dataType string, precision int) bool {
		matched, gap := sqlLocalOwnerClock(actual, original, dataType, precision, o.ClockComparisonRuleVersion)
		if gap && !slices.Contains(result.Gaps, "storage_precision_gap") {
			result.Gaps = append(result.Gaps, "storage_precision_gap")
		}
		return matched
	}
	runs := map[uint]sqlevaluation.SQLHistoricalRun{}
	resources := map[string]int{}
	for _, run := range facts.Runs {
		if run.Scope != "evaluation_run" || run.AssessmentID != id || run.Attempt == 0 || run.ResourceID == "" {
			block("runtime_owner_or_scope_conflict")
			continue
		}
		if _, exists := runs[run.Attempt]; exists {
			block("runtime_attempt_ambiguous")
		}
		runs[run.Attempt] = run
		resources[run.ResourceID]++
		if run.Status != "succeeded" && run.Status != "failed" {
			block("runtime_execution_unfinished_or_unknown")
		}
		if run.FinishedAt == nil || run.LeaseExpiresAt != nil {
			block("runtime_finished_or_lease_conflict")
		}
	}
	for _, run := range runs {
		if run.Status == "failed" && (run.RetryDisposition == "automatic" || run.RetryDisposition == "manual_required" || run.RetryDisposition == "" && run.Retryable) {
			if _, exists := runs[run.Attempt+1]; !exists {
				block("business_retry_responsibility_unclosed")
			}
		} else if run.Status == "failed" && run.RetryDisposition != "" && run.RetryDisposition != "terminal" {
			block("business_retry_disposition_unknown")
		}
	}
	if len(runs) == 0 {
		block("runtime_execution_history_absent")
	}
	if len(facts.Outcomes) > 1 {
		block("canonical_outcome_ambiguous")
	}
	for _, outcome := range facts.Outcomes {
		if outcome.Invalid || outcome.AssessmentID != id || outcome.OrgID != o.OrgID || outcome.TesteeID != o.TesteeID || resources[outcome.RunID] != 1 {
			block("canonical_outcome_owner_or_run_conflict")
		}
	}
	switch o.Status {
	case "evaluated":
		if o.EvaluatedAt == nil || len(facts.Outcomes) != 1 {
			block("evaluated_owner_canonical_facts_missing")
		} else {
			outcome := facts.Outcomes[0]
			if !ownerClock(o.EvaluatedAt, outcome.EvaluatedAt, o.ActualEvaluatedAtDataType, o.ActualEvaluatedAtPrecision) {
				block("evaluated_owner_commit_time_conflict")
			}
			closed := false
			for _, run := range runs {
				if run.ResourceID == outcome.RunID && run.Status == "succeeded" && run.FinishedAt != nil && BusinessTimeEqual(*run.FinishedAt, outcome.EvaluatedAt) && !outcome.Invalid {
					closed = true
				}
			}
			if !closed {
				block("evaluated_owner_original_success_unproven")
			}
		}
	case "failed":
		if o.FailedAt == nil || len(facts.Outcomes) != 0 {
			block("failed_owner_canonical_facts_conflict")
		} else {
			currentFailures := 0
			for _, run := range runs {
				if run.Status == "failed" && run.FinishedAt != nil && ownerClock(o.FailedAt, *run.FinishedAt, o.ActualFailedAtDataType, o.ActualFailedAtPrecision) {
					currentFailures++
				}
			}
			// This checks the current terminal owner only. It never assigns
			// this clock candidate to a source that did not declare a Run.
			if currentFailures != 1 {
				block("failed_owner_actual_terminal_run_unproven")
			}
		}
	default:
		block("assessment_business_unfinished_or_unknown")
	}
	for _, responsibility := range facts.Responsibilities {
		if responsibility.Invalid || responsibility.OrgID != o.OrgID || responsibility.AssessmentID != 0 && responsibility.AssessmentID != id || responsibility.TesteeID != 0 && responsibility.TesteeID != o.TesteeID {
			block("current_message_or_replay_identity_conflict")
		}
		if responsibility.Unfinished || responsibility.LeasePresent {
			result.CurrentResponsibilityCount++
			block("current_sql_message_or_replay_responsibility_unfinished")
		}
	}
	switch source.EventType {
	case "evaluation.requested", "evaluation.retry.requested":
		p := source.Requested
		if p.AnswerSheetID != strconv.FormatUint(o.AnswerSheetID, 10) || source.BusinessIDs["answersheet_id"] != p.AnswerSheetID || p.QuestionnaireCode != o.QuestionnaireCode || p.QuestionnaireVer != o.QuestionnaireVersion {
			return result, ErrSourceIdentity
		}
		if p.HasModelIdentity() {
			kind, code, version := p.ModelKind, p.ModelCode, p.ModelVersion
			if code == "" {
				kind, code, version = "scale", p.ScaleCode, p.ScaleVersion
			}
			if kind != o.ModelKind || code != o.ModelCode || version != o.ModelVersion || p.ModelAlgorithm != "" && p.ModelAlgorithm != o.ModelAlgorithm {
				return result, ErrSourceIdentity
			}
			// Intake's fixed model is a SQL-local identity check, not proof of
			// the original cross-store Admission or original execution payload.
			result.Gaps = append(result.Gaps, "original_model_binding_requires_full_chain_verification")
		} else {
			result.Gaps = append(result.Gaps, "original_model_identity_absent")
		}
		if source.EventType == "evaluation.requested" {
			if p.ExpectedAttempt != 0 || p.AttemptOrigin != "" || p.ActionRequestID != "" || p.Mode != "" || !ownerClock(o.SubmittedAt, p.RequestedAt, o.ActualSubmittedAtDataType, o.ActualSubmittedAtPrecision) {
				return result, ErrSourceIdentity
			}
			result.Gaps = append(result.Gaps, "missing_original_run_identity")
		} else {
			if p.ExpectedAttempt <= 0 || p.Mode != "next_attempt" || (p.AttemptOrigin != "automatic" && p.AttemptOrigin != "manual" && p.AttemptOrigin != "force") {
				return result, ErrSourceIdentity
			}
			matched := 0
			for _, run := range runs {
				if run.RetryEventID == source.EventID {
					matched++
					if run.Status != "failed" || run.Attempt != uint(p.ExpectedAttempt) || run.ActionRequestID != p.ActionRequestID || run.RetryDisposition != "automatic" {
						block("retry_original_authorization_conflict")
						continue
					}
					result.AuthorizationRun = &evidence.HistoricalRunReferenceV1{RunID: run.ResourceID, Attempt: run.Attempt}
					if next, exists := runs[run.Attempt+1]; exists && next.Origin == p.AttemptOrigin && next.ActionRequestID == p.ActionRequestID {
						result.ExecutionRun = &evidence.HistoricalRunReferenceV1{RunID: next.ResourceID, Attempt: next.Attempt}
					} else {
						block("retry_authorized_execution_link_unproven")
					}
				}
			}
			if matched != 1 {
				block("retry_original_authorization_missing_or_ambiguous")
			}
			// ExpectedAttempt is never promoted to an original executed Run.
			result.Gaps = append(result.Gaps, "retry_source_does_not_declare_execution_run")
		}
	case "evaluation.failed":
		result.Gaps = append(result.Gaps, "missing_original_run_identity")
		candidates := 0
		for _, run := range runs {
			if run.Status == "failed" && run.FinishedAt != nil && BusinessTimeEqual(*run.FinishedAt, source.Failed.FailedAt) {
				candidates++
			}
		}
		if candidates > 1 {
			block("original_failure_execution_ambiguous")
		}
		if o.Status == "failed" {
			if !ownerClock(o.FailedAt, source.Failed.FailedAt, o.ActualFailedAtDataType, o.ActualFailedAtPrecision) || o.FailureReasonSHA256 != evidence.SourceDigest("sql-owner-failure-reason/v1", []byte(source.Failed.Reason)).SHA256 {
				block("original_failure_business_anchor_conflict")
			}
		} else {
			result.Gaps = append(result.Gaps, "original_failure_business_anchor_not_retained")
		}
	case "evaluation.outcome.committed":
		p := source.OutcomeCommitted
		found := 0
		for _, outcome := range facts.Outcomes {
			if strconv.FormatUint(outcome.ID, 10) == p.OutcomeID {
				found++
				if outcome.RunID != p.EvaluationRunID || !BusinessTimeEqual(outcome.EvaluatedAt, p.CommittedAt) {
					block("original_outcome_binding_conflict")
				}
			}
		}
		if found != 1 {
			block("original_outcome_missing_or_ambiguous")
		}
		for _, run := range runs {
			if run.ResourceID == p.EvaluationRunID {
				if resources[run.ResourceID] != 1 || run.Status != "succeeded" || run.FinishedAt == nil {
					block("original_outcome_run_conflict")
				} else {
					result.OriginalRun = &evidence.HistoricalRunReferenceV1{RunID: run.ResourceID, Attempt: run.Attempt}
				}
			}
		}
		if result.OriginalRun == nil {
			block("original_outcome_run_absent")
		}
	}
	result.OwnerLocalTerminal = len(result.BlockingReasons) == 0 && (o.Status == "evaluated" || o.Status == "failed")
	return result, nil
}

// This rule is limited to an actual Assessment DATETIME column header. It is
// not a nearest-clock lookup and never supplies a missing original Run.
func sqlLocalOwnerClock(actual *time.Time, original time.Time, dataType string, precision int, rule string) (matched, gap bool) {
	if actual == nil || actual.IsZero() || original.IsZero() || dataType != "datetime" || rule != sqlevaluation.SQLHistoricalClockComparisonRuleVersion {
		return false, false
	}
	switch precision {
	case 0:
		if actual.Nanosecond() != 0 || !actual.UTC().Truncate(time.Second).Equal(original.UTC().Truncate(time.Second)) {
			return false, false
		}
		return true, original.UTC().Truncate(time.Millisecond).Nanosecond() != 0
	case 3, 6:
		return BusinessTimeEqual(*actual, original), false
	default:
		return false, false
	}
}
