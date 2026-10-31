package aibridge

import (
	"context"
	"database/sql"
	"errors"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
	"time"
)

// MessagingRuntime owns only the final event subscriber and its observations.
// The host owns the shared transaction sender, database pool and Checker.
type MessagingRuntime struct {
	rocket              *rocketmq.Subscriber
	Options             opts.AIWorkflowMessagingOptions
	DB                  *sql.DB
	Store               *store.MessagingStore
	Receiver            *store.MessagingEventReceiver
	CommandTransactions *store.TransactionCommands
	mu                  sync.Mutex
	started, stopping   bool
	handlerCancel       context.CancelFunc
	observations        *messagingCollector
}

func NewMessagingRuntime(o opts.AIWorkflowMessagingOptions, db *sql.DB, s *store.MessagingStore, r *store.MessagingEventReceiver) (*MessagingRuntime, error) {
	if o.SelectedProvider() != "rocketmq" {
		return nil, errors.New("AI runtime requires RocketMQ transaction owners")
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if !o.Enabled || db == nil || s == nil || r == nil || r.DB != db || r.Store != s || r.Transactions == nil || r.Bodies == nil || r.Seal == nil || len(r.Keys.Decrypt) == 0 || len(r.Keys.Signers) == 0 {
		return nil, errors.New("AI MQ runtime dependencies unavailable")
	}
	return &MessagingRuntime{Options: o, DB: db, Store: s, Receiver: r}, nil
}
func (r *MessagingRuntime) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return errors.New("AI MQ shutdown has not drained")
	}
	if r.started {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.preflightStorage(ctx); err != nil {
		return err
	}
	return r.startRocketMQ(ctx)
}
func (r *MessagingRuntime) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stop(ctx)
}
func (r *MessagingRuntime) stop(ctx context.Context) error {
	r.stopping = true
	if r.rocket != nil {
		if err := r.rocket.Close(ctx); err != nil {
			return err
		}
	}
	if r.handlerCancel != nil {
		r.handlerCancel()
	}
	if r.observations != nil {
		prometheus.DefaultRegisterer.Unregister(r.observations)
		r.observations = nil
	}
	r.rocket = nil
	r.handlerCancel = nil
	r.started = false
	r.stopping = false
	return nil
}

func (r *MessagingRuntime) preflightStorage(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if r.CommandTransactions != nil {
		if err := r.CommandTransactions.Validate(ctx); err != nil {
			return err
		}
	}
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return errors.New("AI MQ storage unavailable")
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := (store.MessagingAdmission{}).Inspect(ctx, tx); err != nil {
		return errors.New("AI MQ required runtime admission gate unavailable")
	}
	for _, query := range []string{"SELECT next_sequence FROM ai_messaging_aggregates LIMIT 0", "SELECT event_sequence FROM ai_messaging_evaluation_states LIMIT 0", "SELECT outcome FROM ai_messaging_inbox LIMIT 0", "SELECT wire,attempts FROM ai_messaging_failures LIMIT 0", "SELECT decision FROM ai_messaging_operations LIMIT 0", "SELECT wire_sha256 FROM ai_messaging_quarantine LIMIT 0", "SELECT kind,recorded_count,recording_since,last_observed_at FROM ai_messaging_observations LIMIT 0"} {
		rows, e := tx.QueryContext(ctx, query)
		if e != nil {
			return errors.New("AI MQ required schema unavailable")
		}
		if e = rows.Close(); e != nil {
			return errors.New("AI MQ storage unavailable")
		}
	}
	if err := store.RequireMessagingOperationSchema(ctx, tx); err != nil {
		return err
	}
	return store.RequireMessagingObservations(ctx, tx)
}
