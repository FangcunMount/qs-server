package standardoutbox

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// OriginalRequestInspection is a read-only snapshot, never an authorization
// to publish. A recovery transaction must repeat every check under its own
// locks before changing the original Outbox row.
type OriginalRequestInspection struct {
	State   string
	EventID string
	Reason  string
}

// InspectOriginalRequest checks one Assessment against its original request,
// immutable Outbox bytes and every historical Run, including soft-deleted Runs.
// It deliberately fails closed when an Assessment has more than one request.
func InspectOriginalRequest(ctx context.Context, db *sql.DB, orgID int64, assessmentID uint64, submittedBefore time.Time) (OriginalRequestInspection, error) {
	if db == nil || orgID <= 0 || assessmentID == 0 || assessmentID > math.MaxInt64 || submittedBefore.IsZero() {
		return OriginalRequestInspection{}, errors.New("invalid original request inspection input")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return OriginalRequestInspection{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := inspectOriginalRequest(ctx, tx, orgID, assessmentID, submittedBefore)
	if err != nil {
		return OriginalRequestInspection{}, err
	}
	if err := tx.Commit(); err != nil {
		return OriginalRequestInspection{}, err
	}
	return result, nil
}

type originalAssessment struct {
	orgID, testeeID, answerSheetID                     uint64
	status, questionnaireCode, questionnaireVersion    string
	modelKind, modelAlgorithm, modelCode, modelVersion sql.NullString
	submittedAt, deletedAt                             sql.NullString
	hasRun                                             bool
}

func inspectOriginalRequest(ctx context.Context, tx *sql.Tx, orgID int64, assessmentID uint64, submittedBefore time.Time) (OriginalRequestInspection, error) {
	var a originalAssessment
	err := tx.QueryRowContext(ctx, `SELECT org_id,status,DATE_FORMAT(submitted_at,'%Y-%m-%d %H:%i:%s.%f'),
	DATE_FORMAT(deleted_at,'%Y-%m-%d %H:%i:%s.%f'),testee_id,answer_sheet_id,
	questionnaire_code,questionnaire_version,evaluation_model_kind,evaluation_model_algorithm,
	evaluation_model_code,evaluation_model_version,
	EXISTS (SELECT 1 FROM runtime_checkpoint rc WHERE rc.assessment_id=a.id AND rc.scope='evaluation_run')
	FROM assessment a WHERE a.id=?`, assessmentID).Scan(&a.orgID, &a.status, &a.submittedAt,
		&a.deletedAt, &a.testeeID, &a.answerSheetID, &a.questionnaireCode, &a.questionnaireVersion,
		&a.modelKind, &a.modelAlgorithm, &a.modelCode, &a.modelVersion, &a.hasRun)
	if errors.Is(err, sql.ErrNoRows) {
		return OriginalRequestInspection{State: "manual_required", Reason: "assessment_missing"}, nil
	}
	if err != nil {
		return OriginalRequestInspection{}, err
	}
	if a.orgID != uint64(orgID) || a.deletedAt.Valid || a.status != "submitted" || !a.submittedAt.Valid {
		return OriginalRequestInspection{State: "manual_required", Reason: "assessment_not_submitted"}, nil
	}
	cutoffWall := submittedBefore.In(time.FixedZone("UTC+8", 8*60*60)).Format("2006-01-02 15:04:05.000000")
	if a.submittedAt.String > cutoffWall {
		return OriginalRequestInspection{State: "within_grace"}, nil
	}
	if a.hasRun {
		return OriginalRequestInspection{State: "manual_required", Reason: "ever_claimed"}, nil
	}
	if !a.modelKind.Valid || a.modelKind.String == "" || !a.modelCode.Valid || a.modelCode.String == "" {
		return OriginalRequestInspection{State: "manual_required", Reason: "model_missing"}, nil
	}
	refs, err := tx.QueryContext(ctx, `SELECT event_id,org_id FROM qs_rm_evaluation_request_ref
	WHERE assessment_id=? LIMIT 2`, assessmentID)
	if err != nil {
		return OriginalRequestInspection{}, err
	}
	var eventIDs []string
	for refs.Next() {
		var eventID string
		var refOrg int64
		if err := refs.Scan(&eventID, &refOrg); err != nil {
			_ = refs.Close()
			return OriginalRequestInspection{}, err
		}
		if refOrg != orgID || eventID == "" {
			_ = refs.Close()
			return OriginalRequestInspection{State: "manual_required", Reason: "reference_mismatch"}, nil
		}
		eventIDs = append(eventIDs, eventID)
	}
	if err := refs.Err(); err != nil {
		_ = refs.Close()
		return OriginalRequestInspection{}, err
	}
	if err := refs.Close(); err != nil {
		return OriginalRequestInspection{}, err
	}
	if len(eventIDs) != 1 {
		return OriginalRequestInspection{State: "manual_required", Reason: "reference_missing_or_ambiguous"}, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT producer,message_id,destination,event_type,schema_version,scope,
	content_type,occurred_at,payload,fingerprint,state,transport_confirmed_at IS NOT NULL
	FROM rm_outbox WHERE message_id=? LIMIT 2`, eventIDs[0])
	if err != nil {
		return OriginalRequestInspection{}, err
	}
	var found []originalOutbox
	for rows.Next() {
		var o originalOutbox
		if err := rows.Scan(&o.in.Producer, &o.in.ID, &o.in.Destination, &o.in.EventType,
			&o.in.SchemaVersion, &o.in.Scope, &o.in.ContentType, &o.in.OccurredAt,
			&o.in.Payload, &o.fingerprint, &o.state, &o.confirmed); err != nil {
			_ = rows.Close()
			return OriginalRequestInspection{}, err
		}
		found = append(found, o)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return OriginalRequestInspection{}, err
	}
	if err := rows.Close(); err != nil {
		return OriginalRequestInspection{}, err
	}
	if len(found) != 1 {
		return OriginalRequestInspection{State: "manual_required", Reason: "outbox_missing_or_ambiguous"}, nil
	}
	if found[0].state != "published" || !found[0].confirmed {
		return OriginalRequestInspection{State: "delivery_pending"}, nil
	}
	if reason := validateOriginalOutbox(found[0], a, orgID, assessmentID, eventIDs[0]); reason != "" {
		return OriginalRequestInspection{State: "manual_required", Reason: reason}, nil
	}
	return OriginalRequestInspection{State: "candidate_never_claimed", EventID: eventIDs[0]}, nil
}

type originalOutbox struct {
	in          message.Input
	fingerprint []byte
	state       string
	confirmed   bool
}

func validateOriginalOutbox(o originalOutbox, a originalAssessment, orgID int64, assessmentID uint64, eventID string) string {
	if o.in.Producer != "qs-server" || o.in.ID != eventID || o.in.Destination != "qs.evaluation.lifecycle" ||
		o.in.EventType != "evaluation.requested" || o.in.SchemaVersion != "v1" ||
		o.in.Scope != fmt.Sprintf("org:%d", orgID) || o.in.ContentType != "application/json" {
		return "outbox_identity_mismatch"
	}
	m, err := message.New(o.in)
	fingerprint := m.Fingerprint()
	if err != nil || !bytes.Equal(o.fingerprint, fingerprint[:]) {
		return "outbox_fingerprint_mismatch"
	}
	transport, recognized, err := legacy.Decode(o.in.Payload)
	if err != nil || !recognized || transport.UUID != eventID ||
		transport.Metadata["event_type"] != o.in.EventType ||
		transport.Metadata["aggregate_type"] != "Evaluation" ||
		transport.Metadata["aggregate_id"] != strconv.FormatUint(assessmentID, 10) ||
		transport.Metadata["source"] == "" {
		return "wire_identity_mismatch"
	}
	canonical, err := legacy.Encode(transport, legacy.Revision2)
	if err != nil || !bytes.Equal(canonical, o.in.Payload) {
		return "wire_revision_mismatch"
	}
	domain, err := domainwire.DecodeEnvelope(transport.Payload)
	if err != nil || domain.ID != eventID || domain.EventType != o.in.EventType ||
		domain.AggregateType != "Evaluation" || domain.AggregateID != strconv.FormatUint(assessmentID, 10) {
		return "domain_identity_mismatch"
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, o.in.OccurredAt)
	if err != nil || !domain.OccurredAt.Equal(occurredAt) ||
		transport.Metadata["occurred_at"] != domain.OccurredAt.Format(domainwire.OccurredAtLayout) {
		return "occurred_at_mismatch"
	}
	var data eventpayload.EvaluationRequestedData
	if err := json.Unmarshal(domain.Data, &data); err != nil || data.OrgID != orgID ||
		data.AssessmentID != int64(assessmentID) || data.TesteeID != a.testeeID ||
		data.AnswerSheetID != strconv.FormatUint(a.answerSheetID, 10) ||
		data.QuestionnaireCode != a.questionnaireCode || data.QuestionnaireVer != a.questionnaireVersion ||
		data.ModelKind != a.modelKind.String || data.ModelAlgorithm != a.modelAlgorithm.String ||
		data.ModelCode != a.modelCode.String || data.ModelVersion != a.modelVersion.String ||
		data.ExpectedAttempt != 0 || data.AttemptOrigin != "" || data.ActionRequestID != "" || data.Mode != "" {
		return "frozen_payload_mismatch"
	}
	return ""
}
