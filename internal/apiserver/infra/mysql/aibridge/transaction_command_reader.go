package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var _ app.PlannedStartReader = (*MessagingCommandStore)(nil)

// ReadPlannedStart is an internal application port, not a public body read. It
// keeps uncommitted planning separate from the original business request table.
func (s *MessagingCommandStore) ReadPlannedStart(ctx context.Context, actor app.Actor, id string) (*app.Start, bool, error) {
	if s == nil {
		return nil, true, app.ErrManagementUnavailable
	}
	if s.Transactions == nil {
		return nil, false, nil
	}
	return s.Transactions.plannedStart(ctx, actor, id)
}

func (s *TransactionCommands) plannedStart(ctx context.Context, actor app.Actor, id string) (*app.Start, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	q, found, err := s.archive.commandRequest(ctx, actor.OrgID, id)
	if err != nil || !found {
		return nil, found, err
	}
	winner, err := s.archive.store.Lookup(ctx, q)
	if err != nil {
		return nil, true, err
	}
	c, a, err := app.ReadTransactionCommand(winner.Execution.Plan)
	if err != nil {
		return nil, true, err
	}
	if c.ID() != id || c.Aggregate() != id || c.Kind() != pb.MessagingKind_START || c.Scope().OrganizationID != actor.OrgID || c.Scope().SubjectID != actor.SubjectID {
		return nil, true, app.ErrNotFound
	}
	body, err := app.ParseMessagingBody(a.Envelope, a.Body)
	if err != nil {
		return nil, true, err
	}
	request := originalStart(body.GetStart())
	return &request, true, nil // this says nothing about business root commitment
}

