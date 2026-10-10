//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/sys/unix"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestHistoryCLINativeInitialTwoSnapshotInputRounds(t *testing.T) {
	for _, nonempty := range []bool{false, true} {
		t.Run(strconv.FormatBool(nonempty), func(t *testing.T) {
			testSource(t)
			pool, client, db, _ := nativeFixture(t, nonempty)
			_, _, a := nativeInputs(t, pool, client, db)
			if nonempty {
				wire := []byte("synthetic-unbound-cli-input")
				if _, e := pool.ExecContext(t.Context(), "INSERT INTO ai_messaging_quarantine(wire_sha256,wire,code,attempts,first_seen_at,last_seen_at) VALUES(?,?,?,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", rawHash(wire), wire, "synthetic_unknown"); e != nil {
					t.Fatal("owned unknown AI fixture")
				}
			}
			host, err := openDatabases(t.Context(), a)
			if err != nil {
				t.Fatal(safeCategory(err))
			}
			defer func() {
				if host.close() != nil {
					t.Error("host close")
				}
			}()
			// This separately probes the same host scope: actual driver state must
			// be snapshot-only, and Mongo must reject transaction creation.
			err = host.snapshotInputScope(t.Context(), func(ctx context.Context) error {
				s := mongo.SessionFromContext(ctx)
				x, ok := s.(mongo.XSession) //nolint:staticcheck // pinned native mode, not imported facts
				if !ok || !x.ClientSession().Snapshot || x.ClientSession().TransactionRunning() || s.StartTransaction() == nil {
					t.Fatal("host opened a Mongo transaction input scope")
				}
				return nil
			})
			if err != nil {
				t.Fatal(safeCategory(err))
			}
			j, err := newHistoryWriteJournal(filepath.Join(privateTestDir(t), "input"), a)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if j.dir.Close() != nil {
					t.Error("journal close")
				}
			}()
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			input, err := captureHistoryInitialInputs(t.Context(), a, host, j)
			if err != nil {
				t.Fatal("actual initial input capture: " + safeCategory(err))
			}
			defer func() {
				if input.close() != nil {
					t.Error("input close")
				}
			}()
			if input.sources.ValidateFrozen(t.Context()) != nil || input.ai.ValidateFrozen(t.Context()) != nil || !input.sources.Summary().TwoIndependentInputsMatched || !input.ai.Summary().TwoIndependentInputsMatched || input.ai.Summary().CASAuthority || input.sources.Summary().DropReady {
				t.Fatal("native input pair forged authorization or lost original bytes")
			}
			if input.components == nil || input.components.ValidateInputSources(t.Context(), input.sources) != nil || input.ownerSpool == nil || nonempty && len(input.components.Components()) == 0 || !nonempty && len(input.components.Components()) != 0 {
				t.Fatal("actual stopped input owner recipes missing or incomplete")
			}
			for round := range 2 {
				s := input.sourceEpochs[round].Summary()
				if !s.CaptureStopped || s.LiveSQLGraphRetained || len(input.sqlFacts[round].Ledgers) != 8 || len(input.mongoFacts[round].Collections) != 11 || len(input.aiEpochs[round].Summary().Ledgers) != 14 || !s.CompleteInput || input.sourceEpochs[round].StopCapture(t.Context()) == nil {
					t.Fatal("capture graph retained or stopped scope reused")
				}
			}
			if nonempty && input.sources.Summary().Sources[3].Records != 1 {
				t.Fatal("original submitted source skipped")
			}
			if nonempty && (input.ai.Summary().Unknown == 0 || input.ai.Summary().Blocking == 0) {
				t.Fatal("actual unknown AI responsibility was erased from pure input")
			}
			if !nonempty && input.sources.Summary().Sources[3].Records != 0 {
				t.Fatal("empty source fabricated")
			}
			runtime.GC()
			runtime.ReadMemStats(&after)
			var usage unix.Rusage
			if unix.Getrusage(unix.RUSAGE_SELF, &usage) != nil {
				t.Fatal("actual RSS unavailable")
			}
			t.Logf("actual_initial_input_rounds=2 sql8_rounds=2 mongo11_rounds=2 native_four_source_rounds=2 ai14_rounds=2 input_files=6 owner_spool_files=1 owner_components=%d retained_sql_graphs=0 heap_before_bytes=%d heap_after_gc_bytes=%d actual_process_peak_rss_platform_units=%d", len(input.components.Components()), before.HeapAlloc, after.HeapAlloc, usage.Maxrss)
			if input.close() != nil {
				t.Fatal("actual input file close")
			}
			if len(j.created) != 10 || j.sequence != 3 {
				t.Fatal("input/journal original members changed")
			}
			if hash, e := j.snapshotMaterials(t.Context()); e != nil || hash == "" {
				t.Fatal("original material handoff: " + safeCategory(e))
			}
		})
	}
}

