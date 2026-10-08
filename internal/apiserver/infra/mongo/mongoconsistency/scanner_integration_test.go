//go:build integration

package mongoconsistency

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	answersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	basemongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	evaluationfact "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/FangcunMount/reliable-messaging/message"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"testing"
	"time"
)

type auditResolver struct{}

func (auditResolver) GetTopicForEvent(kind string) (string, bool) { return "test." + kind, true }

type auditFacts struct{ err error }

func (f auditFacts) FindByID(_ context.Context, id meta.ID) (*evaluationfact.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	if id == 0 {
		return nil, evaluationfact.ErrNotFound
	}
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: id, OrgID: 1, AssessmentID: meta.ID(7), TesteeID: 8}), nil
}
func (f auditFacts) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	return nil, evaluationfact.ErrNotFound
}
func eventRow(t *testing.T, evt event.DomainEvent, binding string) (*evidence.EventEvidenceV1, bson.M) {
	t.Helper()
	intents, err := standard.PrepareIntents([]event.DomainEvent{evt}, auditResolver{}, "api-server")
	if err != nil {
		t.Fatal(err)
	}
	msg := intents[0].Message
	in := msg.Input()
	fp := msg.Fingerprint()
	ref := standard.ReferenceFromMessage(msg)
	proof, err := evidence.NewStandard(ref, binding)
	if err != nil {
		t.Fatal(err)
	}
	return proof, bson.M{"_id": identity(ref), "producer": in.Producer, "message_id": in.ID, "destination": in.Destination, "event_type": in.EventType, "schema_version": in.SchemaVersion, "scope": in.Scope, "content_type": in.ContentType, "occurred_at": in.OccurredAt, "payload": in.Payload, "fingerprint": fp[:], "state": "published", "next_attempt_at": time.Now().Add(24 * time.Hour)}
}
func newSheetFixture(t *testing.T, id uint64) (sheetmongo.AnswerSheetPO, bson.M) {
	t.Helper()
	at := time.Date(2026, 10, 8, 1, 2, 3, 123456789, time.UTC)
	data := eventpayload.AnswerSheetSubmittedData{AnswerSheetID: fmt.Sprint(id), OrgID: 1, TesteeID: 8, FillerID: 9, FillerType: "self", QuestionnaireCode: "q1", QuestionnaireVersion: "v1", SubmittedAt: at, RequestID: "original-trace"}
	evt := event.New(answersheet.EventTypeSubmitted, answersheet.AggregateType, fmt.Sprint(id), data)
	binding, err := eventevidencebinding.AnswerSheet(data)
	if err != nil {
		t.Fatal(err)
	}
	proof, row := eventRow(t, evt, binding)
	sheet := sheetmongo.AnswerSheetPO{BaseDocument: basemongo.BaseDocument{DomainID: meta.ID(id)}, OrgID: 1, TesteeID: 8, FillerID: 9, FillerType: "self", QuestionnaireCode: "q1", QuestionnaireVersion: "v1", FilledAt: at, DurableAcceptance: &sheetmongo.DurableAcceptancePO{SchemaVersion: 1, EventID: evt.EventID(), EventEvidence: proof, RequestID: data.RequestID}}
	return sheet, row
}
func TestScannerDetectsEverySupportedDriftAgainstReplicaSet(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	insertAuditDriftFixtures(t, ctx, db)
	stats := scanAllPhases(t, ctx, NewScanner(db, nil).WithOutcomeFacts(auditFacts{}))
	for kind := range appaudit.DriftSeverities {
		if stats.Findings[kind] == 0 {
			t.Errorf("drift %s was not detected; findings=%v", kind, stats.Findings)
		}
	}
}
func TestScannerAcceptsStandardAndClosedHistoryWithoutLegacyCollection(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	sheet, row := newSheetFixture(t, 101)
	mustInsertMany(t, ctx, db.Collection("answersheets"), []any{sheet})
	mustInsertMany(t, ctx, db.Collection("rm_outbox"), []any{row})
	for i, class := range []evidence.Class{evidence.RetiredVerified, evidence.Unverifiable} {
		historical, _ := newSheetFixture(t, uint64(102+i))
		proof := historical.DurableAcceptance.EventEvidence
		proof.Class = class
		proof.Reference = nil
		proof.Origin = "compatibility_retirement"
		proof.Digest = evidence.SourceDigest("mongo-selected-source-bson-v1", []byte("selected old source bytes"))
		proof.Verification = evidence.Verification{Method: "business-source-verification", Version: "v1", OperationID: "fixture-operation", Reason: "original_source_absent", VerifiedAt: time.Now(), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}
		if err := proof.Validate(); err != nil {
			t.Fatal(err)
		}
		mustInsertMany(t, ctx, db.Collection("answersheets"), []any{historical})
	}
	stats := scanAllPhases(t, ctx, NewScanner(db, nil).WithOutcomeFacts(auditFacts{}))
	if stats.Total() != 0 || stats.EvidenceClasses["standard_reference"] != 1 || stats.EvidenceClasses["retired_verified"] != 1 || stats.EvidenceClasses["unverifiable"] != 1 {
		t.Fatalf("classified healthy facts = %#v", stats)
	}
	names, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name == "domain_event_outbox" {
			t.Fatal("audit recreated legacy collection")
		}
	}
}
func TestScannerRejectsWireScopeAndBusinessCorruption(t *testing.T) {
	for _, kind := range []string{"payload", "fingerprint", "scope", "request_id", "business", "historical_open", "extra_destination"} {
		t.Run(kind, func(t *testing.T) {
			_, db := mongodbtest.ReplicaSetDatabase(t)
			ctx := t.Context()
			sheet, row := newSheetFixture(t, 101)
			switch kind {
			case "payload":
				row["payload"] = []byte("{}")
			case "fingerprint":
				row["fingerprint"] = make([]byte, 32)
			case "scope":
				row["scope"] = "org:999"
			case "request_id":
				sheet.SubmitMeta = &sheetmongo.SubmitMetaPO{RequestID: "conflicting-trace"}
			case "business":
				sheet.TesteeID = 999
			case "historical_open":
				p := sheet.DurableAcceptance.EventEvidence
				p.Class = evidence.Unverifiable
				p.Reference = nil
				p.Digest = evidence.Digest{}
				p.Origin = "retirement"
				p.Verification = evidence.Verification{Method: "business", Version: "v1", OperationID: "op", Reason: "source_missing", VerifiedAt: time.Now(), BusinessTerminal: true, OwnershipVerified: true}
			case "extra_destination":
				extra := bson.M{}
				for k, v := range row {
					extra[k] = v
				}
				extra["destination"] = "other.destination"
				extra["_id"] = bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: row["message_id"]}, {Key: "destination", Value: "other.destination"}}
				mustInsertMany(t, ctx, db.Collection("rm_outbox"), []any{extra})
			}
			mustInsertMany(t, ctx, db.Collection("answersheets"), []any{sheet})
			mustInsertMany(t, ctx, db.Collection("rm_outbox"), []any{row})
			stats := scanAllPhases(t, ctx, NewScanner(db, nil).WithOutcomeFacts(auditFacts{}))
			if kind != "extra_destination" && stats.Findings[appaudit.DriftAnswerSheetMissingOutbox] == 0 {
				t.Fatalf("forward accepted %s corruption", kind)
			}
			if stats.Findings[appaudit.DriftOutboxBusinessMismatch] == 0 {
				t.Fatalf("reverse accepted %s corruption: %#v", kind, stats)
			}
		})
	}
}
func TestScannerAuditsGeneratedAndAllRetryOriginsUsingFrozenBusinessClock(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	scanner := NewScanner(db, nil).WithOutcomeFacts(auditFacts{})
	at := time.Date(2026, 10, 8, 1, 2, 3, 123456789, time.UTC)
	g := interpretmongo.ReportGenerationPO{BaseDocument: basemongo.BaseDocument{DomainID: 201}, OutcomeID: 42, ReportType: "standard", TemplateVersion: "v1", Status: "generated", LatestRunID: 301, ReportID: 401, TransactionSchemaVersion: 1}
	run := interpretmongo.InterpretationRunPO{BaseDocument: basemongo.BaseDocument{DomainID: 301}, GenerationID: 201, Attempt: 1, Status: "succeeded"}
	artifact := interpretmongo.InterpretReportPO{BaseDocument: basemongo.BaseDocument{DomainID: 401}, GenerationID: 201, OutcomeID: 42, InterpretationRunID: 301, OrgID: 1, AssessmentID: 7, TesteeID: 8, ReportType: "standard", TemplateVersion: "v1", BuilderIdentity: "builder-v1", ContentSchemaVersion: "v1", GeneratedAt: at, Model: &interpretmongo.ModelIdentityPO{Kind: "scale", Code: "PHQ9", Version: "v1"}}
	mustInsertMany(t, ctx, db.Collection("report_generations"), []any{g})
	mustInsertMany(t, ctx, db.Collection("interpretation_runs"), []any{run})
	mustInsertMany(t, ctx, db.Collection("interpret_report_artifacts"), []any{artifact})
	payload, err := scanner.generatedPayload(ctx, g)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := eventevidencebinding.Generated(payload)
	if err != nil {
		t.Fatal(err)
	}
	evt := event.New(domaininterpretation.EventTypeReportGenerated, domaininterpretation.AggregateType, "201", payload)
	proof, row := eventRow(t, evt, binding)
	if _, err := db.Collection("report_generations").UpdateOne(ctx, bson.M{"domain_id": uint64(201)}, bson.M{"$set": bson.M{"generated_event_id": evt.EventID(), "generated_event_evidence": proof}}); err != nil {
		t.Fatal(err)
	}
	mustInsertMany(t, ctx, db.Collection("rm_outbox"), []any{row})
	for i, origin := range []string{"automatic", "manual", "force"} {
		id := uint64(302 + i)
		action := ""
		if origin != "automatic" {
			action = "operator:request:1"
		}
		retry := domaininterpretation.NewInterpretationRetryRequestedEvent(domaininterpretation.RetryRequestedEventInput{OrgID: 1, GenerationID: "201", RunID: fmt.Sprint(id), AssessmentID: "7", OutcomeID: "42", TesteeID: 8, ExpectedAttempt: 1, AttemptOrigin: origin, ActionRequestID: action, Mode: "next_attempt", RequestedAt: at})
		binding, err := eventevidencebinding.Retry(retry.Data)
		if err != nil {
			t.Fatal(err)
		}
		proof, row := eventRow(t, retry, binding)
		retryRun := interpretmongo.InterpretationRunPO{BaseDocument: basemongo.BaseDocument{DomainID: meta.ID(id)}, GenerationID: 201, Attempt: 1, Status: "failed", RetryDisposition: "automatic", NextAttemptAt: &at, RetryEventID: retry.EventID(), RetryEventEvidence: proof, ActionRequestID: action}
		mustInsertMany(t, ctx, db.Collection("interpretation_runs"), []any{retryRun})
		mustInsertMany(t, ctx, db.Collection("rm_outbox"), []any{row})
	}
	stats := scanAllPhases(t, ctx, scanner)
	if stats.Total() != 0 || stats.EvidenceClasses["standard_reference"] != 4 {
		t.Fatalf("terminal and retries = %#v", stats)
	}
	// Transport may move its due clock and retry counters. These are not the
	// original immutable schedule and must not invalidate a native reference.
	if _, err := db.Collection("rm_outbox").UpdateMany(ctx, bson.M{}, bson.M{"$set": bson.M{"next_attempt_at": at.Add(99 * time.Hour), "attempt_count": 999, "failure_count": 7}}); err != nil {
		t.Fatal(err)
	}
	if stats := scanAllPhases(t, ctx, scanner); stats.Total() != 0 {
		t.Fatalf("transport retry mutable fields changed original evidence: %#v", stats)
	}
	if _, err := db.Collection("interpret_report_artifacts").UpdateOne(ctx, bson.M{"domain_id": uint64(401)}, bson.M{"$set": bson.M{"org_id": 999}}); err != nil {
		t.Fatal(err)
	}
	if stats := scanAllPhases(t, ctx, scanner); stats.Findings[appaudit.DriftGeneratedMissingTerminalOutbox] == 0 {
		t.Fatal("artifact cross-org drift was missed")
	}
}
func TestScannerPagesActualCompoundBSONIdentitiesAndResumes(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	var docs []any
	for _, producer := range []string{"alpha", "qs-server", "zeta"} {
		for _, id := range []string{"1", "10", "100", "2"} {
			msg, err := message.New(message.Input{Producer: producer, ID: id, Destination: "test.other", EventType: "uncovered.type", SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: "2026-10-08T00:00:00Z", Payload: []byte("{}")})
			if err != nil {
				t.Fatal(err)
			}
			in := msg.Input()
			fp := msg.Fingerprint()
			docs = append(docs, bson.M{"_id": identity(standard.ReferenceFromMessage(msg)), "producer": in.Producer, "message_id": in.ID, "destination": in.Destination, "event_type": in.EventType, "fingerprint": fp[:]})
		}
	}
	mustInsertMany(t, ctx, db.Collection("rm_outbox"), docs)
	scanner := NewScanner(db, nil)
	upper, err := scanner.OutboxUpperBound(ctx, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req := appaudit.BatchRequest{Phase: appaudit.PhaseOutboxAnswerSheet, OutboxUpperBound: upper, Limit: 2, MaxTime: 3 * time.Second}
	var total int
	seen := map[string]bool{}
	for page := 0; page < 20; page++ {
		// Rebuild the scanner each page to prove the bytes alone resume correctly.
		batch, err := NewScanner(db, nil).ScanBatch(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if batch.Scanned > 0 {
			if seen[hex.EncodeToString(batch.NextOutboxCursor)] {
				t.Fatal("repeated BSON page token")
			}
			seen[hex.EncodeToString(batch.NextOutboxCursor)] = true
		}
		total += batch.Scanned
		// These identity-only fixtures intentionally omit valid wire fields.
		// They still advance exact BSON pages, while full row verification reports
		// every malformed message instead of trusting its noncovered type.
		if len(batch.Findings) != batch.Scanned {
			t.Fatalf("malformed noncovered rows escaped verification: %#v", batch)
		}
		if batch.Exhausted {
			if !bytes.Equal(batch.NextOutboxCursor, upper) {
				t.Fatal("last token changed frozen boundary")
			}
			break
		}
		req.OutboxCursor = append([]byte(nil), batch.NextOutboxCursor...)
	}
	if total != len(docs) {
		t.Fatalf("scanned %d, want %d", total, len(docs))
	}
}

func TestReverseAuditDetectsCoveredOrphanHiddenByCorruptOuterType(t *testing.T) {
	for _, kind := range []string{"unknown.type", "questionnaire.published"} {
		for _, recompute := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/recompute=%t", kind, recompute), func(t *testing.T) {
				_, db := mongodbtest.ReplicaSetDatabase(t)
				ctx := t.Context()
				_, row := newSheetFixture(t, 101)
				row["event_type"] = kind
				if recompute {
					body, err := bson.Marshal(row)
					if err != nil {
						t.Fatal(err)
					}
					var record standardEventRow
					if err := bson.Unmarshal(body, &record); err != nil {
						t.Fatal(err)
					}
					msg, err := message.New(record.input())
					if err != nil {
						t.Fatal(err)
					}
					fingerprint := msg.Fingerprint()
					row["fingerprint"] = fingerprint[:]
				}
				// No AnswerSheet exists, so forward auditing cannot catch this row.
				mustInsertMany(t, ctx, db.Collection("rm_outbox"), []any{row})
				scanner := NewScanner(db, nil)
				upper, err := scanner.OutboxUpperBound(ctx, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				batch, err := scanner.ScanBatch(ctx, appaudit.BatchRequest{Phase: appaudit.PhaseOutboxAnswerSheet, OutboxUpperBound: upper, Limit: 10, MaxTime: time.Second})
				if err != nil {
					t.Fatal(err)
				}
				if batch.Scanned != 1 || !batch.Exhausted || len(batch.Findings) != 1 || batch.Findings[0].Kind != appaudit.DriftOutboxBusinessMismatch || batch.Findings[0].Severity != appaudit.SeverityHigh {
					t.Fatalf("orphan outer-type corruption escaped reverse audit: %#v", batch)
				}
			})
		}
	}
}

