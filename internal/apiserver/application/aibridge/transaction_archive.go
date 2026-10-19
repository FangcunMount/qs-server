package aibridge

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"strconv"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"google.golang.org/protobuf/proto"
)

const ArchiveContentType = "application/vnd.qs-ai.archive.v1+octet-stream"
const archiveMagic = "qs-ai-archive/v1\x00"
const maxArchiveHeader = InlineMessagingBody + 2048

// FrozenMessagingArchive retains raw plaintext header/body for the local archive
// and first protected wire for the Broker. It never signs, encrypts or sends.
// Header is retained verbatim, including unknown fields; it is not re-encoded.
type FrozenMessagingArchive struct {
	Envelope                 *pb.MessagingEnvelope
	Header, Body, Wire       []byte
	OrganizationID, Sequence uint64
	Ordered, RequiresReceipt bool
}

// FreezeMessagingBranch runs before Reserve and before any database callback.
// A reservation loser must DecodeMessagingBranch from the winner's plan instead
// of using its own newly sealed candidate.
func FreezeMessagingBranch(m *PreparedMessaging, org string, sequence uint64, ordered, receipt bool, id, physical, revision string) (transaction.BranchInput, error) {
	if m == nil || m.Envelope == nil {
		return transaction.BranchInput{}, ErrMessagingContract
	}
	n, err := strconv.ParseUint(org, 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != org {
		return transaction.BranchInput{}, ErrMessagingContract
	}
	header, err := proto.MarshalOptions{Deterministic: true}.Marshal(m.Envelope)
	if err != nil {
		return transaction.BranchInput{}, ErrMessagingContract
	}
	a := FrozenMessagingArchive{Envelope: m.Envelope, Header: header, Body: m.Body, Wire: m.Wire, OrganizationID: n, Sequence: sequence, Ordered: ordered, RequiresReceipt: receipt}
	sdk, err := a.Message()
	if err != nil {
		return transaction.BranchInput{}, err
	}
	if m.Topic != sdk.Input().Destination {
		return transaction.BranchInput{}, ErrMessagingContract
	}
	b := transaction.BranchInput{ID: id, PhysicalTopic: physical, RouteRevision: revision, Message: sdk, Wire: append([]byte(nil), m.Wire...)}
	if _, err = DecodeMessagingBranch(b); err != nil {
		return transaction.BranchInput{}, err
	}
	return b, nil
}

func (a FrozenMessagingArchive) Validate() error {
	if a.Envelope == nil || a.OrganizationID == 0 || a.Sequence == 0 || len(a.Header) == 0 || len(a.Header) > maxArchiveHeader || len(a.Body) == 0 || len(a.Body) > MaxMessagingBody || len(a.Wire) == 0 || len(a.Wire) > 262144 {
		return ErrMessagingContract
	}
	header := new(pb.MessagingEnvelope)
	if proto.Unmarshal(a.Header, header) != nil || !proto.Equal(header, a.Envelope) {
		return ErrMessagingContract
	}
	body, err := ParseMessagingBody(header, a.Body)
	if err != nil {
		return err
	}
	org := strconv.FormatUint(a.OrganizationID, 10)
	if ref := header.GetPayloadReference(); ref != nil && ref.OrganizationId != org {
		return ErrMessagingContract
	}
	var bodyOrg string
	switch header.Kind {
	case pb.MessagingKind_START:
		bodyOrg = body.GetStart().GetActor().GetOrgId()
	case pb.MessagingKind_CHANGE:
		bodyOrg = body.GetChange().GetActor().GetOrgId()
	case pb.MessagingKind_PARTICIPANT_RETRY:
		bodyOrg = strconv.FormatInt(body.GetParticipantRetry().GetScope().GetOrganizationId(), 10)
	case pb.MessagingKind_EVALUATION_START:
		bodyOrg = strconv.FormatInt(body.GetEvaluationStart().GetScope().GetOrganizationId(), 10)
	case pb.MessagingKind_EVALUATION_CANCEL:
		bodyOrg = strconv.FormatInt(body.GetEvaluationCancel().GetScope().GetOrganizationId(), 10)
	case pb.MessagingKind_INTERPRETATION_STATE:
		bodyOrg = body.GetInterpretationState().GetActor().GetOrgId()
	case pb.MessagingKind_EVALUATION_STATE:
		bodyOrg = body.GetEvaluationState().GetOrganizationId()
	}
	if header.Kind != pb.MessagingKind_COMMAND_RECEIPT && header.Kind != pb.MessagingKind_EVENT_ACKNOWLEDGEMENT && bodyOrg != org {
		return ErrMessagingContract
	}
	outer, ok, err := legacy.Decode(a.Wire)
	if err != nil || !ok || outer.UUID != header.MessageId || !reflect.DeepEqual(outer.Metadata, messagingMetadata()) {
		return ErrMessagingContract
	}
	return nil
}

// Encode uses big-endian unsigned integers and exact original byte fields, shared
// with Python. It is local plan/archive data; the Broker still gets first Wire.
func (a FrozenMessagingArchive) Encode() ([]byte, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	raw := make([]byte, len(archiveMagic)+25+len(a.Header)+len(a.Body))
	copy(raw, archiveMagic)
	at := len(archiveMagic)
	binary.BigEndian.PutUint64(raw[at:], a.OrganizationID)
	binary.BigEndian.PutUint64(raw[at+8:], a.Sequence)
	if a.Ordered {
		raw[at+16] |= 1
	}
	if a.RequiresReceipt {
		raw[at+16] |= 2
	}
	binary.BigEndian.PutUint32(raw[at+17:], uint32(len(a.Header)))
	binary.BigEndian.PutUint32(raw[at+21:], uint32(len(a.Body)))
	copy(raw[at+25:], a.Header)
	copy(raw[at+25+len(a.Header):], a.Body)
	return raw, nil
}

// Message reconstructs the immutable local intent without re-marshalling Header.
func (a FrozenMessagingArchive) Message() (message.Message, error) {
	payload, err := a.Encode()
	if err != nil {
		return message.Message{}, err
	}
	_, _, topic, err := MessagingRoute(a.Envelope.Kind)
	if err != nil {
		return message.Message{}, err
	}
	return message.New(message.Input{Producer: a.Envelope.Producer, ID: a.Envelope.MessageId, Destination: topic, EventType: a.Envelope.Kind.String(), SchemaVersion: MessagingSchema, Scope: "org:" + strconv.FormatUint(a.OrganizationID, 10), ContentType: ArchiveContentType, OccurredAt: a.Envelope.OriginalOccurredAt, Payload: payload})
}

func DecodeMessagingArchive(payload, wire []byte) (FrozenMessagingArchive, error) {
	var a FrozenMessagingArchive
	prefix := len(archiveMagic) + 25
	if len(payload) < prefix || !bytes.Equal(payload[:len(archiveMagic)], []byte(archiveMagic)) {
		return a, ErrMessagingContract
	}
	at := len(archiveMagic)
	headerLen := int(binary.BigEndian.Uint32(payload[at+17:]))
	bodyLen := int(binary.BigEndian.Uint32(payload[at+21:]))
	flags := payload[at+16]
	if flags > 3 || headerLen == 0 || headerLen > maxArchiveHeader || bodyLen == 0 || bodyLen > MaxMessagingBody || headerLen+bodyLen != len(payload)-prefix {
		return a, ErrMessagingContract
	}
	a = FrozenMessagingArchive{Envelope: new(pb.MessagingEnvelope), Header: append([]byte(nil), payload[prefix:prefix+headerLen]...), Body: append([]byte(nil), payload[prefix+headerLen:]...), Wire: append([]byte(nil), wire...), OrganizationID: binary.BigEndian.Uint64(payload[at:]), Sequence: binary.BigEndian.Uint64(payload[at+8:]), Ordered: flags&1 != 0, RequiresReceipt: flags&2 != 0}
	if proto.Unmarshal(a.Header, a.Envelope) != nil {
		return FrozenMessagingArchive{}, ErrMessagingContract
	}
	return a, a.Validate()
}

func DecodeMessagingBranch(b transaction.BranchInput) (FrozenMessagingArchive, error) {
	in := b.Message.Input()
	a, err := DecodeMessagingArchive(in.Payload, b.Wire)
	if err != nil {
		return a, err
	}
	_, _, topic, err := MessagingRoute(a.Envelope.Kind)
	if err != nil || b.ID == "" || b.PhysicalTopic == "" || b.RouteRevision != "m3-1" || in.Producer != a.Envelope.Producer || in.ID != a.Envelope.MessageId || in.Destination != topic || in.EventType != a.Envelope.Kind.String() || in.SchemaVersion != MessagingSchema || in.Scope != "org:"+strconv.FormatUint(a.OrganizationID, 10) || in.ContentType != ArchiveContentType || in.OccurredAt != a.Envelope.OriginalOccurredAt {
		return a, fmt.Errorf("frozen archive message identity differs")
	}
	return a, nil
}