// These are real borrowed database scopes, not an imported Q or successful
// receipt. A zero-value external object must not authorize even the actual
// terminal business/source fixture, and neither store may retain evidence.
func TestHistoryCLINativeFreshComponentMissingAIQualificationRollsBack(t *testing.T) {
	testSource(t)
	pool, client, db, _ := nativeFixture(t, true)
	closed, err := pool.ExecContext(t.Context(), "UPDATE ai_messaging_admission SET closed=1,revision=revision+1")
	if err != nil {
		t.Fatal("owned native admission close")
	}
	if rows, e := closed.RowsAffected(); e != nil || rows != 1 {
		t.Fatal("owned admission singleton missing")
	}
	_, _, a := nativeInputs(t, pool, client, db)
	host, err := openDatabases(t.Context(), a)
	if err != nil {
		t.Fatal(safeCategory(err))
	}
	defer func() {
		if host.close() != nil {
			t.Error("host close")
		}
	}()
	readOriginal := func(name string, filter bson.D) bson.Raw {
		t.Helper()
		raw, e := db.Collection(name).FindOne(t.Context(), filter).Raw()
		if e != nil {
			t.Fatal("actual original fixture read")
		}
		return append(bson.Raw(nil), raw...)
	}
	sheetFilter := bson.D{{Key: "id", Value: uint64(10042)}}
	sourceFilter := bson.D{{Key: "event_id", Value: "owned-original-submission"}}
	sheetBefore, sourceBefore := readOriginal("answersheets", sheetFilter), readOriginal("domain_event_outbox", sourceFilter)
	var operationsBefore, outboxBefore uint64
	if pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM ai_messaging_operations").Scan(&operationsBefore) != nil || pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_outbox").Scan(&outboxBefore) != nil {
		t.Fatal("actual SQL baseline read")
	}
	j, err := newHistoryWriteJournal(filepath.Join(privateTestDir(t), "missing-ai"), a)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if j.dir.Close() != nil {
			t.Error("journal close")
		}
	}()
	inputs, err := captureHistoryInitialInputs(t.Context(), a, host, j)
	if err != nil {
		t.Fatal("actual initial input capture: " + safeCategory(err))
	}
	defer func() {
		if inputs.close() != nil {
			t.Error("input close")
		}
	}()
	components := inputs.components.Components()
	if len(components) != 1 || j.sequence != 3 {
		t.Fatal("actual complete original owner input missing")
	}
	sqlStatements, mongoStatement, refs, report, err := host.writeHistoricalComponent(t.Context(), a, inputs, components[0], 0, j, &retirement.AIExternalExecutionQualification{}, &retirement.AICommandPersistenceBatch{})
	if err == nil || safeCategory(err) != "history_write_qualification_or_statement_failed" || len(sqlStatements) != 0 || mongoStatement != nil || refs != 0 || report.ActualSQLCommitResponse || report.ActualMongoCommitResponse || report.CommitState != "not_attempted" || j.unknown || j.sequence != 4 {
		t.Fatal("unqualified external input entered effects or cleanup became unknown: " + safeCategory(err))
	}
	var operationsAfter, outboxAfter uint64
	if pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM ai_messaging_operations").Scan(&operationsAfter) != nil || pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_outbox").Scan(&outboxAfter) != nil || operationsAfter != operationsBefore || outboxAfter != outboxBefore || !bytes.Equal(sheetBefore, readOriginal("answersheets", sheetFilter)) || !bytes.Equal(sourceBefore, readOriginal("domain_event_outbox", sourceFilter)) {
		t.Fatal("rejected native component retained SQL/Mongo effects")
	}
	if inputs.sources.ValidateFrozen(t.Context()) != nil || inputs.ai.ValidateFrozen(t.Context()) != nil || a.verifyFullFiles(t.Context()) != nil {
		t.Fatal("rejected native component changed actual original inputs")
	}
	t.Log("actual_fresh_rw_scope=true missing_external_qualification_rejected=true sql_commit_attempts=0 mongo_commit_attempts=0 original_business_source_and_mq_unchanged=true")
}

