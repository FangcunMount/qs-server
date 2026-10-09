package retirement

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func mongoLocalSheet() sheetmongo.AnswerSheetPO {
	return sheetmongo.AnswerSheetPO{BaseDocument: base.BaseDocument{ID: primitive.NewObjectID(), DomainID: meta.FromUint64(10042)}, OrgID: 7, TesteeID: 21, FillerID: 21, FillerType: "testee", QuestionnaireCode: "Q", QuestionnaireVersion: "1.0", FilledAt: time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC), Admission: &sheetmongo.AdmissionPO{Purpose: "independent_questionnaire", QuestionnaireCode: "Q", QuestionnaireVersion: "1.0"}}
}

func mongoLocalSource(t *testing.T, row sheetmongo.AnswerSheetPO) *DecodedSourceEvent {
	t.Helper()
	p, err := mongoSubmissionPayload(row)
	if err != nil {
		t.Fatal(err)
	}
	return &DecodedSourceEvent{Source: evidence.HistoricalSourceReferenceV1{Database: "mongodb", Object: "domain_event_outbox", PrimaryKeyKind: "mongodb_objectid", PrimaryKeySHA256: strings.Repeat("a", 64), Digest: evidence.Digest{Kind: MongoRowDigestKind, SHA256: strings.Repeat("b", 64)}}, ContentDigest: evidence.Digest{Kind: ContentDigestKind, SHA256: strings.Repeat("c", 64)}, SupportedSchema: "legacy-domain-json-v1", EventID: "original-submission", EventType: "answersheet.submitted", AggregateType: "AnswerSheet", AggregateID: row.DomainID.String(), OrgID: row.OrgID, BusinessAt: row.FilledAt, Submitted: &p}
}

func TestMongoLocalFrozenAdmissionRejectsMissingOrInferredPurpose(t *testing.T) {
	for _, kind := range []string{"missing", "unknown", "model_on_independent", "conflicting_questionnaire", "trimmed_model"} {
		t.Run(kind, func(t *testing.T) {
			row := mongoLocalSheet()
			switch kind {
			case "missing":
				row.Admission = nil
			case "unknown":
				row.Admission.Purpose = "unknown"
			case "model_on_independent":
				row.Admission.ModelCode = "model"
			case "conflicting_questionnaire":
				row.Admission.QuestionnaireCode = "other"
			case "trimmed_model":
				row.Admission.Purpose = "assessment"
				row.Admission.ModelKind = " scale "
				row.Admission.ModelCode = "M"
				row.Admission.ModelVersion = "v1"
			}
			if _, err := mongoSubmissionPayload(row); err == nil {
				t.Fatal("unproven frozen Admission accepted")
			}
		})
	}
	row := mongoLocalSheet()
	row.Admission.Purpose = "assessment"
	row.Admission.ModelKind = "scale"
	row.Admission.ModelCode = "M"
	row.Admission.ModelVersion = "v1"
	if p, err := mongoSubmissionPayload(row); err != nil || p.Admission.Purpose != eventpayload.AdmissionPurposeAssessment {
		t.Fatal("positive frozen assessment Admission rejected", err)
	}
}

func TestMongoLocalSourceCloneAndNoApprovalSerialization(t *testing.T) {
	row := mongoLocalSheet()
	source := mongoLocalSource(t, row)
	copy, err := copyMongoSource(source)
	if err != nil {
		t.Fatal(err)
	}
	source.Submitted.Admission.QuestionnaireCode = "edited"
	if copy.Submitted.Admission.QuestionnaireCode != "Q" {
		t.Fatal("source DTO aliases local baseline")
	}
	for _, value := range []any{MongoLocalResolution{}, DecodedSourceEvent{}} {
		if _, err := json.Marshal(value); err == nil {
			t.Fatal("private facts serialize as approval")
		}
	}
	if got := (*MongoOwnerResolution)(nil).Local(); !got.SourceAuthenticationRequired || !got.SQLCrossClosureRequired || !got.SQLResponsibilityRequired || !got.GlobalUnboundResponsibilityCoverageRequired || got.OwnerLocalTerminal {
		t.Fatal("empty local result claims closure")
	}
	if _, err = ResolveMongoOwner(context.Background(), nil, MongoOwnerConfig{}, source, nil); err == nil {
		t.Fatal("missing borrowed owner accepted")
	}
}

