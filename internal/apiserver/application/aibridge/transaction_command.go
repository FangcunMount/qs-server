package aibridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"google.golang.org/protobuf/proto"
)

const commandOwner = "qs.mysql"
const commandRevision = "m3-1"

// TransactionCommand owns stable authenticated input before any clock, Seal,
// sequence reservation or Broker I/O. It has no database/resource lifecycle.
type TransactionCommand struct {
	kind          pb.MessagingKind
	id, aggregate string
	scope         OperationScope
	body          []byte
	hash          [32]byte
}

func NewTransactionCommand(kind pb.MessagingKind, id, aggregate string, scope OperationScope, body *pb.MessagingBody) (TransactionCommand, error) {
	header, raw, err := PrepareMessaging(kind, id, aggregate, "", body)
	if err != nil {
		return TransactionCommand{}, err
	}
	return commandFromOriginal(header, raw, scope)
}

func commandFromOriginal(header *pb.MessagingEnvelope, raw []byte, scope OperationScope) (TransactionCommand, error) {
	if header == nil || header.CorrelationCommandId != "" || header.Kind < pb.MessagingKind_START || header.Kind > pb.MessagingKind_EVALUATION_CANCEL || len(scope.SubjectID) == 0 || len(scope.SubjectID) > 128 || !utf8.ValidString(scope.SubjectID) || !validID(scope.ResourceID) {
		return TransactionCommand{}, ErrMessagingContract
	}
	body, err := ParseMessagingBody(header, raw)
	if err != nil {
		return TransactionCommand{}, err
	}
	actual, err := CommandScope(header, body)
	if err != nil || actual != scope {
		return TransactionCommand{}, ErrMessagingContract
	}
	// Generated OriginalOccurredAt, first ciphertext and sequence are absent.
	// Read uses these exact original body bytes, never a protobuf re-marshal.
	value, err := json.Marshal(struct {
		Version       int
		Kind          int32
		ID, Aggregate string
		Scope         OperationScope
		Body          []byte
	}{1, int32(header.Kind), header.MessageId, header.AggregateKey, scope, raw})
	if err != nil {
		return TransactionCommand{}, err
	}
	return TransactionCommand{kind: header.Kind, id: header.MessageId, aggregate: header.AggregateKey, scope: scope, body: bytes.Clone(raw), hash: sha256.Sum256(value)}, nil
}

// CommandScope derives the same original organization/actor/resource boundary
// used by command admission. The caller still supplies authenticated scope.
func CommandScope(e *pb.MessagingEnvelope, b *pb.MessagingBody) (OperationScope, error) {
	if e == nil || b == nil {
		return OperationScope{}, ErrMessagingContract
	}
	var s OperationScope
	switch e.Kind {
	case pb.MessagingKind_START:
		c := b.GetStart()
		if c == nil || e.AggregateKey != c.RequestId {
			return s, ErrMessagingContract
		}
		s = OperationScope{c.GetActor().GetOrgId(), c.GetActor().GetSubjectId(), c.RequestId}
	case pb.MessagingKind_CHANGE:
		c := b.GetChange()
		if c == nil {
			return s, ErrMessagingContract
		}
		s = OperationScope{c.GetActor().GetOrgId(), c.GetActor().GetSubjectId(), c.SessionId}
	case pb.MessagingKind_PARTICIPANT_RETRY:
		c := b.GetParticipantRetry()
		if c == nil {
			return s, ErrMessagingContract
		}
		s = OperationScope{strconv.FormatInt(c.GetScope().GetOrganizationId(), 10), strconv.FormatInt(c.GetScope().GetOperatorUserId(), 10), c.SessionId}
	case pb.MessagingKind_EVALUATION_START:
		c := b.GetEvaluationStart()
		if c == nil {
			return s, ErrMessagingContract
		}
		s = OperationScope{strconv.FormatInt(c.GetScope().GetOrganizationId(), 10), strconv.FormatInt(c.GetScope().GetOperatorUserId(), 10), c.GetScope().GetRunId()}
	case pb.MessagingKind_EVALUATION_CANCEL:
		c := b.GetEvaluationCancel()
		if c == nil {
			return s, ErrMessagingContract
		}
		s = OperationScope{strconv.FormatInt(c.GetScope().GetOrganizationId(), 10), strconv.FormatInt(c.GetScope().GetOperatorUserId(), 10), c.GetScope().GetRunId()}
	default:
		return s, ErrMessagingContract
	}
	if !validNumber(s.OrganizationID) || s.SubjectID == "" || len(s.SubjectID) > 128 || !utf8.ValidString(s.SubjectID) || !validID(s.ResourceID) {
		return OperationScope{}, ErrMessagingContract
	}
	return s, nil
}