// Reuse the original native Outcome/Run fixture and the actual domain event
// encoder. Both old sources belong to the same persisted Assessment/AnswerSheet;
// this is not a supplied evidence entry or a serialized qualification object.
func nativeHistoryDualStoreOriginalOwner(t *testing.T, pool *sql.DB, db *mongo.Database) {
	t.Helper()
	at := time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)
	reverseNativeStatement(t, pool, "INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,evaluation_model_kind,evaluation_model_algorithm,evaluation_model_code,evaluation_model_version,created_at,updated_at,submitted_at,evaluated_at,version) VALUES(42,7,21,'Q','1.0',10042,'adhoc','evaluated','scale',?,'M','1.0',?,?,?,?,1)", string(modelcatalog.AlgorithmScaleDefault), at, at, at, at)
	reverseNativeStatement(t, pool, "INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,started_at,finished_at) VALUES('evaluation_run','42:1',1,42,'succeeded',?,?)", at, at)
	sqlDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal("actual borrowed outcome repository unavailable")
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{ID: meta.FromUint64(9001), OrgID: 7, AssessmentID: meta.FromUint64(42), TesteeID: 21, RunID: "42:1", Model: domainoutcome.ModelIdentity{Kind: modelcatalog.KindScale, Algorithm: modelcatalog.AlgorithmScaleDefault, Code: "M", Version: "1.0", Title: "Model"}, Payload: []byte(`{"score":5}`), SchemaVersion: 2, EvaluatedAt: at})
	if err != nil || sqlevaluation.NewOutcomeRepository(sqlDB).Save(t.Context(), record) != nil {
		t.Fatal("actual original outcome producer rejected")
	}
	sheet := sheetmongo.AnswerSheetPO{BaseDocument: base.BaseDocument{ID: primitive.NewObjectID(), DomainID: meta.FromUint64(10042)}, OrgID: 7, TesteeID: 21, FillerID: 21, FillerType: "testee", QuestionnaireCode: "Q", QuestionnaireVersion: "1.0", FilledAt: at, Admission: &sheetmongo.AdmissionPO{Purpose: "assessment", QuestionnaireCode: "Q", QuestionnaireVersion: "1.0", ModelKind: "scale", ModelAlgorithm: string(modelcatalog.AlgorithmScaleDefault), ModelCode: "M", ModelVersion: "1.0", ModelTitle: "Model"}}
	if _, err = db.Collection("answersheets").InsertOne(t.Context(), sheet); err != nil {
		t.Fatal("actual original assessment sheet producer rejected")
	}
	submission := event.Event[eventpayload.AnswerSheetSubmittedData]{BaseEvent: event.BaseEvent{ID: "owned-original-submission", EventTypeValue: "answersheet.submitted", AggregateTypeValue: "AnswerSheet", AggregateIDValue: "10042", OccurredAtValue: at.Add(time.Second)}, Data: eventpayload.AnswerSheetSubmittedData{AnswerSheetID: "10042", QuestionnaireCode: "Q", QuestionnaireVersion: "1.0", OrgID: 7, TesteeID: 21, FillerID: 21, FillerType: "testee", SubmittedAt: at, Admission: &eventpayload.AssessmentAdmission{Purpose: eventpayload.AdmissionPurposeAssessment, QuestionnaireCode: "Q", QuestionnaireVersion: "1.0", ModelKind: "scale", ModelAlgorithm: string(modelcatalog.AlgorithmScaleDefault), ModelCode: "M", ModelVersion: "1.0", ModelTitle: "Model"}}}
	body, err := domainwire.EncodeEvent(submission)
	if err != nil {
		t.Fatal("actual original submission encoder rejected")
	}
	if _, err = db.Collection("domain_event_outbox").InsertOne(t.Context(), bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "event_id", Value: submission.ID}, {Key: "event_type", Value: submission.EventTypeValue}, {Key: "aggregate_type", Value: submission.AggregateTypeValue}, {Key: "aggregate_id", Value: submission.AggregateIDValue}, {Key: "org_id", Value: int64(7)}, {Key: "topic_name", Value: "qs-survey"}, {Key: "payload_json", Value: string(body)}, {Key: "status", Value: "published"}, {Key: "attempt_count", Value: int32(0)}, {Key: "next_attempt_at", Value: at}, {Key: "created_at", Value: at}, {Key: "updated_at", Value: at.Add(time.Second)}, {Key: "published_at", Value: at.Add(time.Second)}}); err != nil {
		t.Fatal("actual original submission ledger rejected")
	}
	for n, id := range []string{"owned-original-outcome", "owned-original-outcome-2"} {
		committed := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: id, EventTypeValue: "evaluation.outcome.committed", AggregateTypeValue: "Evaluation", AggregateIDValue: "42", OccurredAtValue: at}, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "42:1", CommittedAt: at}}
		body, err = domainwire.EncodeEvent(committed)
		if err != nil {
			t.Fatal("actual original outcome encoder rejected")
		}
		reverseNativeStatement(t, pool, "INSERT INTO domain_event_outbox(id,event_id,event_type,aggregate_type,aggregate_id,org_id,topic_name,payload_json,status,attempt_count,next_attempt_at,created_at,updated_at,published_at) VALUES(?,?,'evaluation.outcome.committed','Evaluation','42',7,'qs-evaluation',?,'published',0,?,?,?,?)", n+1, committed.ID, body, at, at, at.Add(time.Second), at.Add(time.Second))
	}
}