func TestMongoLocalStrictBSONNoPrecisionOrUnknownProjection(t *testing.T) {
	row := mongoLocalSheet()
	raw, err := bson.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if err = mongoPOShape(raw, reflect.TypeOf(row)); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"double_id", "negative_uint", "unknown_field", "nested_unknown", "duplicate", "string_clock"} {
		t.Run(kind, func(t *testing.T) {
			var d bson.D
			if bson.Unmarshal(raw, &d) != nil {
				t.Fatal("fixture")
			}
			switch kind {
			case "double_id":
				d = setMongoField(d, "domain_id", float64(10042))
			case "negative_uint":
				d = setMongoField(d, "org_id", int64(-7))
			case "unknown_field":
				d = append(d, bson.E{Key: "unknown_admission_v2", Value: true})
			case "nested_unknown":
				d = setMongoField(d, "admission", bson.D{{Key: "purpose", Value: "independent_questionnaire"}, {Key: "questionnaire_code", Value: "Q"}, {Key: "questionnaire_version", Value: "1.0"}, {Key: "unknown", Value: true}})
			case "duplicate":
				d = append(d, bson.E{Key: "org_id", Value: int64(7)})
			case "string_clock":
				d = setMongoField(d, "filled_at", row.FilledAt.Format(time.RFC3339Nano))
			}
			changed := marshalBSON(t, d)
			if mongoUniqueBSON(changed, 0) == nil && mongoPOShape(changed, reflect.TypeOf(row)) == nil {
				t.Fatal("unsupported BSON projected into valid owner")
			}
		})
	}
}

func TestMongoLocalMetadataContextDoesNotRetainSessionValues(t *testing.T) {
	type key struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "private-session"))
	ctx, done := mongoMetadataContext(parent)
	defer done()
	if ctx.Value(key{}) != nil {
		t.Fatal("session value retained")
	}
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("parent cancellation lost")
	}
}

func TestMongoLocalSQLSubmissionClockRetainsActualStoragePrecisionGap(t *testing.T) {
	original := mongoLocalSheet().FilledAt
	for _, kind := range []string{"same_second_gap", "exact_millis", "micro_storage", "different_second", "different_millisecond", "unknown_precision", "unknown_type", "absent_time"} {
		t.Run(kind, func(t *testing.T) {
			clock := original
			owner := sqlevaluation.SQLHistoricalOwner{SubmittedAt: &clock, ActualSubmittedAtDataType: "datetime", ActualSubmittedAtPrecision: 3}
			switch kind {
			case "same_second_gap":
				clock = clock.Truncate(time.Second)
				owner.ActualSubmittedAtPrecision = 0
			case "micro_storage":
				clock = clock.Add(123 * time.Microsecond)
				owner.ActualSubmittedAtPrecision = 6
			case "different_second":
				clock = clock.Truncate(time.Second).Add(time.Second)
				owner.ActualSubmittedAtPrecision = 0
			case "different_millisecond":
				clock = clock.Add(time.Millisecond)
			case "unknown_precision":
				owner.ActualSubmittedAtPrecision = 1
			case "unknown_type":
				owner.ActualSubmittedAtDataType = "timestamp"
			case "absent_time":
				owner.SubmittedAt = nil
			}
			got, err := mongoSQLSubmissionClock(owner, original)
			switch kind {
			case "same_second_gap":
				if err != nil || got.ExactBusinessMilliseconds || got.Precision != 0 || got.ComparisonRule != "sql-datetime0-utc-second-bucket/v1" {
					t.Fatal("SQL seconds upgraded to full original millis", err)
				}
			case "exact_millis", "micro_storage":
				if err != nil || !got.ExactBusinessMilliseconds {
					t.Fatal(err)
				}
			default:
				if err == nil {
					t.Fatal("unproved/conflicting clock accepted")
				}
			}
		})
	}
}

