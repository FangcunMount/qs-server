package subsystem

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/FangcunMount/component-base/pkg/logger"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type ConsumerHandler func(context.Context, string, []byte) error
type SDKSubscriber interface {
	Subscribe(string, string, rmtransport.Handler) error
	Stop()
	Close() error
}
type SDKSubscriberFactory func() (SDKSubscriber, error)

type consumerSubscriber interface {
	Stop()
	Close() error
}

type ConsumerOptions struct {
	Enabled bool
	Channel string
}

type Options struct {
	MySQLDB              *gorm.DB
	MongoDB              *mongo.Database
	Catalog              *eventcatalog.Catalog
	WirePublisher        eventruntime.WirePublisher
	PublisherMode        eventruntime.PublishMode
	SDKSubscriberFactory SDKSubscriberFactory
	Consumers            map[string]ConsumerOptions
	Observer             eventobservability.Observer
}

type consumerRuntime struct {
	spec       eventcatalog.ConsumerSpec
	eventType  string
	topic      string
	enabled    bool
	handler    ConsumerHandler
	subscriber consumerSubscriber
	healthy    bool
	lastError  string
}

type Subsystem struct {
	mu                   sync.Mutex
	catalog              *eventcatalog.Catalog
	registry             *eventcatalog.EffectiveRegistry
	publisher            *eventruntime.RoutingPublisher
	consumers            map[string]*consumerRuntime
	sdkSubscriberFactory SDKSubscriberFactory
	observer             eventobservability.Observer
	started              bool
	closed               bool
	cancel               context.CancelFunc
	closeStarted         bool
	closeDone            chan struct{}
	closeErr             error
	startDone            chan struct{}
	closeAsyncOnce       sync.Once
	transactions         *transactionState
}

// New constructs consumer-only wiring. Durable hosts require transaction owners.
func New(opts Options) (*Subsystem, error) {
	if opts.MongoDB != nil || opts.MySQLDB != nil {
		return nil, errors.New("durable host databases require transaction owners")
	}
	s, err := newBase(opts)
	if err != nil {
		return nil, err
	}
	s.buildConsumers(opts.Consumers)
	return s, nil
}

func newBase(opts Options) (*Subsystem, error) {
	registry, err := eventcatalog.NewEffectiveRegistry(opts.Catalog, eventcatalog.DefaultSpecs())
	if err != nil {
		return nil, err
	}
	if opts.Observer == nil {
		opts.Observer = eventobservability.DefaultObserver()
	}
	publisher := eventruntime.NewRoutingPublisher(eventruntime.RoutingPublisherOptions{
		Catalog: opts.Catalog, WirePublisher: opts.WirePublisher, Mode: opts.PublisherMode,
		Source: eventruntime.SourceAPIServer, Observer: opts.Observer,
	})
	s := &Subsystem{
		catalog: opts.Catalog, registry: registry, publisher: publisher,
		consumers:            make(map[string]*consumerRuntime),
		sdkSubscriberFactory: opts.SDKSubscriberFactory,
		observer:             opts.Observer,
		closeDone:            make(chan struct{}),
	}
	return s, nil
}

func (s *Subsystem) buildConsumers(options map[string]ConsumerOptions) {
	for _, evt := range s.registry.Snapshot() {
		for _, spec := range evt.AdditionalConsumers {
			configured, ok := options[spec.ID]
			enabled := true
			if ok {
				enabled = configured.Enabled
				if configured.Channel != "" {
					spec.Channel = configured.Channel
				}
			}
			s.consumers[spec.ID] = &consumerRuntime{spec: spec, eventType: evt.Type, topic: evt.Topic, enabled: enabled}
		}
	}
}

func (s *Subsystem) Catalog() *eventcatalog.Catalog            { return s.catalog }
func (s *Subsystem) Registry() *eventcatalog.EffectiveRegistry { return s.registry }
func (s *Subsystem) Publisher() event.EventPublisher           { return s.publisher }

// Profile retains the module binding boundary without a publication fallback.
// Durable module operations require their explicit transaction owner.
func (s *Subsystem) Profile(_ eventcatalog.OutboxProfile) appEventing.ProfileBinding {
	return appEventing.ProfileBinding{}
}

func (s *Subsystem) RegisterConsumer(id string, handler ConsumerHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.closed {
		return fmt.Errorf("event subsystem consumer registration is closed")
	}
	consumer, ok := s.consumers[id]
	if !ok {
		return fmt.Errorf("consumer %q is not declared", id)
	}
	if consumer.handler != nil {
		return fmt.Errorf("consumer %q is already registered", id)
	}
	if handler == nil {
		return fmt.Errorf("consumer %q handler is nil", id)
	}
	consumer.handler = handler
	return nil
}