// The runtime helper builds the real pinned qs-ai image in the independently
// owned Linux job. Its fixed producer, database boundaries and driver reads
// supply the Q; no successful fixture DTO or external footer is imported.
func TestHistoryCLINativeQualifiedEvidenceWriteActualExternalAndDualCommitReadback(t *testing.T) {
	testSource(t)
	pool, client, db, _ := nativeFixture(t, false)
	nativeHistoryDualStoreOriginalOwner(t, pool, db)
	reverseNativeStatement(t, pool, "UPDATE ai_messaging_admission SET closed=1,revision=revision+1")
	_, _, a := nativeInputs(t, pool, client, db)
	in := nativeHistoryExternalFixture(t, pool, client, db, a)
	host, err := openDatabases(t.Context(), a)
	if err != nil {
		t.Fatal(safeCategory(err))
	}
	defer func() {
		if host.close() != nil {
			t.Error("actual host close")
		}
	}()
	filter := bson.D{{Key: "event_id", Value: "owned-original-submission"}}
	before, err := db.Collection("domain_event_outbox").FindOne(t.Context(), filter).Raw()
	if err != nil {
		t.Fatal("actual original source read")
	}
	var sqlOriginalBefore []byte
	var sqlStatusBefore string
	var sqlAttemptsBefore uint64
	if pool.QueryRowContext(t.Context(), "SELECT CAST(payload_json AS BINARY),status,attempt_count FROM domain_event_outbox WHERE event_id='owned-original-outcome'").Scan(&sqlOriginalBefore, &sqlStatusBefore, &sqlAttemptsBefore) != nil {
		t.Fatal("actual original SQL source baseline missing")
	}
	out := filepath.Join(privateTestDir(t), "actual-write")
	report, err := executeHistoricalEvidenceWrite(t.Context(), a, host, out, in)
	if err != nil {
		t.Fatal("actual complete write pipeline: " + safeCategory(err))
	}
	if !report.AICommandPersistenceComplete || report.AIOriginalCommands != 0 || report.AISourceReferences != 0 || !report.ActualSQLCommitResponse || !report.ActualMongoCommitResponse || report.MongoCommitRequirement != "required" || report.CommitState != "both_responses_success_non_atomic" || report.EventReferences != 3 || report.PreparedPages != 1 || report.ReadBackPages != 1 || !report.LimitedEventPersistenceObserved || report.MaterialManifestSHA256 == "" || report.DropReady || report.CASComplete || report.WriterFenceProven || report.MutationBackendEnabled || report.FullExternalAIClosureVerified {
		t.Fatal("actual commit/readback facts or unproved production boundaries misstated")
	}
	var sheet struct {
		Legacy *evidence.HistoricalReferenceSetV1 `bson:"legacy_submission_evidence"`
	}
	if db.Collection("answersheets").FindOne(t.Context(), bson.D{{Key: "id", Value: uint64(10042)}}).Decode(&sheet) != nil || sheet.Legacy == nil || sheet.Legacy.Validate() != nil || len(sheet.Legacy.Entries) != 1 || sheet.Legacy.Entries[0].EventID != "owned-original-submission" || sheet.Legacy.Entries[0].Proof.Verification.Method != "actual-fresh-owner-component-source-business-related-ai" {
		t.Fatal("actual persisted original identity/reference missing")
	}
	var historicalSQL []byte
	if pool.QueryRowContext(t.Context(), "SELECT CAST(historical_committed_evidence AS BINARY) FROM evaluation_outcome WHERE id=9001 AND committed_event_id IS NULL AND committed_event_evidence IS NULL").Scan(&historicalSQL) != nil {
		t.Fatal("actual independently persisted SQL original outcome evidence missing")
	}
	var sqlSet evidence.HistoricalReferenceSetV1
	if json.Unmarshal(historicalSQL, &sqlSet) != nil || sqlSet.Validate() != nil || len(sqlSet.Entries) != 2 {
		t.Fatal("actual SQL identity/original Run and scoped verification missing")
	}
	seenSQL := map[string]bool{}
	for _, entry := range sqlSet.Entries {
		if entry.Run == nil || entry.Run.RunID != "42:1" || entry.Proof.Verification.Method != "actual-fresh-owner-component-source-business-related-ai" || seenSQL[entry.EventID] {
			t.Fatal("same-owner SQL original identity or Run reference changed")
		}
		seenSQL[entry.EventID] = true
	}
	if !seenSQL["owned-original-outcome"] || !seenSQL["owned-original-outcome-2"] {
		t.Fatal("same-owner original SQL reference omitted")
	}
	after, err := db.Collection("domain_event_outbox").FindOne(t.Context(), filter).Raw()
	if err != nil || !bytes.Equal(before, after) || a.verifyFullFiles(t.Context()) != nil {
		t.Fatal("evidence write altered the original message")
	}
	var sqlOriginalAfter []byte
	var sqlStatusAfter string
	var sqlAttemptsAfter uint64
	if pool.QueryRowContext(t.Context(), "SELECT CAST(payload_json AS BINARY),status,attempt_count FROM domain_event_outbox WHERE event_id='owned-original-outcome'").Scan(&sqlOriginalAfter, &sqlStatusAfter, &sqlAttemptsAfter) != nil || !bytes.Equal(sqlOriginalBefore, sqlOriginalAfter) || sqlStatusBefore != sqlStatusAfter || sqlAttemptsBefore != sqlAttemptsAfter {
		t.Fatal("evidence write altered actual original SQL message or delivery facts")
	}
	material, err := os.ReadFile(filepath.Join(out, "history.materials.private.json"))
	var manifest historyTemporaryMaterialManifest
	if err != nil || strictDecode(material, &manifest) != nil || manifest.Version != 2 || rawHash(material) != report.MaterialManifestSHA256 || len(manifest.Files) != 7+int(manifest.JournalSequence) {
		t.Fatal("actual seven input material handoff incomplete")
	}
	stages := map[string]int{}
	for n := uint64(1); n <= manifest.JournalSequence; n++ {
		raw, e := os.ReadFile(filepath.Join(out, "journal-"+strconv.FormatUint(n, 10)+".json"))
		var record historyWriteJournalRecord
		if e != nil || json.Unmarshal(raw, &record) != nil || record.Sequence != n || record.SourceSHA != a.request.SourceSHA || record.OperationID != a.request.OperationID || record.RunID != a.request.RunID || record.Complete || record.CASAuthorized || record.DropReady || record.ExecutionReady {
			t.Fatal("actual native journal lineage or unproved authority changed")
		}
		stages[record.Stage]++
	}
	if stages["initial_input_epoch_frozen"] != 2 || stages["sql_commit_success"] != 2 || stages["mongo_commit_success"] != 1 || stages["ai_independent_readback_finished"] != 1 || stages["fresh_page_verified"] != 1 || stages["limited_event_readback_finished"] != 1 || stages["sql_commit_unknown"] != 0 || stages["mongo_commit_unknown"] != 0 {
		t.Fatal("actual initial scopes, AI commit/readback or event dual commit/readback incomplete")
	}
	t.Log("actual_fixed_qs_ai_producer=true initial_native_scopes=2 actual_ai_rw_commit_and_different_rrro_readback=true event_dual_known_commit_and_different_native_readback=true material_protocol=2 input_files=7 drop_ready=false")
}

