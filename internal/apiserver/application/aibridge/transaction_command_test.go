package aibridge

import (
	"bytes"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func commandFixture(t *testing.T) (TransactionCommand, *PreparedMessaging) {
	t.Helper()
	id := "b1941896-e7df-4b9d-9417-b80bd05276b9"
	body := &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: &pb.StartCommand{RequestId: id, Actor: &pb.Actor{OrgId: "7", SubjectId: "operator:9"}, Goal: "first goal"}}}
	c, err := NewTransactionCommand(pb.MessagingKind_START, id, id, OperationScope{"7", "operator:9", id}, body)
	require.NoError(t, err)
	e, raw, err := PrepareMessaging(pb.MessagingKind_START, id, id, "", body)
	require.NoError(t, err)
	e.OriginalOccurredAt = time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	// Opaque unit wire proves ownership/reuse only; existing JOSE tests cover
	// actual authentication. This fixture makes no signing/security claim.
	wire, err := legacy.Encode(legacy.Envelope{UUID: id, Metadata: messagingMetadata(), Payload: []byte("opaque-first-protected-header")}, legacy.Revision2)
	require.NoError(t, err)
	return c, &PreparedMessaging{Envelope: e, Body: raw, Wire: wire, Topic: CommandsTopic}
}

func TestTransactionCommandWinnerKeepsOriginalWireAcrossChangedClocksAndSequences(t *testing.T) {
	c, m := commandFixture(t)
	first, err := c.Freeze(m, 1, messagingTestRoutes(t))
	require.NoError(t, err)
	original := bytes.Clone(m.Wire)
	m.Envelope.OriginalOccurredAt = "2026-10-21T03:10:00Z"
	m.Wire, err = legacy.Encode(legacy.Envelope{UUID: c.ID(), Metadata: messagingMetadata(), Payload: []byte("another-local-candidate")}, legacy.Revision2)
	require.NoError(t, err)
	candidate, err := c.Freeze(m, 2, messagingTestRoutes(t))
	require.NoError(t, err)
	require.Equal(t, first.Input().InputHash, candidate.Input().InputHash)
	require.Equal(t, first.Input().IdempotencyKey, candidate.Input().IdempotencyKey)
	require.NotEqual(t, first.Digest(), candidate.Digest())
	raw, err := transaction.EncodePlan(first)
	require.NoError(t, err)
	restored, err := transaction.DecodePlan(raw)
	require.NoError(t, err)
	request, archive, err := ReadTransactionCommand(restored, messagingTestRoutes(t))
	require.NoError(t, err)
	require.Equal(t, c.Scope(), request.Scope())
	require.Equal(t, c.InputHash(), request.InputHash())
	require.EqualValues(t, 1, archive.Sequence)
	require.Equal(t, original, archive.Wire)
	require.Equal(t, "2026-10-21T03:00:00Z", archive.Envelope.OriginalOccurredAt)
	require.True(t, archive.Ordered)
	require.True(t, archive.RequiresReceipt)
	// None of the returned mutable buffers/protobufs is a plan capability.
	archive.Wire[0] ^= 1
	archive.Envelope.MessageId = "mutated"
	_, again, err := ReadTransactionCommand(first, messagingTestRoutes(t))
	require.NoError(t, err)
	require.Equal(t, original, again.Wire)
}

func TestTransactionCommandOwnsBodyAndBindsOriginalScopeBeforeSeal(t *testing.T) {
	c, m := commandFixture(t)
	body, err := c.Body()
	require.NoError(t, err)
	body.GetStart().Goal = "mutated by caller"
	again, err := c.Body()
	require.NoError(t, err)
	require.Equal(t, "first goal", again.GetStart().Goal)
	for _, scope := range []OperationScope{{"8", "operator:9", c.ID()}, {"7", "other", c.ID()}, {"7", "operator:9", "a1941896-e7df-4b9d-9417-b80bd05276b9"}} {
		_, err := NewTransactionCommand(pb.MessagingKind_START, c.ID(), c.Aggregate(), scope, again)
		require.ErrorIs(t, err, ErrMessagingContract)
	}
	changed, err := NewTransactionCommand(pb.MessagingKind_START, c.ID(), c.Aggregate(), c.Scope(), body)
	require.NoError(t, err)
	require.NotEqual(t, c.InputHash(), changed.InputHash())
	_, err = changed.Freeze(m, 1, messagingTestRoutes(t))
	require.ErrorIs(t, err, ErrMessagingContract)
	for _, seq := range []uint64{0, ^uint64(0)} {
		_, err = c.Freeze(m, seq, messagingTestRoutes(t))
		require.ErrorIs(t, err, ErrMessagingContract)
	}
}

