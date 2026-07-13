package subsystem

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/messaging"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	mongoEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	mysqlEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/outboxready"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/eventruntime"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type ConsumerHandler func(context.Context, string, []byte) error
type SubscriberFactory func() (messaging.Subscriber, error)

type ConsumerOptions struct {
	Enabled bool
	Channel string
}

type ProfileOptions struct {
	Interval               time.Duration
	BatchSize              int
	PublishWorkers         int
	ImmediateMaxConcurrent int
}

type Options struct {
	MySQLDB           *gorm.DB
	MongoDB           *mongo.Database
	OpsRedis          redis.UniversalClient
	Catalog           *eventcatalog.Catalog
	MQPublisher       messaging.Publisher
	PublisherMode     eventruntime.PublishMode
	MySQLLimiter      backpressure.Acquirer
	MongoLimiter      backpressure.Acquirer
	Mongo             ProfileOptions
	Assessment        ProfileOptions
	SubscriberFactory SubscriberFactory
	Consumers         map[string]ConsumerOptions
	Observer          eventobservability.Observer
}

type profileRuntime struct {
	name       string
	binding    appEventing.ProfileBinding
	relay      appEventing.OutboxRelay
	immediate  *appEventing.ImmediateDispatcher
	readyIndex *outboxready.Index
	reconciler *outboxready.Reconciler
	status     appEventing.NamedOutboxStatusReader
	interval   time.Duration
}

type consumerRuntime struct {
	spec       eventcatalog.ConsumerSpec
	eventType  string
	topic      string
	enabled    bool
	handler    ConsumerHandler
	subscriber messaging.Subscriber
	healthy    bool
	lastError  string
}

type Subsystem struct {
	mu                sync.Mutex
	catalog           *eventcatalog.Catalog
	registry          *eventcatalog.EffectiveRegistry
	publisher         *eventruntime.RoutingPublisher
	profiles          map[eventcatalog.OutboxProfile]*profileRuntime
	consumers         map[string]*consumerRuntime
	subscriberFactory SubscriberFactory
	observer          eventobservability.Observer
	started           bool
	closed            bool
	cancel            context.CancelFunc
	wg                sync.WaitGroup
}

func New(opts Options) (*Subsystem, error) {
	registry, err := eventcatalog.NewEffectiveRegistry(opts.Catalog, eventcatalog.DefaultSpecs())
	if err != nil {
		return nil, err
	}
	if opts.Observer == nil {
		opts.Observer = eventobservability.DefaultObserver()
	}
	publisher := eventruntime.NewRoutingPublisher(eventruntime.RoutingPublisherOptions{
		Catalog: opts.Catalog, MQPublisher: opts.MQPublisher, Mode: opts.PublisherMode,
		Source: event.SourceAPIServer, Observer: opts.Observer,
	})
	s := &Subsystem{
		catalog: opts.Catalog, registry: registry, publisher: publisher,
		profiles:  make(map[eventcatalog.OutboxProfile]*profileRuntime),
		consumers: make(map[string]*consumerRuntime), subscriberFactory: opts.SubscriberFactory,
		observer: opts.Observer,
	}
	if err := s.buildMongoProfile(opts); err != nil {
		return nil, err
	}
	if err := s.buildAssessmentProfile(opts); err != nil {
		return nil, err
	}
	s.buildConsumers(opts.Consumers)
	return s, nil
}

