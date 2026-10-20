package aibridge

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// A previous durable HELD receipt still blocks its aggregate, as in the old
// ordered Relay. This is not permission to repeat a model or mint another ID.
var ErrCommandPredecessorPending = errors.New("previous AI command admission receipt is unresolved")

type commandOrderReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireCommandOrderIndex(ctx context.Context, q commandOrderReader) error {
	rows, err := q.QueryContext(ctx, `SELECT SEQ_IN_INDEX,COLUMN_NAME,SUB_PART,NON_UNIQUE,INDEX_TYPE,IS_VISIBLE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='ai_messaging_operations' AND INDEX_NAME='uk_ai_messaging_command_sequence' ORDER BY SEQ_IN_INDEX`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	expected := []string{"aggregate_key", "aggregate_sequence"}
	n := 0
	for rows.Next() {
		var ordinal, nonUnique int
		var column, kind, visible string
		var prefix sql.NullInt64
		if err = rows.Scan(&ordinal, &column, &prefix, &nonUnique, &kind, &visible); err != nil {
			return err
		}
		if n >= len(expected) || ordinal != n+1 || column != expected[n] || prefix.Valid || nonUnique != 0 || !strings.EqualFold(kind, "BTREE") || visible != "YES" {
			return app.ErrMessagingContract
		}
		n++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if n != len(expected) {
		return app.ErrMessagingContract
	}
	return nil
}

// The planning read never allocates a sequence or locks across Broker I/O.
// The original business root repeats it under the aggregate row lock and CAS.
func readCommandSequence(ctx context.Context, q commandOrderReader, c app.TransactionCommand, lock bool) (uint64, error) {
	if c.ID() == "" || c.Aggregate() == "" {
		return 0, app.ErrMessagingContract
	}
	var next uint64
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	err := q.QueryRowContext(ctx, "SELECT next_sequence FROM ai_messaging_aggregates WHERE aggregate_key=?"+suffix, c.Aggregate()).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) && !lock {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	if next == 0 || next == ^uint64(0) {
		return 0, app.ErrMessagingContract
	}
	if next > 1 {
		if err := requirePredecessorReceipt(ctx, q, c, next-1, suffix); err != nil {
			return 0, err
		}
	}
	return next, nil
}

func requirePredecessorReceipt(ctx context.Context, q commandOrderReader, c app.TransactionCommand, sequence uint64, lock string) error {
	var id, hash, resource, decision string
	var kind int32
	var receiptID sql.NullString
	var raw []byte
	var decided sql.NullTime
	err := q.QueryRowContext(ctx, `SELECT command_id,kind,body_sha256,resource_id,decision,receipt_id,receipt,decided_at FROM ai_messaging_operations WHERE organization_id=? AND aggregate_key=? AND aggregate_sequence=? AND retired=FALSE`+lock, c.Scope().OrganizationID, c.Aggregate(), sequence).Scan(&id, &kind, &hash, &resource, &decision, &receiptID, &raw, &decided)
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrMessagingContract
	} // missing/retired history is not completion
	if err != nil {
		return err
	}
	if !validCommandID(id) || !validCommandID(resource) || kind < int32(pb.MessagingKind_START) || kind > int32(pb.MessagingKind_EVALUATION_CANCEL) || (pb.MessagingKind(kind) == pb.MessagingKind_START && id != c.Aggregate()) {
		return app.ErrMessagingContract
	}
	if decision == "" {
		if receiptID.Valid || len(raw) > 0 || decided.Valid {
			return app.ErrMessagingContract
		}
		return ErrCommandPredecessorPending
	}
	if !receiptID.Valid || !validCommandID(receiptID.String) || len(raw) == 0 || !decided.Valid || decided.Time.IsZero() {
		return app.ErrMessagingContract
	}
	body := new(pb.MessagingBody)
	if proto.Unmarshal(raw, body) != nil || body.GetCommandReceipt() == nil {
		return app.ErrMessagingContract
	}
	r := body.GetCommandReceipt()
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != hash || r.CommandId != id || r.CommandBodySha256 != hash || len(r.Code) > 128 || r.GrpcStatusCode < 0 || r.GrpcStatusCode > 16 {
		return app.ErrMessagingContract
	}
	switch decision {
	case "held":
		if r.Decision != pb.MessagingDecision_HELD {
			return app.ErrMessagingContract
		}
		return ErrCommandPredecessorPending
	case "rejected":
		if r.Decision != pb.MessagingDecision_REJECTED {
			return app.ErrMessagingContract
		}
	case "accepted":
		if r.Decision != pb.MessagingDecision_ACCEPTED {
			return app.ErrMessagingContract
		}
		return validateAcceptedCommandReceipt(pb.MessagingKind(kind), resource, r)
	default:
		return app.ErrMessagingContract
	}
	return nil
}

func validateAcceptedCommandReceipt(kind pb.MessagingKind, resource string, r *pb.MessagingCommandReceipt) error {
	switch kind {
	case pb.MessagingKind_EVALUATION_START, pb.MessagingKind_EVALUATION_CANCEL:
		v := r.GetEvaluationReceipt()
		if v == nil || v.RunId != resource || v.Version < 1 {
			return app.ErrMessagingContract
		}
	case pb.MessagingKind_START, pb.MessagingKind_CHANGE, pb.MessagingKind_PARTICIPANT_RETRY:
		v := r.GetWorkflowReceipt()
		if v == nil || !validCommandID(v.SessionId) || v.Version < 1 || v.Status == "" || (kind != pb.MessagingKind_START && v.SessionId != resource) {
			return app.ErrMessagingContract
		}
	default:
		return app.ErrMessagingContract
	}
	return nil
}

func validCommandID(id string) bool { u, err := uuid.Parse(id); return err == nil && u.String() == id }

func guardCommandSequence(ctx context.Context, tx *sql.Tx, c app.TransactionCommand, sequence uint64) error {
	if tx == nil || sequence == 0 || sequence == ^uint64(0) {
		return app.ErrMessagingContract
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO ai_messaging_aggregates(aggregate_key,next_sequence) VALUES(?,1) ON DUPLICATE KEY UPDATE aggregate_key=aggregate_key", c.Aggregate()); err != nil {
		return err
	}
	actual, err := readCommandSequence(ctx, tx, c, true)
	if err != nil {
		return err
	}
	if actual != sequence {
		return app.ErrConflict
	}
	gate, err := (MessagingAdmission{}).Read(ctx, tx)
	if err != nil {
		return err
	}
	if gate.Closed {
		return app.ErrRuntimeAdmissionClosed
	}
	return nil
}
