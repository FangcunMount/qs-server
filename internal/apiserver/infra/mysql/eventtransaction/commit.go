// Package eventtransaction joins the SDK to the original QS MySQL root.
package eventtransaction

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
)

// Binding returns protocol identity only; it transfers no database ownership.
func (c *Committer) Binding() transaction.Binding { return c.store.Binding() }

func (c *Committer) HasTopic(topic string) bool { return c != nil && c.topics[topic] }

// UsesPrimary validates adapters sharing this owner without transferring DB
// ownership or constructing another producer.
func (c *Committer) UsesPrimary(db *gorm.DB) bool {
	if db == nil || db.Statement == nil {
		return false
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return false
	}
	pool, err := db.DB()
	return err == nil && pool == c.primary
}

type sender interface {
	transaction.Sender
	Reserve(context.Context, transaction.Request, transaction.FrozenPlan) (transaction.Reservation, error)
}

type Committer struct {
	primary  *sql.DB
	store    *txmysql.Store
	uow      *hostmysql.UnitOfWork
	sender   sender
	runtime  *messagingruntime.TransactionRuntime
	resolver catalog.TopicResolver
	topics   map[string]bool
}

var _ port.Committer = (*Committer)(nil)

// New is inert. The store, runner and sender all borrow this exact original
// primary. The host starts/closes Runtime and explicitly installs schema.
func New(db *gorm.DB, o *options.MessagingOptions, resolver catalog.TopicResolver, topics []string) (*Committer, error) {
	if db == nil || o == nil || o.RocketMQ == nil || resolver == nil || len(topics) == 0 {
		return nil, fmt.Errorf("original QS MySQL primary, explicit configuration and routes required")
	}
	// A transaction handle must never become the long-lived primary authority.
	if db.Statement == nil {
		return nil, transaction.ErrTransactionRequired
	}
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return nil, transaction.ErrTransactionRequired
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	config, ok := o.RocketMQ.Transactions[owner]
	if !ok {
		return nil, transaction.ErrBinding
	}
	store, err := txmysql.New(pool, transaction.Binding{Owner: owner, StoreID: config.StoreID, WriterEpoch: config.WriterEpoch})
	if err != nil {
		return nil, err
	}
	native, err := messagingruntime.NewSQLNativeLimits(pool, store.Binding(), config.RouteRevision)
	if err != nil {
		return nil, err
	}
	runtime, err := messagingruntime.NewTransactionalSender(o, owner, store, topics, native)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(topics))
	for _, topic := range topics {
		allowed[topic] = true
	}
	return &Committer{primary: pool, store: store, uow: hostmysql.NewUnitOfWork(db), sender: runtime, runtime: runtime, resolver: resolver, topics: allowed}, nil
}

func (c *Committer) Runtime() *messagingruntime.TransactionRuntime     { return c.runtime }
func (c *Committer) Artifact(p transaction.FrozenPlan) ([]byte, error) { return Artifact(p) }

