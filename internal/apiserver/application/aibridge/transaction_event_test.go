package aibridge

import (
	"crypto/sha256"
	"strings"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIncomingReceiptRequiresOriginalCommandCorrelationAndFreezesACK(t *testing.T) {
	id, command, run := uuid.NewString(), uuid.NewString(), uuid.NewString()
	body := &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: command, CommandBodySha256: strings.Repeat("a", 64), Decision: pb.MessagingDecision_ACCEPTED, OriginalReceipt: &pb.MessagingCommandReceipt_EvaluationReceipt{EvaluationReceipt: &pb.EvaluationState{RunId: run, Version: 1}}}}}
	for _, wrong := range []string{"", uuid.NewString()} {
		_, _, err := PrepareMessaging(pb.MessagingKind_COMMAND_RECEIPT, id, run, wrong, body)
		require.ErrorIs(t, err, ErrMessagingContract)
	}
	header, raw, err := PrepareMessaging(pb.MessagingKind_COMMAND_RECEIPT, id, run, command, body)
	require.NoError(t, err)
	header.OriginalOccurredAt = "2026-10-21T05:00:00+08:00"
	wire, err := legacy.Encode(legacy.Envelope{UUID: id, Metadata: messagingMetadata(), Payload: []byte("opaque-correlated-receipt-unit-wire")}, legacy.Revision2)
	require.NoError(t, err)
	incoming, err := FreezeIncomingEvent(header, raw, wire, "7")
	require.NoError(t, err)
	plan, err := incoming.FreezeACK(eventACKFixture(t, incoming, false), uuid.NewString())
	require.NoError(t, err)
	_, err = incoming.ReadACK(plan)
	require.NoError(t, err)
}

func incomingEventFixture(t *testing.T) FrozenIncomingEvent {
	t.Helper()
	id, aggregate := uuid.NewString(), uuid.NewString()
	body := &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: id, RequestId: aggregate, SessionId: uuid.NewString(), Actor: &pb.Actor{OrgId: "7", SubjectId: "9"}, TesteeId: "19", Version: 1, Status: "running"}}}
	header, raw, err := PrepareMessaging(pb.MessagingKind_INTERPRETATION_STATE, id, aggregate, "", body)
	require.NoError(t, err)
	header.OriginalOccurredAt = "2026-10-21T04:45:00+08:00"
	// Opaque unit wire proves frozen ownership/identity only. It does not claim
	// authentication; the real receiver verifies JOSE before constructing input.
	wire, err := legacy.Encode(legacy.Envelope{UUID: id, Metadata: messagingMetadata(), Payload: []byte("opaque-first-incoming-wire")}, legacy.Revision2)
	require.NoError(t, err)
	e, err := FreezeIncomingEvent(header, raw, wire, "7")
	require.NoError(t, err)
	return e
}

func eventACKFixture(t *testing.T, e FrozenIncomingEvent, held bool) *PreparedMessaging {
	t.Helper()
	in, _, _, _ := e.Input()
	outcome := pb.MessagingEventAcknowledgement_STORED
	if held {
		outcome = pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
	}
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: in.MessageId, EventBodySha256: in.BodySha256, EventKind: in.Kind, Outcome: outcome}}}
	id := uuid.NewString()
	header, raw, err := PrepareMessaging(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, id, in.AggregateKey, "", body)
	require.NoError(t, err)
	header.OriginalOccurredAt = "2026-10-21T04:45:01+08:00"
	wire, err := legacy.Encode(legacy.Envelope{UUID: id, Metadata: messagingMetadata(), Payload: []byte("opaque-first-ack-wire")}, legacy.Revision2)
	require.NoError(t, err)
	return &PreparedMessaging{Envelope: header, Body: raw, Wire: wire, Topic: AcksTopic}
}

func TestIncomingEventACKPlanKeepsFirstWireAndSeparateStableBusinessInput(t *testing.T) {
	e := incomingEventFixture(t)
	first := eventACKFixture(t, e, false)
	p, err := e.FreezeACK(first, uuid.NewString())
	require.NoError(t, err)
	other, err := e.FreezeACK(eventACKFixture(t, e, true), uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, transaction.RequestFor(p), transaction.RequestFor(other), "outcome is a host decision, not a changed incoming event")
	require.NotEqual(t, p.Digest(), other.Digest())
	raw, err := transaction.EncodePlan(p)
	require.NoError(t, err)
	restored, err := transaction.DecodePlan(raw)
	require.NoError(t, err)
	a, err := e.ReadACK(restored)
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(first.Wire), sha256.Sum256(a.Wire))
	require.Equal(t, first.Envelope.MessageId, a.Envelope.MessageId)
	require.False(t, a.Ordered)
	require.False(t, a.RequiresReceipt)
	// Changing callback-owned copies cannot change the durable input identity.
	header, body, wire, _ := e.Input()
	header.AggregateKey = "changed"
	body[0] ^= 1
	wire[0] ^= 1
	_, err = e.ReadACK(restored)
	require.NoError(t, err)
}

func TestIncomingEventChangedWireOrBodyCannotReuseACKWinner(t *testing.T) {
	e := incomingEventFixture(t)
	p, err := e.FreezeACK(eventACKFixture(t, e, false), uuid.NewString())
	require.NoError(t, err)
	header, body, _, _ := e.Input()
	otherWire, err := legacy.Encode(legacy.Envelope{UUID: header.MessageId, Metadata: messagingMetadata(), Payload: []byte("changed-protected-wire")}, legacy.Revision2)
	require.NoError(t, err)
	changed, err := FreezeIncomingEvent(header, body, otherWire, "7")
	require.NoError(t, err)
	_, err = changed.ReadACK(p)
	require.ErrorIs(t, err, transaction.ErrInputConflict)
	_, err = FreezeIncomingEvent(header, body, otherWire, "8")
	require.ErrorIs(t, err, ErrMessagingContract)
	body[0] ^= 1
	_, err = FreezeIncomingEvent(header, body, otherWire, "7")
	require.ErrorIs(t, err, ErrMessagingContract)
}

func TestIncomingEventRejectsACKForAnotherEventAndWrongRoute(t *testing.T) {
	e := incomingEventFixture(t)
	ack := eventACKFixture(t, incomingEventFixture(t), false)
	_, err := e.FreezeACK(ack, uuid.NewString())
	require.ErrorIs(t, err, ErrMessagingContract)
	p, err := e.FreezeACK(eventACKFixture(t, e, false), uuid.NewString())
	require.NoError(t, err)
	for _, name := range []string{"physical", "owner", "key", "operation"} {
		t.Run(name, func(t *testing.T) {
			in := p.Input()
			switch name {
			case "physical":
				in.Branches[0].PhysicalTopic = "wrong-physical"
			case "owner":
				in.Owner = "ai.mysql"
			case "key":
				in.IdempotencyKey = "another-event"
			case "operation":
				in.OperationID = "not-a-uuid"
			}
			changed, err := transaction.NewPlan(in)
			require.NoError(t, err)
			_, err = e.ReadACK(changed)
			require.Error(t, err)
		})
	}
}