func (s *Subsystem) Start(parent context.Context) (startErr error) {
	if parent == nil {
		parent = context.Background()
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("event subsystem is closed")
	}
	if s.started {
		s.mu.Unlock()
		return nil
	}
	for _, id := range s.orderedConsumerIDs() {
		consumer := s.consumers[id]
		if s.consumerEnabled(consumer) && consumer.handler == nil {
			s.mu.Unlock()
			return fmt.Errorf("event consumer %q has no runtime binding", id)
		}
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.started = true
	s.startDone = make(chan struct{})
	startDone := s.startDone
	s.mu.Unlock()
	defer func() {
		// Close must see every subscriber created by this startup, including
		// one whose Subscribe was still running when shutdown was requested.
		close(startDone)
		if startErr != nil {
			startErr = errors.Join(startErr, s.Close())
		}
	}()
	if err := s.startTransactionOwners(ctx); err != nil {
		return err
	}

	if s.publisher.IsMQBacked() {
		if err := s.startConsumers(ctx); err != nil {
			return err
		}
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return errors.New("event subsystem closed during startup")
	}
	return nil
}

func (s *Subsystem) startConsumers(ctx context.Context) error {
	if s.sdkSubscriberFactory == nil && s.hasEnabledConsumers() {
		return fmt.Errorf("event consumer subscriber factory is not configured")
	}
	for _, id := range s.orderedConsumerIDs() {
		consumer := s.consumers[id]
		if !s.consumerEnabled(consumer) {
			continue
		}
		subscriber, err := s.sdkSubscriberFactory()
		if err != nil {
			s.setConsumerError(consumer, err)
			return fmt.Errorf("create subscriber for %s: %w", consumer.spec.ID, err)
		}
		if subscriber == nil {
			err = errors.New("event consumer subscriber factory returned nil")
			s.setConsumerError(consumer, err)
			return fmt.Errorf("create subscriber for %s: %w", consumer.spec.ID, err)
		}
		s.mu.Lock()
		consumer.subscriber = subscriber
		s.mu.Unlock()
		if err := subscriber.Subscribe(consumer.topic, consumer.spec.Channel, s.consumerSDKMessageHandler(consumer)); err != nil {
			s.setConsumerError(consumer, err)
			return fmt.Errorf("subscribe consumer %s: %w", consumer.spec.ID, err)
		}
		s.setConsumerHealthy(consumer)
		logger.L(ctx).Infow("event projection consumer started", "consumer", consumer.spec.ID, "topic", consumer.topic, "channel", consumer.spec.Channel)
	}
	return nil
}

func (s *Subsystem) consumerSDKMessageHandler(consumer *consumerRuntime) rmtransport.Handler {
	observe := func(ctx context.Context, msg rmtransport.Received, eventType string, outcome eventobservability.ConsumeOutcome) {
		s.observer.ObserveConsume(ctx, eventobservability.ConsumeEvent{
			Service: consumer.spec.ID, Topic: consumer.topic, EventType: eventType,
			Outcome: outcome, Attempts: max(int(msg.Attempts), 1),
		})
	}
	ack := func(ctx context.Context, delivery rmtransport.Delivery, msg rmtransport.Received, eventType string, success, failure eventobservability.ConsumeOutcome) error {
		if err := delivery.Ack(); err != nil {
			slog.Warn("failed to ack event projection delivery", "consumer", consumer.spec.ID, "topic", consumer.topic, "msg_id", msg.ID, "error", err)
			observe(ctx, msg, eventType, failure)
			return err
		}
		observe(ctx, msg, eventType, success)
		return nil
	}
	return func(ctx context.Context, delivery rmtransport.Delivery) error {
		if delivery == nil {
			return errors.New("event projection SDK delivery is nil")
		}
		msg := delivery.Message()
		eventType := msg.Metadata["event_type"]
		if eventType == "" {
			envelope, err := domainwire.DecodeEnvelope(msg.Payload)
			if err != nil {
				slog.Warn("event projection delivery missing event_type", "consumer", consumer.spec.ID, "topic", consumer.topic, "msg_id", msg.ID, "error", err)
				observe(ctx, msg, "", eventobservability.ConsumeOutcomeDecodeFailed)
				return err
			}
			eventType = envelope.EventType
			if eventType == "" {
				slog.Warn("event projection delivery has empty event_type", "consumer", consumer.spec.ID, "topic", consumer.topic, "msg_id", msg.ID)
				observe(ctx, msg, "", eventobservability.ConsumeOutcomeDecodeFailed)
				return errors.New("event envelope has no event_type")
			}
		}
		if eventType != consumer.eventType {
			return ack(ctx, delivery, msg, eventType, eventobservability.ConsumeOutcomeUnknownAcked, eventobservability.ConsumeOutcomeUnknownAckFailed)
		}
		if err := consumer.handler(ctx, eventType, msg.Payload); err != nil {
			s.setConsumerError(consumer, err)
			slog.Error("failed to dispatch event projection", "consumer", consumer.spec.ID, "topic", consumer.topic, "event_type", eventType, "msg_id", msg.ID, "error", err)
			observe(ctx, msg, eventType, eventobservability.ConsumeOutcomeDispatchFailed)
			return err
		}
		s.setConsumerHealthy(consumer)
		return ack(ctx, delivery, msg, eventType, eventobservability.ConsumeOutcomeAcked, eventobservability.ConsumeOutcomeAckFailed)
	}
}

func (s *Subsystem) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closeDone == nil {
		s.closeDone = make(chan struct{})
	}
	if s.closeStarted {
		done := s.closeDone
		s.mu.Unlock()
		<-done
		s.mu.Lock()
		err := s.closeErr
		s.mu.Unlock()
		return err
	}
	s.closeStarted = true
	s.closed = true
	s.pauseTransactionAdmission()
	cancel := s.cancel
	startDone := s.startDone
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if startDone != nil {
		<-startDone
	}
	s.mu.Lock()
	consumerIDs := s.orderedConsumerIDs()
	consumers := make([]*consumerRuntime, 0, len(consumerIDs))
	for _, id := range consumerIDs {
		consumers = append(consumers, s.consumers[id])
	}
	s.mu.Unlock()
	var closeErrors []error
	for i := len(consumers) - 1; i >= 0; i-- {
		consumer := consumers[i]
		if consumer.subscriber != nil {
			consumer.subscriber.Stop()
			if err := consumer.subscriber.Close(); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close event consumer %s: %w", consumer.spec.ID, err))
			}
		}
	}
	if err := s.closeTransactionOwners(); err != nil {
		closeErrors = append(closeErrors, err)
	}
	closeErr := errors.Join(closeErrors...)
	s.mu.Lock()
	for _, consumer := range consumers {
		consumer.healthy = false
	}
	s.closeErr = closeErr
	close(s.closeDone)
	s.mu.Unlock()
	return closeErr
}