func (c *Committer) Find(ctx context.Context, scope, key string) (transaction.Reservation, error) {
	if _, nested := hostmysql.TxFromContext(ctx); nested {
		return transaction.Reservation{}, transaction.ErrTransactionRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	q := transaction.Request{Owner: owner, Scope: scope, Key: key}
	if !q.Valid() {
		return transaction.Reservation{}, transaction.ErrRequestIdentity
	}
	digest := q.KeyDigest()
	var actualOwner, actualScope, actualKey string
	var input []byte
	// The SDK requires the original input hash even for Lookup. Recover that
	// immutable hash from the fixed host primary, then let SDK Lookup validate
	// binding, raw identity, plan and active execution at its fresh boundary.
	err := c.primary.QueryRowContext(ctx, `SELECT owner,scope,idempotency_key,input_hash FROM rm_tx_request WHERE request_key=?`, digest[:]).Scan(&actualOwner, &actualScope, &actualKey, &input)
	if errors.Is(err, sql.ErrNoRows) {
		return transaction.Reservation{}, transaction.ErrNotFound
	}
	if err != nil {
		return transaction.Reservation{}, err
	}
	if actualOwner != owner || actualScope != scope || actualKey != key || len(input) != len(q.InputHash) {
		return transaction.Reservation{}, transaction.ErrRequestIdentity
	}
	copy(q.InputHash[:], input)
	r, err := c.store.Lookup(ctx, q)
	if err != nil {
		return transaction.Reservation{}, err
	}
	if err := c.validatePlan(r.Execution.Plan); err != nil {
		return transaction.Reservation{}, err
	}
	return r, nil
}

func (c *Committer) validatePlan(p transaction.FrozenPlan) error {
	if _, err := Artifact(p); err != nil {
		return err
	}
	for _, b := range p.Input().Branches {
		in := b.Message.Input()
		topic, ok := c.resolver.GetTopicForEvent(in.EventType)
		if !ok || topic != in.Destination || !c.topics[topic] {
			return transaction.ErrBinding
		}
	}
	return nil
}

func (c *Committer) Reserve(ctx context.Context, q port.Request) (transaction.Reservation, error) {
	if _, nested := hostmysql.TxFromContext(ctx); nested {
		return transaction.Reservation{}, transaction.ErrTransactionRequired
	}
	p, err := freeze(q, c.resolver)
	if err != nil {
		return transaction.Reservation{}, err
	}
	if err := c.validatePlan(p); err != nil {
		return transaction.Reservation{}, err
	}
	r, err := c.sender.Reserve(ctx, transaction.RequestFor(p), p)
	if err != nil {
		return transaction.Reservation{}, err
	}
	if err := c.validatePlan(r.Execution.Plan); err != nil {
		return transaction.Reservation{}, err
	}
	return r, transaction.RequestFor(p).Match(r.Execution.Plan)
}

func (c *Committer) Commit(ctx context.Context, e transaction.Execution, write func(context.Context) error) (transaction.Result, error) {
	if _, nested := hostmysql.TxFromContext(ctx); nested {
		return transaction.InitialResult(e), transaction.ErrTransactionRequired
	}
	if write == nil || !e.Valid() {
		return transaction.InitialResult(e), transaction.ErrInvalidPlan
	}
	if err := c.validatePlan(e.Plan); err != nil {
		return transaction.InitialResult(e), err
	}
	// Execute can return on cancellation while an admitted callback is still
	// running. Join it before returning host-owned result memory. Closing this
	// gate also prevents a late callback from opening a new business root.
	var mu sync.Mutex
	started, closed, rootCommitted := false, false, false
	done := make(chan struct{})
	r, err := c.sender.Execute(ctx, e, func(localCtx context.Context, actual transaction.Execution) (transaction.LocalOutcome, error) {
		mu.Lock()
		if closed || started {
			mu.Unlock()
			return transaction.LocalUnknown, transaction.ErrStaleExecution
		}
		started = true
		mu.Unlock()
		defer close(done)
		if actual.Plan.Digest() != e.Plan.Digest() || actual.Fence != e.Fence || actual.WriterEpoch != e.WriterEpoch || actual.Token != e.Token {
			return transaction.LocalUnknown, transaction.ErrStaleExecution
		}
		err := c.uow.WithinTransaction(localCtx, func(txCtx context.Context) error {
			tx, err := hostmysql.RequireTx(txCtx)
			if err != nil {
				return err
			}
			guard, err := c.store.BindGORM(txCtx, tx, e)
			if err != nil {
				return err
			}
			bound, closeRoot, err := bindOriginal(txCtx, c.primary, e)
			if err != nil {
				return err
			}
			defer closeRoot()
			if err := write(bound); err != nil {
				return err
			}
			return guard.CommitWitness(bound)
		})
		if err != nil {
			return transaction.LocalUnknown, err
		}
		mu.Lock()
		rootCommitted = true
		mu.Unlock()
		return transaction.LocalCommitted, nil
	})
	mu.Lock()
	closed = true
	join := started
	mu.Unlock()
	if join {
		<-done
	}
	mu.Lock()
	committed := rootCommitted
	mu.Unlock()
	if committed {
		r.Local = transaction.LocalCommitted
		return r, nil
	}
	// A fresh primary read resolves commit-response loss without invoking write
	// again. A canceled request cannot force a rollback or free a running root.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	r.Local = c.Confirm(readCtx, e)
	if r.Local == transaction.LocalCommitted {
		return r, nil
	}
	return r, errors.Join(port.ErrUnresolved, err)
}

func (c *Committer) Confirm(ctx context.Context, e transaction.Execution) transaction.LocalOutcome {
	if _, nested := hostmysql.TxFromContext(ctx); nested || c.validatePlan(e.Plan) != nil {
		return transaction.LocalUnknown
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	winner, err := c.store.Lookup(ctx, transaction.RequestFor(e.Plan))
	if err != nil || winner.Execution.Plan.Digest() != e.Plan.Digest() || winner.Execution.Fence != e.Fence || winner.Execution.WriterEpoch != e.WriterEpoch {
		return transaction.LocalUnknown
	}
	refs := e.References()
	facts := make([]*transaction.Evidence, 0, len(refs))
	for _, ref := range refs {
		fact, err := c.store.Evidence(ctx, ref)
		if err != nil {
			return transaction.LocalUnknown
		}
		facts = append(facts, fact)
	}
	return transaction.DecideLocal(refs, facts)
}
