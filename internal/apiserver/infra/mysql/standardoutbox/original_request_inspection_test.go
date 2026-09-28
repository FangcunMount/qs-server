//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

const (
	originalAssessmentID = uint64(9001)
	originalEventID      = "original-evaluation-event-9001"
)

func TestInspectOriginalRequestNeverClaimedWithVerifiedWire(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mockOriginalAssessment(mock, false)
	mock.ExpectQuery("FROM qs_rm_evaluation_request_ref").WithArgs(originalAssessmentID).
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "org_id"}).AddRow(originalEventID, int64(12)))
	in, fingerprint := originalMessage(t)
	mockOriginalOutbox(mock, in, fingerprint)
	mock.ExpectCommit()
	got, err := InspectOriginalRequest(context.Background(), db, 12, originalAssessmentID, time.Date(2026, 9, 29, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)))
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "candidate_never_claimed" || got.EventID != originalEventID || got.Reason != "" {
		t.Fatalf("inspection = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInspectOriginalRequestSoftDeletedRunBlocksRecovery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	// The SQL EXISTS deliberately has no deleted_at predicate. Even a deleted
	// historical Run makes this a previously claimed request.
	mockOriginalAssessment(mock, true)
	mock.ExpectCommit()
	got, err := InspectOriginalRequest(context.Background(), db, 12, originalAssessmentID, time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "manual_required" || got.Reason != "ever_claimed" || got.EventID != "" {
		t.Fatalf("inspection = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInspectOriginalRequestAmbiguousReferenceBlocksRecovery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mockOriginalAssessment(mock, false)
	mock.ExpectQuery("FROM qs_rm_evaluation_request_ref").WithArgs(originalAssessmentID).
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "org_id"}).AddRow(originalEventID, int64(12)).AddRow("retry-event", int64(12)))
	mock.ExpectCommit()
	got, err := InspectOriginalRequest(context.Background(), db, 12, originalAssessmentID, time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "manual_required" || got.Reason != "reference_missing_or_ambiguous" || got.EventID != "" {
		t.Fatalf("inspection = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOriginalRequestRejectsChangedWireAndFingerprint(t *testing.T) {
	in, fingerprint := originalMessage(t)
	a := originalAssessment{orgID: 12, testeeID: 34, answerSheetID: 56, questionnaireCode: "Q", questionnaireVersion: "v1",
		modelKind: sql.NullString{String: "typology", Valid: true}, modelAlgorithm: sql.NullString{String: "mbti", Valid: true},
		modelCode: sql.NullString{String: "MBTI-16P", Valid: true}, modelVersion: sql.NullString{String: "1.0", Valid: true}}
	if reason := validateOriginalOutbox(originalOutbox{in: in, fingerprint: fingerprint}, a, 12, originalAssessmentID, originalEventID); reason != "" {
		t.Fatalf("valid wire: %s", reason)
	}
	changed := in
	changed.Payload = append([]byte(nil), in.Payload...)
	changed.Payload[0] ^= 1
	if reason := validateOriginalOutbox(originalOutbox{in: changed, fingerprint: fingerprint}, a, 12, originalAssessmentID, originalEventID); reason != "outbox_fingerprint_mismatch" {
		t.Fatalf("changed wire: %s", reason)
	}
	a.modelVersion.String = "2.0"
	if reason := validateOriginalOutbox(originalOutbox{in: in, fingerprint: fingerprint}, a, 12, originalAssessmentID, originalEventID); reason != "frozen_payload_mismatch" {
		t.Fatalf("changed frozen model: %s", reason)
	}
}

func mockOriginalAssessment(mock sqlmock.Sqlmock, hasRun bool) {
	mock.ExpectQuery("FROM assessment a WHERE a.id=").WithArgs(originalAssessmentID).
		WillReturnRows(sqlmock.NewRows([]string{"org_id", "status", "submitted_at", "deleted_at", "testee_id", "answer_sheet_id", "questionnaire_code", "questionnaire_version", "evaluation_model_kind", "evaluation_model_algorithm", "evaluation_model_code", "evaluation_model_version", "has_run"}).
			AddRow(uint64(12), "submitted", "2026-09-29 09:00:00.000000", nil, uint64(34), uint64(56), "Q", "v1", "typology", "mbti", "MBTI-16P", "1.0", hasRun))
}

func mockOriginalOutbox(mock sqlmock.Sqlmock, in message.Input, fingerprint []byte) {
	mock.ExpectQuery("FROM rm_outbox WHERE message_id=").WithArgs(originalEventID).
		WillReturnRows(sqlmock.NewRows([]string{"producer", "message_id", "destination", "event_type", "schema_version", "scope", "content_type", "occurred_at", "payload", "fingerprint", "state", "confirmed"}).
			AddRow(in.Producer, in.ID, in.Destination, in.EventType, in.SchemaVersion, in.Scope, in.ContentType, in.OccurredAt, in.Payload, fingerprint, "published", true))
}

func originalMessage(t *testing.T) (message.Input, []byte) {
	t.Helper()
	domain, err := json.Marshal(map[string]any{
		"id": originalEventID, "eventType": "evaluation.requested", "aggregateType": "Evaluation", "aggregateID": "9001",
		"occurredAt": "2026-09-29T09:00:00+08:00",
		"data":       map[string]any{"org_id": 12, "assessment_id": 9001, "testee_id": 34, "answersheet_id": "56", "questionnaire_code": "Q", "questionnaire_version": "v1", "model_kind": "typology", "model_algorithm": "mbti", "model_code": "MBTI-16P", "model_version": "1.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := legacy.Encode(legacy.Envelope{UUID: originalEventID, Payload: domain,
		Metadata: map[string]string{"event_type": "evaluation.requested", "aggregate_type": "Evaluation", "aggregate_id": "9001", "occurred_at": "2026-09-29T09:00:00.000+08:00", "source": "api-server"}}, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	in := message.Input{Producer: "qs-server", ID: originalEventID, Destination: "qs.evaluation.lifecycle", EventType: "evaluation.requested", SchemaVersion: "v1", Scope: "org:12", ContentType: "application/json", OccurredAt: "2026-09-29T09:00:00+08:00", Payload: wire}
	m, err := message.New(in)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := m.Fingerprint()
	return in, fingerprint[:]
}