func TestTransactionCancelKeepsExplicitDiscardPresenceAndValue(t *testing.T) {
	id := "b1941896-e7df-4b9d-9417-b80bd05276b9"
	run := "c1941896-e7df-4b9d-9417-b80bd05276b9"
	scope := OperationScope{OrganizationID: "7", SubjectID: "9", ResourceID: run}
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationCancel{EvaluationCancel: &pb.EvaluationCancelCommand{Scope: &pb.EvaluationQuery{OrganizationId: 7, OperatorUserId: 9, RunId: run}, ExpectedVersion: 1, Confirm: true}}}
	_, err := NewTransactionCommand(pb.MessagingKind_EVALUATION_CANCEL, id, run, scope, body)
	require.ErrorIs(t, err, ErrMessagingContract)
	discard := false
	body.GetEvaluationCancel().Discard = &discard
	keep, err := NewTransactionCommand(pb.MessagingKind_EVALUATION_CANCEL, id, run, scope, body)
	require.NoError(t, err)
	stored, err := keep.Body()
	require.NoError(t, err)
	require.NotNil(t, stored.GetEvaluationCancel().Discard)
	require.False(t, *stored.GetEvaluationCancel().Discard)
	discard = true
	drop, err := NewTransactionCommand(pb.MessagingKind_EVALUATION_CANCEL, id, run, scope, body)
	require.NoError(t, err)
	require.NotEqual(t, keep.InputHash(), drop.InputHash())
	stored, err = keep.Body()
	require.NoError(t, err)
	require.False(t, *stored.GetEvaluationCancel().Discard, "the first immutable intent owns its explicit false")
}

func TestTransactionCommandRejectsAlteredOwnerRouteHashScopeAndArchiveFlags(t *testing.T) {
	for _, name := range []string{"owner", "route", "hash", "scope", "key", "operation", "branch-id", "unordered", "receipt-free", "correlation"} {
		t.Run(name, func(t *testing.T) {
			c, m := commandFixture(t)
			first, err := c.Freeze(m, 1, messagingTestRoutes(t))
			require.NoError(t, err)
			in := first.Input()
			switch name {
			case "owner":
				in.Owner = "ai.mysql"
			case "route":
				in.Branches[0].PhysicalTopic = "another.physical.topic"
			case "hash":
				in.InputHash = [32]byte{9}
			case "scope":
				in.Scope = "org:8"
				input := in.Branches[0].Message.Input()
				input.Scope = in.Scope
				in.Branches[0].Message, err = message.New(input)
				require.NoError(t, err)
			case "key":
				in.IdempotencyKey = "different/request"
			case "operation":
				in.OperationID = "a1941896-e7df-4b9d-9417-b80bd05276b9"
			case "branch-id":
				in.Branches[0].ID = "different-branch"
			default:
				_, a, err := ReadTransactionCommand(first, messagingTestRoutes(t))
				require.NoError(t, err)
				if name == "unordered" {
					a.Ordered = false
				}
				if name == "receipt-free" {
					a.RequiresReceipt = false
				}
				if name == "correlation" {
					a.Envelope.CorrelationCommandId = "another-command"
					a.Header, err = proto.MarshalOptions{Deterministic: true}.Marshal(a.Envelope)
					require.NoError(t, err)
					m.Envelope.CorrelationCommandId = "another-command"
					_, err = c.Freeze(m, 1, messagingTestRoutes(t))
					require.ErrorIs(t, err, ErrMessagingContract)
				}

				in.Branches[0].Message, err = a.Message()
				require.NoError(t, err)
			}
			altered, err := transaction.NewPlan(in)
			require.NoError(t, err)
			_, _, err = ReadTransactionCommand(altered, messagingTestRoutes(t))
			require.Error(t, err)
		})
	}
}
