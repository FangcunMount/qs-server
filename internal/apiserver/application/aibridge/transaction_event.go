package aibridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strconv"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"google.golang.org/protobuf/proto"
)

// FrozenIncomingEvent is created only AFTER host signature/topic authentication
// and exact body resolution. It owns callback input; it neither authenticates a
// sender nor authorizes a model or business retry. The hash binds the first
// incoming wire/header/body, excluding generated ACK ID/time/ciphertext/outcome.
type FrozenIncomingEvent struct {
	envelope *pb.MessagingEnvelope
	body     []byte
	wire     []byte
	org      string
	hash     [32]byte
}

func FreezeIncomingEvent(header *pb.MessagingEnvelope, raw, wire []byte, organization string) (FrozenIncomingEvent, error) {
	if header == nil || header.Producer != "qs-ai" || header.Destination != "qs-server" || header.Kind < pb.MessagingKind_COMMAND_RECEIPT || header.Kind > pb.MessagingKind_EVALUATION_STATE || !validNumber(organization) || len(wire) == 0 || len(wire) > 262144 {
		return FrozenIncomingEvent{}, ErrMessagingContract
	}
	body, err := ParseMessagingBody(header, raw)
	if err != nil {
		return FrozenIncomingEvent{}, err
	}
	if ref := header.GetPayloadReference(); ref != nil && ref.OrganizationId != organization {
		return FrozenIncomingEvent{}, ErrMessagingContract
	}
	switch header.Kind {
	case pb.MessagingKind_INTERPRETATION_STATE:
		if body.GetInterpretationState().GetActor().GetOrgId() != organization {
			return FrozenIncomingEvent{}, ErrMessagingContract
		}
	case pb.MessagingKind_EVALUATION_STATE:
		if body.GetEvaluationState().GetOrganizationId() != organization {
			return FrozenIncomingEvent{}, ErrMessagingContract
		}
	}
	outer, ok, err := legacy.Decode(wire)
	if err != nil || !ok || outer.UUID != header.MessageId || !reflect.DeepEqual(outer.Metadata, messagingMetadata()) {
		return FrozenIncomingEvent{}, ErrMessagingContract
	}
	headerBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(header)
	if err != nil {
		return FrozenIncomingEvent{}, err
	}
	input, err := json.Marshal(struct {
		Version            int
		Organization       string
		Header, Body, Wire [32]byte
	}{1, organization, sha256.Sum256(headerBytes), sha256.Sum256(raw), sha256.Sum256(wire)})
	if err != nil {
		return FrozenIncomingEvent{}, err
	}
	return FrozenIncomingEvent{envelope: proto.Clone(header).(*pb.MessagingEnvelope), body: bytes.Clone(raw), wire: bytes.Clone(wire), org: organization, hash: sha256.Sum256(input)}, nil
}

func (e FrozenIncomingEvent) Request() transaction.Request {
	if e.envelope == nil || e.hash == [32]byte{} {
		return transaction.Request{}
	}
	return transaction.Request{Owner: commandOwner, Scope: "org:" + e.org, Key: "ai-event/" + e.envelope.Producer + "/" + e.envelope.MessageId, InputHash: e.hash}
}

// Input returns owned copies, suitable for one original-root callback. On a
// redelivery the host MUST match Request against the durable winner before use.
func (e FrozenIncomingEvent) Input() (*pb.MessagingEnvelope, []byte, []byte, string) {
	if e.envelope == nil {
		return nil, nil, nil, ""
	}
	return proto.Clone(e.envelope).(*pb.MessagingEnvelope), bytes.Clone(e.body), bytes.Clone(e.wire), e.org
}

func (e FrozenIncomingEvent) FreezeACK(message *PreparedMessaging, operationID string, routes *transport.RouteResolver) (transaction.FrozenPlan, error) {
	if !e.Request().Valid() || !validID(operationID) || message == nil || message.Envelope == nil {
		return transaction.FrozenPlan{}, ErrMessagingContract
	}
	if err := e.validateACK(message.Envelope, message.Body); err != nil {
		return transaction.FrozenPlan{}, err
	}
	physical, err := messagingPhysicalTopic(routes, AcksTopic, "")
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	branch, err := FreezeMessagingBranch(message, e.org, 1, false, false, message.Envelope.MessageId, physical, commandRevision)
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	q := e.Request()
	return transaction.NewPlan(transaction.PlanInput{Owner: q.Owner, Scope: q.Scope, IdempotencyKey: q.Key, OperationID: operationID, InputHash: q.InputHash, Branches: []transaction.BranchInput{branch}})
}

// ReadACK validates a reservation winner against the exact incoming delivery.
// Checker uses persisted witnesses only, never this business callback input.
func (e FrozenIncomingEvent) ReadACK(plan transaction.FrozenPlan, routes *transport.RouteResolver) (FrozenMessagingArchive, error) {
	var empty FrozenMessagingArchive
	if !e.Request().Valid() || !plan.Valid() || len(plan.Input().Branches) != 1 || !validID(plan.Input().OperationID) {
		return empty, ErrMessagingContract
	}
	if err := e.Request().Match(plan); err != nil {
		return empty, err
	}
	b := plan.Input().Branches[0]
	a, err := DecodeMessagingBranch(b)
	if err != nil {
		return empty, err
	}
	physical, err := messagingPhysicalTopic(routes, AcksTopic, b.PhysicalTopic)
	if err != nil || b.PhysicalTopic != physical || b.ID != a.Envelope.MessageId || strconv.FormatUint(a.OrganizationID, 10) != e.org || a.Sequence != 1 || a.Ordered || a.RequiresReceipt {
		return empty, ErrMessagingContract
	}
	if err = e.validateACK(a.Envelope, a.Body); err != nil {
		return empty, err
	}
	return a, nil
}

func (e FrozenIncomingEvent) validateACK(header *pb.MessagingEnvelope, raw []byte) error {
	if e.envelope == nil || header == nil || header.Kind != pb.MessagingKind_EVENT_ACKNOWLEDGEMENT || header.AggregateKey != e.envelope.AggregateKey || header.CorrelationCommandId != "" {
		return ErrMessagingContract
	}
	body, err := ParseMessagingBody(header, raw)
	if err != nil {
		return err
	}
	a := body.GetEventAcknowledgement()
	if a.EventId != e.envelope.MessageId || a.EventBodySha256 != e.envelope.BodySha256 || a.EventKind != e.envelope.Kind || (a.Outcome != pb.MessagingEventAcknowledgement_STORED && a.Outcome != pb.MessagingEventAcknowledgement_TECHNICALLY_HELD) {
		return ErrMessagingContract
	}
	return nil
}