func TestReverseAuditSkipsBusinessLookupOnlyAfterValidatingNoncoveredMessage(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	evt := event.New("questionnaire.published", "Questionnaire", "1", struct {
		OrgID int64 `json:"org_id"`
	}{1})
	_, row := eventRow(t, evt, evidence.BindingDigest("noncovered"))
	mustInsertMany(t, ctx, db.Collection("rm_outbox"), []any{row})
	scanner := NewScanner(db, nil)
	upper, err := scanner.OutboxUpperBound(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := scanner.ScanBatch(ctx, appaudit.BatchRequest{Phase: appaudit.PhaseOutboxAnswerSheet, OutboxUpperBound: upper, Limit: 10, MaxTime: time.Second})
	if err != nil || batch.Scanned != 1 || !batch.Exhausted || len(batch.Findings) != 0 {
		t.Fatalf("valid noncovered type changed existing business scope: result=%#v err=%v", batch, err)
	}
}

func TestScannerDoesNotSkipHistoricalRetryProofWithoutOriginalIdentity(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	at := time.Now()
	proof := &evidence.EventEvidenceV1{Version: 1, Class: evidence.Unverifiable, BusinessBindingSHA256: fmt.Sprintf("%064x", 1), Origin: "retirement", Verification: evidence.Verification{Method: "historical-source", Version: "v1", OperationID: "op", Reason: "source_absent", VerifiedAt: at, BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}
	if err := proof.Validate(); err != nil {
		t.Fatal(err)
	}
	mustInsertMany(t, ctx, db.Collection("report_generations"), []any{bson.M{"domain_id": uint64(201), "outcome_id": uint64(42)}})
	mustInsertMany(t, ctx, db.Collection("interpretation_runs"), []any{bson.M{"domain_id": uint64(301), "generation_id": uint64(201), "attempt": 1, "status": "failed", "next_attempt_at": at, "retry_disposition": "automatic", "retry_event_evidence": proof}})
	scanner := NewScanner(db, nil).WithOutcomeFacts(auditFacts{})
	upper, err := scanner.UpperBound(ctx, appaudit.PhaseRetryOutbox, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := scanner.ScanBatch(ctx, appaudit.BatchRequest{Phase: appaudit.PhaseRetryOutbox, UpperBound: upper, Limit: 10, MaxTime: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 1 || len(result.Findings) != 1 || result.Findings[0].Kind != appaudit.DriftRetryMissingScheduledOutbox {
		t.Fatal("missing original retry identity was skipped or invented")
	}
}

func TestScannerRejectsRetrySourceClockDisagreementWithinOneBSONMillisecond(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	at := time.Date(2026, 10, 8, 1, 2, 3, 123456789, time.UTC)
	mustInsertMany(t, ctx, db.Collection("report_generations"), []any{bson.M{"domain_id": uint64(201), "outcome_id": uint64(42)}})
	retry := domaininterpretation.NewInterpretationRetryRequestedEvent(domaininterpretation.RetryRequestedEventInput{OrgID: 1, GenerationID: "201", RunID: "301", AssessmentID: "7", OutcomeID: "42", TesteeID: 8, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: at})
	retry.OccurredAtValue = at.Add(time.Nanosecond)
	binding, err := eventevidencebinding.Retry(retry.Data)
	if err != nil {
		t.Fatal(err)
	}
	proof, row := eventRow(t, retry, binding)
	mustInsertMany(t, ctx, db.Collection("rm_outbox"), []any{row})
	mustInsertMany(t, ctx, db.Collection("interpretation_runs"), []any{interpretmongo.InterpretationRunPO{BaseDocument: basemongo.BaseDocument{DomainID: 301}, GenerationID: 201, Attempt: 1, Status: "failed", NextAttemptAt: &at, RetryDisposition: "automatic", RetryEventID: retry.EventID(), RetryEventEvidence: proof}})
	scanner := NewScanner(db, nil).WithOutcomeFacts(auditFacts{})
	result, err := scanner.ScanBatch(ctx, appaudit.BatchRequest{Phase: appaudit.PhaseRetryOutbox, UpperBound: 301, Limit: 10, MaxTime: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatal("source original retry clock disagreement accepted")
	}
}
func scanAllPhases(t *testing.T, ctx context.Context, scanner *Scanner) appaudit.Statistics {
	t.Helper()
	stats := appaudit.NewStatistics()
	for _, phase := range appaudit.AuditPhases {
		req := appaudit.BatchRequest{Phase: phase, Limit: 200, MaxTime: 3 * time.Second, MaxSamples: 10}
		var err error
		if phase == appaudit.PhaseOutboxAnswerSheet {
			req.OutboxUpperBound, err = scanner.OutboxUpperBound(ctx, req.MaxTime)
		} else {
			req.UpperBound, err = scanner.UpperBound(ctx, phase, req.MaxTime)
		}
		if err != nil {
			t.Fatalf("upper bound %s: %v", phase, err)
		}
		result, err := scanner.ScanBatch(ctx, req)
		if err != nil {
			t.Fatalf("scan %s: %v", phase, err)
		}
		if !result.Exhausted {
			t.Fatalf("single fixture phase %s not exhausted", phase)
		}
		stats.Add(result, 10)
	}
	return stats
}
func mustInsertMany(t *testing.T, ctx context.Context, collection *mongo.Collection, docs []any) {
	t.Helper()
	if _, err := collection.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert %s fixtures: %v", collection.Name(), err)
	}
}

func insertAuditDriftFixtures(t *testing.T, ctx context.Context, db *mongo.Database) {
	t.Helper()
	mustInsertMany(t, ctx, db.Collection("answersheets"), []any{
		bson.M{"domain_id": uint64(101), "durable_acceptance": bson.M{"schema_version": 1, "event_id": "evt-sheet-missing"}},
	})
	_, row := newSheetFixture(t, 102)
	mustInsertMany(t, ctx, db.Collection("rm_outbox"), []any{row})
	mustInsertMany(t, ctx, db.Collection("report_generations"), []any{
		bson.M{"domain_id": uint64(201), "transaction_schema_version": 1, "status": "generating", "latest_run_id": uint64(0)},
		bson.M{"domain_id": uint64(202), "transaction_schema_version": 1, "status": "generating", "latest_run_id": uint64(302)},
		bson.M{"domain_id": uint64(203), "transaction_schema_version": 1, "status": "generated", "latest_run_id": uint64(303), "report_id": uint64(403)},
	})
	mustInsertMany(t, ctx, db.Collection("interpretation_runs"), []any{
		bson.M{"domain_id": uint64(302), "generation_id": uint64(999), "status": "failed"},
		bson.M{"domain_id": uint64(303), "generation_id": uint64(203), "status": "succeeded"},
		bson.M{"domain_id": uint64(304), "generation_id": uint64(204), "status": "failed", "retry_event_id": "evt-retry-missing"},
	})
	mustInsertMany(t, ctx, db.Collection("assessment_models"), []any{
		bson.M{"domain_id": uint64(801), "record_role": "head", "status": "published", "code": "missing-active", "kind": "scale", "algorithm": "scale_default", "questionnaire_code": "q1", "questionnaire_version": "v1"},
		bson.M{"domain_id": uint64(802), "record_role": "head", "status": "published", "code": "bad-active", "kind": "scale", "algorithm": "scale_default", "questionnaire_code": "q2", "questionnaire_version": "v1"},
		bson.M{
			"record_role": "published_snapshot", "release_status": "active", "status": "published",
			"code": "bad-active", "release_version": "v1", "schema_version": "v2",
			"kind": "typology", "algorithm": "personality_typology", "decision_kind": "score_range",
			"questionnaire_code": "missing-q", "questionnaire_version": "v9",
			"definition_v2": bson.M{"calibration": bson.M{"norm_refs": []any{bson.M{"factor_code": "total", "norm_table_version": "missing-norm"}}}},
			"source":        bson.M{"definition_hash": "wrong"},
		},
	})
}
