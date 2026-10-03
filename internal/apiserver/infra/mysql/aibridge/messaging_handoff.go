package aibridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

var ErrLegacyCommandOrderUnknown = errors.New("legacy aggregate commit order requires review")

// MessagingLegacyHandoff borrows the maintenance transaction. The host must stop
// intake and both old/new relays before migration; this is not a live dual writer.
// Older rows contain retry time, not immutable command submission order. This
// conservative boundary only transfers an aggregate with one unowned pending
// command; ambiguous histories remain intact for a separate reviewed manifest.
type MessagingLegacyHandoff struct {
	Store *MessagingStore
	Seal  MessagingSeal
}

func (h *MessagingLegacyHandoff) StageSingle(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	if h == nil || h.Store == nil || h.Seal == nil || tx == nil {
		return false, app.ErrInvalid
	}
	var requestID, kind, sourceHash string
	var source []byte
	var attempts int
	var delivered bool
	var available time.Time
	err := tx.QueryRowContext(ctx, `SELECT request_id,kind,payload,payload_hash,attempts,delivered,available_at FROM ai_bridge_commands WHERE command_id=? FOR UPDATE`, id).Scan(&requestID, &kind, &source, &sourceHash, &attempts, &delivered, &available)
	if errors.Is(err, sql.ErrNoRows) {
		return false, app.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	// A previously delivered gRPC row is historical evidence, not a fresh MQ task.
	if delivered {
		return false, nil
	}
	var prior, priorKind, priorRequest, priorBody, auditBody string
	err = tx.QueryRowContext(ctx, "SELECT m.source_payload_hash,m.source_kind,m.request_id,b.body_sha256,m.messaging_body_sha256 FROM ai_messaging_legacy_commands m LEFT JOIN ai_messaging_outbox b ON b.producer='qs-server' AND b.destination='qs-ai' AND b.message_id=m.command_id WHERE m.command_id=?", id).Scan(&prior, &priorKind, &priorRequest, &priorBody, &auditBody)
	if err == nil {
		if prior != sourceHash || priorKind != kind || priorRequest != requestID || len(priorBody) != 64 || priorBody != auditBody {
			return false, app.ErrConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var existing int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_operations WHERE command_id=?", id).Scan(&existing); err != nil {
		return false, err
	}
	if existing != 0 {
		return false, app.ErrConflict
	}
	var original []byte
	var requestHash string
	var created sql.NullTime
	var indexedOrg, indexedSubject, indexedTestee sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT payload,request_hash,created_at,CAST(organization_id AS CHAR),subject_id,CAST(testee_id AS CHAR) FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", requestID).Scan(&original, &requestHash, &created, &indexedOrg, &indexedSubject, &indexedTestee); err != nil {
		return false, err
	}
	var request app.Start
	if json.Unmarshal(original, &request) != nil || request.RequestID != requestID {
		return false, app.ErrConflict
	}
	if (indexedOrg.Valid && indexedOrg.String != request.Actor.OrgID) || (indexedSubject.Valid && indexedSubject.String != request.Actor.SubjectID) || (indexedTestee.Valid && indexedTestee.String != request.TesteeID) {
		return false, app.ErrConflict
	}
	_, hash, err := encode(request)
	if err != nil || hash != requestHash {
		return false, app.ErrConflict
	}
	rows, err := tx.QueryContext(ctx, `SELECT c.command_id FROM ai_bridge_commands c WHERE c.request_id=? AND c.delivered=FALSE AND NOT EXISTS(SELECT 1 FROM ai_messaging_legacy_commands m WHERE m.command_id=c.command_id) FOR UPDATE`, requestID)
	if err != nil {
		return false, err
	}
	count := 0
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			_ = rows.Close()
			return false, err
		}
		count++
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return false, err
	}
	if count != 1 {
		return false, ErrLegacyCommandOrderUnknown
	}
	var body *pb.MessagingBody
	var messageKind pb.MessagingKind
	var resource, at string
	switch kind {
	case "start":
		var value app.Start
		if json.Unmarshal(source, &value) != nil {
			return false, app.ErrConflict
		}
		_, hash, err = encode(value)
		if err != nil || hash != sourceHash || id != value.RequestID || value.RequestID != requestID || sourceHash != requestHash {
			return false, app.ErrConflict
		}
		body = startMessagingBody(value)
		messageKind = pb.MessagingKind_START
		resource = requestID
		if created.Valid {
			at = created.Time.UTC().In(time.FixedZone("UTC+8", 28800)).Format(time.RFC3339Nano)
		}
	case "answer", "cancel":
		var value app.Change
		if json.Unmarshal(source, &value) != nil {
			return false, app.ErrConflict
		}
		_, hash, err = encode(value)
		if err != nil || hash != sourceHash || id != value.CommandID || value.Actor != request.Actor || value.Action != kind {
			return false, app.ErrConflict
		}
		if err = validateChange(ctx, tx, requestID, value); err != nil {
			return false, err
		}
		body = changeMessagingBody(value)
		messageKind = pb.MessagingKind_CHANGE
		resource = value.SessionID
		// Original per-command time was never stored. Preserve the unknown value;
		// available_at is a retry timestamp and cannot replace it.
	default:
		return false, app.ErrInvalid
	}
	org, eOrg := strconv.ParseUint(request.Actor.OrgID, 10, 64)
	testee, eTestee := strconv.ParseUint(request.TesteeID, 10, 64)
	if eOrg != nil || eTestee != nil || org == 0 || testee == 0 || strconv.FormatUint(org, 10) != request.Actor.OrgID || strconv.FormatUint(testee, 10) != request.TesteeID || attempts < 0 {
		return false, app.ErrConflict
	}
	e, raw, err := app.PrepareMessaging(messageKind, id, requestID, "", body)
	if err != nil {
		return false, err
	}
	e.OriginalOccurredAt = at
	scope := app.OperationScope{OrganizationID: request.Actor.OrgID, SubjectID: request.Actor.SubjectID, ResourceID: resource}
	_, err = h.Store.StageOperation(ctx, tx, e, raw, scope, func() (*app.PreparedMessaging, error) {
		return h.Seal(messageKind, id, requestID, scope.OrganizationID, at, body)
	})
	if err != nil {
		return false, err
	}
	// Backfill only nullable perimeter indexes from the immutable first request;
	// never invent a created_at, overwrite non-null identity, or change execution state.
	if _, err = tx.ExecContext(ctx, "UPDATE ai_bridge_requests SET organization_id=COALESCE(organization_id,?),subject_id=COALESCE(subject_id,?),testee_id=COALESCE(testee_id,?) WHERE request_id=?", org, request.Actor.SubjectID, testee, requestID); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_messaging_legacy_commands(command_id,request_id,source_kind,source_payload,source_payload_hash,source_attempts,source_available_at,source_original_time,messaging_body_sha256,transferred_at) VALUES(?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6))`, id, requestID, kind, source, sourceHash, attempts, available, at, e.BodySha256)
	if err != nil {
		return false, err
	}
	budget := attempts
	stage, errorCode := "staged", ""
	if budget >= 8 {
		budget = 8
		stage = "held"
		errorCode = "legacy_delivery_budget_exhausted"
	}
	if _, err = tx.ExecContext(ctx, "UPDATE ai_messaging_outbox SET attempts=?,available_at=?,stage=?,error_code=? WHERE producer='qs-server' AND destination='qs-ai' AND message_id=?", budget, available, stage, errorCode, id); err != nil {
		return false, err
	}
	// Do not mark delivered, reset business state, or erase the original source row.
	return true, nil
}
