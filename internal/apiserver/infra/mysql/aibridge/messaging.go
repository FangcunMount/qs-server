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
	durable "github.com/FangcunMount/reliable-messaging/delivery/mysql"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// MessagingStore borrows host transactions. Constructors perform no I/O and
// methods never commit, schedule delivery or close a database connection.
type MessagingStore struct{ Outbox *durable.Outbox }

func NewMessagingStore() *MessagingStore {
	outbox, _ := durable.New("ai_messaging_outbox") // fixed host-owned table
	return &MessagingStore{Outbox: outbox}
}

func messagingHash(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func (s *MessagingStore) put(ctx context.Context, tx *sql.Tx, m *app.PreparedMessaging, org string, sequence uint64, ordered, receipt bool) error {
	if tx == nil || m == nil || m.Envelope == nil || len(m.Wire) == 0 || len(m.Wire) > 262144 || sequence == 0 {
		return app.ErrMessagingContract
	}
	e := m.Envelope
	if _, err := app.ParseMessagingBody(e, m.Body); err != nil {
		return err
	}
	p, d, topic, err := app.MessagingRoute(e.Kind)
	orgID, orgErr := strconv.ParseUint(org, 10, 64)
	if err != nil || orgErr != nil || orgID == 0 || strconv.FormatUint(orgID, 10) != org || p != "qs-server" || d != "qs-ai" || topic != m.Topic || (e.GetPayloadReference() != nil && e.GetPayloadReference().OrganizationId != org) {
		return app.ErrMessagingContract
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_messaging_outbox
 (producer,destination,message_id,body_sha256,body,wire,wire_sha256,topic,kind,organization_id,aggregate_key,aggregate_sequence,ordered,requires_receipt,stage,available_at,created_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'staged',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE message_id=message_id`, p, d, e.MessageId, e.BodySha256, m.Body, m.Wire, messagingHash(m.Wire), topic, e.Kind, orgID, e.AggregateKey, sequence, ordered, receipt)
	if err != nil {
		return err
	}
	var body []byte
	var hash, aggregate string
	var kind int32
	var oldOrg, seq uint64
	var oldOrdered, oldReceipt bool
	err = tx.QueryRowContext(ctx, `SELECT body,body_sha256,kind,organization_id,aggregate_key,aggregate_sequence,ordered,requires_receipt FROM ai_messaging_outbox WHERE producer=? AND destination=? AND message_id=? FOR UPDATE`, p, d, e.MessageId).Scan(&body, &hash, &kind, &oldOrg, &aggregate, &seq, &oldOrdered, &oldReceipt)
	if err != nil {
		return err
	}
	if !bytes.Equal(body, m.Body) || hash != e.BodySha256 || kind != int32(e.Kind) || oldOrg != orgID || aggregate != e.AggregateKey || seq != sequence || oldOrdered != ordered || oldReceipt != receipt {
		return app.ErrConflict
	}
	return nil // duplicate retains first wire, hash and original created_at
}

// StageOperation allocates aggregate order in the original host transaction.
// seal is invoked only for a new identity; duplicate requests reuse first bytes.
// Original business authorization/validation precedes this method in the host.
func (s *MessagingStore) StageOperation(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope, raw []byte, scope app.OperationScope, seal func() (*app.PreparedMessaging, error)) (uint64, error) {
	if tx == nil || e == nil || seal == nil || e.Producer != "qs-server" || e.Kind < pb.MessagingKind_START || e.Kind > pb.MessagingKind_EVALUATION_CANCEL || scope.SubjectID == "" || len(scope.SubjectID) > 128 {
		return 0, app.ErrMessagingContract
	}
	body, err := app.ParseMessagingBody(e, raw)
	if err != nil {
		return 0, err
	}
	if !operationScopeMatches(e, body, scope) {
		return 0, app.ErrConflict
	}
	// The aggregate row serializes sequence assignment and same-aggregate retries.
	if _, err = tx.ExecContext(ctx, "INSERT INTO ai_messaging_aggregates(aggregate_key,next_sequence) VALUES(?,1) ON DUPLICATE KEY UPDATE aggregate_key=aggregate_key", e.AggregateKey); err != nil {
		return 0, err
	}
	var sequence uint64
	if err = tx.QueryRowContext(ctx, "SELECT next_sequence FROM ai_messaging_aggregates WHERE aggregate_key=? FOR UPDATE", e.AggregateKey).Scan(&sequence); err != nil {
		return 0, err
	}
	var oldHash sql.NullString
	var org, subject, resource, aggregate string
	var retired bool
	var kind int32
	var oldSeq sql.Null[uint64]
	err = tx.QueryRowContext(ctx, "SELECT retired,body_sha256,CAST(organization_id AS CHAR),subject_id,resource_id,aggregate_key,kind,aggregate_sequence FROM ai_messaging_operations WHERE command_id=? FOR UPDATE", e.MessageId).Scan(&retired, &oldHash, &org, &subject, &resource, &aggregate, &kind, &oldSeq)
	if err == nil {
		if retired || !oldHash.Valid || !oldSeq.Valid || oldSeq.V == 0 || oldHash.String != e.BodySha256 || org != scope.OrganizationID || subject != scope.SubjectID || resource != scope.ResourceID || aggregate != e.AggregateKey || kind != int32(e.Kind) {
			return 0, app.ErrConflict
		}
		return oldSeq.V, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	message, err := seal()
	if err != nil {
		return 0, err
	}
	if message == nil || message.Envelope == nil || !bytes.Equal(message.Body, raw) || message.Envelope.MessageId != e.MessageId || message.Envelope.Kind != e.Kind || message.Envelope.AggregateKey != e.AggregateKey || message.Envelope.CorrelationCommandId != e.CorrelationCommandId || message.Envelope.OriginalOccurredAt != e.OriginalOccurredAt {
		return 0, app.ErrMessagingContract
	}
	if err = s.put(ctx, tx, message, scope.OrganizationID, sequence, true, true); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_messaging_operations(command_id,kind,body_sha256,organization_id,subject_id,resource_id,aggregate_key,aggregate_sequence,created_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6))`, e.MessageId, e.Kind, e.BodySha256, scope.OrganizationID, scope.SubjectID, scope.ResourceID, e.AggregateKey, sequence)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE ai_messaging_aggregates SET next_sequence=next_sequence+1 WHERE aggregate_key=?", e.AggregateKey)
	return sequence, err
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
// missing ID (which would introduce cross-aggregate gap-lock deadlocks). Racing
// open-gate submissions are still serialized and validated by StageOperation.
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

// Operation returns only an operation owned by the trusted organization/subject.
// Wider admin/audit access must use the host's separate authorization boundary.
func (s *MessagingStore) Operation(ctx context.Context, tx *sql.Tx, scope app.OperationScope, id string) (app.MessagingOperation, error) {
	result := app.MessagingOperation{OperationID: id, CommandID: id, Status: "submitted"}
	org, orgErr := strconv.ParseUint(scope.OrganizationID, 10, 64)
	commandID, idErr := uuid.Parse(id)
	if tx == nil || orgErr != nil || org == 0 || strconv.FormatUint(org, 10) != scope.OrganizationID || scope.SubjectID == "" || idErr != nil || commandID.String() != id {
		return result, app.ErrInvalid
	}
	var receipt []byte
	err := tx.QueryRowContext(ctx, `SELECT o.resource_id,o.decision,o.code,o.receipt,b.stage FROM ai_messaging_operations o JOIN ai_messaging_outbox b ON b.producer='qs-server' AND b.destination='qs-ai' AND b.message_id=o.command_id WHERE o.retired=FALSE AND o.command_id=? AND o.organization_id=? AND o.subject_id=?`, id, scope.OrganizationID, scope.SubjectID).Scan(&result.ResourceID, &result.Decision, &result.Code, &receipt, &result.TransportStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return result, app.ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if result.Decision != "" {
		result.Status = result.Decision
	}
	if len(receipt) != 0 {
		original := new(pb.MessagingBody)
		if err := proto.Unmarshal(receipt, original); err != nil || original.GetCommandReceipt() == nil {
			if err == nil {
				err = app.ErrMessagingContract
			}
			return result, err
		}
		result.Receipt, err = protojson.MarshalOptions{UseProtoNames: true}.Marshal(original.GetCommandReceipt())
	}
	return result, err
}

// ReceiveEvent is called only after signature, destination, actual topic and
// original bytes have been authenticated by the transport. It commits nothing.
// sealAck is called once, after the business write; duplicate delivery rearms the
// exact persisted final ACK without reapplying business effects.
func (s *MessagingStore) ReceiveEvent(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope, raw, wire []byte, sealAck func(string, string) (*app.PreparedMessaging, error)) error {
	return s.receiveEvent(ctx, tx, e, raw, wire, false, sealAck)
}

func (s *MessagingStore) receiveEvent(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope, raw, wire []byte, held bool, sealAck func(string, string) (*app.PreparedMessaging, error)) error {
	if tx == nil || e == nil || sealAck == nil || len(wire) == 0 || len(wire) > 262144 || e.Producer != "qs-ai" || e.Destination != "qs-server" || e.Kind < pb.MessagingKind_COMMAND_RECEIPT || e.Kind > pb.MessagingKind_EVALUATION_STATE {
		return app.ErrMessagingContract
	}
	body, err := app.ParseMessagingBody(e, raw)
	if err != nil {
		return err
	}
	// An uncommitted reservation cannot survive a crash. This insert serializes
	// duplicate physical deliveries before business CAS/projection checks.
	ackID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_messaging_inbox(producer,message_id,body_sha256,body,wire_sha256,kind,aggregate_key,ack_id,received_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE message_id=message_id`, e.Producer, e.MessageId, e.BodySha256, raw, messagingHash(wire), e.Kind, e.AggregateKey, ackID)
	if err != nil {
		return err
	}
	var oldHash, aggregate, storedAck string
	var kind int32
	err = tx.QueryRowContext(ctx, "SELECT body_sha256,kind,aggregate_key,ack_id FROM ai_messaging_inbox WHERE producer=? AND message_id=? FOR UPDATE", e.Producer, e.MessageId).Scan(&oldHash, &kind, &aggregate, &storedAck)
	if err != nil {
		return err
	}
	if oldHash != e.BodySha256 || kind != int32(e.Kind) || aggregate != e.AggregateKey {
		return app.ErrConflict
	}
	if storedAck != ackID {
		var hash string
		if err = tx.QueryRowContext(ctx, "SELECT body_sha256 FROM ai_messaging_outbox WHERE producer='qs-server' AND destination='qs-ai' AND message_id=?", storedAck).Scan(&hash); err != nil {
			return err
		}
		if err = s.Outbox.RearmAck(ctx, tx, durable.Identity{Producer: "qs-server", Destination: "qs-ai", MessageID: storedAck}, hash); err != nil {
			return err
		}
		return RecordMessagingObservation(ctx, tx, "duplicate_event")
	}
	var org string
	if held {
		org, err = s.heldEventOrganization(ctx, tx, e, body)
	} else {
		org, err = s.applyEvent(ctx, tx, e, body, raw)
	}
	if err != nil {
		return err
	}
	if e.GetPayloadReference() != nil && e.GetPayloadReference().OrganizationId != org {
		return app.ErrConflict
	}
	ack, err := sealAck(ackID, org)
	if err != nil {
		return err
	}
	if ack == nil || ack.Envelope == nil || ack.Envelope.MessageId != ackID || ack.Envelope.Kind != pb.MessagingKind_EVENT_ACKNOWLEDGEMENT || ack.Envelope.AggregateKey != e.AggregateKey {
		return app.ErrMessagingContract
	}
	ackBody, err := app.ParseMessagingBody(ack.Envelope, ack.Body)
	if err != nil {
		return err
	}
	a := ackBody.GetEventAcknowledgement()
	outcome := pb.MessagingEventAcknowledgement_STORED
	if held {
		outcome = pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
		if _, err = tx.ExecContext(ctx, "UPDATE ai_messaging_inbox SET outcome='held' WHERE producer=? AND message_id=?", e.Producer, e.MessageId); err != nil {
			return err
		}
	}
	if a.EventId != e.MessageId || a.EventBodySha256 != e.BodySha256 || a.EventKind != e.Kind || a.Outcome != outcome {
		return app.ErrConflict
	}
	return s.put(ctx, tx, ack, org, 1, false, false)
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
	id := durable.Identity{Producer: "qs-server", Destination: "qs-ai", MessageID: r.CommandId}
	if decision == "held" {
		err = s.Outbox.Hold(ctx, tx, id, hash.String, "receiver_technical_hold")
	} else {
		err = s.Outbox.Confirm(ctx, tx, id, hash.String)
	}
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

// Payload reads only the exact retained reference for the authenticated workload.
// It does not trust a caller-supplied organization or body hash as authorization.
func (s *MessagingStore) Payload(ctx context.Context, tx *sql.Tx, r *pb.MessagePayloadReference, workload string) ([]byte, error) {
	if tx == nil || r == nil || workload != "qs-ai" || r.Producer != "qs-server" || r.Destination != workload || r.BodyLength == 0 || r.BodyLength > app.MaxMessagingBody {
		return nil, app.ErrConflict
	}
	org, orgErr := strconv.ParseUint(r.OrganizationId, 10, 64)
	id, idErr := uuid.Parse(r.MessageId)
	if orgErr != nil || org == 0 || strconv.FormatUint(org, 10) != r.OrganizationId || idErr != nil || id.String() != r.MessageId {
		return nil, app.ErrConflict
	}
	var raw []byte
	err := tx.QueryRowContext(ctx, "SELECT body FROM ai_messaging_outbox WHERE producer=? AND destination=? AND message_id=? AND organization_id=? AND body_sha256=?", r.Producer, r.Destination, r.MessageId, org, r.BodySha256).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, app.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if uint64(len(raw)) != r.BodyLength || messagingHash(raw) != r.BodySha256 {
		return nil, app.ErrConflict
	}
	return raw, nil
}