// operation distinguishes absence from a damaged/uncommitted new identity.
// Only true absence permits the caller to consult the historical Outbox reader.
// No wire, token, private plan or body is returned through this public boundary.
func (s *TransactionCommands) operation(ctx context.Context, scope app.OperationScope, requestID, id string) (result app.MessagingOperation, found bool, err error) {
	org, parseErr := strconv.ParseUint(scope.OrganizationID, 10, 64)
	if parseErr != nil || org == 0 || strconv.FormatUint(org, 10) != scope.OrganizationID || scope.SubjectID == "" || !validCommandID(id) || (requestID != "" && !validCommandID(requestID)) {
		return result, true, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	a, found, err := s.archive.ReadCommitted(ctx, id)
	if err != nil {
		return result, true, err
	}
	if !found {
		return s.pendingOperation(ctx, scope, requestID, id)
	}
	body, err := app.ParseMessagingBody(a.Envelope, a.Body)
	if err != nil {
		return result, true, err
	}
	actual, err := app.CommandScope(a.Envelope, body)
	if err != nil || a.Envelope.CorrelationCommandId != "" || !a.Ordered || !a.RequiresReceipt {
		return result, true, app.ErrMessagingContract
	}
	if !operationVisible(scope, requestID, actual, a.Envelope) {
		return result, true, app.ErrNotFound
	}
	result = app.MessagingOperation{OperationID: id, CommandID: id, ResourceID: actual.ResourceID, Status: "submitted", TransportStatus: "awaiting_receipt"}
	var kind int32
	var hash, aggregate, resource string
	var sequence uint64
	var receiptID sql.NullString
	var raw []byte
	var decided sql.NullTime
	err = s.archive.db.QueryRowContext(ctx, `SELECT kind,body_sha256,resource_id,aggregate_key,aggregate_sequence,decision,code,receipt_id,receipt,decided_at FROM ai_messaging_operations WHERE command_id=? AND organization_id=? AND subject_id=? AND retired=FALSE`, id, scope.OrganizationID, scope.SubjectID).Scan(&kind, &hash, &resource, &aggregate, &sequence, &result.Decision, &result.Code, &receiptID, &raw, &decided)
	if errors.Is(err, sql.ErrNoRows) {
		return result, true, app.ErrMessagingContract
	}
	if err != nil {
		return result, true, err
	}
	if kind != int32(a.Envelope.Kind) || hash != a.Envelope.BodySha256 || resource != actual.ResourceID || aggregate != a.Envelope.AggregateKey || sequence != a.Sequence {
		return result, true, app.ErrMessagingContract
	}
	if result.Decision == "" {
		if receiptID.Valid || len(raw) != 0 || decided.Valid || result.Code != "" {
			return result, true, app.ErrMessagingContract
		}
		return result, true, nil
	}
	r := new(pb.MessagingBody)
	if !receiptID.Valid || !validCommandID(receiptID.String) || !decided.Valid || decided.Time.IsZero() || proto.Unmarshal(raw, r) != nil || r.GetCommandReceipt() == nil {
		return result, true, app.ErrMessagingContract
	}
	receipt := r.GetCommandReceipt()
	expected := map[string]pb.MessagingDecision{"accepted": pb.MessagingDecision_ACCEPTED, "rejected": pb.MessagingDecision_REJECTED, "held": pb.MessagingDecision_HELD}
	decision, ok := expected[result.Decision]
	if !ok || receipt.Decision != decision || receipt.CommandId != id || receipt.CommandBodySha256 != hash || receipt.Code != result.Code || len(receipt.Code) > 128 || receipt.GrpcStatusCode < 0 || receipt.GrpcStatusCode > 16 {
		return result, true, app.ErrMessagingContract
	}
	if decision == pb.MessagingDecision_ACCEPTED {
		if err := validateAcceptedCommandReceipt(a.Envelope.Kind, resource, receipt); err != nil {
			return result, true, err
		}
	}
	result.Status, result.TransportStatus = result.Decision, "confirmed"
	if result.Decision == "held" {
		result.TransportStatus = "held"
	}
	result.Receipt, err = protojson.MarshalOptions{UseProtoNames: true}.Marshal(receipt)
	return result, true, err
}

func operationVisible(want app.OperationScope, requestID string, actual app.OperationScope, e *pb.MessagingEnvelope) bool {
	return actual.OrganizationID == want.OrganizationID && actual.SubjectID == want.SubjectID && (requestID == "" || (e.AggregateKey == requestID && (e.Kind == pb.MessagingKind_START || e.Kind == pb.MessagingKind_CHANGE)))
}

func (s *TransactionCommands) pendingOperation(ctx context.Context, scope app.OperationScope, requestID, id string) (result app.MessagingOperation, found bool, err error) {
	q, found, err := s.archive.commandRequest(ctx, scope.OrganizationID, id)
	if err != nil || !found {
		return result, found, err
	}
	winner, err := s.archive.store.Lookup(ctx, q)
	if err != nil {
		return result, true, err
	}
	c, a, err := app.ReadTransactionCommand(winner.Execution.Plan)
	if err != nil {
		return result, true, err
	}
	if !operationVisible(scope, requestID, c.Scope(), a.Envelope) {
		return result, true, app.ErrNotFound
	}
	result = app.MessagingOperation{OperationID: id, CommandID: id, ResourceID: c.Scope().ResourceID, Status: "pending", TransportStatus: "transaction_pending"}
	switch s.archive.confirmCommand(ctx, winner.Execution) {
	case transaction.LocalCommitted:
		// The archive was absent at the first read but may have committed since.
		// Request a fresh query instead of fabricating business success.
		return result, true, ErrCommandUnresolved
	case transaction.LocalAborted:
		result.Status, result.TransportStatus = "aborted", "transaction_aborted"
	}
	return result, true, nil
}

// Presence alone blocks historical payload fallback. This also covers a
// Prepared/Aborted plan whose business archive has never been committed.
func (s *TransactionArchive) commandRequest(ctx context.Context, organization, id string) (q transaction.Request, found bool, err error) {
	org, parseErr := strconv.ParseUint(organization, 10, 64)
	if parseErr != nil || org == 0 || strconv.FormatUint(org, 10) != organization || !validCommandID(id) {
		return q, true, app.ErrInvalid
	}
	q = transaction.Request{Owner: "qs.mysql", Scope: "org:" + organization, Key: "ai-command/" + id}
	key := q.KeyDigest()
	var owner, storedScope, storedKey string
	var hash []byte
	err = s.db.QueryRowContext(ctx, `SELECT owner,scope,idempotency_key,input_hash FROM rm_tx_request WHERE request_key=?`, key[:]).Scan(&owner, &storedScope, &storedKey, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return q, false, nil
	}
	if err != nil {
		return q, true, err
	}
	if owner != q.Owner || storedScope != q.Scope || storedKey != q.Key || len(hash) != len(q.InputHash) {
		return q, true, app.ErrMessagingContract
	}
	copy(q.InputHash[:], hash)
	return q, true, nil
}
