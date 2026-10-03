package aibridge

import (
	"encoding/json"
	"reflect"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	"google.golang.org/protobuf/proto"
)

const InlineMessagingBody = 32 * 1024

func messagingMetadata() map[string]string {
	return map[string]string{"secure_profile": protected.Profile}
}
func messagingChannel(topic string) string {
	switch topic {
	case CommandsTopic:
		return "qs-ai.commands.v1"
	case EventsTopic:
		return "qs-server.ai-events.v1"
	case AcksTopic:
		return "qs-ai.acks.v1"
	default:
		return ""
	}
}

type PreparedMessaging struct {
	Envelope *pb.MessagingEnvelope
	Body     []byte
	Wire     []byte
	Topic    string
}

// ProtectMessaging chooses inline/reference before sealing. Hosts persist Body and
// Wire in the same original transaction and never reconstruct them for retries.
func ProtectMessaging(kind pb.MessagingKind, id, aggregate, correlation, organizationID, originalTime string, body *pb.MessagingBody, signing, recipient jose.JSONWebKey) (*PreparedMessaging, error) {
	envelope, raw, err := PrepareMessaging(kind, id, aggregate, correlation, body)
	if err != nil {
		return nil, err
	}
	producer, destination, topic, _ := MessagingRoute(kind)
	envelope.OriginalOccurredAt = originalTime
	if len(raw) > InlineMessagingBody {
		envelope.Body = &pb.MessagingEnvelope_PayloadReference{PayloadReference: &pb.MessagePayloadReference{
			Producer: producer, Destination: destination, MessageId: id,
			BodySha256: envelope.BodySha256, BodyLength: envelope.BodyLength, OrganizationId: organizationID,
		}}
	}
	if err := ValidateMessagingHeader(envelope, topic); err != nil {
		return nil, err
	}
	header, err := proto.MarshalOptions{Deterministic: true}.Marshal(envelope)
	if err != nil {
		return nil, ErrMessagingContract
	}
	metadata := messagingMetadata()
	sealed, err := protected.Seal(protected.Context{Producer: producer, Destination: destination, Topic: topic, MessageID: id, Metadata: metadata}, header, signing, recipient)
	if err != nil {
		return nil, err
	}
	outer := legacy.Envelope{UUID: id, Payload: sealed, Metadata: metadata}
	if !protected.FitsNSQ(outer, topic, messagingChannel(topic)) {
		return nil, ErrMessagingContract
	}
	wire, err := legacy.Encode(outer, legacy.Revision2)
	if err != nil {
		return nil, err
	}
	return &PreparedMessaging{Envelope: envelope, Body: raw, Wire: wire, Topic: topic}, nil
}

// AuthenticateMessaging validates JOSE before a caller can fetch a reference or
// enter the original business handler. It performs no I/O or business authorization.
func AuthenticateMessaging(wire []byte, topic string, keys protected.Keyring) (*pb.MessagingEnvelope, error) {
	if messagingChannel(topic) == "" || len(wire) > protected.NSQMaxBytes {
		return nil, ErrMessagingContract
	}
	var header struct {
		Revision int `json:"schema_revision"`
	}
	if json.Unmarshal(wire, &header) != nil || header.Revision != int(legacy.Revision2) {
		return nil, ErrMessagingContract
	}
	outer, recognized, err := legacy.Decode(wire)
	if err != nil || !recognized || !validID(outer.UUID) || !reflect.DeepEqual(outer.Metadata, messagingMetadata()) {
		return nil, ErrMessagingContract
	}
	producer, destination := "qs-server", "qs-ai"
	if topic == EventsTopic {
		producer, destination = "qs-ai", "qs-server"
	}
	raw, err := protected.Open(outer.Payload, protected.Context{Producer: producer, Destination: destination, Topic: topic, MessageID: outer.UUID, Metadata: outer.Metadata}, keys, InlineMessagingBody+2048)
	if err != nil {
		return nil, err
	}
	envelope := new(pb.MessagingEnvelope)
	if proto.Unmarshal(raw, envelope) != nil || envelope.MessageId != outer.UUID {
		return nil, ErrMessagingContract
	}
	if err := ValidateMessagingHeader(envelope, topic); err != nil {
		return nil, err
	}
	return envelope, nil
}
