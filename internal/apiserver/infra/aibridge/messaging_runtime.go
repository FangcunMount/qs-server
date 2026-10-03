package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdk "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	driver "github.com/nsqio/go-nsq"
	"github.com/prometheus/client_golang/prometheus"
)

// MessagingRuntime replaces the old host relay; New creates no clients or work.
// Start/Stop own only explicitly created NSQ/HTTP resources and one scan loop.
type MessagingRuntime struct {
	Options                    opts.AIWorkflowMessagingOptions
	DB                         *sql.DB
	Store                      *store.MessagingStore
	Receiver                   *store.MessagingEventReceiver
	mu                         sync.Mutex
	started                    bool
	stopping                   bool
	handlersReady              bool
	business, failure          *driver.Consumer
	binding                    *sdk.Subscription
	handoff                    *sdk.DirectHandoff
	publisher                  *sdk.ManagedPublisher
	httpTransport              *http.Transport
	relayCancel, handlerCancel context.CancelFunc
	relayDone                  chan struct{}
	observations               *messagingCollector
}
type rawMessagingKey struct{}
type fixedHandoff struct {
	*sdk.DirectHandoff
	sources map[string]string
}

func (h fixedHandoff) Ready(ctx context.Context, address, topic string) error {
	if _, ok := h.sources[address]; !ok {
		return errors.New("unconfigured NSQD failure source")
	}
	return h.DirectHandoff.Ready(ctx, address, topic)
}

func NewMessagingRuntime(o opts.AIWorkflowMessagingOptions, db *sql.DB, s *store.MessagingStore, r *store.MessagingEventReceiver) (*MessagingRuntime, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if !o.Enabled || db == nil || s == nil || r == nil || r.DB != db || r.Store != s || r.Bodies == nil || r.Seal == nil || len(r.Keys.Decrypt) == 0 || len(r.Keys.Signers) == 0 {
		return nil, errors.New("AI MQ runtime dependencies unavailable")
	}
	sources := map[string]string{}
	for tcp, http := range o.NSQD {
		sources[tcp] = http
	}
	o.NSQD = sources
	return &MessagingRuntime{Options: o, DB: db, Store: s, Receiver: r}, nil
}

