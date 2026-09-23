//go:build reliable_messaging_m4

package process

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	sdktransport "github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	goNSQ "github.com/nsqio/go-nsq"
)

// standardGovernedStatusReader keeps one selected profile's status and
// durable replay owner together when the subsystem exports its Outboxes.
// Neither capability is exposed for an unselected legacy profile.
type standardGovernedStatusReader struct {
	outboxport.StatusReader
	outboxport.DurableManualReplayAuthorizer
	systemgov.PendingReplayResolver
	systemgov.OutboxGovernanceReader
}

// configuredEventSubsystem keeps the ordinary configuration on the existing
// implementation. Each M4 opt-in replaces a whole writer/runner profile.
func configuredEventSubsystem(cfg *config.Config) func(eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
	if cfg == nil || cfg.Eventing == nil || cfg.Eventing.StandardOutbox == nil ||
		(!cfg.Eventing.StandardOutbox.Mongo && !cfg.Eventing.StandardOutbox.Assessment) {
		return eventsubsystem.New
	}
	selected := *cfg.Eventing.StandardOutbox
	return func(opts eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
		return buildM4StandardEventSubsystem(opts, cfg, selected)
	}
}

// buildM4StandardEventSubsystem runs only inside the candidate build. It
// explicitly owns one additional go-nsq producer; SDK adapters borrow it and
// the two selected profiles share one bounded publisher and drain boundary.
func buildM4StandardEventSubsystem(opts eventsubsystem.Options, cfg *config.Config, selected options.StandardOutboxOptions) (*eventsubsystem.Subsystem, error) {
	if cfg == nil || cfg.MessagingOptions == nil || !cfg.MessagingOptions.Enabled ||
		cfg.MessagingOptions.Provider != "nsq" || cfg.MessagingOptions.NSQAddr == "" ||
		opts.MQPublisher == nil || opts.PublisherMode != eventruntime.PublishModeMQ {
		return nil, errors.New("M4 standard outbox requires a live NSQ-backed messaging configuration")
	}
	if selected.Mongo && opts.MongoDB == nil || selected.Assessment && opts.MySQLDB == nil {
		return nil, errors.New("M4 standard outbox requires its host database")
	}
	registry, err := eventcatalog.NewEffectiveRegistry(opts.Catalog, eventcatalog.DefaultSpecs())
	if err != nil {
		return nil, err
	}
	routes := make(map[string]string)
	for _, event := range registry.Snapshot() {
		if event.OutboxProfile == eventcatalog.OutboxProfileMongoDomain && selected.Mongo ||
			event.OutboxProfile == eventcatalog.OutboxProfileAssessmentMySQL && selected.Assessment {
			routes[event.Topic] = event.Topic
		}
	}
	if len(routes) == 0 {
		return nil, errors.New("M4 standard profiles have no reliable NSQ routes")
	}
	preflightCtx, cancelPreflight := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPreflight()
	if err := preflightM4StandardStorage(preflightCtx, opts, selected); err != nil {
		return nil, err
	}
	// These are candidate bounds; M4-07 must measure throughput and recovery
	// before any release. The old profile remains available for unselected flows.
	const (
		lease          = 30 * time.Second
		publishTimeout = 10 * time.Second
		writeTimeout   = 5 * time.Second
		drainTimeout   = 15 * time.Second
	)
	concurrency := 0
	if selected.Mongo {
		concurrency += opts.Mongo.PublishWorkers
	}
	if selected.Assessment {
		concurrency += opts.Assessment.PublishWorkers
	}
	if concurrency < 1 || concurrency > 1000 {
		return nil, fmt.Errorf("M4 standard outbox concurrency must be 1..1000, got %d", concurrency)
	}
	producerConfig := goNSQ.NewConfig()
	producerConfig.DialTimeout = 5 * time.Second
	producerConfig.HeartbeatInterval = 5 * time.Second
	producerConfig.ReadTimeout = 15 * time.Second
	producerConfig.WriteTimeout = 10 * time.Second
	producer, err := goNSQ.NewProducer(cfg.MessagingOptions.NSQAddr, producerConfig)
	if err != nil {
		return nil, fmt.Errorf("create M4 host NSQ producer: %w", err)
	}
	owned := true
	defer func() {
		if owned {
			producer.Stop()
		}
	}()
	if err := producer.Ping(); err != nil {
		return nil, fmt.Errorf("connect M4 host NSQ producer: %w", err)
	}
	publisher, err := sdknsq.New(producer, routes, concurrency)
	if err != nil {
		return nil, err
	}
	var drainOnce sync.Once
	var drainErr error
	drain := func(ctx context.Context) error {
		drainOnce.Do(func() {
			drainErr = publisher.Drain(ctx)
			producer.Stop()
			if drainErr != nil {
				// A driver call may outlive its SDK publish deadline. Stop the
				// producer, then verify that its in-flight call has actually left.
				finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := publisher.Drain(finishCtx); err != nil {
					drainErr = errors.Join(drainErr, fmt.Errorf("M4 NSQ producer did not finish after stop: %w", err))
				}
			}
		})
		return drainErr
	}
	replacements := make(map[eventcatalog.OutboxProfile]eventsubsystem.StandardProfile)
	newProfile := func(name string, store outbox.Store, stager appEventing.EventStager, wake *standardoutbox.PostCommitWake,
		status appEventing.NamedOutboxStatusReader, profile eventsubsystem.ProfileOptions,
	) (eventsubsystem.StandardProfile, error) {
		if profile.Interval <= 0 || profile.PublishWorkers < 1 || profile.PublishWorkers > 1000 {
			return eventsubsystem.StandardProfile{}, fmt.Errorf("M4 standard profile %s has invalid scan/concurrency bounds", name)
		}
		supervisor, err := standardoutbox.NewRelaySupervisor(standardoutbox.SupervisorOptions{
			Name: name, InitialBackoff: 500 * time.Millisecond, MaxBackoff: 30 * time.Second,
			NewRelay: func(observe relay.Observer) (standardoutbox.RelayRunner, error) {
				return relay.New(store, publisher, relay.Config{
					Concurrency: profile.PublishWorkers, PollInterval: profile.Interval,
					Lease: lease, PublishTimeout: publishTimeout, WriteTimeout: writeTimeout,
					Wake: wake.Wake(), Retry: standardoutbox.SDKRetryPolicy(), Observe: observe,
				})
			},
		})
		if err != nil {
			return eventsubsystem.StandardProfile{}, err
		}
		return eventsubsystem.StandardProfile{
			Binding:    appEventing.ProfileBinding{Stager: stager, PostCommit: wake},
			Supervisor: supervisor, Drain: drain, DrainTimeout: drainTimeout, Status: status,
		}, nil
	}
	if selected.Mongo {
		collection := opts.MongoDB.Collection("rm_outbox")
		store, err := sdkmongo.New(collection)
		if err != nil {
			return nil, err
		}
		stager, err := mongostandard.NewStager(collection, opts.Catalog, eventruntime.SourceAPIServer)
		if err != nil {
			return nil, err
		}
		status, err := mongostandard.NewStatusReader(collection)
		if err != nil {
			return nil, err
		}
		replay, err := mongostandard.NewReplayLedger(opts.MongoDB, "mongo-domain-events")
		if err != nil {
			return nil, err
		}
		profile, err := newProfile("mongo-domain-events", store, stager, standardoutbox.NewPostCommitWake(),
			appEventing.NamedOutboxStatusReader{Name: "mongo-domain-events", Reader: standardGovernedStatusReader{
				StatusReader: status, DurableManualReplayAuthorizer: replay, PendingReplayResolver: replay,
				OutboxGovernanceReader: status,
			}}, opts.Mongo)
		if err != nil {
			return nil, err
		}
		replacements[eventcatalog.OutboxProfileMongoDomain] = profile
	}
	if selected.Assessment {
		sqlDB, err := opts.MySQLDB.DB()
		if err != nil {
			return nil, err
		}
		store, err := sdkmysql.New(sqlDB)
		if err != nil {
			return nil, err
		}
		stager, err := mysqlstandard.NewStager(opts.Catalog, eventruntime.SourceAPIServer)
		if err != nil {
			return nil, err
		}
		status, err := mysqlstandard.NewStatusReader(sqlDB)
		if err != nil {
			return nil, err
		}
		replay, err := mysqlstandard.NewReplayLedger(sqlDB, "assessment-mysql-outbox")
		if err != nil {
			return nil, err
		}
		profile, err := newProfile("assessment-mysql-outbox", store, stager, standardoutbox.NewPostCommitWake(),
			appEventing.NamedOutboxStatusReader{Name: "assessment-mysql-outbox", Reader: standardGovernedStatusReader{
				StatusReader: status, DurableManualReplayAuthorizer: replay, PendingReplayResolver: replay,
				OutboxGovernanceReader: status,
			}}, opts.Assessment)
		if err != nil {
			return nil, err
		}
		replacements[eventcatalog.OutboxProfileAssessmentMySQL] = profile
	}
	subsystem, err := eventsubsystem.NewWithStandardProfiles(opts, replacements)
	if err != nil {
		return nil, err
	}
	owned = false
	return subsystem, nil
}

var _ sdktransport.Publisher = (*sdknsq.Publisher)(nil)