func (s *Subsystem) orderedConsumerIDs() []string {
	ids := make([]string, 0, len(s.consumers))
	for id := range s.consumers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (s *Subsystem) StatusService() appEventing.StatusService {
	if s == nil {
		return nil
	}
	outboxes := s.Outboxes()
	return appEventing.NewStatusService(appEventing.StatusServiceOptions{
		Catalog: s.catalog, Registry: s.registry, Outboxes: outboxes, RuntimeSnapshot: s.runtimeStatusSnapshot,
	})
}

func (s *Subsystem) consumerEnabled(consumer *consumerRuntime) bool {
	return consumer != nil && consumer.enabled && s.publisher != nil && s.publisher.IsMQBacked()
}

func (s *Subsystem) hasEnabledConsumers() bool {
	for _, consumer := range s.consumers {
		if s.consumerEnabled(consumer) {
			return true
		}
	}
	return false
}

func (s *Subsystem) setConsumerError(consumer *consumerRuntime, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	consumer.healthy = false
	if err != nil {
		consumer.lastError = err.Error()
	}
}

func (s *Subsystem) setConsumerHealthy(consumer *consumerRuntime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	consumer.healthy = true
	consumer.lastError = ""
}

func (s *Subsystem) runtimeStatusSnapshot() appEventing.RuntimeStatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := appEventing.RuntimeStatusSnapshot{
		Profiles:  make(map[eventcatalog.OutboxProfile]appEventing.ProfileRuntimeStatus, 0),
		Consumers: make(map[string]appEventing.ConsumerRuntimeStatus, len(s.consumers)),
	}
	for id, consumer := range s.consumers {
		result.Consumers[id] = appEventing.ConsumerRuntimeStatus{
			Topic: consumer.topic, Enabled: s.consumerEnabled(consumer), Healthy: consumer.healthy, LastError: consumer.lastError,
		}
	}
	return result
}

// Outboxes is empty for the current transaction-only host. Historical storage
// readers are maintenance dependencies, never an active publication profile.
func (s *Subsystem) Outboxes() []appEventing.NamedOutboxStatusReader { return nil }