func (s *Subsystem) buildMongoProfile(opts Options) error {
	if opts.MongoDB == nil {
		return nil
	}
	storeOpts := []mongoEventOutbox.StoreOption{mongoEventOutbox.WithPriorityTiers(s.registry.PriorityTiers(eventcatalog.OutboxProfileMongoDomain))}
	if opts.MongoLimiter != nil {
		storeOpts = append(storeOpts, mongoEventOutbox.WithLimiter(opts.MongoLimiter))
	}
	store, err := mongoEventOutbox.NewStoreWithTopicResolver(opts.MongoDB, s.catalog, storeOpts...)
	if err != nil {
		return err
	}
	ready := outboxready.NewIndexWithRegistry(opts.OpsRedis, outboxready.StoreMongoDomainEvents, s.registry)
	immediate := appEventing.NewImmediateDispatcher(appEventing.ImmediateDispatcherOptions{
		Name: "mongo-domain-events", Store: store, Publisher: s.publisher, Observer: s.observer,
		Enabled: true, RequireDurablePublisher: true, MaxConcurrent: opts.Mongo.ImmediateMaxConcurrent,
		ReadyIndex: ready, ImmediateEventTypes: s.registry.ImmediateTypes(eventcatalog.OutboxProfileMongoDomain),
	})
	relay := appEventing.NewOutboxRelayWithOptions(appEventing.OutboxRelayOptions{
		Name: "mongo-domain-events", Store: store, Publisher: s.publisher, Observer: s.observer,
		BatchSize: opts.Mongo.BatchSize, PublishWorkers: opts.Mongo.PublishWorkers,
		RequireDurablePublisher: true, ReadyIndex: ready, ReadyBuckets: s.registry.ReadyIndexBuckets(),
	})
	s.profiles[eventcatalog.OutboxProfileMongoDomain] = &profileRuntime{
		name: "mongo-domain-events", binding: appEventing.ProfileBinding{Stager: store, PostCommit: immediate},
		relay: relay, immediate: immediate, readyIndex: ready, reconciler: outboxready.NewReconciler(ready, store, 0),
		status:   appEventing.NamedOutboxStatusReader{Name: "mongo-domain-events", Reader: store},
		interval: normalizedInterval(opts.Mongo.Interval, 500*time.Millisecond),
	}
	return nil
}