func TestHistoryCLINativeGenuineAIQualificationCannotWriteFromReadonlyComponent(t *testing.T) {
	testSource(t)
	pool, client, db, _ := nativeFixture(t, true)
	reverseNativeStatement(t, pool, "UPDATE ai_messaging_admission SET closed=1,revision=revision+1")
	_, _, a := nativeInputs(t, pool, client, db)
	in := nativeHistoryExternalFixture(t, pool, client, db, a)
	host, err := openDatabases(t.Context(), a)
	if err != nil {
		t.Fatal(safeCategory(err))
	}
	defer func() {
		if host.close() != nil {
			t.Error("actual host close")
		}
	}()
	j, err := newHistoryWriteJournal(filepath.Join(privateTestDir(t), "readonly-component"), a)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if j.dir.Close() != nil {
			t.Error("journal close")
		}
	}()
	inputs, err := captureHistoryInitialInputs(t.Context(), a, host, j)
	if err != nil {
		t.Fatal("actual initial inputs: " + safeCategory(err))
	}
	defer func() {
		if inputs.close() != nil {
			t.Error("original input close")
		}
	}()
	in.Binding = retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}
	canonicalRun, err := aiHostRun(a.request.RunID)
	if err != nil || in.RunID != canonicalRun || retirement.RequireAIExternalExecQuiescence(t.Context(), retirement.AIExternalExecQuiescenceInput{OperationDirectory: in.OperationDirectory, SourceSHA: in.Binding.SourceSHA, OperationID: in.Binding.OperationID, RuntimeSourceSHA: in.RuntimeSourceSHA, ImageID: in.ImageID, ContainerID: in.ContainerID, SudoDocker: in.SudoDocker}) != nil {
		t.Fatal("actual external execution lineage/quiescence missing")
	}
	var q *retirement.AIExternalExecutionQualification
	var batch *retirement.AICommandPersistenceBatch
	if host.epoch(t.Context(), func(scope context.Context) error {
		var e error
		q, e = retirement.PrepareHistoricalAIExternalExecution(scope, inputs.sources, inputs.ai, in)
		if e != nil {
			return e
		}
		batch, e = retirement.PrepareHistoricalAICommandPersistenceBatch(scope, inputs.sources, inputs.ai, q)
		return e
	}) != nil {
		t.Fatal("actual fixed producer and same native Q/batch scope failed")
	}
	_, aiReport, err := host.writeHistoricalAI(t.Context(), batch, j)
	if err != nil || !aiReport.ActualSQLCommitResponse {
		t.Fatal("actual independent AI write transaction failed")
	}
	if host.epoch(t.Context(), func(scope context.Context) error {
		read, e := batch.VerifyHistoricalReadback(scope, inputs.sources, inputs.ai)
		if e != nil {
			return e
		}
		if !read.IndependentReadbackMatched {
			return retirement.ErrCoordinatorCASQualification
		}
		return nil
	}) != nil {
		t.Fatal("actual different native AI persisted readback failed")
	}
	if host.epoch(t.Context(), func(scope context.Context) error {
		for _, component := range inputs.components.Components() {
			observed, e := retirement.PrepareHistoricalComponentObservation(scope, component, inputs.sources, inputs.ai, host.mongo, 20*time.Second, false)
			if e != nil {
				return e
			}
			sql, mongo, refs, e := retirement.ApplyQualifiedHistoricalComponent(scope, observed, q, batch)
			if len(sql) != 0 || mongo != nil || refs != 0 || !errors.Is(e, retirement.ErrCoordinatorCASQualification) {
				return retirement.ErrSourceOriginFresh
			}
		}
		return nil
	}) != nil {
		t.Fatal("genuine external qualification did not reject actual readonly component")
	}
	var count int64
	if count, err = db.Collection("answersheets").CountDocuments(t.Context(), bson.D{{Key: "legacy_submission_evidence", Value: bson.D{{Key: "$exists", Value: true}}}}); err != nil || count != 0 {
		t.Fatal("readonly component retained Mongo evidence")
	}
	t.Log("actual_fixed_q_and_committed_ai_readback=true actual_rrro_component_rejected_before_statements=true event_sql_commit_attempts=0 event_mongo_commit_attempts=0 event_evidence_remaining=0")
}
