package aibridge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"google.golang.org/protobuf/proto"
)

const MessagingSchema = "qs-ai-messaging/v1"
const CommandsTopic = "qs.ai.commands.v1"
const EventsTopic = "qs.ai.events.v1"
const AcksTopic = "qs.ai.acks.v1"
const MaxMessagingBody = 16*1024*1024 - 1 // MEDIUMBLOB capacity.

var ErrMessagingContract = errors.New("invalid AI messaging contract")

func validBodyHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

// MessagingRoute is fixed by kind, never by an untrusted broker metadata source.
func MessagingRoute(kind pb.MessagingKind) (producer, destination, topic string, err error) {
	switch kind {
	case pb.MessagingKind_START, pb.MessagingKind_CHANGE, pb.MessagingKind_PARTICIPANT_RETRY, pb.MessagingKind_EVALUATION_START, pb.MessagingKind_EVALUATION_CANCEL:
		return "qs-server", "qs-ai", CommandsTopic, nil
	case pb.MessagingKind_COMMAND_RECEIPT, pb.MessagingKind_INTERPRETATION_STATE, pb.MessagingKind_EVALUATION_STATE:
		return "qs-ai", "qs-server", EventsTopic, nil
	case pb.MessagingKind_EVENT_ACKNOWLEDGEMENT:
		return "qs-server", "qs-ai", AcksTopic, nil
	default:
		return "", "", "", ErrMessagingContract
	}
}

// PrepareMessaging freezes the transport body once. Hosts persist it and the wire;
// legacy business payload/hash/time remain separate and are not reconstructed here.
func PrepareMessaging(kind pb.MessagingKind, id, aggregate, correlation string, body *pb.MessagingBody) (*pb.MessagingEnvelope, []byte, error) {
	producer, destination, _, err := MessagingRoute(kind)
	if err != nil {
		return nil, nil, err
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(body)
	if err != nil {
		return nil, nil, ErrMessagingContract
	}
	hash := sha256.Sum256(raw)
	envelope := &pb.MessagingEnvelope{SchemaVersion: MessagingSchema, Producer: producer, Destination: destination, MessageId: id, Kind: kind, AggregateKey: aggregate, CorrelationCommandId: correlation, BodySha256: hex.EncodeToString(hash[:]), BodyLength: uint64(len(raw)), Body: &pb.MessagingEnvelope_InlineBody{InlineBody: raw}}
	if _, err := ParseMessagingBody(envelope, raw); err != nil {
		return nil, nil, err
	}
	return envelope, raw, nil
}

// ValidateMessagingHeader is safe before an authenticated reference fetch. It does
// not authenticate a sender: the host first verifies JOSE and the actual topic.
func ValidateMessagingHeader(envelope *pb.MessagingEnvelope, actualTopic string) error {
	if envelope == nil || envelope.SchemaVersion != MessagingSchema || !validID(envelope.MessageId) || envelope.AggregateKey == "" || len(envelope.AggregateKey) > 192 || envelope.BodyLength == 0 || envelope.BodyLength > MaxMessagingBody || len(envelope.OriginalOccurredAt) > 64 {
		return ErrMessagingContract
	}
	producer, destination, topic, err := MessagingRoute(envelope.Kind)
	if err != nil || envelope.Producer != producer || envelope.Destination != destination || actualTopic != topic {
		return ErrMessagingContract
	}
	if !validBodyHash(envelope.BodySha256) {
		return ErrMessagingContract
	}
	switch value := envelope.Body.(type) {
	case *pb.MessagingEnvelope_InlineBody:
		if uint64(len(value.InlineBody)) != envelope.BodyLength {
			return ErrMessagingContract
		}
	case *pb.MessagingEnvelope_PayloadReference:
		r := value.PayloadReference
		if r == nil || r.Producer != envelope.Producer || r.Destination != envelope.Destination || r.MessageId != envelope.MessageId || r.BodySha256 != envelope.BodySha256 || r.BodyLength != envelope.BodyLength || !validNumber(r.OrganizationId) {
			return ErrMessagingContract
		}
	default:
		return ErrMessagingContract
	}
	return nil
}

// ParseMessagingBody validates exact persisted bytes before mapping into the one
// existing business path. Receipt checks/CAS/authorization remain host responsibilities.
func ParseMessagingBody(envelope *pb.MessagingEnvelope, raw []byte) (*pb.MessagingBody, error) {
	if envelope == nil {
		return nil, ErrMessagingContract
	}
	_, _, topic, err := MessagingRoute(envelope.Kind)
	if err != nil {
		return nil, err
	}
	if err := ValidateMessagingHeader(envelope, topic); err != nil {
		return nil, err
	}
	hash := sha256.Sum256(raw)
	if uint64(len(raw)) != envelope.BodyLength || fmt.Sprintf("%x", hash) != envelope.BodySha256 {
		return nil, ErrMessagingContract
	}
	body := new(pb.MessagingBody)
	if proto.Unmarshal(raw, body) != nil {
		return nil, ErrMessagingContract
	}
	valid := false
	switch envelope.Kind {
	case pb.MessagingKind_START:
		value := body.GetStart()
		valid = value != nil && value.RequestId == envelope.MessageId
	case pb.MessagingKind_CHANGE:
		value := body.GetChange()
		valid = value != nil && value.CommandId == envelope.MessageId
	case pb.MessagingKind_PARTICIPANT_RETRY:
		value := body.GetParticipantRetry()
		valid = value != nil && value.CommandId == envelope.MessageId
	case pb.MessagingKind_EVALUATION_START:
		valid = body.GetEvaluationStart() != nil
	case pb.MessagingKind_EVALUATION_CANCEL:
		valid = body.GetEvaluationCancel() != nil && body.GetEvaluationCancel().Discard != nil
	case pb.MessagingKind_COMMAND_RECEIPT:
		value := body.GetCommandReceipt()
		valid = value != nil && validID(value.CommandId) && validBodyHash(value.CommandBodySha256) && value.CommandId == envelope.CorrelationCommandId && (value.Decision == pb.MessagingDecision_ACCEPTED || value.Decision == pb.MessagingDecision_REJECTED || value.Decision == pb.MessagingDecision_HELD)
	case pb.MessagingKind_INTERPRETATION_STATE:
		value := body.GetInterpretationState()
		valid = value != nil && value.EventId == envelope.MessageId
	case pb.MessagingKind_EVALUATION_STATE:
		value := body.GetEvaluationState()
		valid = value != nil && validID(value.RunId) && validNumber(value.OrganizationId) && value.EventSequence > 0
	case pb.MessagingKind_EVENT_ACKNOWLEDGEMENT:
		value := body.GetEventAcknowledgement()
		valid = value != nil && validID(value.EventId) && validBodyHash(value.EventBodySha256) && (value.EventKind == pb.MessagingKind_COMMAND_RECEIPT || value.EventKind == pb.MessagingKind_INTERPRETATION_STATE || value.EventKind == pb.MessagingKind_EVALUATION_STATE) && (value.Outcome == pb.MessagingEventAcknowledgement_STORED || value.Outcome == pb.MessagingEventAcknowledgement_TECHNICALLY_HELD)
	}
	if !valid {
		return nil, ErrMessagingContract
	}
	return body, nil
}
