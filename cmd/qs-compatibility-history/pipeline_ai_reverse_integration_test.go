//go:build integration

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"io"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"github.com/go-jose/go-jose/v4"
)

func reverseNativeStatement(t *testing.T, pool *sql.DB, q string, args ...any) {
	t.Helper()
	if _, e := pool.ExecContext(t.Context(), q, args...); e != nil {
		t.Fatal("owned reverse fixture statement rejected", rawHash([]byte(q)))
	}
}
func reverseNativeJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal("synthetic body serialize")
	}
	return raw
}
func reverseNativeProtector(t *testing.T, aggregate string) func(pb.MessagingKind, string, string, *pb.MessagingBody) *app.PreparedMessaging {
	t.Helper()
	key := func(id string) jose.JSONWebKey {
		v, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal("synthetic fixture key")
		}
		return jose.JSONWebKey{Key: v, KeyID: id}
	}
	sign, encrypt := key("pipeline-reverse-sign"), key("pipeline-reverse-encrypt")
	return func(kind pb.MessagingKind, id, correlation string, body *pb.MessagingBody) *app.PreparedMessaging {
		v, e := app.ProtectMessaging(kind, id, aggregate, correlation, "7", "", body, sign, encrypt.Public())
		if e != nil {
			t.Fatal("synthetic original protocol envelope")
		}
		return v
	}
}
func reverseNativeBox(t *testing.T, pool *sql.DB, m *app.PreparedMessaging, ordered bool) {
	t.Helper()
	flag := 0
	if ordered {
		flag = 1
	}
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_outbox VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", "qs-server", "qs-ai", m.Envelope.MessageId, m.Envelope.BodySha256, m.Body, m.Wire, rawHash(m.Wire), int(m.Envelope.Kind), 7, m.Topic, m.Envelope.AggregateKey, 1, flag, flag, "confirmed", 3, "2026-10-08 11:12:13.123456", "2026-10-08 11:12:13.123456", "2026-10-08 11:12:14.123456", "2026-10-08 11:12:14.123456", "")
}
func reverseNativeIncoming(t *testing.T, pool *sql.DB, protect func(pb.MessagingKind, string, string, *pb.MessagingBody) *app.PreparedMessaging, m *app.PreparedMessaging, ackID string) {
	t.Helper()
	ack := protect(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, ackID, "", &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: m.Envelope.MessageId, EventBodySha256: m.Envelope.BodySha256, EventKind: m.Envelope.Kind, Outcome: pb.MessagingEventAcknowledgement_STORED}}})
	reverseNativeBox(t, pool, ack, false)
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_inbox VALUES(?,?,?,?,?,?,?,?,?,?)", "qs-ai", m.Envelope.MessageId, m.Envelope.BodySha256, m.Body, rawHash(m.Wire), int(m.Envelope.Kind), m.Envelope.AggregateKey, ackID, "2026-10-08 11:12:14.123456", "stored")
}
func reverseNativeMappedGraph(t *testing.T, pool *sql.DB) {
	t.Helper()
	requestID := "10000000-0000-4000-8000-000000000001"
	sessionID := "10000000-0000-4000-8000-000000000002"
	eventID := "10000000-0000-4000-8000-000000000003"
	receiptID := "10000000-0000-4000-8000-000000000004"
	runID := "10000000-0000-4000-8000-000000000005"
	request := app.Start{RequestID: requestID, Actor: app.Actor{OrgID: "7", SubjectID: "42"}, TesteeID: "9", AssessmentIDs: []string{"10022"}, Goal: "private fixture goal", Evidence: []app.EvidenceItem{{AssessmentID: "10022", TesteeID: "9", ReportID: "fixture-report", SourceVersion: "v1", Facts: []app.Fact{{Ref: "score", Value: "3"}}}}}
	raw := reverseNativeJSON(t, request)
	projection := app.Event{EventID: eventID, RequestID: requestID, SessionID: sessionID, Actor: request.Actor, TesteeID: request.TesteeID, Version: 2, Status: "cancelled"}
	projectionRaw := reverseNativeJSON(t, projection)
	reverseNativeStatement(t, pool, "INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,created_at,updated_at,version) VALUES(10022,7,9,'fixture','1',10023,'adhoc','evaluated',UTC_TIMESTAMP(),UTC_TIMESTAMP(),1)")
	reverseNativeStatement(t, pool, "INSERT INTO ai_bridge_requests(request_id,request_hash,payload,session_id,version,status,projection,organization_id,subject_id,testee_id) VALUES(?,?,?,?,2,'cancelled',?,7,'42',9)", requestID, rawHash(raw), raw, sessionID, projectionRaw)
	reverseNativeStatement(t, pool, "INSERT INTO ai_bridge_request_assessments VALUES(?,10022)", requestID)
	protect := reverseNativeProtector(t, requestID)
	start := &pb.StartCommand{RequestId: requestID, Actor: &pb.Actor{OrgId: "7", SubjectId: "42"}, TesteeId: "9", AssessmentIds: []string{"10022"}, Goal: request.Goal, Evidence: []*pb.EvidenceItem{{AssessmentId: "10022", TesteeId: "9", ReportId: "fixture-report", SourceVersion: "v1", Facts: []*pb.Fact{{Ref: "score", Value: "3"}}}}}
	command := protect(pb.MessagingKind_START, requestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: start}})
	receipt := protect(pb.MessagingKind_COMMAND_RECEIPT, receiptID, requestID, &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: requestID, CommandBodySha256: command.Envelope.BodySha256, Decision: pb.MessagingDecision_ACCEPTED, Code: "accepted", OriginalReceipt: &pb.MessagingCommandReceipt_WorkflowReceipt{WorkflowReceipt: &pb.Receipt{SessionId: sessionID, RunId: runID, Version: 1, Status: "queued"}}}}})
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_operations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,NULL,NULL)", requestID, int(pb.MessagingKind_START), command.Envelope.BodySha256, 7, "42", requestID, requestID, 1, "accepted", "accepted", receiptID, receipt.Body, "2026-10-08 11:12:13.123456", "2026-10-08 11:12:14.123456")
	reverseNativeBox(t, pool, command, true)
	reverseNativeIncoming(t, pool, protect, receipt, "10000000-0000-4000-8000-000000000006")
	state := protect(pb.MessagingKind_INTERPRETATION_STATE, eventID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: eventID, RequestId: requestID, SessionId: sessionID, Actor: &pb.Actor{OrgId: "7", SubjectId: "42"}, TesteeId: "9", Version: 2, Status: "cancelled"}}})
	reverseNativeIncoming(t, pool, protect, state, "10000000-0000-4000-8000-000000000007")
	reverseNativeStatement(t, pool, "INSERT INTO ai_bridge_events VALUES(?,?,2,?)", eventID, requestID, rawHash(projectionRaw))
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", requestID)
	reverseNativeStatement(t, pool, "INSERT INTO ai_bridge_commands VALUES(?,?,'start',?,?,0,3,?)", requestID, requestID, raw, rawHash(raw), "2026-10-08 11:12:13.123456")
	var actual []byte
	if pool.QueryRowContext(t.Context(), "SELECT CAST(payload AS BINARY) FROM ai_bridge_commands WHERE command_id=?", requestID).Scan(&actual) != nil {
		t.Fatal("actual source binary copy")
	}
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_legacy_commands VALUES(?,?,'start',?,?,3,?,'',?,?)", requestID, requestID, actual, rawHash(raw), "2026-10-08 11:12:13.123456", command.Envelope.BodySha256, "2026-10-08 11:12:14.123456")
}
func reverseNativeEvaluationGraph(t *testing.T, pool *sql.DB) string {
	t.Helper()
	run := "20000000-0000-4000-8000-000000000001"
	id := "20000000-0000-4000-8000-000000000002"
	receiptID := "20000000-0000-4000-8000-000000000003"
	protect := reverseNativeProtector(t, run)
	command := protect(pb.MessagingKind_EVALUATION_START, id, "", &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationStart{EvaluationStart: &pb.EvaluationStartCommand{Scope: &pb.EvaluationQuery{RunId: run, OrganizationId: 7, OperatorUserId: 42}, ExpectedVersion: 1, Confirm: true, Reason: "original fixture"}}})
	receipt := protect(pb.MessagingKind_COMMAND_RECEIPT, receiptID, id, &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: id, CommandBodySha256: command.Envelope.BodySha256, Decision: pb.MessagingDecision_ACCEPTED, Code: "accepted", OriginalReceipt: &pb.MessagingCommandReceipt_EvaluationReceipt{EvaluationReceipt: &pb.EvaluationState{RunId: run, Version: 1}}}}})
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_operations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,NULL,NULL)", id, int(pb.MessagingKind_EVALUATION_START), command.Envelope.BodySha256, 7, "42", run, run, 1, "accepted", "accepted", receiptID, receipt.Body, "2026-10-08 11:12:13.123456", "2026-10-08 11:12:14.123456")
	reverseNativeBox(t, pool, command, true)
	reverseNativeIncoming(t, pool, protect, receipt, "20000000-0000-4000-8000-000000000004")
	reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", run)
	return id
}
func TestHistoryAIReverseNativeWholePipelineEmptyOrphanMappedAndEvaluation(t *testing.T) {
	for _, which := range []string{"empty_open_admission", "empty_closed_admission", "empty_current_orphan", "mapped_undelivered", "outside_evaluation"} {
		t.Run(which, func(t *testing.T) {
			testSource(t)
			pool, client, db, _ := nativeFixture(t, false)
			switch which {
			case "empty_closed_admission":
				reverseNativeStatement(t, pool, "UPDATE ai_messaging_admission SET closed=1,revision=revision+1")
			case "empty_current_orphan":
				reverseNativeStatement(t, pool, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", "30000000-0000-4000-8000-000000000001")
			case "mapped_undelivered":
				reverseNativeMappedGraph(t, pool)
			case "outside_evaluation":
				reverseNativeEvaluationGraph(t, pool)
			}
			_, _, a := nativeInputs(t, pool, client, db)
			host, e := openDatabases(t.Context(), a)
			if e != nil {
				t.Fatal(safeCategory(e))
			}
			defer func() {
				if host.close() != nil {
					t.Error("owned host close")
				}
			}()
			r, e := executePipeline(t.Context(), a, host)
			if e != nil {
				t.Fatal("actual whole reverse pipeline", safeCategory(e))
			}
			if !r.CompletedReadOnlyPipeline || r.IndependentEpochs != 2 || r.AIReverseGlobal.LedgerCount != 14 || !r.AIReverseGlobal.WholeLedgerEOF || !r.AIReverseGlobal.IndependentEpochRechecked {
				t.Fatal("whole actual 14-table independent epoch coverage missing")
			}
			if which == "empty_current_orphan" {
				if r.AIReverseGlobal.Blocking == 0 || r.BlockingReasons["ai_reverse_global_blocking_responsibility"] == 0 || r.BlockingReasons["ai_reverse_global_unknown_responsibility"] == 0 {
					t.Fatal("empty old sources hid actual unreferenced current orphan")
				}
			} else if r.AIReverseGlobal.Blocking != 0 || r.AIReverseGlobal.Unknown != 0 {
				t.Fatal("legal mapped/unrelated current traffic falsely blocked")
			}
			if which == "mapped_undelivered" {
				var delivered int
				if pool.QueryRow("SELECT delivered FROM ai_bridge_commands").Scan(&delivered) != nil || delivered != 0 || r.AIReverseGlobal.Related == 0 || r.LocalCandidates != 2 {
					t.Fatal("handoff source identity/count/delivery changed")
				}
			}
			if r.DropReady || r.CASComplete || r.WriterFenceProven || r.FullExternalAIClosureVerified || r.MutationBackendEnabled {
				t.Fatal("readonly reverse pipeline granted production authority")
			}
		})
	}
}
func TestHistoryAIReverseNativeFirstHandleSurvivesRollbackAndRejectsRealDrift(t *testing.T) {
	for _, which := range []string{"identical", "admission_drift", "evaluation_original_run_drift", "current_row_deleted", "schema", "NULL_clock", "source_reader_cursor", "source_expected_changed", "proof_mixed_coordinator"} {
		t.Run(which, func(t *testing.T) {
			testSource(t)
			pool, client, db, _ := nativeFixture(t, false)
			operation := reverseNativeEvaluationGraph(t, pool)
			_, _, a := nativeInputs(t, pool, client, db)
			host, e := openDatabases(t.Context(), a)
			if e != nil {
				t.Fatal(safeCategory(e))
			}
			defer func() {
				if host.close() != nil {
					t.Error("owned host close")
				}
			}()
			var first *epochResult
			if e = host.epoch(t.Context(), func(ctx context.Context) error {
				var err error
				first, err = buildEpoch(ctx, a, host)
				if err != nil {
					return err
				}
				if proof, e := first.recheckAIReverse(ctx, first, a); e == nil || proof != nil {
					return fixedError("history_test_same_epoch_wrongly_accepted")
				}
				before := jsonHash(first.coordinator)
				originalSQL, originalCoordinator := first.sql, first.aiReverseCoordinator
				if err = first.compactOrigin(ctx); err != nil {
					return err
				}
				if a.rewind() != nil {
					return fixedError("history_asset_read_failed")
				}
				if proof, e := first.reverseAnchor.RecheckFresh(ctx, originalSQL, a.inventory.Migrations["mysql"], originalCoordinator, a.copies()); e == nil || proof != nil {
					return fixedError("history_test_live_old_tx_accepted")
				}
				if originalSQL.ValidateBorrowedSnapshot(ctx) != nil {
					return fixedError("history_test_host_scope_ended")
				}
				if first.reverseAnchor == nil || first.aiReverse != nil || first.aiReverseCoordinator != nil || first.sql != nil || first.mongo != nil || first.origin != nil || first.anchor != nil || jsonHash(first.coordinator) != before {
					return fixedError("history_test_reverse_handle_lost")
				}
				if err = first.compactOrigin(ctx); err == nil {
					return fixedError("history_test_compaction_repeated")
				}
				return nil
			}); e != nil {
				t.Fatal("actual first borrowed epoch", safeCategory(e))
			}
			switch which {
			case "admission_drift":
				reverseNativeStatement(t, pool, "UPDATE ai_messaging_admission SET closed=NOT closed,revision=revision+1")
			case "evaluation_original_run_drift":
				reverseNativeStatement(t, pool, "UPDATE ai_messaging_operations SET resource_id=? WHERE command_id=?", "20000000-0000-4000-8000-000000000009", operation)
			case "current_row_deleted":
				reverseNativeStatement(t, pool, "DELETE FROM ai_messaging_outbox WHERE kind=?", int(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT))
			case "schema":
				reverseNativeStatement(t, pool, "ALTER TABLE ai_messaging_observations ADD COLUMN future_unknown BIGINT NULL")
			case "NULL_clock":
				reverseNativeStatement(t, pool, "UPDATE ai_messaging_operations SET decided_at=NULL WHERE command_id=?", operation)
			}
			// Deliberately skip compareEpochs: the real primitive must reject raw
			// drift by actual ended/new transaction and complete native reread, even
			// when no diagnostic DTO comparison is invoked by this test.
			e = host.epoch(t.Context(), func(ctx context.Context) error {
				second, err := buildEpoch(ctx, a, host)
				if err != nil {
					return err
				}
				if which == "source_reader_cursor" {
					if a.rewind() != nil {
						return fixedError("history_asset_read_failed")
					}
					copies := a.copies()
					for _, copy := range copies {
						if _, e := io.Copy(io.Discard, copy.Input); e != nil {
							return fixedError("history_asset_read_failed")
						}
					}
					proof, e := first.reverseAnchor.RecheckFresh(ctx, second.sql, a.inventory.Migrations["mysql"], second.aiReverseCoordinator, copies)
					if e == nil || proof != nil {
						return fixedError("history_test_cursor_repaired")
					}
					return fixedError("history_ai_reverse_independent_epoch_failed")
				}
				if which == "source_expected_changed" {
					if a.rewind() != nil {
						return fixedError("history_asset_read_failed")
					}
					copies := a.copies()
					copies[0].Expected.Boundary.UpperToken = "invalid-alternate-boundary"
					proof, e := first.reverseAnchor.RecheckFresh(ctx, second.sql, a.inventory.Migrations["mysql"], second.aiReverseCoordinator, copies)
					if e == nil || proof != nil {
						return fixedError("history_test_expected_replaced")
					}
					return fixedError("history_ai_reverse_independent_epoch_failed")
				}
				aiProof, err := first.recheckAIReverse(ctx, second, a)
				if err != nil {
					return err
				}
				if a.rewind() != nil {
					return fixedError("history_asset_read_failed")
				}
				coordinator := second.aiReverseCoordinator
				if which == "proof_mixed_coordinator" {
					if a.rewind() != nil {
						return fixedError("history_asset_read_failed")
					}
					coordinator, err = retirement.PrepareHistoricalCoordinator(ctx, retirement.HistoricalCoordinatorBinding{SourceSHA: a.request.SourceSHA, OperationID: a.request.OperationID}, a.copies(), retirement.DefaultHistoricalCoordinatorLimits())
					if err != nil {
						return fixedError("history_test_new_coordinator_failed")
					}
					if a.rewind() != nil {
						return fixedError("history_asset_read_failed")
					}
				}
				proof, err := first.reverseAnchor.RecheckOrigin(ctx, aiProof, second.sql, second.mongo, coordinator, a.readers())
				if err != nil {
					return fixedError("history_actual_origin_independent_epoch_failed")
				}
				r := proof.Report()
				if !r.ActualOriginMatched || !r.IndependentEpochRechecked || !r.SourceFilesMatched || r.CASAuthorized || r.DropReady || !r.IndependentApprovalRequired || !r.FirstAuthMetadataContinuityUnproven {
					return fixedError("history_test_origin_authority_changed")
				}
				return nil
			})
			if which == "identical" {
				if e != nil {
					t.Fatal("ended-old/new actual graphless AI/origin proof", safeCategory(e))
				}
			} else {
				expected := "history_ai_reverse_independent_epoch_failed"
				if which == "schema" {
					expected = "history_ai_reverse_scan_failed"
				}
				if which == "proof_mixed_coordinator" {
					expected = "history_actual_origin_independent_epoch_failed"
				}
				if e == nil || safeCategory(e) != expected {
					t.Fatal("actual AI/schema/NULL/source/proof drift not rejected at its intended gate", safeCategory(e))
				}
			}
		})
	}
}