func (r *MessagingRuntime) Start(ctx context.Context) (resultErr error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return errors.New("AI MQ shutdown has not drained")
	}
	if r.started {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := r.preflightStorage(ctx); err != nil {
		return err
	}
	r.httpTransport = http.DefaultTransport.(*http.Transport).Clone()
	defer func() {
		if resultErr != nil {
			stopCtx, c := context.WithTimeout(context.Background(), 20*time.Second)
			defer c()
			if e := r.stop(stopCtx); e != nil {
				resultErr = errors.Join(resultErr, e)
			}
		}
	}()
	client := &http.Client{Transport: r.httpTransport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("topology redirects disabled") }}
	if err := PreflightMessagingTopology(ctx, client, r.Options.NSQD); err != nil {
		r.httpTransport.CloseIdleConnections()
		return err
	}
	cfg := driver.NewConfig()
	cfg.MaxInFlight = 1
	cfg.MaxAttempts = 0
	cfg.DialTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.HeartbeatInterval = time.Second
	failedTopic := legacy.FailedHandoffTopic(app.EventsTopic, "qs-server.ai-events.v1")
	var err error
	if r.failure, err = driver.NewConsumer(failedTopic, legacy.FailedHandoffChannel, cfg); err != nil {
		return err
	}
	if r.business, err = driver.NewConsumer(app.EventsTopic, "qs-server.ai-events.v1", cfg); err != nil {
		return err
	}
	logger := log.New(io.Discard, "", 0)
	r.failure.SetLogger(logger, driver.LogLevelError)
	r.business.SetLogger(logger, driver.LogLevelError)
	if r.handoff, err = sdk.NewDirectHandoff(r.failure, cfg, 1); err != nil {
		return err
	}
	handlerCtx, cancel := context.WithCancel(context.Background())
	r.handlerCancel = cancel
	r.binding, err = sdk.NewSubscription(sdk.SubscriptionConfig{Topic: app.EventsTopic, Channel: "qs-server.ai-events.v1", MaxAttempts: 8,
		Retry: sdk.Backoff{BaseDelay: time.Second, MaxDelay: 60 * time.Second}, Handoff: fixedHandoff{r.handoff, r.Options.NSQD},
		Handler: func(ctx context.Context, _ transport.Delivery) error {
			raw, ok := ctx.Value(rawMessagingKey{}).([]byte)
			if !ok {
				return errors.New("original wire unavailable")
			}
			return r.Receiver.Receive(ctx, raw)
		},
		FailedHandler: func(ctx context.Context, f legacy.FailedHandoff) error {
			audit, _ := ctx.Value(rawMessagingKey{}).([]byte)
			if f.Topic != app.EventsTopic || f.Channel != "qs-server.ai-events.v1" {
				return r.Receiver.Isolate(ctx, audit, "authentication_failed")
			}
			wire, e := legacy.Encode(legacy.Envelope{UUID: f.UUID, Payload: f.Payload, Metadata: f.Metadata}, legacy.Revision2)
			if e != nil {
				return r.Receiver.Isolate(ctx, audit, "authentication_failed")
			}
			return r.Receiver.FailedHandoff(ctx, wire, audit)
		},
	})
	if err != nil {
		return err
	}
	r.failure.AddHandler(driver.HandlerFunc(func(m *driver.Message) error {
		return r.binding.HandleFailure(context.WithValue(handlerCtx, rawMessagingKey{}, append([]byte(nil), m.Body...)), m)
	}))
	r.business.AddHandler(driver.HandlerFunc(func(m *driver.Message) error {
		if _, err := app.AuthenticateMessaging(m.Body, app.EventsTopic, r.Receiver.Keys); err != nil {
			return r.Receiver.Isolate(handlerCtx, m.Body, "authentication_failed")
		}
		return r.binding.HandleBusiness(context.WithValue(handlerCtx, rawMessagingKey{}, append([]byte(nil), m.Body...)), m)
	}))
	r.handlersReady = true
	addresses := make([]string, 0, len(r.Options.NSQD))
	for address := range r.Options.NSQD {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	for _, address := range addresses {
		if err = r.failure.ConnectToNSQD(address); err != nil {
			return err
		}
	}
	for _, address := range addresses {
		if err = r.business.ConnectToNSQD(address); err != nil {
			return err
		}
	}
	r.publisher, err = sdk.NewManagedPublisher(sdk.ManagedPublisherConfig{Address: addresses[0], Driver: cfg, MaxInFlight: 1})
	if err != nil {
		return err
	}
	relay, err := store.NewMessagingRelay(r.DB, r.Store.Outbox, r.publisher)
	if err != nil {
		return err
	}
	observations := newMessagingCollector(r.DB)
	if err = prometheus.DefaultRegisterer.Register(observations); err != nil {
		return errors.New("AI MQ observation registration unavailable")
	}
	r.observations = observations
	relayCtx, relayCancel := context.WithCancel(ctx)
	r.relayCancel = relayCancel
	r.relayDone = make(chan struct{})
	go func() {
		defer close(r.relayDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if err := relay.Step(relayCtx); err != nil && relayCtx.Err() == nil {
				log.Print("AI MQ relay storage unavailable")
			}
			select {
			case <-relayCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	r.started = true
	return nil
}

func (r *MessagingRuntime) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stop(ctx)
}
func (r *MessagingRuntime) stop(ctx context.Context) error {
	r.stopping = true
	if r.relayCancel != nil {
		r.relayCancel()
	}
	for _, c := range []*driver.Consumer{r.business, r.failure} {
		if c != nil {
			if !r.handlersReady {
				c.AddHandler(driver.HandlerFunc(func(*driver.Message) error { return errors.New("AI MQ startup aborted") }))
			}
			c.ChangeMaxInFlight(0)
			c.Stop()
		}
	}
	for _, c := range []*driver.Consumer{r.business, r.failure} {
		if c != nil {
			select {
			case <-c.StopChan:
			case <-ctx.Done():
				if r.handlerCancel != nil {
					r.handlerCancel()
				}
				return ctx.Err()
			}
		}
	}
	if r.relayDone != nil {
		select {
		case <-r.relayDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if r.binding != nil {
		if err := r.binding.WaitIdle(ctx); err != nil {
			return err
		}
	}
	if r.handoff != nil {
		if err := r.handoff.Close(ctx); err != nil {
			return err
		}
	}
	if r.publisher != nil {
		if err := r.publisher.Close(ctx); err != nil {
			r.publisher.Interrupt()
			return err
		}
	}
	if r.handlerCancel != nil {
		r.handlerCancel()
	}
	if r.httpTransport != nil {
		r.httpTransport.CloseIdleConnections()
	}
	if r.observations != nil {
		prometheus.DefaultRegisterer.Unregister(r.observations)
		r.observations = nil
	}
	r.started = false
	r.stopping = false
	r.handlersReady = false
	r.business, r.failure, r.binding, r.handoff, r.publisher = nil, nil, nil, nil, nil
	r.relayCancel, r.handlerCancel, r.relayDone, r.httpTransport = nil, nil, nil, nil
	return nil
}

func (r *MessagingRuntime) preflightStorage(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return errors.New("AI MQ storage unavailable")
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := (store.MessagingAdmission{}).Inspect(ctx, tx); err != nil {
		return errors.New("AI MQ required runtime admission gate unavailable")
	}
	for _, query := range []string{"SELECT next_sequence FROM ai_messaging_aggregates LIMIT 0", "SELECT event_sequence FROM ai_messaging_evaluation_states LIMIT 0", "SELECT outcome FROM ai_messaging_inbox LIMIT 0", "SELECT wire,attempts FROM ai_messaging_failures LIMIT 0", "SELECT stage,wire FROM ai_messaging_outbox LIMIT 0", "SELECT decision FROM ai_messaging_operations LIMIT 0", "SELECT wire_sha256 FROM ai_messaging_quarantine LIMIT 0", "SELECT kind,recorded_count,recording_since,last_observed_at FROM ai_messaging_observations LIMIT 0"} {
		rows, e := tx.QueryContext(ctx, query)
		if e != nil {
			return errors.New("AI MQ required schema unavailable")
		}
		if e = rows.Close(); e != nil {
			return errors.New("AI MQ storage unavailable")
		}
	}
	return store.RequireMessagingObservations(ctx, tx)
}
