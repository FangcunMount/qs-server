package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

// FailureAttempts is keyed by authenticated logical identity, not broker attempts.
// The caller owns this active transaction; mismatched contents never inherit it.
func (s *MessagingStore) FailureAttempts(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope) (uint64, error) {
	if tx == nil || e == nil || app.ValidateMessagingHeader(e, app.EventsTopic) != nil {
		return 0, app.ErrMessagingContract
	}
	var hash, aggregate string
	var kind int32
	var attempts uint64
	err := tx.QueryRowContext(ctx, "SELECT body_sha256,kind,aggregate_key,attempts FROM ai_messaging_failures WHERE producer=? AND message_id=? FOR UPDATE", e.Producer, e.MessageId).Scan(&hash, &kind, &aggregate, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if hash != e.BodySha256 || kind != int32(e.Kind) || aggregate != e.AggregateKey {
		return 0, app.ErrConflict
	}
	return attempts, nil
}

func (s *MessagingStore) RecordTechnicalFailure(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope, wire []byte) (uint64, error) {
	if _, err := s.FailureAttempts(ctx, tx, e); err != nil {
		return 0, err
	}
	if len(wire) == 0 || len(wire) > 262144 {
		return 0, app.ErrMessagingContract
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO ai_messaging_failures(producer,message_id,body_sha256,kind,aggregate_key,wire,attempts,first_seen_at,last_seen_at)
 VALUES(?,?,?,?,?,?,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE attempts=LEAST(8,attempts+1),last_seen_at=UTC_TIMESTAMP(6)`, e.Producer, e.MessageId, e.BodySha256, e.Kind, e.AggregateKey, wire)
	if err != nil {
		return 0, err
	}
	return s.FailureAttempts(ctx, tx, e)
}

// ReceiveHeldEvent archives a known technical hold with a final HELD ACK.
// It never applies a projection or decides a command. Stored duplicates reuse
// their original ACK, even if a failed commit acknowledgement raised the budget.
func (s *MessagingStore) ReceiveHeldEvent(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope, raw, wire []byte, seal func(string, string) (*app.PreparedMessaging, error)) error {
	attempts, err := s.FailureAttempts(ctx, tx, e)
	if err != nil {
		return err
	}
	if attempts < 8 {
		return app.ErrInvalid
	}
	return s.receiveEvent(ctx, tx, e, raw, wire, true, seal)
}

func (s *MessagingStore) heldEventOrganization(ctx context.Context, tx *sql.Tx, e *pb.MessagingEnvelope, b *pb.MessagingBody) (string, error) {
	var org string
	switch e.Kind {
	case pb.MessagingKind_COMMAND_RECEIPT:
		receipt := b.GetCommandReceipt()
		var hash, aggregate string
		err := tx.QueryRowContext(ctx, "SELECT CAST(organization_id AS CHAR),body_sha256,aggregate_key FROM ai_messaging_operations WHERE command_id=? FOR UPDATE", receipt.CommandId).Scan(&org, &hash, &aggregate)
		if err != nil {
			return "", err
		}
		if hash != receipt.CommandBodySha256 || aggregate != e.AggregateKey {
			return "", app.ErrConflict
		}
	case pb.MessagingKind_INTERPRETATION_STATE:
		event := b.GetInterpretationState()
		var subject, testee string
		err := tx.QueryRowContext(ctx, "SELECT CAST(organization_id AS CHAR),subject_id,CAST(testee_id AS CHAR) FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", event.RequestId).Scan(&org, &subject, &testee)
		if err != nil {
			return "", err
		}
		if e.AggregateKey != event.RequestId || org != event.GetActor().GetOrgId() || subject != event.GetActor().GetSubjectId() || testee != event.TesteeId {
			return "", app.ErrConflict
		}
	case pb.MessagingKind_EVALUATION_STATE:
		event := b.GetEvaluationState()
		org = event.OrganizationId
		if e.AggregateKey != event.RunId {
			return "", app.ErrConflict
		}
	default:
		return "", app.ErrMessagingContract
	}
	id, err := strconv.ParseUint(org, 10, 64)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != org {
		return "", app.ErrConflict
	}
	return org, nil
}
