package transactionplan

import (
	"bytes"
	"encoding/json"
	"fmt"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
)

// The private plan binds the complete artifact and the original consumer wire.
// The Broker receives Branch.Wire, never this private payload.
type artifact struct {
	Version int    `json:"version"`
	Body    []byte `json:"body"`
	Wire    []byte `json:"wire"`
}

// Freeze retains the legacy v2 codec for explicit legacy callers.
func Freeze(q port.Request, resolver catalog.TopicResolver, owner, revision string) (transaction.FrozenPlan, error) {
	return freeze(q, resolver, owner, revision, nil)
}

// FreezeWithRoutes selects exclusively from the owner's active manifest.
func FreezeWithRoutes(q port.Request, resolver catalog.TopicResolver, owner, revision string, routes *transport.RouteResolver) (transaction.FrozenPlan, error) {
	if routes == nil {
		return transaction.FrozenPlan{}, transaction.ErrBinding
	}
	return freeze(q, resolver, owner, revision, routes)
}
func freeze(q port.Request, resolver catalog.TopicResolver, owner, revision string, routes *transport.RouteResolver) (transaction.FrozenPlan, error) {
	count := len(q.Events) + len(q.Prepared)
	if q.Key == "" || len(q.Artifact) == 0 || len(q.Artifact) > 1<<20 || count == 0 || count > transaction.MaxBranches || (len(q.Events) > 0 && len(q.Prepared) > 0) || resolver == nil {
		return transaction.FrozenPlan{}, transaction.ErrInvalidPlan
	}
	intents, err := prepare(q, resolver)
	if err != nil {
		return transaction.FrozenPlan{}, err
	}
	branches := make([]transaction.BranchInput, 0, len(intents))
	scope := intents[0].Message.Input().Scope
	for _, intent := range intents {
		in := intent.Message.Input()
		if in.Scope != scope || len(in.Payload) > 262144 {
			return transaction.FrozenPlan{}, transaction.ErrInvalidPlan
		}
		wire := bytes.Clone(in.Payload)
		in.Payload, err = json.Marshal(artifact{Version: 1, Body: bytes.Clone(q.Artifact), Wire: wire})
		if err != nil {
			return transaction.FrozenPlan{}, err
		}
		msg, err := message.New(in)
		if err != nil {
			return transaction.FrozenPlan{}, err
		}
		physical, err := physicalRoute(in.Destination, owner, revision, "", routes)
		if err != nil {
			return transaction.FrozenPlan{}, err
		}
		branches = append(branches, transaction.BranchInput{ID: in.ID, Message: msg, Wire: wire, PhysicalTopic: physical, RouteRevision: revision})
	}
	return transaction.NewPlan(transaction.PlanInput{Owner: owner, Scope: scope, OperationID: intents[0].Message.Input().ID, IdempotencyKey: q.Key, InputHash: q.InputHash, Branches: branches})
}

func prepare(q port.Request, resolver catalog.TopicResolver) ([]standard.PreparedIntent, error) {
	if len(q.Prepared) == 0 {
		return standard.PrepareIntents(q.Events, resolver, "api-server")
	}
	result := make([]standard.PreparedIntent, 0, len(q.Prepared))
	for _, input := range q.Prepared {
		topic, ok := resolver.GetTopicForEvent(input.EventType)
		if !ok || topic != input.Destination {
			return nil, transaction.ErrBinding
		}
		if classes, ok := resolver.(catalog.DeliveryClassResolver); ok {
			delivery, exists := classes.GetDeliveryClass(input.EventType)
			if !exists || delivery != catalog.DeliveryClassDurableOutbox {
				return nil, transaction.ErrBinding
			}
		}
		msg, err := message.New(input)
		if err != nil {
			return nil, err
		}
		ref := standard.ReferenceFromMessage(msg)
		if _, err := standard.VerifyReference(msg.Input(), ref.Fingerprint, ref); err != nil {
			return nil, err
		}
		result = append(result, standard.PreparedIntent{Message: msg})
	}
	return result, nil
}

// Artifact reads the reservation winner without re-encoding business JSON.
func Artifact(p transaction.FrozenPlan, owner, revision string) ([]byte, error) {
	return artifactBody(p, owner, revision, nil)
}

// ArtifactWithRoutes accepts exact active or historical evidence. It grants no
// execution capability and never rebuilds a recorded plan or its first wire.
func ArtifactWithRoutes(p transaction.FrozenPlan, owner, revision string, routes *transport.RouteResolver) ([]byte, error) {
	if routes == nil {
		return nil, transaction.ErrBinding
	}
	return artifactBody(p, owner, revision, routes)
}
func artifactBody(p transaction.FrozenPlan, owner, revision string, routes *transport.RouteResolver) ([]byte, error) {
	if !p.Valid() || p.Input().Owner != owner {
		return nil, transaction.ErrInvalidPlan
	}
	var body []byte
	for _, branch := range p.Input().Branches {
		in := branch.Message.Input()
		var a artifact
		if json.Unmarshal(in.Payload, &a) != nil || a.Version != 1 || len(a.Body) == 0 || len(a.Body) > 1<<20 || !bytes.Equal(a.Wire, branch.Wire) {
			return nil, transaction.ErrInvalidPlan
		}
		canonical, err := json.Marshal(a)
		physical, routeErr := physicalRoute(in.Destination, owner, revision, branch.PhysicalTopic, routes)
		if err != nil || routeErr != nil || !bytes.Equal(canonical, in.Payload) || branch.ID != in.ID || branch.RouteRevision != revision || branch.PhysicalTopic != physical || in.Producer != "qs-server" {
			return nil, transaction.ErrInvalidPlan
		}
		in.Payload = a.Wire
		original, err := message.New(in)
		if err != nil {
			return nil, err
		}
		ref := standard.ReferenceFromMessage(original)
		if _, err := standard.VerifyReference(in, ref.Fingerprint, ref); err != nil {
			return nil, err
		}
		if body != nil && !bytes.Equal(body, a.Body) {
			return nil, fmt.Errorf("transaction branches bind different business artifacts")
		}
		body = bytes.Clone(a.Body)
	}
	return body, nil
}

func physicalRoute(topic, owner, revision, recorded string, routes *transport.RouteResolver) (string, error) {
	if routes == nil {
		return transport.PhysicalTopicV2(topic, transport.TransactionMessageType, owner, revision)
	}
	key := transport.RouteKey{Topic: topic, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: revision}
	var row transport.RouteBinding
	var err error
	if recorded == "" {
		row, err = routes.PublishRoute(key)
	} else {
		row, err = routes.RecordedRoute(key, recorded)
	}
	return row.PhysicalTopic, err
}