func TestMongoLocalSQLCurrentOwnerRequiresActualCanonicalExecution(t *testing.T) {
	at := mongoLocalSheet().FilledAt
	seconds := at.Truncate(time.Second)
	for _, kind := range []string{"evaluated_closed", "outcome_failed_run", "outcome_finish_clock", "owner_commit_clock", "failed_closed", "failed_without_clock", "failed_with_outcome", "failed_ambiguous_clock", "clock_rule_unknown"} {
		t.Run(kind, func(t *testing.T) {
			facts := sqlevaluation.SQLHistoricalFactsSnapshot{Owner: sqlevaluation.SQLHistoricalOwner{AssessmentID: 42, OrgID: 7, TesteeID: 21, Status: "evaluated", EvaluatedAt: &seconds, ActualEvaluatedAtDataType: "datetime", ActualEvaluatedAtPrecision: 0, ActualFailedAtDataType: "datetime", ActualFailedAtPrecision: 0, ClockComparisonRuleVersion: "sql-assessment-clock/v1"}, Runs: []sqlevaluation.SQLHistoricalRun{{ID: 1, AssessmentID: 42, ResourceID: "42:1", Scope: "evaluation_run", Status: "succeeded", Attempt: 1, FinishedAt: &at}}, Outcomes: []sqlevaluation.SQLHistoricalOutcome{{ID: 9001, AssessmentID: 42, OrgID: 7, TesteeID: 21, RunID: "42:1", EvaluatedAt: at}}}
			switch kind {
			case "outcome_failed_run":
				facts.Runs[0].Status = "failed"
				facts.Runs[0].RetryDisposition = "terminal"
			case "outcome_finish_clock":
				changed := at.Add(time.Millisecond)
				facts.Runs[0].FinishedAt = &changed
			case "owner_commit_clock":
				changed := seconds.Add(time.Second)
				facts.Owner.EvaluatedAt = &changed
			case "failed_closed", "failed_without_clock", "failed_with_outcome", "failed_ambiguous_clock":
				facts.Owner.Status = "failed"
				facts.Owner.FailedAt = &seconds
				facts.Runs[0].Status = "failed"
				facts.Runs[0].RetryDisposition = "terminal"
				if kind != "failed_with_outcome" {
					facts.Outcomes = nil
				}
				if kind == "failed_without_clock" {
					facts.Owner.FailedAt = nil
				}
				if kind == "failed_ambiguous_clock" {
					copy := facts.Runs[0]
					copy.ID = 2
					copy.ResourceID = "42:2"
					copy.Attempt = 2
					facts.Runs = append(facts.Runs, copy)
				}
			case "clock_rule_unknown":
				facts.Owner.ClockComparisonRuleVersion = ""
			}
			r := &MongoOwnerResolution{source: &DecodedSourceEvent{OrgID: 7}, local: MongoLocalResolution{OrgID: 7, TesteeID: 21, OwnerLocalTerminal: true}}
			r.checkSQLTerminal(t.Context(), facts)
			if kind == "evaluated_closed" || kind == "failed_closed" {
				if !r.local.OwnerLocalTerminal || len(r.local.BlockingReasons) != 0 {
					t.Fatal("actual current canonical owner rejected")
				}
			} else if r.local.OwnerLocalTerminal || len(r.local.BlockingReasons) == 0 {
				t.Fatal("mere resource existence promoted to current terminal closure")
			}
			if r.local.OriginalRun != nil {
				t.Fatal("current owner clock inferred undeclared original Run")
			}
		})
	}
}

func TestMongoLocalSQLMissingRunRequiresActualGlobalBatchObservation(t *testing.T) {
	at := mongoLocalSheet().FilledAt
	seconds := at.Truncate(time.Second)
	for _, tc := range []struct {
		name   string
		mutate func(*sqlevaluation.SQLHistoricalFactsSnapshot)
	}{
		{"nil snapshot Runs", func(*sqlevaluation.SQLHistoricalFactsSnapshot) {}},
		{"other terminal Run", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) {
			f.Runs = []sqlevaluation.SQLHistoricalRun{{ID: 1, Scope: "evaluation_run", AssessmentID: 42, ResourceID: "42:2", Attempt: 2, Status: "succeeded", FinishedAt: &at}}
		}},
		{"original cross scope", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) {
			f.Runs = []sqlevaluation.SQLHistoricalRun{{ID: 1, Scope: "other_scope", AssessmentID: 42, ResourceID: "42:1", Attempt: 1, Status: "succeeded", FinishedAt: &at}}
		}},
		{"pending responsibility", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) {
			f.Responsibilities = []sqlevaluation.SQLHistoricalResponsibility{{Store: "rm_outbox", AssessmentID: 42, OrgID: 7, Unfinished: true}}
		}},
		{"lease responsibility", func(f *sqlevaluation.SQLHistoricalFactsSnapshot) {
			f.Responsibilities = []sqlevaluation.SQLHistoricalResponsibility{{Store: "retry_event_hold", AssessmentID: 42, OrgID: 7, LeasePresent: true}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := sqlevaluation.SQLHistoricalFactsSnapshot{Owner: sqlevaluation.SQLHistoricalOwner{AssessmentID: 42, OrgID: 7, TesteeID: 21, Status: "evaluated", EvaluatedAt: &seconds, ActualEvaluatedAtDataType: "datetime", ActualEvaluatedAtPrecision: 0, ClockComparisonRuleVersion: "sql-assessment-clock/v1"}, Outcomes: []sqlevaluation.SQLHistoricalOutcome{{ID: 9001, AssessmentID: 42, OrgID: 7, TesteeID: 21, RunID: "42:1", EvaluatedAt: at}}}
			tc.mutate(&facts)
			r := &MongoOwnerResolution{source: &DecodedSourceEvent{OrgID: 7}, local: MongoLocalResolution{TesteeID: 21, OwnerLocalTerminal: true}}
			r.checkSQLTerminal(t.Context(), facts)
			if r.local.OwnerLocalTerminal || len(r.local.BlockingReasons) == 0 || r.local.OriginalRun != nil {
				t.Fatal("unproven absence settled original/current responsibility")
			}
			for _, gap := range r.local.Gaps {
				if gap == "original_outcome_run_absent" {
					t.Fatal("a DTO created a global absence gap")
				}
			}
		})
	}
}
