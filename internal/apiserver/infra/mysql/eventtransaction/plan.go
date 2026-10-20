package eventtransaction

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

const owner = "qs.mysql"
const revision = "m3-1"

// The private plan binds the complete artifact and the original consumer wire.
// The Broker receives Branch.Wire, never this private payload.
type artifact struct {
	Version int    `json:"version"`
	Body    []byte `json:"body"`
	Wire    []byte `json:"wire"`
}

func freeze(q port.Request, resolver catalog.TopicResolver) (transaction.FrozenPlan, error) {
	if q.Key == "" || len(q.Artifact) == 0 || len(q.Artifact) > 1<<20 || len(q.Events) == 0 || len(q.Events) > transaction.MaxBranches {
		return transaction.FrozenPlan{}, transaction.ErrInvalidPlan
	}
	intents, err := standard.PrepareIntents(q.Events, resolver, "api-server")
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
		physical, err := transport.PhysicalTopicV2(in.Destination, transport.TransactionMessageType, owner, revision)
		if err != nil {
			return transaction.FrozenPlan{}, err
		}
		branches = append(branches, transaction.BranchInput{ID: in.ID, Message: msg, Wire: wire, PhysicalTopic: physical, RouteRevision: revision})
	}
	return transaction.NewPlan(transaction.PlanInput{Owner: owner, Scope: scope, OperationID: intents[0].Message.Input().ID, IdempotencyKey: q.Key, InputHash: q.InputHash, Branches: branches})
}

// Artifact reads the reservation winner without re-encoding business JSON.
func Artifact(p transaction.FrozenPlan) ([]byte, error) {
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
		physical, routeErr := transport.PhysicalTopicV2(in.Destination, transport.TransactionMessageType, owner, revision)
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
