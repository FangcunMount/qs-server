// Package eventtransaction binds one shared qs.mongo runtime to the original
// database. The host owns its client, transaction runners and runtime lifecycle.
package eventtransaction

import (
	"context"
	"errors"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	driveroptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

const ownerName = "qs.mongo"
const routeRevision = "m3-1"

type Owner struct {
	db        *mongo.Database
	primary   *mongo.Database
	authority *txmongo.Store
	runtime   *messagingruntime.TransactionRuntime
	sender    transaction.Sender
	topics    map[string]bool
}

// New is inert. All topics owned by this process must be supplied even when
// only an idle Checker will run. Schema and indexes are installed explicitly.
func New(db *mongo.Database, o *options.MessagingOptions, topics []string) (*Owner, error) {
	if db == nil || db.Client() == nil || db.Name() == "" || o == nil || o.RocketMQ == nil || len(topics) == 0 {
		return nil, transaction.ErrBinding
	}
	b, ok := o.RocketMQ.Transactions[ownerName]
	if !ok || b.RouteRevision != routeRevision {
		return nil, transaction.ErrBinding
	}
	allowed := make(map[string]bool, len(topics))
	for _, topic := range topics {
		if topic == "" || allowed[topic] {
			return nil, transaction.ErrBinding
		}
		allowed[topic] = true
	}
	store, err := txmongo.New(db, transaction.Binding{Owner: ownerName, StoreID: b.StoreID, WriterEpoch: b.WriterEpoch})
	if err != nil {
		return nil, err
	}
	native, err := messagingruntime.NewMongoNativeLimits(db, store.Binding(), b.RouteRevision)
	if err != nil {
		return nil, err
	}
	runtime, err := messagingruntime.NewTransactionalSender(o, ownerName, store, append([]string(nil), topics...), native)
	if err != nil {
		return nil, err
	}
	primary := db.Client().Database(db.Name(), driveroptions.Database().SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Majority()).SetWriteConcern(writeconcern.Majority()))
	return &Owner{db: db, primary: primary, authority: store, runtime: runtime, sender: runtime, topics: allowed}, nil
}

func (o *Owner) Runtime() *messagingruntime.TransactionRuntime { return o.runtime }
func (o *Owner) Binding() transaction.Binding                  { return o.authority.Binding() }

func (o *Owner) UsesDatabase(db *mongo.Database) bool {
	return o != nil && o.db != nil && db != nil && db.Client() == o.db.Client() && db.Name() == o.db.Name()
}

func (o *Owner) HasTopic(topic string) bool { return o != nil && o.topics[topic] }

func (o *Owner) validatePlan(p transaction.FrozenPlan) error {
	if o == nil || !p.Valid() || p.Input().Owner != ownerName {
		return transaction.ErrInvalidPlan
	}
	for _, b := range p.Input().Branches {
		in := b.Message.Input()
		row, err := o.runtime.Routes().RecordedRoute(transport.RouteKey{Topic: in.Destination, MessageType: transport.TransactionMessageType, Owner: ownerName, RouteRevision: routeRevision}, b.PhysicalTopic)
		if err != nil || !o.HasTopic(in.Destination) || b.PhysicalTopic != row.PhysicalTopic || b.RouteRevision != routeRevision || in.Producer != "qs-server" || len(b.Wire) > 262144 {
			return transaction.ErrBinding
		}
	}
	return nil
}

func (o *Owner) Reserve(ctx context.Context, q transaction.Request, p transaction.FrozenPlan) (transaction.Reservation, error) {
	if mongo.SessionFromContext(ctx) != nil {
		return transaction.Reservation{}, transaction.ErrTransactionRequired
	}
	if err := o.validatePlan(p); err != nil {
		return transaction.Reservation{}, err
	}
	r, err := o.runtime.Reserve(ctx, q, p)
	if err != nil {
		return r, err
	}
	if err := o.validatePlan(r.Execution.Plan); err != nil {
		return transaction.Reservation{}, err
	}
	return r, q.Match(r.Execution.Plan)
}

// Find recovers the stored stable input hash from the fixed primary, then lets
// SDK Lookup verify binding, original byte identity and the active execution.
func (o *Owner) Find(ctx context.Context, scope, key string) (transaction.Reservation, error) {
	if mongo.SessionFromContext(ctx) != nil {
		return transaction.Reservation{}, transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	q := transaction.Request{Owner: ownerName, Scope: scope, Key: key}
	if o == nil || !q.Valid() {
		return transaction.Reservation{}, transaction.ErrRequestIdentity
	}
	digest := q.KeyDigest()
	var stored struct {
		Owner string `bson:"owner"`
		Scope string `bson:"scope"`
		Key   string `bson:"idempotency_key"`
		Hash  []byte `bson:"input_hash"`
	}
	err := o.primary.Collection("rm_tx_request").FindOne(ctx, bson.M{"_id": digest[:]}).Decode(&stored)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return transaction.Reservation{}, transaction.ErrNotFound
	}
	if err != nil {
		return transaction.Reservation{}, err
	}
	if stored.Owner != ownerName || stored.Scope != scope || stored.Key != key || len(stored.Hash) != len(q.InputHash) {
		return transaction.Reservation{}, transaction.ErrRequestIdentity
	}
	copy(q.InputHash[:], stored.Hash)
	return o.Lookup(ctx, q)
}

func (o *Owner) Lookup(ctx context.Context, q transaction.Request) (transaction.Reservation, error) {
	if mongo.SessionFromContext(ctx) != nil {
		return transaction.Reservation{}, transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	r, err := o.authority.Lookup(ctx, q)
	if err != nil {
		return r, err
	}
	if err := o.validatePlan(r.Execution.Plan); err != nil {
		return transaction.Reservation{}, err
	}
	return r, nil
}

func (o *Owner) Evidence(ctx context.Context, ref transaction.Reference) (*transaction.Evidence, error) {
	if mongo.SessionFromContext(ctx) != nil {
		return nil, transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return o.authority.Evidence(ctx, ref)
}