func (c TransactionCommand) Kind() pb.MessagingKind { return c.kind }
func (c TransactionCommand) ID() string             { return c.id }
func (c TransactionCommand) Aggregate() string      { return c.aggregate }
func (c TransactionCommand) Scope() OperationScope  { return c.scope }
func (c TransactionCommand) InputHash() [32]byte    { return c.hash }
func (c TransactionCommand) Key() string            { return "ai-command/" + c.id }
func (c TransactionCommand) Body() (*pb.MessagingBody, error) {
	if c.id == "" || len(c.body) == 0 {
		return nil, ErrMessagingContract
	}
	b := new(pb.MessagingBody)
	if err := proto.Unmarshal(c.body, b); err != nil {
		return nil, ErrMessagingContract
	}
	return b, nil
}

// Freeze runs outside the business root, after the host reads the
// original aggregate sequence, which the original root must CAS again. It never seals or
// sends. The SDK reservation winner alone determines future callback input.
func (c TransactionCommand) Freeze(m *PreparedMessaging, sequence uint64) (transaction.FrozenPlan, error) {
	if c.id == "" || c.hash == [32]byte{} || m == nil || m.Envelope == nil || !bytes.Equal(c.body, m.Body) || m.Envelope.Kind != c.kind || m.Envelope.MessageId != c.id || m.Envelope.AggregateKey != c.aggregate || m.Envelope.CorrelationCommandId != "" || sequence == 0 || sequence == ^uint64(0) {
		return transaction.FrozenPlan{}, ErrMessagingContract
	}
	actual, err := commandFromOriginal(m.Envelope, m.Body, c.scope)
	if err != nil || actual.hash != c.hash {
		return transaction.FrozenPlan{}, ErrMessagingContract
	}
	physical, err := transport.PhysicalTopicV2(CommandsTopic, transport.TransactionMessageType, commandOwner, commandRevision)
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	branch, err := FreezeMessagingBranch(m, c.scope.OrganizationID, sequence, true, true, c.id, physical, commandRevision)
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	return transaction.NewPlan(transaction.PlanInput{Owner: commandOwner, Scope: "org:" + c.scope.OrganizationID, OperationID: c.id, IdempotencyKey: c.Key(), InputHash: c.hash, Branches: []transaction.BranchInput{branch}})
}

// ReadTransactionCommand validates a complete winner. It reuses the unchanged
// v1 archive codec, including unknown header fields and first protected wire.
func ReadTransactionCommand(p transaction.FrozenPlan) (TransactionCommand, FrozenMessagingArchive, error) {
	var empty TransactionCommand
	var archive FrozenMessagingArchive
	if !p.Valid() || p.Input().Owner != commandOwner || len(p.Input().Branches) != 1 {
		return empty, archive, ErrMessagingContract
	}
	in := p.Input()
	branch := in.Branches[0]
	a, err := DecodeMessagingBranch(branch)
	if err != nil {
		return empty, archive, err
	}
	body, err := ParseMessagingBody(a.Envelope, a.Body)
	if err != nil {
		return empty, archive, err
	}
	scope, err := CommandScope(a.Envelope, body)
	if err != nil {
		return empty, archive, err
	}
	c, err := commandFromOriginal(a.Envelope, a.Body, scope)
	if err != nil {
		return empty, archive, err
	}
	physical, err := transport.PhysicalTopicV2(CommandsTopic, transport.TransactionMessageType, commandOwner, commandRevision)
	if err != nil || in.Scope != "org:"+scope.OrganizationID || in.OperationID != c.id || branch.ID != c.id || branch.PhysicalTopic != physical || !a.Ordered || !a.RequiresReceipt || a.Sequence == 0 || a.Sequence == ^uint64(0) {
		return empty, archive, ErrMessagingContract
	}
	if err = p.MatchRequest(commandOwner, in.Scope, c.Key(), c.hash); err != nil {
		return empty, archive, err
	}
	return c, a, nil
}