func (s *Subsystem) buildAssessmentProfile(opts Options) error {
	if opts.MySQLDB == nil {
		return nil
	}
	store := mysqlEventOutbox.NewStoreWithTopicResolver(opts.MySQLDB, s.catalog,
		mysqlEventOutbox.WithPriorityTiers(s.registry.PriorityTiers(eventcatalog.OutboxProfileAssessmentMySQL)))
	ready := outboxready.NewIndexWithRegistry(opts.OpsRedis, outboxready.StoreAssessmentMySQLOutbox, s.registry)
	immediate := appEventing.NewImmediateDispatcher(appEventing.ImmediateDispatcherOptions{
		Name: "assessment-mysql-outbox", Store: store, Publisher: s.publisher, Observer: s.observer,
		Enabled: true, RequireDurablePublisher: true, MaxConcurrent: opts.Assessment.ImmediateMaxConcurrent,
		ReadyIndex: ready, ImmediateEventTypes: s.registry.ImmediateTypes(eventcatalog.OutboxProfileAssessmentMySQL),
	})
	relay := appEventing.NewOutboxRelayWithOptions(appEventing.OutboxRelayOptions{
		Name: "assessment-mysql-outbox", Store: store, Publisher: s.publisher, Observer: s.observer,
		BatchSize: opts.Assessment.BatchSize, PublishWorkers: opts.Assessment.PublishWorkers,
		RequireDurablePublisher: true, ReadyIndex: ready, ReadyBuckets: s.registry.ReadyIndexBuckets(),
	})
	s.profiles[eventcatalog.OutboxProfileAssessmentMySQL] = &profileRuntime{
		name: "assessment-mysql-outbox", binding: appEventing.ProfileBinding{Stager: store, PostCommit: immediate},
		relay: relay, immediate: immediate, readyIndex: ready, reconciler: outboxready.NewReconciler(ready, store, 0),
		status:   appEventing.NamedOutboxStatusReader{Name: "assessment-mysql-outbox", Reader: store},
		interval: normalizedInterval(opts.Assessment.Interval, 500*time.Millisecond),
	}
	return nil
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

func normalizedInterval(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func (s *Subsystem) Catalog() *eventcatalog.Catalog            { return s.catalog }
func (s *Subsystem) Registry() *eventcatalog.EffectiveRegistry { return s.registry }
func (s *Subsystem) Publisher() event.EventPublisher           { return s.publisher }

func (s *Subsystem) Profile(profile eventcatalog.OutboxProfile) appEventing.ProfileBinding {
	if s == nil || s.profiles[profile] == nil {
		return appEventing.ProfileBinding{}
	}
	return s.profiles[profile].binding
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

func (s *Subsystem) Start(parent context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("event subsystem is closed")
	}
	if s.started {
		s.mu.Unlock()
		return nil
	}
	for id, consumer := range s.consumers {
		if s.consumerEnabled(consumer) && consumer.handler == nil {
			s.mu.Unlock()
			return fmt.Errorf("event consumer %q has no runtime binding", id)
		}
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.started = true
	s.mu.Unlock()

	if s.publisher.IsMQBacked() {
		if err := s.startConsumers(ctx); err != nil {
			_ = s.Close()
			return err
		}
	}
	for _, profile := range s.profiles {
		if profile.reconciler != nil {
			profile.reconciler.Start(ctx)
		}
		if profile.relay != nil {
			s.startRelay(ctx, profile)
		}
	}
	return nil
}

func (s *Subsystem) startConsumers(ctx context.Context) error {
	if s.subscriberFactory == nil && s.hasEnabledConsumers() {
		return fmt.Errorf("event consumer subscriber factory is not configured")
	}
	for _, consumer := range s.consumers {
		if !s.consumerEnabled(consumer) {
			continue
		}
		subscriber, err := s.subscriberFactory()
		if err != nil {
			s.setConsumerError(consumer, err)
			return fmt.Errorf("create subscriber for %s: %w", consumer.spec.ID, err)
		}
		consumer.subscriber = subscriber
		if err := subscriber.Subscribe(consumer.topic, consumer.spec.Channel, s.consumerMessageHandler(consumer)); err != nil {
			s.setConsumerError(consumer, err)
			return fmt.Errorf("subscribe consumer %s: %w", consumer.spec.ID, err)
		}
		s.setConsumerHealthy(consumer)
		logger.L(ctx).Infow("event projection consumer started", "consumer", consumer.spec.ID, "topic", consumer.topic, "channel", consumer.spec.Channel)
	}
	return nil
}

func (s *Subsystem) consumerMessageHandler(consumer *consumerRuntime) messaging.Handler {
	extractor := eventruntime.MessageEventExtractor{}
	settlement := eventruntime.NewMessageSettlementPolicy(slog.Default(), consumer.spec.ID, consumer.topic, s.observer)
	return func(ctx context.Context, msg *messaging.Message) error {
		eventType, err := extractor.Extract(msg)
		if err != nil {
			settlement.AckInvalid(msg, err)
			return nil
		}
		if eventType != consumer.eventType {
			_, err := settlement.AckUnknown(msg)
			return err
		}
		if err := consumer.handler(ctx, eventType, msg.Payload); err != nil {
			s.setConsumerError(consumer, err)
			settlement.NackFailed(msg, eventType, err)
			return err
		}
		s.setConsumerHealthy(consumer)
		_, err = settlement.AckSuccess(msg)
		return err
	}
}

func (s *Subsystem) startRelay(ctx context.Context, profile *profileRuntime) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(profile.interval)
		defer ticker.Stop()
		for {
			if err := profile.relay.DispatchDue(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("event outbox relay failed", "profile", profile.name, "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Subsystem) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancel := s.cancel
	consumers := make([]*consumerRuntime, 0, len(s.consumers))
	for _, consumer := range s.consumers {
		consumers = append(consumers, consumer)
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	for _, profile := range s.profiles {
		if profile.reconciler != nil {
			profile.reconciler.Close()
		}
		if profile.immediate != nil {
			profile.immediate.Close()
		}
	}
	for _, consumer := range consumers {
		if consumer.subscriber != nil {
			consumer.subscriber.Stop()
			if err := consumer.subscriber.Close(); err != nil {
				return err
			}
		}
	}
	return nil
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
	return consumer != nil && consumer.enabled && s.publisher.IsMQBacked()
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
		Profiles:  make(map[eventcatalog.OutboxProfile]appEventing.ProfileRuntimeStatus, len(s.profiles)),
		Consumers: make(map[string]appEventing.ConsumerRuntimeStatus, len(s.consumers)),
	}
	for profile, runtime := range s.profiles {
		result.Profiles[profile] = appEventing.ProfileRuntimeStatus{
			Running: s.started && !s.closed, RelayEnabled: runtime.relay != nil,
			ReconcilerEnabled: runtime.reconciler != nil, ImmediateEnabled: s.publisher.IsMQBacked(),
		}
	}
	for id, consumer := range s.consumers {
		result.Consumers[id] = appEventing.ConsumerRuntimeStatus{
			Topic: consumer.topic, Enabled: s.consumerEnabled(consumer), Healthy: consumer.healthy, LastError: consumer.lastError,
		}
	}
	return result
}

func (s *Subsystem) Outboxes() []appEventing.NamedOutboxStatusReader {
	if s == nil {
		return nil
	}
	outboxes := make([]appEventing.NamedOutboxStatusReader, 0, len(s.profiles))
	for _, profile := range s.profiles {
		outboxes = append(outboxes, profile.status)
	}
	sort.Slice(outboxes, func(i, j int) bool { return outboxes[i].Name < outboxes[j].Name })
	return outboxes
}
