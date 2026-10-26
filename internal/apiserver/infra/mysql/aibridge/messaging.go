package aibridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
)

// MessagingStore borrows host transactions. Constructors perform no I/O and
// methods never commit, schedule delivery or close a database connection.
type MessagingStore struct {
	Archive *TransactionArchive
}

func NewMessagingStore() *MessagingStore {
	return &MessagingStore{}
}

func messagingHash(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func operationScopeMatches(e *pb.MessagingEnvelope, b *pb.MessagingBody, scope app.OperationScope) bool {
	var org, subject, resource string
	switch e.Kind {
	case pb.MessagingKind_START:
		c := b.GetStart()
		org, subject, resource = c.GetActor().GetOrgId(), c.GetActor().GetSubjectId(), c.RequestId
		if e.AggregateKey != c.RequestId {
			return false
		}
	case pb.MessagingKind_CHANGE:
		c := b.GetChange()
		org, subject, resource = c.GetActor().GetOrgId(), c.GetActor().GetSubjectId(), c.SessionId
	case pb.MessagingKind_PARTICIPANT_RETRY:
		c := b.GetParticipantRetry()
		org, subject, resource = strconv.FormatInt(c.GetScope().GetOrganizationId(), 10), strconv.FormatInt(c.GetScope().GetOperatorUserId(), 10), c.SessionId
	case pb.MessagingKind_EVALUATION_START:
		c := b.GetEvaluationStart().GetScope()
		org, subject, resource = strconv.FormatInt(c.GetOrganizationId(), 10), strconv.FormatInt(c.GetOperatorUserId(), 10), c.GetRunId()
	case pb.MessagingKind_EVALUATION_CANCEL:
		c := b.GetEvaluationCancel().GetScope()
		org, subject, resource = strconv.FormatInt(c.GetOrganizationId(), 10), strconv.FormatInt(c.GetOperatorUserId(), 10), c.GetRunId()
	}
	parsedOrg, err := strconv.ParseUint(org, 10, 64)
	id, idErr := uuid.Parse(resource)
	return err == nil && parsedOrg > 0 && strconv.FormatUint(parsedOrg, 10) == org && idErr == nil && id.String() == resource && org == scope.OrganizationID && subject == scope.SubjectID && resource == scope.ResourceID
}

// existingOperation reads immutable submission fields, without reserving a
// missing ID. It only excludes retained identities from current execution;
// it does not authorize replay or return historical operation results.
func (s *MessagingStore) existingOperation(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope, scope app.OperationScope) (bool, error) {
	var hash sql.NullString
	var org, subject, resource, aggregate string
	var retired bool
	var kind int32
	err := tx.QueryRowContext(ctx, "SELECT retired,body_sha256,CAST(organization_id AS CHAR),subject_id,resource_id,aggregate_key,kind FROM ai_messaging_operations WHERE command_id=?", e.MessageId).Scan(&retired, &hash, &org, &subject, &resource, &aggregate, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if retired || !hash.Valid || hash.String != e.BodySha256 || org != scope.OrganizationID || subject != scope.SubjectID || resource != scope.ResourceID || aggregate != e.AggregateKey || kind != int32(e.Kind) {
		return false, app.ErrConflict
	}
	return true, nil
}

func (s *MessagingStore) applyEvent(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope, body *pb.MessagingBody, raw []byte) (string, error) {
	switch e.Kind {
	case pb.MessagingKind_COMMAND_RECEIPT:
		return s.applyReceipt(ctx, tx, e, body.GetCommandReceipt(), raw)
	case pb.MessagingKind_INTERPRETATION_STATE:
		v := body.GetInterpretationState()
		event := app.Event{EventID: v.EventId, RequestID: v.RequestId, SessionID: v.SessionId, Actor: app.Actor{OrgID: v.GetActor().GetOrgId(), SubjectID: v.GetActor().GetSubjectId()}, TesteeID: v.TesteeId, Version: v.Version, Status: v.Status, QuestionID: v.QuestionId, Question: v.Question, CanSkip: v.CanSkip, FailureCode: v.FailureCode, ArtifactJSON: v.ArtifactJson}
		if e.AggregateKey != event.RequestID {
			return "", app.ErrConflict
		}
		if err := app.ValidateEvent(event); err != nil {
			return "", err
		}
		return event.Actor.OrgID, acceptInTransaction(ctx, tx, event)
	case pb.MessagingKind_EVALUATION_STATE:
		v := body.GetEvaluationState()
		if e.AggregateKey != v.RunId || v.Version < 1 || v.Status == "" || len(v.Status) > 64 {
			return "", app.ErrMessagingContract
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO ai_messaging_evaluation_states(run_id,organization_id,event_sequence,version,state,updated_at) VALUES(?,?,0,0,?,UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE run_id=run_id`, v.RunId, v.OrganizationId, []byte{})
		if err != nil {
			return "", err
		}
		var org string
		var sequence uint64
		var version int64
		var oldRaw []byte
		err = tx.QueryRowContext(ctx, "SELECT CAST(organization_id AS CHAR),event_sequence,version,state FROM ai_messaging_evaluation_states WHERE run_id=? FOR UPDATE", v.RunId).Scan(&org, &sequence, &version, &oldRaw)
		if err != nil {
			return "", err
		}
		if org != v.OrganizationId || (v.EventSequence > sequence && v.Version < version) || (v.EventSequence == sequence && !bytes.Equal(oldRaw, raw)) {
			return "", app.ErrConflict
		}
		if v.EventSequence > sequence {
			_, err = tx.ExecContext(ctx, "UPDATE ai_messaging_evaluation_states SET event_sequence=?,version=?,state=?,updated_at=UTC_TIMESTAMP(6) WHERE run_id=?", v.EventSequence, v.Version, raw, v.RunId)
		}
		return org, err
	default:
		return "", app.ErrMessagingContract
	}
}

func (s *MessagingStore) applyReceipt(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope, r *pb.MessagingCommandReceipt, raw []byte) (string, error) {
	if s == nil || s.Archive == nil {
		return "", app.ErrManagementUnavailable
	}
	var hash sql.NullString
	var org, aggregate, resource string
	var retired bool
	var receiptID sql.NullString
	var kind int32
	err := tx.QueryRowContext(ctx, "SELECT retired,body_sha256,CAST(organization_id AS CHAR),aggregate_key,resource_id,kind,receipt_id FROM ai_messaging_operations WHERE command_id=? FOR UPDATE", r.CommandId).Scan(&retired, &hash, &org, &aggregate, &resource, &kind, &receiptID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", app.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if retired || !hash.Valid || hash.String != r.CommandBodySha256 || aggregate != e.AggregateKey || (receiptID.Valid && receiptID.String != e.MessageId) || len(r.Code) > 128 || r.GrpcStatusCode < 0 || r.GrpcStatusCode > 16 {
		return "", app.ErrConflict
	}
	decision := "held"
	switch r.Decision {
	case pb.MessagingDecision_ACCEPTED:
		decision = "accepted"
		if pb.MessagingKind(kind) == pb.MessagingKind_EVALUATION_START || pb.MessagingKind(kind) == pb.MessagingKind_EVALUATION_CANCEL {
			v := r.GetEvaluationReceipt()
			if v == nil || v.RunId != resource || v.Version < 1 {
				return "", app.ErrConflict
			}
		} else {
			v := r.GetWorkflowReceipt()
			if v == nil || v.Version < 1 || v.Status == "" {
				return "", app.ErrConflict
			}
			sessionID, idErr := uuid.Parse(v.SessionId)
			if idErr != nil || sessionID.String() != v.SessionId {
				return "", app.ErrConflict
			}
			if pb.MessagingKind(kind) != pb.MessagingKind_START && v.SessionId != resource {
				return "", app.ErrConflict
			}
			if pb.MessagingKind(kind) == pb.MessagingKind_START || pb.MessagingKind(kind) == pb.MessagingKind_CHANGE {
				var session sql.NullString
				if err = tx.QueryRowContext(ctx, "SELECT session_id FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", aggregate).Scan(&session); err != nil {
					return "", err
				}
				if session.Valid && session.String != v.SessionId {
					return "", app.ErrConflict
				}
				// A late receipt only binds identity. It must not overwrite projection,
				// version, status or result-received timestamp.
				if _, err = tx.ExecContext(ctx, "UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", v.SessionId, aggregate); err != nil {
					return "", err
				}
			}
		}
	case pb.MessagingDecision_REJECTED:
		decision = "rejected"
	}
	_, err = tx.ExecContext(ctx, "UPDATE ai_messaging_operations SET decision=?,code=?,receipt_id=?,receipt=?,decided_at=UTC_TIMESTAMP(6) WHERE command_id=?", decision, r.Code, e.MessageId, raw, r.CommandId)
	if err != nil {
		return "", err
	}
	_, err = s.transactionCommandReceipt(ctx, tx, r.CommandId, org, resource, aggregate, pb.MessagingKind(kind), hash.String)

	return org, err
}

// Quarantine persists only a sanitized classification and exact raw bytes/hash.
// It never inserts a trusted Inbox identity or affects a model/task.
func (s *MessagingStore) Quarantine(ctx context.Context, tx *sql.Tx, wire []byte, code string) error {
	if tx == nil || (code != "authentication_failed" && code != "identity_conflict" && code != "technical_budget_exhausted" && code != "failed_delivery") {
		return app.ErrInvalid
	}
	hash := messagingHash(wire)
	if len(wire) > 262144 {
		wire = []byte{}
	} // retain full wire hash without oversized bytes
	_, err := tx.ExecContext(ctx, `INSERT INTO ai_messaging_quarantine(wire_sha256,wire,code,attempts,first_seen_at,last_seen_at) VALUES(?,?,?,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE attempts=attempts+1,last_seen_at=UTC_TIMESTAMP(6)`, hash, wire, code)
	return err
}
